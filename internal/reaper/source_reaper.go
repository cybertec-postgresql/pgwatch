package reaper

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/jackc/pgx/v5"
)

const minTickInterval = 1 // seconds - floor for GCD to help handle zero/negative intervals

// SourceReaper manages metric collection for a single monitored source.
// Instead of one goroutine per metric it runs a single GCD-based tick loop
// and batches SQL queries via pgx.Batch when the source is a real Postgres
// connection (non-pgbouncer, non-pgpool).
type SourceReaper struct {
	reaper          *Reaper
	md              *sources.SourceConn
	lastFetch       map[string]time.Time
	lastUptimeS     int64               // last seen postmaster_uptime_s for restart detection
	degradedMetrics map[string]struct{} // metrics that failed individual retry; executed via fetchMetric until they recover
}

// NewSourceReaper creates a SourceReaper for the given source connection.
func NewSourceReaper(r *Reaper, md *sources.SourceConn) *SourceReaper {
	sr := &SourceReaper{
		reaper:          r,
		md:              md,
		lastFetch:       make(map[string]time.Time),
		degradedMetrics: make(map[string]struct{}),
	}
	return sr
}

// activeMetrics returns a snapshot copy of the currently active metric intervals
// based on the source's recovery state. Copying under the lock prevents data
// races when the caller iterates after the lock is released.
func (sr *SourceReaper) activeMetrics() map[string]time.Duration {
	sr.md.RLock()
	defer sr.md.RUnlock()
	am := sr.md.Metrics
	if sr.md.IsInRecovery && len(sr.md.MetricsStandby) > 0 {
		am = sr.md.MetricsStandby
	}
	c := make(map[string]time.Duration, len(am))
	for k, v := range am {
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
	sr.md.RLock()
	defer sr.md.RUnlock()
	return (m.PrimaryOnly() && sr.md.IsInRecovery) || (m.StandbyOnly() && !sr.md.IsInRecovery)
}

// sendEnvelope adds sysinfo and dispatches a MeasurementEnvelope to the
// measurement channel.
func (sr *SourceReaper) sendEnvelope(ctx context.Context, name, storageName string, data metrics.Measurements) {
	log.GetLogger(ctx).WithField("metric", name).WithField("rows", len(data)).Info("measurements fetched")
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
	sr.sendEnvelope(ctx, name, metric.StorageName, data)
	if name == "db_stats" {
		sr.detectServerRestart(ctx, data)
	}
}

// batchEntry holds the minimum info needed to execute and dispatch a metric query.
type batchEntry struct {
	metricName string
	metric     metrics.Metric
	sql        string
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

		for metricName, metricInterval := range sr.activeMetrics() {
			if metricInterval <= 0 {
				continue
			}
			if lf := sr.lastFetch[metricName]; !lf.IsZero() && now.Sub(lf) < metricInterval {
				continue
			}

			metric, ok := metricDefs.GetMetricDef(metricName)
			if !ok || sr.isRoleExcluded(metric) {
				continue
			}

			switch {
			case metricName == specialMetricServerLogEventCounts:
				if sr.lastFetch[metricName].IsZero() {
					go func() {
						if e := sr.runLogParser(ctx); e != nil {
							l.WithError(e).Error("log parser error")
						}
					}()
				}
			case IsDirectlyFetchableMetric(sr.md, metricName):
				err = sr.fetchOSMetric(ctx, metricName)
				sr.lastFetch[metricName] = time.Now()
			case metricName == specialMetricChangeEvents || metricName == specialMetricInstanceUp:
				err = sr.fetchSpecialMetric(ctx, metricName, metric.StorageName)
				sr.lastFetch[metricName] = time.Now()
			default:
				if cached := sr.reaper.GetMeasurementCache(sr.cacheKey(metric, metricName)); len(cached) > 0 {
					l.WithField("metric", metricName).Info("instance level cache hit")
					sr.sendEnvelope(ctx, metricName, metric.StorageName, cached)
					sr.lastFetch[metricName] = time.Now()
					break
				}
				sr.md.RLock()
				version := sr.md.Version
				sr.md.RUnlock()
				sql := metric.GetSQL(version)
				if sql == "" {
					l.WithField("metric", metricName).WithField("version", version).Warning("no SQL found for metric version")
					sr.lastFetch[metricName] = time.Now()
					break
				}
				if _, degraded := sr.degradedMetrics[metricName]; degraded {
					if err = sr.fetchMetric(ctx, batchEntry{metricName: metricName, metric: metric, sql: sql}); err != nil {
						l.WithError(err).WithField("metric", metricName).Error("degraded metric fetch failed")
					} else {
						l.WithField("metric", metricName).Info("degraded metric recovered, returning to batch execution")
						delete(sr.degradedMetrics, metricName)
					}
					sr.lastFetch[metricName] = time.Now()
					break
				}
				batch = append(batch, batchEntry{metricName: metricName, metric: metric, sql: sql})
			}
			if err != nil {
				l.WithError(err).WithField("metric", metricName).Error("failed to fetch metric")
			}
		}

		if len(batch) > 0 {
			// executeBatch/fetchMetrics log every failure individually with the
			// metric name attached, so there is nothing to re-log here.
			if sr.md.IsPostgresSource() {
				sr.executeBatch(ctx, batch)
			} else {
				sr.fetchMetrics(ctx, batch)
			}

			now := time.Now()
			for _, e := range batch {
				sr.lastFetch[e.metricName] = now
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
// each result immediately as it arrives. If any query fails, PostgreSQL's
// extended protocol aborts all subsequent queries in the same sync boundary
// (cascade failure). Any entry that returns an error from the batch is retried
// individually via fetchMetric to isolate real failures from cascade failures.
// Entries that fail even after the individual retry are marked as degraded
// so that subsequent runs use fetchMetric for them until they recover.
//
// Failures are logged per-metric as they happen.
func (sr *SourceReaper) executeBatch(ctx context.Context, entries []batchEntry) {
	batch := &pgx.Batch{}
	for _, e := range entries {
		batch.Queue(e.sql)
	}

	// A failing query aborts the rest of the batch (cascade), and the per-query
	// retry below may also fail. Those failures are expected and handled, so run
	// them under a context that downgrades pgx tracer errors to debug and avoids
	// flooding the log with BatchClose/Query dumps. We log the real outcome once.
	pgxCtx := log.WithSuppressedPgxErrors(ctx)
	br := sr.md.Conn.SendBatch(pgxCtx, batch)
	l := log.GetLogger(ctx)

	var retries []batchEntry
	for _, e := range entries {
		rows, err := br.Query()
		if err != nil {
			// May be a real error or a cascade from an earlier failure; retry individually.
			// Don't log here: a cascade failure is expected noise and the retry will
			// log a single accurate error if the metric truly fails.
			retries = append(retries, e)
			continue
		}
		if err = sr.CollectAndDispatch(ctx, rows, e.metricName, e.metric); err != nil {
			retries = append(retries, e)
		}
	}

	// Close the batch explicitly before retrying to release the connection back to the
	// pool. A deferred close would hold the connection through the retry loop, causing a
	// potential deadlock
	if err := br.Close(); err != nil {
		l.WithError(err).Debug("failed to close batch")
	}

	sr.fetchMetrics(ctx, retries)
}

// fetchMetrics fetches a batch of entries individually, marking any that fail
// as degraded so subsequent runs will continue to use individual fetching for them.
// Each failure is logged on its own line with the metric name attached.
func (sr *SourceReaper) fetchMetrics(ctx context.Context, entries []batchEntry) {
	for _, e := range entries {
		if err := sr.fetchMetric(ctx, e); err != nil {
			log.GetLogger(ctx).
				WithField("metric", e.metricName).
				WithError(err).
				Error("failed to fetch metric")
			sr.degradedMetrics[e.metricName] = struct{}{}
		}
	}
}

// fetchMetric executes a single SQL query and returns the resulting measurements.
// pgx tracer errors are suppressed because the caller logs a single, accurate
// message (with the metric name) on failure; the raw SQL/args dump is only
// emitted at debug level.
func (sr *SourceReaper) fetchMetric(ctx context.Context, entry batchEntry) error {
	rows, err := sr.md.Conn.Query(log.WithSuppressedPgxErrors(ctx), entry.sql, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return err
	}
	return sr.CollectAndDispatch(ctx, rows, entry.metricName, entry.metric)
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
		log.GetLogger(ctx).WithField("metric", name).WithField("rows", len(msg.Data)).Info("measurements fetched")
		sr.reaper.measurementCh <- *msg
	}
	return nil
}

// fetchSpecialMetric handles change_events and instance_up metrics.
func (sr *SourceReaper) fetchSpecialMetric(ctx context.Context, name, storageName string) error {
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
		sr.sendEnvelope(ctx, name, storageName, data)
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
