package reaper

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/jackc/pgx/v5"
)

const minTickInterval = 5 // seconds – floor for GCD to prevent excessive wake-ups

// SourceReaper manages metric collection for a single monitored source.
// Instead of one goroutine per metric it runs a single GCD-based tick loop
// and batches SQL queries via pgx.Batch when the source is a real Postgres
// connection (non-pgbouncer, non-pgpool).
type SourceReaper struct {
	reaper      *Reaper
	md          *sources.SourceConn
	lastFetch   map[string]time.Time
	lastUptimeS int64 // last seen postmaster_uptime_s for restart detection
}

// NewSourceReaper creates a SourceReaper for the given source connection.
func NewSourceReaper(r *Reaper, md *sources.SourceConn) *SourceReaper {
	sr := &SourceReaper{
		reaper:    r,
		md:        md,
		lastFetch: make(map[string]time.Time),
	}
	return sr
}

// activeMetrics returns a snapshot copy of the currently active metric intervals
// based on the source's recovery state. Copying under the lock prevents data
// races when the caller iterates after the lock is released.
func (sr *SourceReaper) activeMetrics() map[string]time.Duration {
	sr.md.RLock()
	defer sr.md.RUnlock()
	src := sr.md.Metrics
	if sr.md.IsInRecovery && len(sr.md.MetricsStandby) > 0 {
		src = sr.md.MetricsStandby
	}
	c := make(map[string]time.Duration, len(src))
	for k, v := range src {
		c[k] = time.Duration(v) * time.Second
	}
	return c
}

// GCDSlice computes GCD across a slice. Returns 0 for empty input.
func GCDSlice(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	g := vals[0]
	for _, v := range vals[1:] {
		for v != 0 {
			g, v = v, g%v
		}
	}
	return g
}

// calcTickInterval computes GCD of all metric intervals with a minimum floor.
func (sr *SourceReaper) calcTickInterval() time.Duration {
	am := sr.activeMetrics()
	intervals := make([]int, 0, len(am))
	for _, d := range am {
		intervals = append(intervals, max(int(d.Seconds()), minTickInterval))
	}
	return time.Duration(max(GCDSlice(intervals), minTickInterval)) * time.Second
}

// cacheKey returns the instance-level cache key for the given metric.
func (sr *SourceReaper) cacheKey(m metrics.Metric, name string) string {
	age := sr.reaper.Metrics.CacheAge()
	if m.IsInstanceLevel && age > 0 && sr.md.GetMetricInterval(name) < age {
		return fmt.Sprintf("%s:%s", sr.md.GetClusterIdentifier(), name)
	}
	return ""
}

// isRoleExcluded returns true if the metric should be skipped based on the
// source's recovery state (e.g. primary-only metric on a standby).
func (sr *SourceReaper) isRoleExcluded(m metrics.Metric) bool {
	return (m.PrimaryOnly() && sr.md.IsInRecovery) || (m.StandbyOnly() && !sr.md.IsInRecovery)
}

// sendEnvelope adds sysinfo and dispatches a MeasurementEnvelope to the
// measurement channel.
func (sr *SourceReaper) sendEnvelope(name, storageName string, data metrics.Measurements) {
	sr.reaper.AddSysinfoToMeasurements(data, sr.md)
	sr.reaper.measurementCh <- metrics.MeasurementEnvelope{
		DBName:     sr.md.Name,
		MetricName: cmp.Or(storageName, name),
		Data:       data,
		CustomTags: sr.md.CustomTags,
	}
}

// dispatchMetricData handles the post-fetch workflow for a collected metric:
// caching, sysinfo enrichment, sending, and restart detection.
func (sr *SourceReaper) dispatchMetricData(ctx context.Context, name string, metric metrics.Metric, data metrics.Measurements) {
	if key := sr.cacheKey(metric, name); key != "" {
		sr.reaper.measurementCache.Put(key, data)
	}
	sr.sendEnvelope(name, metric.StorageName, data)
	if name == "db_stats" {
		sr.detectServerRestart(ctx, data)
	}
}

// batchEntry holds the minimum info needed to execute and dispatch a metric query.
type batchEntry struct {
	name   string
	metric metrics.Metric
	sql    string
}

// Run is the main loop for a single source. It replaces N per-metric goroutines
// with one goroutine that batches SQL queries at GCD-aligned ticks.
func (sr *SourceReaper) Run(ctx context.Context) {
	l := log.GetLogger(ctx).WithField("source", sr.md.Name)
	ctx = log.WithLogger(ctx, l)
	var err error
	for {
		if err = sr.md.FetchRuntimeInfo(ctx, false); err != nil {
			l.WithError(err).Warning("could not refresh runtime info")
		}

		now := time.Now()
		var batch []batchEntry

		for name, interval := range sr.activeMetrics() {
			if interval <= 0 {
				continue
			}
			if lf := sr.lastFetch[name]; !lf.IsZero() && now.Sub(lf) < interval {
				continue
			}
			switch {
			case name == specialMetricServerLogEventCounts:
				if sr.lastFetch[name].IsZero() {
					go func() {
						if e := sr.runLogParser(ctx); e != nil {
							l.WithError(e).Error("log parser error")
						}
					}()
				}
			case IsDirectlyFetchableMetric(sr.md, name):
				err = sr.fetchOSMetric(ctx, name)
			case name == specialMetricChangeEvents || name == specialMetricInstanceUp:
				err = sr.fetchSpecialMetric(ctx, name)
			default:
				metric, ok := metricDefs.GetMetricDef(name)
				if !ok {
					l.WithField("metric", name).Warning("metric definition not found")
					continue
				}
				if sr.isRoleExcluded(metric) {
					continue
				}
				if cached := sr.reaper.GetMeasurementCache(sr.cacheKey(metric, name)); len(cached) > 0 {
					sr.sendEnvelope(name, metric.StorageName, cached)
					break
				}
				sql := metric.GetSQL(sr.md.Version)
				if sql == "" {
					l.WithField("source", sr.md.Name).WithField("version", sr.md.Version).Warning("no SQL found for metric version")
					break
				}
				batch = append(batch, batchEntry{name: name, metric: metric, sql: sql})
			}
			if err != nil {
				l.WithError(err).WithField("metric", name).Error("failed to fetch metric")
			}
			sr.lastFetch[name] = now
		}

		if len(batch) > 0 {
			if sr.md.IsPostgresSource() {
				err = sr.executeBatch(ctx, batch)
			} else {
				for _, e := range batch {
					err = sr.fetchMetric(ctx, e)
				}
			}
			if err != nil {
				l.WithError(err).Error("failed to fetch metrics")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sr.calcTickInterval()):
		}
	}
}

// executeBatch sends all SQLs in a single pgx.Batch round-trip, dispatching
// each result immediately as it arrives. Individual query failures produce nil
// data and are aggregated into the returned error.
func (sr *SourceReaper) executeBatch(ctx context.Context, entries []batchEntry) error {
	batch := &pgx.Batch{}
	for _, e := range entries {
		batch.Queue(e.sql)
	}

	br := sr.md.Conn.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	var errs []error
	for _, e := range entries {
		rows, err := br.Query()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to fetch metric %s: %v", e.name, err))
			continue
		}
		errs = append(errs, sr.CollectAndDispatch(ctx, rows, e.name, e.metric))
	}
	return errors.Join(errs...)
}

// fetchMetric executes a single SQL query and returns the resulting measurements.
func (sr *SourceReaper) fetchMetric(ctx context.Context, entry batchEntry) error {
	rows, err := sr.md.Conn.Query(ctx, entry.sql, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return err
	}
	return sr.CollectAndDispatch(ctx, rows, entry.name, entry.metric)
}

// CollectAndDispatch is a helper that collects rows from a pgx.Rows and dispatches them.
func (sr *SourceReaper) CollectAndDispatch(ctx context.Context, rows pgx.Rows, name string, metric metrics.Metric) error {
	data, err := pgx.CollectRows(rows, metrics.RowToMeasurement)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		sr.dispatchMetricData(ctx, name, metric, data)
	}
	return nil
}

// fetchOSMetric handles gopsutil-based OS metrics.
func (sr *SourceReaper) fetchOSMetric(ctx context.Context, name string) error {
	msg, err := sr.reaper.FetchStatsDirectlyFromOS(ctx, sr.md, name)
	if err != nil {
		return fmt.Errorf("could not read metric from OS: %v", err)
	}
	if msg != nil && len(msg.Data) > 0 {
		sr.reaper.measurementCh <- *msg
	}
	return nil
}

// fetchSpecialMetric handles change_events and instance_up metrics.
func (sr *SourceReaper) fetchSpecialMetric(ctx context.Context, name string) error {
	metric, ok := metricDefs.GetMetricDef(name)
	if !ok {
		return fmt.Errorf("metric definition not found for %s", name)
	}
	if sr.isRoleExcluded(metric) {
		return nil
	}
	var (
		data metrics.Measurements
		err  error
	)
	switch name {
	case specialMetricChangeEvents:
		data, err = sr.reaper.GetObjectChangesMeasurement(ctx, sr.md)
	case specialMetricInstanceUp:
		data, err = sr.reaper.GetInstanceUpMeasurement(ctx, sr.md)
	}
	if err != nil {
		return fmt.Errorf("failed to fetch special metric: %v", err)
	}
	if len(data) > 0 {
		sr.sendEnvelope(name, metric.StorageName, data)
	}
	return err
}

// runLogParser launches the server log event counts parser.
func (sr *SourceReaper) runLogParser(ctx context.Context) error {
	lp, err := NewLogParser(ctx, sr.md, sr.reaper.measurementCh)
	if err != nil {
		return fmt.Errorf("failed to initialize log parser: %v", err)
	}
	if err := lp.ParseLogs(); err != nil {
		return fmt.Errorf("log parser error: %v", err)
	}
	return nil
}

// detectServerRestart checks for PostgreSQL server restarts via postmaster_uptime_s
// in db_stats metric data and emits an object_changes measurement if detected.
func (sr *SourceReaper) detectServerRestart(ctx context.Context, data metrics.Measurements) {
	if len(data) == 0 {
		return
	}
	uptimeS, ok := data[0]["postmaster_uptime_s"].(int64)
	if !ok {
		return
	}
	prev := sr.lastUptimeS
	sr.lastUptimeS = uptimeS
	if prev > 0 && uptimeS < prev {
		l := log.GetLogger(ctx)
		l.Warning("Detected server restart (or failover)")
		entry := metrics.NewMeasurement(data.GetEpoch())
		entry["details"] = "Detected server restart (or failover)"
		sr.reaper.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     sr.md.Name,
			MetricName: "object_changes",
			Data:       metrics.Measurements{entry},
			CustomTags: sr.md.CustomTags,
		}
	}
}
