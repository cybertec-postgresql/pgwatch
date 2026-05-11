package reaper

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/jackc/pgx/v5"
)

const minTickInterval = 1 // seconds - floor for GCD to help handle zero/negative intervals

var _ Reaper = (*DbConnReaper)(nil)

// DbConnReaper manages metric collection for a single monitored database source.
// Instead of one goroutine per metric it runs a single GCD-based tick loop
// and batches SQL queries via pgx.Batch when the source is a real Postgres
// connection (non-pgbouncer, non-pgpool).
type DbConnReaper struct {
	reaper          *reaper
	md              *sources.DbConn
	lastFetch       map[string]time.Time
	lastUptimeS     int64               // last seen postmaster_uptime_s for restart detection
	degradedMetrics map[string]struct{} // metrics that failed individual retry; executed via fetchMetric until they recover
}

// NewDbConnReaper creates a SourceReaper for the given source connection.
func NewDbConnReaper(r *reaper, md *sources.DbConn) *DbConnReaper {
	return &DbConnReaper{
		reaper:          r,
		md:              md,
		lastFetch:       make(map[string]time.Time),
		degradedMetrics: make(map[string]struct{}),
	}
}

// activeMetrics returns a snapshot copy of the currently active metric intervals
// based on the source's recovery state. Copying under the lock prevents data
// races when the caller iterates after the lock is released.
func (sr *DbConnReaper) activeMetrics() map[string]time.Duration {
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
func (sr *DbConnReaper) calcTickInterval() time.Duration {
	am := sr.activeMetrics()
	intervals := make([]int, 0, len(am))
	for _, d := range am {
		intervals = append(intervals, max(int(d.Seconds()), minTickInterval))
	}
	return time.Duration(max(GCDSlice(intervals), minTickInterval)) * time.Second
}

// cacheKey returns the instance-level cache key for the given metric.
func (sr *DbConnReaper) cacheKey(m metrics.Metric, name string) string {
	age := sr.reaper.Metrics.CacheAge()
	if m.IsInstanceLevel && age > 0 && sr.md.GetMetricInterval(name) < age {
		return fmt.Sprintf("%s:%s", sr.md.GetClusterIdentifier(), name)
	}
	return ""
}

// isRoleExcluded returns true if the metric should be skipped based on the
// source's recovery state (e.g. primary-only metric on a standby).
func (sr *DbConnReaper) isRoleExcluded(m metrics.Metric) bool {
	sr.md.RLock()
	defer sr.md.RUnlock()
	return (m.PrimaryOnly() && sr.md.IsInRecovery) || (m.StandbyOnly() && !sr.md.IsInRecovery)
}

// sendEnvelope adds sysinfo and dispatches a MeasurementEnvelope to the
// measurement channel.
func (sr *DbConnReaper) sendEnvelope(ctx context.Context, name, storageName string, data metrics.Measurements) {
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
func (sr *DbConnReaper) dispatchMetricData(ctx context.Context, name string, metric metrics.Metric, data metrics.Measurements) {
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
	name   string
	metric metrics.Metric
	sql    string
}

// Run is the main loop for a single source. It replaces N per-metric goroutines
// with one goroutine that batches SQL queries at GCD-aligned ticks.
func (sr *DbConnReaper) Reap(ctx context.Context) {
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

			metric, ok := metricDefs.GetMetricDef(name)
			if !ok || sr.isRoleExcluded(metric) {
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
				sr.lastFetch[name] = time.Now()
			case name == specialMetricChangeEvents || name == specialMetricInstanceUp:
				err = sr.fetchSpecialMetric(ctx, name, metric.StorageName)
				sr.lastFetch[name] = time.Now()
			default:
				if cached := sr.reaper.GetMeasurementCache(sr.cacheKey(metric, name)); len(cached) > 0 {
					l.WithField("metric", name).Info("instance level cache hit")
					sr.sendEnvelope(ctx, name, metric.StorageName, cached)
					sr.lastFetch[name] = time.Now()
					break
				}
				sr.md.RLock()
				version := sr.md.Version
				sr.md.RUnlock()
				sql := metric.GetSQL(version)
				if sql == "" {
					l.WithField("source", sr.md.Name).WithField("version", version).Warning("no SQL found for metric version")
					sr.lastFetch[name] = time.Now()
					break
				}
				if _, degraded := sr.degradedMetrics[name]; degraded {
					if err = sr.fetchMetric(ctx, batchEntry{name: name, metric: metric, sql: sql}); err != nil {
						l.WithError(err).WithField("metric", name).Error("degraded metric fetch failed")
					} else {
						l.WithField("metric", name).Info("degraded metric recovered, returning to batch execution")
						delete(sr.degradedMetrics, name)
					}
					sr.lastFetch[name] = time.Now()
					break
				}
				batch = append(batch, batchEntry{name: name, metric: metric, sql: sql})
			}
			if err != nil {
				l.WithError(err).WithField("metric", name).Error("failed to fetch metric")
			}
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

			now := time.Now()
			for _, e := range batch {
				sr.lastFetch[e.name] = now
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
func (sr *DbConnReaper) executeBatch(ctx context.Context, entries []batchEntry) error {
	batch := &pgx.Batch{}
	for _, e := range entries {
		batch.Queue(e.sql)
	}

	br := sr.md.Conn.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	var (
		errs    []error
		retries []batchEntry
	)
	for _, e := range entries {
		rows, err := br.Query()
		if err != nil {
			// May be a real error or a cascade from an earlier failure; retry individually.
			retries = append(retries, e)
			continue
		}
		errs = append(errs, sr.CollectAndDispatch(ctx, rows, e.name, e.metric))
	}

	for _, e := range retries {
		if err := sr.fetchMetric(ctx, e); err != nil {
			errs = append(errs, fmt.Errorf("failed to fetch metric %s: %v", e.name, err))
			log.GetLogger(ctx).WithField("metric", e.name).Warning("metric degraded after repeated failures, switching to individual fetch")
			sr.degradedMetrics[e.name] = struct{}{}
		}
	}
	return errors.Join(errs...)
}

// fetchMetric executes a single SQL query and returns the resulting measurements.
func (sr *DbConnReaper) fetchMetric(ctx context.Context, entry batchEntry) error {
	rows, err := sr.md.Conn.Query(ctx, entry.sql, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return err
	}
	return sr.CollectAndDispatch(ctx, rows, entry.name, entry.metric)
}

// CollectAndDispatch is a helper that collects rows from a pgx.Rows and dispatches them.
func (sr *DbConnReaper) CollectAndDispatch(ctx context.Context, rows pgx.Rows, name string, metric metrics.Metric) error {
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
func (sr *DbConnReaper) fetchOSMetric(ctx context.Context, name string) error {
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
func (sr *DbConnReaper) fetchSpecialMetric(ctx context.Context, name, storageName string) error {
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
func (sr *DbConnReaper) runLogParser(ctx context.Context) error {
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
func (sr *DbConnReaper) detectServerRestart(ctx context.Context, data metrics.Measurements) {
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

func QueryMeasurements(ctx context.Context, md *sources.DbConn, sql string, args ...any) (metrics.Measurements, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL")
	}
	// For non-postgres connections (e.g. pgbouncer, pgpool), use simple protocol
	if !md.IsPostgresSource() {
		args = append([]any{pgx.QueryExecModeSimpleProtocol}, args...)
	}
	// lock_timeout is set at connection level via RuntimeParams, no need for transaction wrapper
	rows, err := md.Conn.Query(ctx, sql, args...)
	if err == nil {
		return pgx.CollectRows(rows, metrics.RowToMeasurement)
	}
	return nil, err
}

func (r *reaper) DetectSprocChanges(ctx context.Context, md *sources.DbConn) (changeCounts ChangeDetectionResults) {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	l := log.GetLogger(ctx)
	changeCounts.Target = "functions"
	l.Debug("checking for sproc changes...")
	if _, ok := md.ChangeState["sproc_hashes"]; !ok {
		firstRun = true
		md.ChangeState["sproc_hashes"] = make(map[string]string)
	}
	mvp, ok := metricDefs.GetMetricDef("sproc_hashes")
	if !ok {
		l.Error("could not get sproc_hashes sql")
		return
	}
	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return
	}
	for _, dr := range data {
		objIdent := dr["tag_sproc"].(string) + dbMetricJoinStr + dr["tag_oid"].(string)
		prevHash, ok := md.ChangeState["sproc_hashes"][objIdent]
		ll := l.WithField("sproc", dr["tag_sproc"]).WithField("oid", dr["tag_oid"])
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				ll.Debug("change detected")
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				md.ChangeState["sproc_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				ll.Debug("new sproc detected")
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			md.ChangeState["sproc_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(md.ChangeState["sproc_hashes"]) != len(data) {
		// turn resultset to map => [oid]=true for faster checks
		currentOidMap := make(map[string]bool)
		for _, dr := range data {
			currentOidMap[dr["tag_sproc"].(string)+dbMetricJoinStr+dr["tag_oid"].(string)] = true
		}
		for sprocIdent := range md.ChangeState["sproc_hashes"] {
			_, ok := currentOidMap[sprocIdent]
			if !ok {
				splits := strings.Split(sprocIdent, dbMetricJoinStr)
				l.WithField("sproc", splits[0]).WithField("oid", splits[1]).Debug("deleted sproc detected")
				m := metrics.NewMeasurement(data.GetEpoch())
				m["event"] = "drop"
				m["tag_sproc"] = splits[0]
				m["tag_oid"] = splits[1]
				detectedChanges = append(detectedChanges, m)
				delete(md.ChangeState["sproc_hashes"], sprocIdent)
				changeCounts.Dropped++
			}
		}
	}
	l.Debugf("sproc changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "sproc_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}
	return changeCounts
}

func (r *reaper) DetectTableChanges(ctx context.Context, md *sources.DbConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "tables"
	l.Debug("checking for table changes...")
	if _, ok := md.ChangeState["table_hashes"]; !ok {
		firstRun = true
		md.ChangeState["table_hashes"] = make(map[string]string)
	}
	mvp, ok := metricDefs.GetMetricDef("table_hashes")
	if !ok {
		l.Error("could not get table_hashes sql")
		return changeCounts
	}
	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return changeCounts
	}
	for _, dr := range data {
		objIdent := dr["tag_table"].(string)
		prevHash, ok := md.ChangeState["table_hashes"][objIdent]
		ll := l.WithField("table", dr["tag_table"])
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				ll.Debug("change detected")
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				md.ChangeState["table_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				ll.Debug("new table detected")
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			md.ChangeState["table_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(md.ChangeState["table_hashes"]) != len(data) {
		deletedTables := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentTableMap := make(map[string]bool)
		for _, dr := range data {
			currentTableMap[dr["tag_table"].(string)] = true
		}
		for table := range md.ChangeState["table_hashes"] {
			_, ok := currentTableMap[table]
			if !ok {
				l.WithField("table", table).Debug("deleted table detected")
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_table"] = table
				detectedChanges = append(detectedChanges, influxEntry)
				deletedTables = append(deletedTables, table)
				changeCounts.Dropped++
			}
		}
		for _, deletedTable := range deletedTables {
			delete(md.ChangeState["table_hashes"], deletedTable)
		}
	}
	l.Debugf("table changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "table_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}
	return changeCounts
}

func (r *reaper) DetectIndexChanges(ctx context.Context, md *sources.DbConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "indexes"
	l.Debug("checking for index changes...")
	if _, ok := md.ChangeState["index_hashes"]; !ok {
		firstRun = true
		md.ChangeState["index_hashes"] = make(map[string]string)
	}
	mvp, ok := metricDefs.GetMetricDef("index_hashes")
	if !ok {
		l.Error("could not get index_hashes sql")
		return changeCounts
	}
	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return changeCounts
	}
	for _, dr := range data {
		objIdent := dr["tag_index"].(string)
		prevHash, ok := md.ChangeState["index_hashes"][objIdent]
		ll := l.WithField("index", dr["tag_index"]).WithField("table", dr["table"])
		if ok { // we have existing state
			if prevHash != (dr["md5"].(string) + dr["is_valid"].(string)) {
				ll.Debug("change detected")
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				md.ChangeState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				ll.Debug("new index detected")
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			md.ChangeState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(md.ChangeState["index_hashes"]) != len(data) {
		deletedIndexes := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentIndexMap := make(map[string]bool)
		for _, dr := range data {
			currentIndexMap[dr["tag_index"].(string)] = true
		}
		for indexName := range md.ChangeState["index_hashes"] {
			_, ok := currentIndexMap[indexName]
			if !ok {
				l.WithField("index", indexName).Debug("deleted index detected")
				influxEntry := metrics.NewMeasurement(data.GetEpoch())
				influxEntry["event"] = "drop"
				influxEntry["tag_index"] = indexName
				detectedChanges = append(detectedChanges, influxEntry)
				deletedIndexes = append(deletedIndexes, indexName)
				changeCounts.Dropped++
			}
		}
		for _, deletedIndex := range deletedIndexes {
			delete(md.ChangeState["index_hashes"], deletedIndex)
		}
	}
	l.Debugf("index changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "index_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}
	return changeCounts
}

func (r *reaper) DetectPrivilegeChanges(ctx context.Context, md *sources.DbConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "privileges"
	l.Debug("checking object privilege changes...")
	if _, ok := md.ChangeState["object_privileges"]; !ok {
		firstRun = true
		md.ChangeState["object_privileges"] = make(map[string]string)
	}
	mvp, ok := metricDefs.GetMetricDef("privilege_changes")
	if !ok || mvp.GetSQL(int(md.Version)) == "" {
		l.Warning("could not get SQL for 'privilege_changes'. cannot detect privilege changes")
		return changeCounts
	}
	// returns rows of: object_type, tag_role, tag_object, privilege_type
	data, err := QueryMeasurements(ctx, md, mvp.GetSQL(int(md.Version)))
	if err != nil {
		l.Error(err)
		return changeCounts
	}
	currentState := make(map[string]bool)
	for _, dr := range data {
		objIdent := fmt.Sprintf("%s#:#%s#:#%s#:#%s", dr["object_type"], dr["tag_role"], dr["tag_object"], dr["privilege_type"])
		ll := l.WithField("role", dr["tag_role"]).
			WithField("object_type", dr["object_type"]).
			WithField("object", dr["tag_object"]).
			WithField("privilege_type", dr["privilege_type"])
		if firstRun {
			md.ChangeState["object_privileges"][objIdent] = ""
		} else {
			_, ok := md.ChangeState["object_privileges"][objIdent]
			if !ok {
				ll.Debug("new object privileges detected")
				dr["event"] = "GRANT"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
				md.ChangeState["object_privileges"][objIdent] = ""
			}
			currentState[objIdent] = true
		}
	}
	// check revokes - exists in old state only
	if !firstRun && len(currentState) > 0 {
		for objPrevRun := range md.ChangeState["object_privileges"] {
			if _, ok := currentState[objPrevRun]; !ok {
				splits := strings.Split(objPrevRun, "#:#")
				l.WithField("role", splits[1]).
					WithField("object_type", splits[0]).
					WithField("object", splits[2]).
					WithField("privilege_type", splits[3]).
					Debug("removed object privileges detected")
				revokeEntry := metrics.NewMeasurement(data.GetEpoch())
				revokeEntry["object_type"] = splits[0]
				revokeEntry["tag_role"] = splits[1]
				revokeEntry["tag_object"] = splits[2]
				revokeEntry["privilege_type"] = splits[3]
				revokeEntry["event"] = "REVOKE"
				detectedChanges = append(detectedChanges, revokeEntry)
				changeCounts.Dropped++
				delete(md.ChangeState["object_privileges"], objPrevRun)
			}
		}
	}
	l.Debugf("object privilege changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "privilege_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}
	return changeCounts
}

func (r *reaper) DetectConfigurationChanges(ctx context.Context, md *sources.DbConn) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults
	l := log.GetLogger(ctx)
	changeCounts.Target = "settings"
	l.Debug("checking for configuration changes...")
	if _, ok := md.ChangeState["configuration_hashes"]; !ok {
		firstRun = true
		md.ChangeState["configuration_hashes"] = make(map[string]string)
	}
	mvp, ok := metricDefs.GetMetricDef("configuration_hashes")
	if !ok {
		l.Error("could not get configuration_hashes sql")
		return changeCounts
	}
	rows, err := md.Conn.Query(ctx, mvp.GetSQL(md.Version))
	if err != nil {
		l.Error(err)
		return changeCounts
	}
	defer rows.Close()
	var (
		objIdent, objValue string
		epoch              int64
	)
	for rows.Next() {
		if rows.Scan(&epoch, &objIdent, &objValue) != nil {
			return changeCounts
		}
		prevРash, ok := md.ChangeState["configuration_hashes"][objIdent]
		ll := l.WithField("setting", objIdent)
		if ok { // we have existing state
			if prevРash != objValue {
				ll.Warningf("settings change detected: %s = %s (prev: %s)", objIdent, objValue, prevРash)
				detectedChanges = append(detectedChanges, metrics.Measurement{
					metrics.EpochColumnName: epoch,
					"tag_setting":           objIdent,
					"value":                 objValue,
					"event":                 "alter"})
				md.ChangeState["configuration_hashes"][objIdent] = objValue
				changeCounts.Altered++
			}
		} else { // check for new, delete not relevant here (pg_upgrade)
			md.ChangeState["configuration_hashes"][objIdent] = objValue
			if firstRun {
				continue
			}
			ll.Debug("new setting detected")
			detectedChanges = append(detectedChanges, metrics.Measurement{
				metrics.EpochColumnName: epoch,
				"tag_setting":           objIdent,
				"value":                 objValue,
				"event":                 "create"})
			changeCounts.Created++
		}
	}
	l.Debugf("configuration changes detected: %d", len(detectedChanges))
	if len(detectedChanges) > 0 {
		r.measurementCh <- metrics.MeasurementEnvelope{
			DBName:     md.Name,
			MetricName: "configuration_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}
	}
	return changeCounts
}

// GetInstanceUpMeasurement returns a single measurement with "instance_up" metric
// used to detect if the instance is up or down
func (r *reaper) GetInstanceUpMeasurement(ctx context.Context, md *sources.DbConn) (metrics.Measurements, error) {
	return metrics.Measurements{
		metrics.Measurement{
			metrics.EpochColumnName: time.Now().UnixNano(),
			"instance_up": func() int {
				if md.Conn.Ping(ctx) == nil {
					return 1
				}
				return 0
			}(), // true if connection is up
		},
	}, nil // always return nil error for the status metric
}

func (r *reaper) GetObjectChangesMeasurement(ctx context.Context, md *sources.DbConn) (metrics.Measurements, error) {
	md.Lock()
	defer md.Unlock()
	spN := r.DetectSprocChanges(ctx, md)
	tblN := r.DetectTableChanges(ctx, md)
	idxN := r.DetectIndexChanges(ctx, md)
	cnfN := r.DetectConfigurationChanges(ctx, md)
	privN := r.DetectPrivilegeChanges(ctx, md)
	if spN.Total()+tblN.Total()+idxN.Total()+cnfN.Total()+privN.Total() == 0 {
		return nil, nil
	}
	m := metrics.NewMeasurement(time.Now().UnixNano())
	m["details"] = strings.Join([]string{spN.String(), tblN.String(), idxN.String(), cnfN.String(), privN.String()}, " ")
	return metrics.Measurements{m}, nil
}

func (r *reaper) CloseResourcesForRemovedMonitoredDBs(hostsToShutDown map[string]bool) {
	for _, prevDB := range r.prevLoopMonitoredDBs {
		if r.monitoredSources.GetMonitoredDatabase(prevDB.GetSource().Name) == nil { // removed from config
			prevDB.Close()
			_ = r.SinksWriter.SyncMetric(prevDB.GetSource().Name, "", sinks.DeleteOp)
		}
	}
	for toShutDownDB := range hostsToShutDown {
		if db := r.monitoredSources.GetMonitoredDatabase(toShutDownDB); db != nil {
			db.Close()
		}
		_ = r.SinksWriter.SyncMetric(toShutDownDB, "", sinks.DeleteOp)
	}
}
