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

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	cacheLimit      = 256
	highLoadTimeout = time.Second * 5
	targetColumns   = [...]string{"time", "dbname", "data", "tag_data"}
)

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
	ctx                      context.Context
	sinkDb                   db.PgxPoolIface
	metricSchema             DbStorageSchemaType
	opts                     *CmdOpts
	retentionInterval        time.Duration
	maintenanceInterval      time.Duration
	input                    chan metrics.MeasurementEnvelope
	lastError                chan error
	forceRecreatePartitions  bool // to signal override PG metrics storage cache
	partitionMapMetric       map[string]ExistingPartitionInfo // metric = min/max bounds
	partitionMapMetricDbname map[string]map[string]ExistingPartitionInfo // metric[dbname = min/max bounds]
}

func NewPostgresWriter(ctx context.Context, connstr string, opts *CmdOpts) (pgw *PostgresWriter, err error) {
	var conn db.PgxPoolIface
	if conn, err = db.New(ctx, connstr); err != nil {
		return
	}
	return NewWriterFromPostgresConn(ctx, conn, opts)
}

var ErrNeedsMigration = errors.New("sink database schema is outdated, please run migrations using `pgwatch config upgrade` command")

func NewWriterFromPostgresConn(ctx context.Context, conn db.PgxPoolIface, opts *CmdOpts) (pgw *PostgresWriter, err error) {
	l := log.GetLogger(ctx).WithField("sink", "postgres").WithField("db", conn.Config().ConnConfig.Database)
	ctx = log.WithLogger(ctx, l)
	pgw = &PostgresWriter{
		ctx:                      ctx,
		opts:                     opts,
		input:                    make(chan metrics.MeasurementEnvelope, cacheLimit),
		lastError:                make(chan error),
		sinkDb:                   conn,
		forceRecreatePartitions:  false,
		partitionMapMetric:       make(map[string]ExistingPartitionInfo),
		partitionMapMetricDbname: make(map[string]map[string]ExistingPartitionInfo),
	}
	l.Info("initialising measurements database...")
	if err = pgw.init(); err != nil {
		return nil, err
	}
	if err = pgw.ReadMetricSchemaType(); err != nil {
		return nil, err
	}
	if err = pgw.EnsureBuiltinMetricDummies(); err != nil {
		return nil, err
	}
	pgw.scheduleJob(pgw.maintenanceInterval, func() {
		pgw.DeleteOldPartitions()
		pgw.MaintainUniqueSources()
	})
	go pgw.poll()
	l.Info(`measurements sink is activated`)
	return
}

func (pgw *PostgresWriter) init() (err error) {
	return db.Init(pgw.ctx, pgw.sinkDb, func(ctx context.Context, conn db.PgxIface) error {
		var isValidPartitionInterval bool
		if err = conn.QueryRow(ctx,
			"SELECT extract(epoch from $1::interval), extract(epoch from $2::interval), $3::interval >= '1h'::interval",
			pgw.opts.RetentionInterval, pgw.opts.MaintenanceInterval, pgw.opts.PartitionInterval,
		).Scan(&pgw.retentionInterval, &pgw.maintenanceInterval, &isValidPartitionInterval); err != nil {
			return err
		}

		// epoch returns seconds but time.Duration represents nanoseconds
		pgw.retentionInterval *= time.Second
		pgw.maintenanceInterval *= time.Second

		if !isValidPartitionInterval {
			return fmt.Errorf("--partition-interval must be at least 1 hour, got: %s", pgw.opts.PartitionInterval)
		}
		if pgw.maintenanceInterval < 0 {
			return errors.New("--maintenance-interval must be a positive PostgreSQL interval or 0 to disable it")
		}
		if pgw.retentionInterval < time.Hour && pgw.retentionInterval != 0 {
			return errors.New("--retention must be at least 1 hour PostgreSQL interval or 0 to disable it")
		}

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
	})
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

type DbStorageSchemaType int

const (
	DbStorageSchemaPostgres DbStorageSchemaType = iota
	DbStorageSchemaTimescale
)

func (pgw *PostgresWriter) scheduleJob(interval time.Duration, job func()) {
	if interval > 0 {
		go func() {
			for {
				select {
				case <-pgw.ctx.Done():
					return
				case <-time.After(interval):
					job()
				}
			}
		}()
	}
}

func (pgw *PostgresWriter) ReadMetricSchemaType() (err error) {
	var isTs bool
	pgw.metricSchema = DbStorageSchemaPostgres
	sqlSchemaType := `SELECT schema_type = 'timescale' FROM admin.storage_schema_type`
	if err = pgw.sinkDb.QueryRow(pgw.ctx, sqlSchemaType).Scan(&isTs); err == nil && isTs {
		pgw.metricSchema = DbStorageSchemaTimescale
	}
	return
}

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
	_, err = pgw.sinkDb.Exec(pgw.ctx, "SELECT admin.ensure_dummy_metrics_table($1)", metric)
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

func (c *copyFromMeasurements) NextEnvelope() bool {
	c.envelopeIdx++
	c.measurementIdx = -1
	return c.envelopeIdx < len(c.envelopes)
}

func (c *copyFromMeasurements) Next() bool {
	for {
		// Check if we need to advance to the next envelope
		if c.envelopeIdx < 0 || c.measurementIdx+1 >= len(c.envelopes[c.envelopeIdx].Data) {
			// Advance to next envelope
			if ok := c.NextEnvelope(); !ok {
				return false // No more envelopes
			}
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
		if after, ok := strings.CutPrefix(k, metrics.TagPrefix); ok {
			tagRow[after] = fmt.Sprintf("%v", v)
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

func (c *copyFromMeasurements) MetricName() (ident pgx.Identifier) {
	if c.envelopeIdx+1 < len(c.envelopes) {
		// Metric name is taken from the next envelope
		ident = pgx.Identifier{c.envelopes[c.envelopeIdx+1].MetricName}
	}
	return
}

// flush sends the cached measurements to the database
func (pgw *PostgresWriter) flush(msgs []metrics.MeasurementEnvelope) {
	if len(msgs) == 0 {
		return
	}
	logger := log.GetLogger(pgw.ctx)
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
		err = pgw.EnsureMetricDbnameTime(pgPartBoundsDbName)
	case DbStorageSchemaTimescale:
		err = pgw.EnsureMetricTimescale(pgPartBounds)
	default:
		logger.Fatal("unknown storage schema...")
	}
	pgw.forceRecreatePartitions = false
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
				pgw.forceRecreatePartitions = PgError.Code == "23514"
			}
			if pgw.forceRecreatePartitions {
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

func (pgw *PostgresWriter) EnsureMetricTimescale(pgPartBounds map[string]ExistingPartitionInfo) (err error) {
	logger := log.GetLogger(pgw.ctx)
	sqlEnsure := `select * from admin.ensure_partition_timescale($1)`
	for metric := range pgPartBounds {
		if _, ok := pgw.partitionMapMetric[metric]; !ok {
			if _, err = pgw.sinkDb.Exec(pgw.ctx, sqlEnsure, metric); err != nil {
				logger.Errorf("Failed to create a TimescaleDB table for metric '%s': %v", metric, err)
				return err
			}
			pgw.partitionMapMetric[metric] = ExistingPartitionInfo{}
		}
	}
	return
}

func (pgw *PostgresWriter) EnsureMetricDbnameTime(metricDbnamePartBounds map[string]map[string]ExistingPartitionInfo) (err error) {
	var rows pgx.Rows
	sqlEnsure := `select * from admin.ensure_partition_metric_dbname_time($1, $2, $3, $4)`
	for metric, dbnameTimestampMap := range metricDbnamePartBounds {
		_, ok := pgw.partitionMapMetricDbname[metric]
		if !ok {
			pgw.partitionMapMetricDbname[metric] = make(map[string]ExistingPartitionInfo)
		}

		for dbname, pb := range dbnameTimestampMap {
			if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
				return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
			}
			partInfo, ok := pgw.partitionMapMetricDbname[metric][dbname]
			if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || pgw.forceRecreatePartitions {
				if rows, err = pgw.sinkDb.Query(pgw.ctx, sqlEnsure, metric, dbname, pb.StartTime, pgw.opts.PartitionInterval); err != nil {
					return
				}
				if partInfo, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[ExistingPartitionInfo]); err != nil {
					return err
				}
				pgw.partitionMapMetricDbname[metric][dbname] = partInfo
			}
			if pb.EndTime.After(partInfo.EndTime) || pb.EndTime.Equal(partInfo.EndTime) || pgw.forceRecreatePartitions {
				if rows, err = pgw.sinkDb.Query(pgw.ctx, sqlEnsure, metric, dbname, pb.EndTime, pgw.opts.PartitionInterval); err != nil {
					return
				}
				if partInfo, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[ExistingPartitionInfo]); err != nil {
					return err
				}
				pgw.partitionMapMetricDbname[metric][dbname] = partInfo
			}
		}
	}
	return nil
}

// DeleteOldPartitions is a background task that deletes old partitions from the measurements DB
func (pgw *PostgresWriter) DeleteOldPartitions() {
	l := log.GetLogger(pgw.ctx)
	var partsDropped int
	err := pgw.sinkDb.QueryRow(pgw.ctx, `SELECT admin.drop_old_time_partitions(older_than => $1::interval)`,
		pgw.opts.RetentionInterval).Scan(&partsDropped)
	if err != nil {
		l.Error("Could not drop old time partitions:", err)
	} else if partsDropped > 0 {
		l.Infof("Dropped %d old time partitions", partsDropped)
	}
}

// MaintainUniqueSources is a background task that maintains a mapping of unique sources
// in each metric table in admin.all_distinct_dbname_metrics.
// This is used to avoid listing the same source multiple times in Grafana dropdowns.
func (pgw *PostgresWriter) MaintainUniqueSources() {
	logger := log.GetLogger(pgw.ctx)
	var rowsAffected int
	if err := pgw.sinkDb.QueryRow(pgw.ctx, `SELECT admin.maintain_unique_sources()`).Scan(&rowsAffected); err != nil {
		logger.Error("Failed to run admin.all_distinct_dbname_metrics maintenance:", err)
		return
	}
	logger.WithField("rows", rowsAffected).Info("Successfully processed admin.all_distinct_dbname_metrics")
}

func (pgw *PostgresWriter) AddDBUniqueMetricToListingTable(dbUnique, metric string) error {
	sql := `INSERT INTO admin.all_distinct_dbname_metrics
			SELECT $1, $2
			WHERE NOT EXISTS (
				SELECT * FROM admin.all_distinct_dbname_metrics WHERE dbname = $1 AND metric = $2
			)`
	_, err := pgw.sinkDb.Exec(pgw.ctx, sql, dbUnique, metric)
	return err
}

var initMigrator = func(pgw *PostgresWriter) (*migrator.Migrator, error) {
	return migrator.New(
		migrator.TableName("admin.migration"),
		migrator.SetNotice(func(s string) {
			log.GetLogger(pgw.ctx).Info(s)
		}),
		migrations(),
	)
}

// Migrate upgrades database with all migrations
func (pgw *PostgresWriter) Migrate() error {
	m, err := initMigrator(pgw)
	if err != nil {
		return fmt.Errorf("cannot initialize migration: %w", err)
	}
	return m.Migrate(pgw.ctx, pgw.sinkDb)
}

// NeedsMigration checks if database needs migration
func (pgw *PostgresWriter) NeedsMigration() (bool, error) {
	m, err := initMigrator(pgw)
	if err != nil {
		return false, err
	}
	return m.NeedUpgrade(pgw.ctx, pgw.sinkDb)
}

// MigrationsCount is the total number of migrations in admin.migration table
const MigrationsCount = 1

// migrations holds function returning all upgrade migrations needed
var migrations func() migrator.Option = func() migrator.Option {
	return migrator.Migrations(
		&migrator.Migration{
			Name: "01110 Apply postgres sink schema migrations",
			Func: func(context.Context, pgx.Tx) error {
				// "migration" table will be created automatically
				return nil
			},
		},

		// adding new migration here, update "admin"."migration" in "admin_schema.sql"!

		// &migrator.Migration{
		// 	Name: "000XX Short description of a migration",
		// 	Func: func(ctx context.Context, tx pgx.Tx) error {
		// 		return executeMigrationScript(ctx, tx, "000XX.sql")
		// 	},
		// },
	)
}
