package sinks

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	cacheLimit      = 256
	highLoadTimeout = time.Second * 5
	deleterDelay    = time.Hour
	targetColumns   = [...]string{"time", "dbname", "data", "tag_data"}
)

func NewPostgresWriter(ctx context.Context, connstr string, opts *CmdOpts) (pgw *PostgresWriter, err error) {
	var conn db.PgxPoolIface
	if conn, err = db.New(ctx, connstr); err != nil {
		return
	}
	return NewWriterFromPostgresConn(ctx, conn, opts)
}

func NewWriterFromPostgresConn(ctx context.Context, conn db.PgxPoolIface, opts *CmdOpts) (pgw *PostgresWriter, err error) {
	l := log.GetLogger(ctx).WithField("sink", "postgres").WithField("db", conn.Config().ConnConfig.Database)
	ctx = log.WithLogger(ctx, l)
	pgw = &PostgresWriter{
		ctx:       ctx,
		opts:      opts,
		input:     make(chan metrics.MeasurementEnvelope, cacheLimit),
		lastError: make(chan error),
		sinkDb:    conn,
	}
	if err = db.Init(ctx, pgw.sinkDb, func(ctx context.Context, conn db.PgxIface) error {
		l.Info("initialising measurements database...")
		exists, err := db.DoesSchemaExist(ctx, conn, "admin")
		if err != nil || exists {
			return err
		}
		for _, sql := range metricSchemaSQLs {
			if _, err = conn.Exec(ctx, sql); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return
	}
	if err = pgw.ReadMetricSchemaType(); err != nil {
		return
	}
	if err = pgw.EnsureBuiltinMetricDummies(); err != nil {
		return
	}
	go pgw.deleteOldPartitions(deleterDelay)
	go pgw.maintainUniqueSources()
	go pgw.poll()
	l.Info(`measurements sink is activated`)
	return
}

//go:embed sql/admin_schema.sql
var sqlMetricAdminSchema string

//go:embed sql/admin_functions.sql
var sqlMetricAdminFunctions string

//go:embed sql/ensure_partition_postgres.sql
var sqlMetricEnsurePartitionPostgres string

//go:embed sql/ensure_partition_timescale.sql
var sqlMetricEnsurePartitionTimescale string

//go:embed sql/change_chunk_interval.sql
var sqlMetricChangeChunkIntervalTimescale string

//go:embed sql/change_compression_interval.sql
var sqlMetricChangeCompressionIntervalTimescale string

var (
	metricSchemaSQLs = []string{
		sqlMetricAdminSchema,
		sqlMetricAdminFunctions,
		sqlMetricEnsurePartitionPostgres,
		sqlMetricEnsurePartitionTimescale,
		sqlMetricChangeChunkIntervalTimescale,
		sqlMetricChangeCompressionIntervalTimescale,
	}
)

// PostgresWriter is a sink that writes metric measurements to a Postgres database.
// At the moment, it supports both Postgres and TimescaleDB as a storage backend.
// However, one is able to use any Postgres-compatible database as a storage backend,
// e.g. PGEE, Citus, Greenplum, CockroachDB, etc.
type PostgresWriter struct {
	ctx          context.Context
	sinkDb       db.PgxPoolIface
	metricSchema DbStorageSchemaType
	opts         *CmdOpts
	input        chan metrics.MeasurementEnvelope
	lastError    chan error
}

type ExistingPartitionInfo struct {
	StartTime time.Time
	EndTime   time.Time
}

type MeasurementMessagePostgres struct {
	Time    time.Time
	DBName  string
	Metric  string
	Data    map[string]any
	TagData map[string]string
}

const sourcesMaintenanceInterval = time.Hour * 24

type DbStorageSchemaType int

const (
	DbStorageSchemaPostgres DbStorageSchemaType = iota
	DbStorageSchemaTimescale
)

func (pgw *PostgresWriter) ReadMetricSchemaType() (err error) {
	var isTs bool
	pgw.metricSchema = DbStorageSchemaPostgres
	sqlSchemaType := `SELECT schema_type = 'timescale' FROM admin.storage_schema_type`
	if err = pgw.sinkDb.QueryRow(pgw.ctx, sqlSchemaType).Scan(&isTs); err == nil && isTs {
		pgw.metricSchema = DbStorageSchemaTimescale
	}
	return
}

var (
	forceRecreatePartitions  = false                                             // to signal override PG metrics storage cache
	partitionMapMetric       = make(map[string]ExistingPartitionInfo)            // metric = min/max bounds
	partitionMapMetricDbname = make(map[string]map[string]ExistingPartitionInfo) // metric[dbname = min/max bounds]
)

// SyncMetric ensures that tables exist for newly added metrics and/or sources
func (pgw *PostgresWriter) SyncMetric(sourceName, metricName string, op SyncOp) error {
	if op == AddOp {
		return errors.Join(
			pgw.AddDBUniqueMetricToListingTable(sourceName, metricName),
			pgw.EnsureMetricDummy(metricName), // ensure that there is at least an empty top-level table not to get ugly Grafana notifications
		)
	}
	return nil
}

// EnsureBuiltinMetricDummies creates empty tables for all built-in metrics if they don't exist
func (pgw *PostgresWriter) EnsureBuiltinMetricDummies() (err error) {
	for _, name := range metrics.GetDefaultBuiltInMetrics() {
		err = errors.Join(err, pgw.EnsureMetricDummy(name))
	}
	return
}

// EnsureMetricDummy creates an empty table for a metric measurements if it doesn't exist
func (pgw *PostgresWriter) EnsureMetricDummy(metric string) (err error) {
	_, err = pgw.sinkDb.Exec(pgw.ctx, "select admin.ensure_dummy_metrics_table($1)", metric)
	return
}

// Write sends the measurements to the cache channel
func (pgw *PostgresWriter) Write(msg metrics.MeasurementEnvelope) error {
	if pgw.ctx.Err() != nil {
		return pgw.ctx.Err()
	}
	select {
	case pgw.input <- msg:
		// msgs sent
	case <-time.After(highLoadTimeout):
		// msgs dropped due to a huge load, check stdout or file for detailed log
	}
	select {
	case err := <-pgw.lastError:
		return err
	default:
		return nil
	}
}

// poll is the main loop that reads from the input channel and flushes the data to the database
func (pgw *PostgresWriter) poll() {
	cache := make([]metrics.MeasurementEnvelope, 0, cacheLimit)
	cacheTimeout := pgw.opts.BatchingDelay
	tick := time.NewTicker(cacheTimeout)
	for {
		select {
		case <-pgw.ctx.Done(): //check context with high priority
			return
		default:
			select {
			case entry := <-pgw.input:
				cache = append(cache, entry)
				if len(cache) < cacheLimit {
					break
				}
				tick.Stop()
				pgw.flush(cache)
				cache = cache[:0]
				tick = time.NewTicker(cacheTimeout)
			case <-tick.C:
				pgw.flush(cache)
				cache = cache[:0]
			case <-pgw.ctx.Done():
				return
			}
		}
	}
}

func newCopyFromMeasurements(rows []metrics.MeasurementEnvelope) *copyFromMeasurements {
	return &copyFromMeasurements{envelopes: rows, envelopeIdx: -1, measurementIdx: -1}
}

type copyFromMeasurements struct {
	envelopes      []metrics.MeasurementEnvelope
	envelopeIdx    int
	measurementIdx int // index of the current measurement in the envelope
	metricName     string
}

func (c *copyFromMeasurements) Next() bool {
	for {
		// Check if we need to advance to the next envelope
		if c.envelopeIdx < 0 || c.measurementIdx+1 >= len(c.envelopes[c.envelopeIdx].Data) {
			// Advance to next envelope
			c.envelopeIdx++
			if c.envelopeIdx >= len(c.envelopes) {
				return false // No more envelopes
			}
			c.measurementIdx = -1 // Reset measurement index for new envelope

			// Set metric name from first envelope, or detect metric boundary
			if c.metricName == "" {
				c.metricName = c.envelopes[c.envelopeIdx].MetricName
			} else if c.metricName != c.envelopes[c.envelopeIdx].MetricName {
				// We've hit a different metric - we're done with current metric
				// Reset position to process this envelope on next call
				c.envelopeIdx--
				c.measurementIdx = len(c.envelopes[c.envelopeIdx].Data) // Set to length so we've "finished" this envelope
				c.metricName = ""                                       // Reset for next metric
				return false
			}
		}

		// Advance to next measurement in current envelope
		c.measurementIdx++
		if c.measurementIdx < len(c.envelopes[c.envelopeIdx].Data) {
			return true // Found valid measurement
		}
		// If we reach here, we've exhausted current envelope, loop will advance to next envelope
	}
}

func (c *copyFromMeasurements) EOF() bool {
	return c.envelopeIdx >= len(c.envelopes)
}

func (c *copyFromMeasurements) Values() ([]any, error) {
	row := maps.Clone(c.envelopes[c.envelopeIdx].Data[c.measurementIdx])
	tagRow := maps.Clone(c.envelopes[c.envelopeIdx].CustomTags)
	if tagRow == nil {
		tagRow = make(map[string]string)
	}
	for k, v := range row {
		if strings.HasPrefix(k, metrics.TagPrefix) {
			tagRow[strings.TrimPrefix(k, metrics.TagPrefix)] = fmt.Sprintf("%v", v)
			delete(row, k)
		}
	}
	jsonTags, terr := jsoniter.ConfigFastest.MarshalToString(tagRow)
	json, err := jsoniter.ConfigFastest.MarshalToString(row)
	if err != nil || terr != nil {
		return nil, errors.Join(err, terr)
	}
	return []any{time.Unix(0, c.envelopes[c.envelopeIdx].Data.GetEpoch()), c.envelopes[c.envelopeIdx].DBName, json, jsonTags}, nil
}

func (c *copyFromMeasurements) Err() error {
	return nil
}

func (c *copyFromMeasurements) MetricName() pgx.Identifier {
	return pgx.Identifier{c.envelopes[c.envelopeIdx+1].MetricName} // Metric name is taken from the next envelope
}

// flush sends the cached measurements to the database
func (pgw *PostgresWriter) flush(msgs []metrics.MeasurementEnvelope) {
	if len(msgs) == 0 {
		return
	}
	logger := log.GetLogger(pgw.ctx)
	// metricsToStorePerMetric := make(map[string][]MeasurementMessagePostgres)
	pgPartBounds := make(map[string]ExistingPartitionInfo)                  // metric=min/max
	pgPartBoundsDbName := make(map[string]map[string]ExistingPartitionInfo) // metric=[dbname=min/max]
	var err error

	slices.SortFunc(msgs, func(a, b metrics.MeasurementEnvelope) int {
		if a.MetricName < b.MetricName {
			return -1
		} else if a.MetricName > b.MetricName {
			return 1
		}
		return 0
	})

	for _, msg := range msgs {
		for _, dataRow := range msg.Data {
			epochTime := time.Unix(0, metrics.Measurement(dataRow).GetEpoch())
			switch pgw.metricSchema {
			case DbStorageSchemaTimescale:
				// set min/max timestamps to check/create partitions
				bounds, ok := pgPartBounds[msg.MetricName]
				if !ok || (ok && epochTime.Before(bounds.StartTime)) {
					bounds.StartTime = epochTime
					pgPartBounds[msg.MetricName] = bounds
				}
				if !ok || (ok && epochTime.After(bounds.EndTime)) {
					bounds.EndTime = epochTime
					pgPartBounds[msg.MetricName] = bounds
				}
			case DbStorageSchemaPostgres:
				_, ok := pgPartBoundsDbName[msg.MetricName]
				if !ok {
					pgPartBoundsDbName[msg.MetricName] = make(map[string]ExistingPartitionInfo)
				}
				bounds, ok := pgPartBoundsDbName[msg.MetricName][msg.DBName]
				if !ok || (ok && epochTime.Before(bounds.StartTime)) {
					bounds.StartTime = epochTime
					pgPartBoundsDbName[msg.MetricName][msg.DBName] = bounds
				}
				if !ok || (ok && epochTime.After(bounds.EndTime)) {
					bounds.EndTime = epochTime
					pgPartBoundsDbName[msg.MetricName][msg.DBName] = bounds
				}
			default:
				logger.Fatal("unknown storage schema...")
			}
		}
	}

	switch pgw.metricSchema {
	case DbStorageSchemaPostgres:
		err = pgw.EnsureMetricDbnameTime(pgPartBoundsDbName, forceRecreatePartitions)
	case DbStorageSchemaTimescale:
		err = pgw.EnsureMetricTimescale(pgPartBounds, forceRecreatePartitions)
	default:
		logger.Fatal("unknown storage schema...")
	}
	forceRecreatePartitions = false
	if err != nil {
		pgw.lastError <- err
	}

	var rowsBatched, n int64
	t1 := time.Now()
	cfm := newCopyFromMeasurements(msgs)
	for !cfm.EOF() {
		n, err = pgw.sinkDb.CopyFrom(context.Background(), cfm.MetricName(), targetColumns[:], cfm)
		rowsBatched += n
		if err != nil {
			logger.Error(err)
			if PgError, ok := err.(*pgconn.PgError); ok {
				forceRecreatePartitions = PgError.Code == "23514"
			}
			if forceRecreatePartitions {
				logger.Warning("Some metric partitions might have been removed, halting all metric storage. Trying to re-create all needed partitions on next run")
			}
		}
	}
	diff := time.Since(t1)
	if err == nil {
		logger.WithField("rows", rowsBatched).WithField("elapsed", diff).Info("measurements written")
		return
	}
	pgw.lastError <- err
}

// EnsureMetricTime creates special partitions if Timescale used for realtime metrics
func (pgw *PostgresWriter) EnsureMetricTime(pgPartBounds map[string]ExistingPartitionInfo, force bool) error {
	logger := log.GetLogger(pgw.ctx)
	sqlEnsure := `select part_available_from, part_available_to from admin.ensure_partition_metric_time($1, $2)`
	for metric, pb := range pgPartBounds {
		if !strings.HasSuffix(metric, "_realtime") {
			continue
		}
		if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
			return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
		}

		partInfo, ok := partitionMapMetric[metric]
		if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || force {
			err := pgw.sinkDb.QueryRow(pgw.ctx, sqlEnsure, metric, pb.StartTime).
				Scan(&partInfo.StartTime, &partInfo.EndTime)
			if err != nil {
				logger.Error("Failed to create partition on 'metrics': ", err)
				return err
			}
			partitionMapMetric[metric] = partInfo
		}
		if pb.EndTime.After(partInfo.EndTime) || force {
			err := pgw.sinkDb.QueryRow(pgw.ctx, sqlEnsure, metric, pb.EndTime).Scan(nil, &partInfo.EndTime)
			if err != nil {
				logger.Error("Failed to create partition on 'metrics': ", err)
				return err
			}
			partitionMapMetric[metric] = partInfo
		}
	}
	return nil
}

func (pgw *PostgresWriter) EnsureMetricTimescale(pgPartBounds map[string]ExistingPartitionInfo, force bool) (err error) {
	logger := log.GetLogger(pgw.ctx)
	sqlEnsure := `select * from admin.ensure_partition_timescale($1)`
	for metric := range pgPartBounds {
		if strings.HasSuffix(metric, "_realtime") {
			continue
		}
		if _, ok := partitionMapMetric[metric]; !ok {
			if _, err = pgw.sinkDb.Exec(pgw.ctx, sqlEnsure, metric); err != nil {
				logger.Errorf("Failed to create a TimescaleDB table for metric '%s': %v", metric, err)
				return err
			}
			partitionMapMetric[metric] = ExistingPartitionInfo{}
		}
	}
	return pgw.EnsureMetricTime(pgPartBounds, force)
}

func (pgw *PostgresWriter) EnsureMetricDbnameTime(metricDbnamePartBounds map[string]map[string]ExistingPartitionInfo, force bool) (err error) {
	var rows pgx.Rows
	sqlEnsure := `select * from admin.ensure_partition_metric_dbname_time($1, $2, $3)`
	for metric, dbnameTimestampMap := range metricDbnamePartBounds {
		_, ok := partitionMapMetricDbname[metric]
		if !ok {
			partitionMapMetricDbname[metric] = make(map[string]ExistingPartitionInfo)
		}

		for dbname, pb := range dbnameTimestampMap {
			if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
				return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
			}
			partInfo, ok := partitionMapMetricDbname[metric][dbname]
			if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || force {
				if rows, err = pgw.sinkDb.Query(pgw.ctx, sqlEnsure, metric, dbname, pb.StartTime); err != nil {
					return
				}
				if partInfo, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[ExistingPartitionInfo]); err != nil {
					return err
				}
				partitionMapMetricDbname[metric][dbname] = partInfo
			}
			if pb.EndTime.After(partInfo.EndTime) || pb.EndTime.Equal(partInfo.EndTime) || force {
				if rows, err = pgw.sinkDb.Query(pgw.ctx, sqlEnsure, metric, dbname, pb.StartTime); err != nil {
					return
				}
				if partInfo, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[ExistingPartitionInfo]); err != nil {
					return err
				}
				partitionMapMetricDbname[metric][dbname] = partInfo
			}
		}
	}
	return nil
}

// deleteOldPartitions is a background task that deletes old partitions from the measurements DB
func (pgw *PostgresWriter) deleteOldPartitions(delay time.Duration) {
	metricAgeDaysThreshold := pgw.opts.Retention
	if metricAgeDaysThreshold <= 0 {
		return
	}
	logger := log.GetLogger(pgw.ctx)
	select {
	case <-pgw.ctx.Done():
		return
	case <-time.After(delay):
		// to reduce distracting log messages at startup
	}

	for {
		if !pgw.StartMaintenanceActivity(pgw.ctx, "partitions_maintenance") {
			continue
		}

		if pgw.metricSchema == DbStorageSchemaTimescale {
			partsDropped, err := pgw.DropOldTimePartitions(metricAgeDaysThreshold)
			if err != nil {
				logger.Errorf("Failed to drop old partitions (>%d days) from Postgres: %v", metricAgeDaysThreshold, err)
				continue
			}
			logger.Infof("Dropped %d old metric partitions...", partsDropped)
		} else if pgw.metricSchema == DbStorageSchemaPostgres {
			partsToDrop, err := pgw.GetOldTimePartitions(metricAgeDaysThreshold)
			if err != nil {
				logger.Errorf("Failed to get a listing of old (>%d days) time partitions from Postgres metrics DB - check that the admin.get_old_time_partitions() function is rolled out: %v", metricAgeDaysThreshold, err)
				time.Sleep(time.Second * 300)
				continue
			}
			if len(partsToDrop) > 0 {
				logger.Infof("Dropping %d old metric partitions one by one...", len(partsToDrop))
				for _, toDrop := range partsToDrop {
					sqlDropTable := `DROP TABLE IF EXISTS ` + toDrop

					if _, err := pgw.sinkDb.Exec(pgw.ctx, sqlDropTable); err != nil {
						logger.Errorf("Failed to drop old partition %s from Postgres metrics DB: %w", toDrop, err)
						time.Sleep(time.Second * 300)
					} else {
						time.Sleep(time.Second * 5)
					}
				}
			} else {
				logger.Infof("No old metric partitions found to drop...")
			}
		}
		select {
		case <-pgw.ctx.Done():
			return
		case <-time.After(time.Hour * 12):
		}
	}
}

func (pgw *PostgresWriter) StartMaintenanceActivity(ctx context.Context, activity string) bool {
	logger := log.GetLogger(ctx)
	tx, err := pgw.sinkDb.Begin(pgw.ctx)
	if err != nil {
		logger.Errorf("Starting transaction for %s maintenance failed: %w", activity, err)
		return false
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(pgw.ctx)
		} else {
			_ = tx.Commit(pgw.ctx)
		}
		_, _ = pgw.sinkDb.Exec(pgw.ctx, `SELECT pg_advisory_unlock(1571543679778230000)`)
	}()

	var check bool
	if err = tx.QueryRow(pgw.ctx, `SELECT now() - value::timestamptz > $1::interval FROM admin.config WHERE key = $2`,
		sourcesMaintenanceInterval, activity).Scan(&check); err != nil {
		logger.Errorf("Checking partition maintenance interval failed: %w", err)
		return false
	}
	if !check {
		logger.Infof("Skipping %s maintenance as it was handled in the last 24 hours...", activity)
		return false
	}

	var lock bool
	logger.Infof("Trying to get %s maintenance advisory lock...", activity)
	if err = tx.QueryRow(pgw.ctx, `SELECT pg_try_advisory_lock(1571543679778230000) AS have_lock`).Scan(&lock); err != nil {
		logger.Error(err)
		return false
	}
	if !lock {
		logger.Infof("Skipping %s maintenance as another instance has the advisory lock...", activity)
		return false
	}

	if _, err = tx.Exec(pgw.ctx, `UPDATE admin.config SET value = now()::text WHERE key = $1`, activity); err != nil {
		logger.Error(err)
		return false
	}
	return true
}

// maintainUniqueSources is a background task that maintains a listing of unique sources for each metric.
// This is used to avoid listing the same source multiple times in Grafana dropdowns.
func (pgw *PostgresWriter) maintainUniqueSources() {
	logger := log.GetLogger(pgw.ctx)
	// due to metrics deletion the listing can go out of sync (a trigger not really wanted)
	sqlTopLevelMetrics := `SELECT table_name FROM admin.get_top_level_metric_tables()`
	sqlDistinct := `WITH RECURSIVE t(dbname) AS (
		SELECT MIN(dbname) AS dbname FROM %s
		UNION
		SELECT (SELECT MIN(dbname) FROM %s WHERE dbname > t.dbname) FROM t )
	SELECT dbname FROM t WHERE dbname NOTNULL ORDER BY 1`
	sqlDelete := `DELETE FROM admin.all_distinct_dbname_metrics WHERE NOT dbname = ANY($1) and metric = $2`
	sqlDeleteAll := `DELETE FROM admin.all_distinct_dbname_metrics WHERE metric = $1`
	sqlAdd := `INSERT INTO admin.all_distinct_dbname_metrics SELECT u, $2 FROM (select unnest($1::text[]) as u) x
		WHERE NOT EXISTS (select * from admin.all_distinct_dbname_metrics where dbname = u and metric = $2)
		RETURNING *`

	for {
		select {
		case <-pgw.ctx.Done():
			return
		case <-time.After(sourcesMaintenanceInterval):
		}

		if !pgw.StartMaintenanceActivity(pgw.ctx, "sources_maintenance") {
			continue
		}

		logger.Info("Refreshing admin.all_distinct_dbname_metrics listing table...")
		rows, _ := pgw.sinkDb.Query(pgw.ctx, sqlTopLevelMetrics)
		allDistinctMetricTables, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			logger.Error(err)
			continue
		}

		for _, tableName := range allDistinctMetricTables {
			foundDbnamesMap := make(map[string]bool)
			foundDbnamesArr := make([]string, 0)
			metricName := strings.Replace(tableName, "public.", "", 1)

			logger.Debugf("Refreshing all_distinct_dbname_metrics listing for metric: %s", metricName)
			rows, _ := pgw.sinkDb.Query(pgw.ctx, fmt.Sprintf(sqlDistinct, tableName, tableName))
			ret, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for '%s': %s", metricName, err)
				break
			}
			for _, drDbname := range ret {
				foundDbnamesMap[drDbname] = true // "set" behaviour, don't want duplicates
			}

			// delete all that are not known and add all that are not there
			for k := range foundDbnamesMap {
				foundDbnamesArr = append(foundDbnamesArr, k)
			}
			if len(foundDbnamesArr) == 0 { // delete all entries for given metric
				logger.Debugf("Deleting Postgres all_distinct_dbname_metrics listing table entries for metric '%s':", metricName)

				_, err = pgw.sinkDb.Exec(pgw.ctx, sqlDeleteAll, metricName)
				if err != nil {
					logger.Errorf("Could not delete Postgres all_distinct_dbname_metrics listing table entries for metric '%s': %s", metricName, err)
				}
				continue
			}
			cmdTag, err := pgw.sinkDb.Exec(pgw.ctx, sqlDelete, foundDbnamesArr, metricName)
			if err != nil {
				logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metricName, err)
			} else if cmdTag.RowsAffected() > 0 {
				logger.Infof("Removed %d stale entries from all_distinct_dbname_metrics listing table for metric: %s", cmdTag.RowsAffected(), metricName)
			}
			cmdTag, err = pgw.sinkDb.Exec(pgw.ctx, sqlAdd, foundDbnamesArr, metricName)
			if err != nil {
				logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metricName, err)
			} else if cmdTag.RowsAffected() > 0 {
				logger.Infof("Added %d entry to the Postgres all_distinct_dbname_metrics listing table for metric: %s", cmdTag.RowsAffected(), metricName)
			}
			time.Sleep(time.Minute)
		}

	}
}

func (pgw *PostgresWriter) DropOldTimePartitions(metricAgeDaysThreshold int) (res int, err error) {
	sqlOldPart := `select admin.drop_old_time_partitions($1, $2)`
	err = pgw.sinkDb.QueryRow(pgw.ctx, sqlOldPart, metricAgeDaysThreshold, false).Scan(&res)
	return
}

func (pgw *PostgresWriter) GetOldTimePartitions(metricAgeDaysThreshold int) ([]string, error) {
	sqlGetOldParts := `select admin.get_old_time_partitions($1)`
	rows, err := pgw.sinkDb.Query(pgw.ctx, sqlGetOldParts, metricAgeDaysThreshold)
	if err == nil {
		return pgx.CollectRows(rows, pgx.RowTo[string])
	}
	return nil, err
}

func (pgw *PostgresWriter) AddDBUniqueMetricToListingTable(dbUnique, metric string) error {
	sql := `insert into admin.all_distinct_dbname_metrics
			select $1, $2
			where not exists (
				select * from admin.all_distinct_dbname_metrics where dbname = $1 and metric = $2
			)`
	_, err := pgw.sinkDb.Exec(pgw.ctx, sql, dbUnique, metric)
	return err
}
