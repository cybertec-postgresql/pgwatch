package sinks

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/jackc/pgx/v5"
)

var (
	cacheLimit      = 512
	highLoadTimeout = time.Second * 5
	deleterDelay    = time.Hour
)

func NewPostgresWriter(ctx context.Context, connstr string, opts *config.MeasurementOpts, metricDefs *metrics.Metrics) (pgw *PostgresWriter, err error) {
	var conn db.PgxPoolIface
	if conn, err = db.New(ctx, connstr); err != nil {
		return
	}
	return NewWriterFromPostgresConn(ctx, conn, opts, metricDefs)
}

func NewWriterFromPostgresConn(ctx context.Context, conn db.PgxPoolIface, opts *config.MeasurementOpts, metricDefs *metrics.Metrics) (pgw *PostgresWriter, err error) {
	pgw = &PostgresWriter{
		сtx:        ctx,
		metricDefs: metricDefs,
		opts:       opts,
		input:      make(chan []metrics.MeasurementMessage, cacheLimit),
		lastError:  make(chan error),
		sinkDb:     conn,
	}
	if err = db.Init(ctx, pgw.sinkDb, func(ctx context.Context, conn db.PgxIface) error {
		log.GetLogger(ctx).Info("initialising the measurement database...")
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
	сtx          context.Context
	sinkDb       db.PgxPoolIface
	metricSchema DbStorageSchemaType
	metricDefs   *metrics.Metrics
	opts         *config.MeasurementOpts
	input        chan []metrics.MeasurementMessage
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
	TagData map[string]any
}

type DbStorageSchemaType int

const (
	DbStorageSchemaPostgres DbStorageSchemaType = iota
	DbStorageSchemaTimescale
)

func (pgw *PostgresWriter) ReadMetricSchemaType() (err error) {
	var isTs bool
	pgw.metricSchema = DbStorageSchemaPostgres
	sqlSchemaType := `SELECT schema_type = 'timescale' FROM admin.storage_schema_type`
	if err = pgw.sinkDb.QueryRow(pgw.сtx, sqlSchemaType).Scan(&isTs); err == nil && isTs {
		pgw.metricSchema = DbStorageSchemaTimescale
	}
	return
}

const (
	epochColumnName string = "epoch_ns" // this column (epoch in nanoseconds) is expected in every metric query
	tagPrefix       string = "tag_"
)

const specialMetricPgbouncer = "^pgbouncer_(stats|pools)$"

var (
	regexIsPgbouncerMetrics         = regexp.MustCompile(specialMetricPgbouncer)
	forceRecreatePGMetricPartitions = false                                             // to signal override PG metrics storage cache
	partitionMapMetric              = make(map[string]ExistingPartitionInfo)            // metric = min/max bounds
	partitionMapMetricDbname        = make(map[string]map[string]ExistingPartitionInfo) // metric[dbname = min/max bounds]
)

// SyncMetric ensures that tables exist for newly added metrics and/or sources
func (pgw *PostgresWriter) SyncMetric(dbUnique, metricName, op string) error {
	if op == "add" {
		return errors.Join(
			pgw.AddDBUniqueMetricToListingTable(dbUnique, metricName),
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
	_, err = pgw.sinkDb.Exec(pgw.сtx, "select admin.ensure_dummy_metrics_table($1)", metric)
	return
}

// Write send the measurements to the cache channel
func (pgw *PostgresWriter) Write(msgs []metrics.MeasurementMessage) error {
	if pgw.сtx.Err() != nil {
		return pgw.сtx.Err()
	}
	select {
	case pgw.input <- msgs:
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
	cache := make([]metrics.MeasurementMessage, 0, cacheLimit)
	cacheTimeout := pgw.opts.BatchingDelay
	tick := time.NewTicker(cacheTimeout)
	for {
		select {
		case <-pgw.сtx.Done(): //check context with high priority
			return
		default:
			select {
			case entry := <-pgw.input:
				cache = append(cache, entry...)
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
			case <-pgw.сtx.Done():
				return
			}
		}
	}
}

// flush sends the cached measurements to the database
func (pgw *PostgresWriter) flush(msgs []metrics.MeasurementMessage) {
	if len(msgs) == 0 {
		return
	}
	logger := log.GetLogger(pgw.сtx).
		WithField("sink", "postgres").
		WithField("db", pgw.sinkDb.Config().ConnConfig.Database)
	tsWarningPrinted := false
	metricsToStorePerMetric := make(map[string][]MeasurementMessagePostgres)
	rowsBatched := 0
	totalRows := 0
	pgPartBounds := make(map[string]ExistingPartitionInfo)                  // metric=min/max
	pgPartBoundsDbName := make(map[string]map[string]ExistingPartitionInfo) // metric=[dbname=min/max]
	var err error

	for _, msg := range msgs {
		if msg.Data == nil || len(msg.Data) == 0 {
			continue
		}
		logger.WithField("data", msg.Data).WithField("len", len(msg.Data)).Debug("Sending To Postgres")

		for _, dataRow := range msg.Data {
			var epochTime time.Time
			var epochNs int64

			tags := make(map[string]any)
			fields := make(map[string]any)

			totalRows++

			if msg.CustomTags != nil {
				for k, v := range msg.CustomTags {
					tags[k] = fmt.Sprintf("%v", v)
				}
			}

			for k, v := range dataRow {
				if v == nil || v == "" {
					continue // not storing NULLs
				}
				if k == epochColumnName {
					epochNs = v.(int64)
				} else if strings.HasPrefix(k, tagPrefix) {
					tag := k[4:]
					tags[tag] = fmt.Sprintf("%v", v)
				} else {
					fields[k] = v
				}
			}

			if epochNs == 0 {
				if !tsWarningPrinted && !regexIsPgbouncerMetrics.MatchString(msg.MetricName) {
					logger.Warning("No timestamp_ns found, server time will be used. measurement:", msg.MetricName)
					tsWarningPrinted = true
				}
				epochTime = time.Now()
			} else {
				epochTime = time.Unix(0, epochNs)
			}

			var metricsArr []MeasurementMessagePostgres
			var ok bool

			metricNameTemp := msg.MetricName

			metricsArr, ok = metricsToStorePerMetric[metricNameTemp]
			if !ok {
				metricsToStorePerMetric[metricNameTemp] = make([]MeasurementMessagePostgres, 0)
			}
			metricsArr = append(metricsArr, MeasurementMessagePostgres{Time: epochTime, DBName: msg.DBName,
				Metric: msg.MetricName, Data: fields, TagData: tags})
			metricsToStorePerMetric[metricNameTemp] = metricsArr

			rowsBatched++

			if pgw.metricSchema == DbStorageSchemaTimescale {
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
			} else if pgw.metricSchema == DbStorageSchemaPostgres {
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
			}
		}
	}

	if pgw.metricSchema == DbStorageSchemaPostgres {
		err = pgw.EnsureMetricDbnameTime(pgPartBoundsDbName, forceRecreatePGMetricPartitions)
	} else if pgw.metricSchema == DbStorageSchemaTimescale {
		err = pgw.EnsureMetricTimescale(pgPartBounds, forceRecreatePGMetricPartitions)
	} else {
		logger.Fatal("should never happen...")
	}
	if forceRecreatePGMetricPartitions {
		forceRecreatePGMetricPartitions = false
	}
	if err != nil {
		pgw.lastError <- err
	}

	// send data to PG, with a separate COPY for all metrics
	logger.Debugf("COPY-ing %d metrics to Postgres metricsDB...", rowsBatched)
	t1 := time.Now()

	for metricName, metrics := range metricsToStorePerMetric {

		getTargetTable := func() pgx.Identifier {
			return pgx.Identifier{metricName}
		}

		getTargetColumns := func() []string {
			return []string{"time", "dbname", "data", "tag_data"}
		}

		for _, m := range metrics {
			l := logger.WithField("db", m.DBName).WithField("metric", m.Metric)
			jsonBytes, err := json.Marshal(m.Data)
			if err != nil {
				logger.Errorf("Skipping 1 metric for [%s:%s] due to JSON conversion error: %s", m.DBName, m.Metric, err)
				continue
			}

			getTagData := func() any {
				if len(m.TagData) > 0 {
					jsonBytesTags, err := json.Marshal(m.TagData)
					if err != nil {
						l.Error(err)
						return nil
					}
					return string(jsonBytesTags)
				}
				return nil
			}

			rows := [][]any{{m.Time, m.DBName, string(jsonBytes), getTagData()}}

			if _, err = pgw.sinkDb.CopyFrom(context.Background(), getTargetTable(), getTargetColumns(), pgx.CopyFromRows(rows)); err != nil {
				l.Error(err)
				forceRecreatePGMetricPartitions = strings.Contains(err.Error(), "no partition")
				if forceRecreatePGMetricPartitions {
					logger.Warning("Some metric partitions might have been removed, halting all metric storage. Trying to re-create all needed partitions on next run")
				}
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
	logger := log.GetLogger(pgw.сtx)
	sqlEnsure := `select * from admin.ensure_partition_metric_time($1, $2)`
	for metric, pb := range pgPartBounds {
		if !strings.HasSuffix(metric, "_realtime") {
			continue
		}
		if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
			return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
		}

		partInfo, ok := partitionMapMetric[metric]
		if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || force {
			err := pgw.sinkDb.QueryRow(pgw.сtx, sqlEnsure, metric, pb.StartTime).Scan(&partInfo)
			if err != nil {
				logger.Error("Failed to create partition on 'metrics':", err)
				return err
			}
			partitionMapMetric[metric] = partInfo
		}
		if pb.EndTime.After(partInfo.EndTime) || force {
			err := pgw.sinkDb.QueryRow(pgw.сtx, sqlEnsure, metric, pb.EndTime).Scan(&partInfo.EndTime)
			if err != nil {
				logger.Error("Failed to create partition on 'metrics':", err)
				return err
			}
			partitionMapMetric[metric] = partInfo
		}
	}
	return nil
}

func (pgw *PostgresWriter) EnsureMetricTimescale(pgPartBounds map[string]ExistingPartitionInfo, force bool) (err error) {
	logger := log.GetLogger(pgw.сtx)
	sqlEnsure := `select * from admin.ensure_partition_timescale($1)`
	for metric := range pgPartBounds {
		if strings.HasSuffix(metric, "_realtime") {
			continue
		}
		if _, ok := partitionMapMetric[metric]; !ok {
			if _, err = pgw.sinkDb.Exec(pgw.сtx, sqlEnsure, metric); err != nil {
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
				if rows, err = pgw.sinkDb.Query(pgw.сtx, sqlEnsure, metric, dbname, pb.StartTime); err != nil {
					return
				}
				if partInfo, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[ExistingPartitionInfo]); err != nil {
					return err
				}
				partitionMapMetricDbname[metric][dbname] = partInfo
			}
			if pb.EndTime.After(partInfo.EndTime) || pb.EndTime.Equal(partInfo.EndTime) || force {
				if rows, err = pgw.sinkDb.Query(pgw.сtx, sqlEnsure, metric, dbname, pb.StartTime); err != nil {
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
	logger := log.GetLogger(pgw.сtx)
	select {
	case <-pgw.сtx.Done():
		return
	case <-time.After(delay):
		// to reduce distracting log messages at startup
	}

	for {
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
					logger.Debugf("Dropping old metric data partition: %s", toDrop)

					if _, err := pgw.sinkDb.Exec(pgw.сtx, sqlDropTable); err != nil {
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
		case <-pgw.сtx.Done():
			return
		case <-time.After(time.Hour * 12):
		}
	}
}

// maintainUniqueSources is a background task that maintains a listing of unique sources for each metric.
// This is used to avoid listing the same source multiple times in Grafana dropdowns.
func (pgw *PostgresWriter) maintainUniqueSources() {
	logger := log.GetLogger(pgw.сtx)
	// due to metrics deletion the listing can go out of sync (a trigger not really wanted)
	sqlGetAdvisoryLock := `SELECT pg_try_advisory_lock(1571543679778230000) AS have_lock` // 1571543679778230000 is just a random bigint
	sqlTopLevelMetrics := `SELECT table_name FROM admin.get_top_level_metric_tables()`
	sqlDistinct := `
	WITH RECURSIVE t(dbname) AS (
		SELECT MIN(dbname) AS dbname FROM %s
		UNION
		SELECT (SELECT MIN(dbname) FROM %s WHERE dbname > t.dbname) FROM t )
	SELECT dbname FROM t WHERE dbname NOTNULL ORDER BY 1`
	sqlDelete := `DELETE FROM admin.all_distinct_dbname_metrics WHERE NOT dbname = ANY($1) and metric = $2`
	sqlDeleteAll := `DELETE FROM admin.all_distinct_dbname_metrics WHERE metric = $1`
	sqlAdd := `
		INSERT INTO admin.all_distinct_dbname_metrics SELECT u, $2 FROM (select unnest($1::text[]) as u) x
		WHERE NOT EXISTS (select * from admin.all_distinct_dbname_metrics where dbname = u and metric = $2)
		RETURNING *`

	for {
		select {
		case <-pgw.сtx.Done():
			return
		case <-time.After(time.Hour * 24):
		}
		var lock bool
		logger.Infof("Trying to get metricsDb listing maintainer advisory lock...") // to only have one "maintainer" in case of a "push" setup, as can get costly
		if err := pgw.sinkDb.QueryRow(pgw.сtx, sqlGetAdvisoryLock).Scan(&lock); err != nil {
			logger.Error("Getting metricsDb listing maintainer advisory lock failed:", err)
			continue
		}
		if !lock {
			logger.Info("Skipping admin.all_distinct_dbname_metrics maintenance as another instance has the advisory lock...")
			continue
		}

		logger.Info("Refreshing admin.all_distinct_dbname_metrics listing table...")
		rows, _ := pgw.sinkDb.Query(pgw.сtx, sqlTopLevelMetrics)
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
			rows, _ := pgw.sinkDb.Query(pgw.сtx, fmt.Sprintf(sqlDistinct, tableName, tableName))
			ret, err := pgx.CollectRows(rows, pgx.RowTo[string])
			// ret, err := DBExecRead(mainContext, metricDb, fmt.Sprintf(sqlDistinct, tableName, tableName))
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

				_, err = pgw.sinkDb.Exec(pgw.сtx, sqlDeleteAll, metricName)
				if err != nil {
					logger.Errorf("Could not delete Postgres all_distinct_dbname_metrics listing table entries for metric '%s': %s", metricName, err)
				}
				continue
			}
			cmdTag, err := pgw.sinkDb.Exec(pgw.сtx, sqlDelete, foundDbnamesArr, metricName)
			if err != nil {
				logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metricName, err)
			} else if cmdTag.RowsAffected() > 0 {
				logger.Infof("Removed %d stale entries from all_distinct_dbname_metrics listing table for metric: %s", cmdTag.RowsAffected(), metricName)
			}
			cmdTag, err = pgw.sinkDb.Exec(pgw.сtx, sqlAdd, foundDbnamesArr, metricName)
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
	err = pgw.sinkDb.QueryRow(pgw.сtx, sqlOldPart, metricAgeDaysThreshold, false).Scan(&res)
	return
}

func (pgw *PostgresWriter) GetOldTimePartitions(metricAgeDaysThreshold int) ([]string, error) {
	sqlGetOldParts := `select admin.get_old_time_partitions($1)`
	rows, err := pgw.sinkDb.Query(pgw.сtx, sqlGetOldParts, metricAgeDaysThreshold)
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
	_, err := pgw.sinkDb.Exec(pgw.сtx, sql, dbUnique, metric)
	return err
}
