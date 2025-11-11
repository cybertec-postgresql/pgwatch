package sinks

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/jackc/pgx/v5"
	jsoniter "github.com/json-iterator/go"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var ctx = context.Background()

func TestReadMetricSchemaType(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	pgw := PostgresWriter{
		ctx:    ctx,
		sinkDb: conn,
	}

	conn.ExpectQuery("SELECT schema_type").
		WillReturnError(errors.New("expected"))
	assert.Error(t, pgw.ReadMetricSchemaType())

	conn.ExpectQuery("SELECT schema_type").
		WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	assert.NoError(t, pgw.ReadMetricSchemaType())
	assert.Equal(t, DbStorageSchemaTimescale, pgw.metricSchema)
}

func TestNewWriterFromPostgresConn(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	conn.ExpectPing()
	conn.ExpectQuery("SELECT extract").WithArgs("1 day", "1 day", "1 hour").WillReturnRows(
		pgxmock.NewRows([]string{"col1", "col2", "col3"}).AddRow(24 * time.Hour / 1_000_000_000, 24 * time.Hour / 1_000_000_000, true),
	)
	conn.ExpectQuery("SELECT EXISTS").WithArgs("admin").WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	conn.ExpectQuery("SELECT schema_type").WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	for _, m := range metrics.GetDefaultBuiltInMetrics() {
		conn.ExpectExec("SELECT admin.ensure_dummy_metrics_table").WithArgs(m).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	}

	opts := &CmdOpts{
		BatchingDelay: time.Hour, 
		Retention: "1 day",
		MaintenanceInterval: "1 day",
		PartitionInterval: "1 hour",
	}
	pgw, err := NewWriterFromPostgresConn(ctx, conn, opts)
	assert.NoError(t, err)
	assert.NotNil(t, pgw)

	assert.NoError(t, conn.ExpectationsWereMet())
}

func TestSyncMetric(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)
	pgw := PostgresWriter{
		ctx:    ctx,
		sinkDb: conn,
	}
	dbUnique := "mydb"
	metricName := "mymetric"
	op := AddOp
	conn.ExpectExec("INSERT INTO admin\\.all_distinct_dbname_metrics").WithArgs(dbUnique, metricName).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	conn.ExpectExec("SELECT admin\\.ensure_dummy_metrics_table").WithArgs(metricName).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	err = pgw.SyncMetric(dbUnique, metricName, op)
	assert.NoError(t, err)
	assert.NoError(t, conn.ExpectationsWereMet())

	op = InvalidOp
	err = pgw.SyncMetric(dbUnique, metricName, op)
	assert.NoError(t, err, "ignore unknown operation")
}

func TestWrite(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)
	ctx, cancel := context.WithCancel(ctx)
	pgw := PostgresWriter{
		ctx:    ctx,
		sinkDb: conn,
	}
	message := metrics.MeasurementEnvelope{
		MetricName: "test_metric",
		Data: metrics.Measurements{
			{"number": 1, "string": "test_data"},
		},
		DBName:     "test_db",
		CustomTags: map[string]string{"foo": "boo"},
	}

	highLoadTimeout = 0
	err = pgw.Write(message)
	assert.NoError(t, err, "messages skipped due to high load")

	highLoadTimeout = time.Second * 5
	pgw.input = make(chan metrics.MeasurementEnvelope, cacheLimit)
	err = pgw.Write(message)
	assert.NoError(t, err, "write successful")

	cancel()
	err = pgw.Write(message)
	assert.Error(t, err, "context canceled")
}

func TestCopyFromMeasurements_Basic(t *testing.T) {
	// Test basic iteration through single envelope with multiple measurements
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{"env": "test"},
			Data: metrics.Measurements{
				{"epoch_ns": int64(1000), "value": 1},
				{"epoch_ns": int64(2000), "value": 2},
				{"epoch_ns": int64(3000), "value": 3},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)

	// Test Next() and Values() for each measurement
	assert.Equal(t, "metric1", cfm.MetricName()[0], "Metric name should be obtained before Next()")
	assert.True(t, cfm.Next(), "Should have first measurement")
	values, err := cfm.Values()
	assert.NoError(t, err)
	assert.Len(t, values, 4) // time, dbname, data, tag_data
	assert.Equal(t, "db1", values[1])

	assert.True(t, cfm.Next(), "Should have second measurement")
	values, err = cfm.Values()
	assert.NoError(t, err)
	assert.Equal(t, "db1", values[1])

	assert.True(t, cfm.Next(), "Should have third measurement")
	values, err = cfm.Values()
	assert.NoError(t, err)
	assert.Equal(t, "db1", values[1])

	assert.False(t, cfm.Next(), "Should not have more measurements")
	assert.True(t, cfm.EOF(), "Should be at end")
}

func TestCopyFromMeasurements_MultipleEnvelopes(t *testing.T) {
	// Test iteration through multiple envelopes of same metric
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{"env": "test1"},
			Data: metrics.Measurements{
				{"epoch_ns": int64(1000), "value": 1},
				{"epoch_ns": int64(2000), "value": 2},
			},
		},
		{
			MetricName: "metric1",
			DBName:     "db2",
			CustomTags: map[string]string{"env": "test2"},
			Data: metrics.Measurements{
				{"epoch_ns": int64(3000), "value": 3},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)

	// First envelope, first measurement
	assert.True(t, cfm.Next())
	values, err := cfm.Values()
	assert.NoError(t, err)
	assert.Equal(t, "db1", values[1])
	// First envelope, second measurement
	assert.True(t, cfm.Next())
	values, err = cfm.Values()
	assert.NoError(t, err)
	assert.Equal(t, "db1", values[1])

	// Second envelope, first measurement
	assert.Equal(t, "metric1", cfm.MetricName()[0])
	assert.True(t, cfm.Next())
	values, err = cfm.Values()
	assert.NoError(t, err)
	assert.Equal(t, "db2", values[1])

	assert.False(t, cfm.Next())
}

func TestCopyFromMeasurements_MetricBoundaries(t *testing.T) {
	// Test metric boundary detection with different metrics
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{},
			Data: metrics.Measurements{
				{"epoch_ns": int64(1000), "value": 1},
				{"epoch_ns": int64(2000), "value": 2},
			},
		},
		{
			MetricName: "metric2", // Different metric
			DBName:     "db1",
			CustomTags: map[string]string{},
			Data: metrics.Measurements{
				{"epoch_ns": int64(3000), "value": 3},
			},
		},
		{
			MetricName: "metric2",
			DBName:     "db2",
			CustomTags: map[string]string{},
			Data: metrics.Measurements{
				{"epoch_ns": int64(4000), "value": 4},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)

	// Process metric1 completely
	assert.Equal(t, "metric1", cfm.MetricName()[0])
	assert.True(t, cfm.Next())
	assert.True(t, cfm.Next())

	// Should stop at metric boundary
	assert.False(t, cfm.Next())
	assert.False(t, cfm.EOF(), "Should not be at EOF yet, there's more data")

	assert.Equal(t, "metric2", cfm.MetricName()[0])
	assert.True(t, cfm.Next())
	assert.True(t, cfm.Next())

	assert.False(t, cfm.Next())
	assert.True(t, cfm.EOF(), "Should be at EOF after processing all measurements")
}

func TestCopyFromMeasurements_EmptyData(t *testing.T) {
	// Test with empty envelopes slice
	cfm := newCopyFromMeasurements([]metrics.MeasurementEnvelope{})
	assert.False(t, cfm.Next())
	assert.True(t, cfm.EOF())
}

func TestCopyFromMeasurements_EmptyMeasurements(t *testing.T) {
	// Test with envelope containing no measurements
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{},
			Data:       metrics.Measurements{}, // Empty measurements
		},
		{
			MetricName: "metric1",
			DBName:     "db2",
			CustomTags: map[string]string{},
			Data: metrics.Measurements{
				{"epoch_ns": int64(1000), "value": 1},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)

	// Should skip empty envelope and go to second one
	assert.True(t, cfm.Next())
	values, err := cfm.Values()
	assert.NoError(t, err)
	assert.Equal(t, "db2", values[1])

	assert.False(t, cfm.Next())
	assert.True(t, cfm.EOF())
}

func TestCopyFromMeasurements_TagProcessing(t *testing.T) {
	// Test that tag_ prefixed fields are moved to CustomTags
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{"existing": "tag"},
			Data: metrics.Measurements{
				{
					"epoch_ns":     int64(1000),
					"value":        1,
					"tag_env":      "production",
					"tag_version":  "1.0",
					"normal_field": "stays",
				},
			},
		},
		{
			MetricName: "metric1",
			DBName:     "db2",
			CustomTags: nil,
			Data: metrics.Measurements{
				{
					"epoch_ns":     int64(1000),
					"value":        1,
					"tag_env":      "production",
					"tag_version":  "1.0",
					"normal_field": "stays",
				},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)
	assert.True(t, cfm.Next())

	values, err := cfm.Values()
	assert.NoError(t, err)
	assert.Len(t, values, 4) // Verify structure: time, dbname, data, tag_data

	// Check that custom tags were updated
	// Check data JSON (should contain normal fields but not tag_ fields)
	dataJSON, ok := values[2].(string)
	assert.True(t, ok, "Data should be JSON string")

	var dataMap map[string]any
	err = jsoniter.ConfigFastest.UnmarshalFromString(dataJSON, &dataMap)
	assert.NoError(t, err)
	assert.Contains(t, dataMap, "normal_field")
	assert.NotContains(t, dataMap, "tag_env", "tag_env should not be in data")
	assert.NotContains(t, dataMap, "tag_version", "tag_version should not be in data")

	// Check tag JSON (should contain converted tags)
	tagJSON, ok := values[3].(string)
	assert.True(t, ok, "Tag data should be JSON string")

	var tagMap map[string]string
	err = jsoniter.ConfigFastest.UnmarshalFromString(tagJSON, &tagMap)
	assert.NoError(t, err)
	assert.Contains(t, tagMap, "existing")
	assert.Contains(t, tagMap, "env", "tag_env should be converted to env")
	assert.Contains(t, tagMap, "version", "tag_version should be converted to version")
	assert.Equal(t, "production", tagMap["env"])
	assert.Equal(t, "1.0", tagMap["version"])

	assert.True(t, cfm.Next())
	_, err = cfm.Values()
	assert.NoError(t, err, "should process nil CustomTags without error")
}

func TestCopyFromMeasurements_JsonMarshaling(t *testing.T) {
	// Test that JSON marshaling works correctly
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{"env": "test"},
			Data: metrics.Measurements{
				{
					"epoch_ns": int64(1000),
					"value":    42,
					"name":     "test_measurement",
				},
				{
					"epoch_ns": int64(1000),
					"value": func() string {
						return "should produce error while marshaled"
					},
					"name": "test_measurement",
				},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)
	assert.True(t, cfm.Next())

	values, err := cfm.Values()
	assert.NoError(t, err)
	assert.Len(t, values, 4)

	// Values should be: [time, dbname, data_json, tag_data_json]
	assert.Equal(t, "db1", values[1])

	// Check that JSON strings are valid
	dataJSON, ok := values[2].(string)
	assert.True(t, ok, "Data should be JSON string")
	assert.Contains(t, dataJSON, `"value":42`)
	assert.Contains(t, dataJSON, `"name":"test_measurement"`)

	tagJSON, ok := values[3].(string)
	assert.True(t, ok, "Tag data should be JSON string")
	assert.Contains(t, tagJSON, `"env":"test"`)

	assert.True(t, cfm.Next())
	_, err = cfm.Values()
	assert.Error(t, err, "cannot marshal function value to JSON")

	cfm.NextEnvelope()
	assert.NotPanics(t, func() { _ = cfm.MetricName() })
}

func TestCopyFromMeasurements_ErrorHandling(t *testing.T) {
	// Test Err() method
	cfm := newCopyFromMeasurements([]metrics.MeasurementEnvelope{})
	assert.NoError(t, cfm.Err(), "Err() should always return nil")
}

func TestCopyFromMeasurements_StateManagement(t *testing.T) {
	// Test that internal state is managed correctly during iteration
	data := []metrics.MeasurementEnvelope{
		{
			MetricName: "metric1",
			DBName:     "db1",
			CustomTags: map[string]string{},
			Data: metrics.Measurements{
				{"epoch_ns": int64(1000), "value": 1},
			},
		},
		{
			MetricName: "metric2", // Different metric
			DBName:     "db1",
			CustomTags: map[string]string{},
			Data: metrics.Measurements{
				{"epoch_ns": int64(2000), "value": 2},
			},
		},
	}

	cfm := newCopyFromMeasurements(data)

	// Initial state
	assert.Equal(t, -1, cfm.envelopeIdx)
	assert.Equal(t, -1, cfm.measurementIdx)
	assert.Equal(t, "", cfm.metricName)

	// After first Next()
	assert.True(t, cfm.Next())
	assert.Equal(t, 0, cfm.envelopeIdx)
	assert.Equal(t, 0, cfm.measurementIdx)
	assert.Equal(t, "metric1", cfm.metricName)

	// After hitting metric boundary
	assert.False(t, cfm.Next())
	// State should be positioned to restart on next metric
	assert.Equal(t, "", cfm.metricName)
}

func TestCopyFromMeasurements_CopyFail(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	const ImageName = "docker.io/postgres:17-alpine"
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	r.NoError(err)
	defer func() { assert.NoError(t, pgContainer.Terminate(ctx)) }()

	connStr, _ := pgContainer.ConnectionString(ctx, "sslmode=disable")
	conn, err := pgx.Connect(ctx, connStr)
	r.NoError(err)

	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS test_metric (
		time timestamptz not null default now(),
		dbname text NOT NULL,
		data jsonb,
		tag_data jsonb)`)
	r.NoError(err)

	msgs := []metrics.MeasurementEnvelope{
		{
			MetricName: "test_metric",
			Data: metrics.Measurements{
				{"epoch_ns": int64(2000), "value": func() {}},
				{"epoch_ns": int64(2000), "value": struct{}{}},
			},
			DBName: "test_db",
		},
	}

	cfm := newCopyFromMeasurements(msgs)

	for !cfm.EOF() {
		_, err = conn.CopyFrom(context.Background(), cfm.MetricName(), targetColumns[:], cfm)
		a.Error(err)
		if err != nil {
			if !cfm.NextEnvelope() {
				break
			}
		}
	}

}

// tests interval string validation for all 
// cli flags that expect a PostgreSQL interval string
func TestIntervalValidation(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	const ImageName = "docker.io/postgres:17-alpine"
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	r.NoError(err)
	defer func() { a.NoError(pgContainer.Terminate(ctx)) }()

	connStr, _ := pgContainer.ConnectionString(ctx, "sslmode=disable")

	opts := &CmdOpts{
		PartitionInterval: "1 minute",
		MaintenanceInterval: "-1 hours",
		Retention: "-1 hours",
		BatchingDelay: time.Second,
	}

	_, err = NewPostgresWriter(ctx, connStr, opts)
	a.EqualError(err, "--partition-interval must be at least 1 hour, got: 1 minute")
	opts.PartitionInterval = "1 hour"

	_, err = NewPostgresWriter(ctx, connStr, opts)
	a.EqualError(err, "--maintenance-interval must be a positive PostgreSQL interval or 0 to disable it")
	opts.MaintenanceInterval = "0 hours"

	_, err = NewPostgresWriter(ctx, connStr, opts)
	a.EqualError(err, "--retention must be a positive PostgreSQL interval or 0 to disable it")

	invalidIntervals := []string {
		"not an interval", "3 dayss",
		"four hours",
	}

	for _, interval := range invalidIntervals {
		opts.PartitionInterval = interval
		_, err = NewPostgresWriter(ctx, connStr, opts)
		a.Error(err)
		opts.PartitionInterval = "1 hour"

		opts.MaintenanceInterval = interval
		_, err = NewPostgresWriter(ctx, connStr, opts)
		a.Error(err)
		opts.MaintenanceInterval = "1 hour"

		opts.Retention = interval
		_, err = NewPostgresWriter(ctx, connStr, opts)
		a.Error(err)
	}

	validIntervals := []string{
		"3 days 4 hours", "1 year",
		"P3D", "PT3H", "0-02", "1 00:00:00",
		"P0-02", "P1", "2 weeks",
	}

	for _, interval := range validIntervals {
		opts.PartitionInterval = interval
		opts.MaintenanceInterval = interval
		opts.Retention = interval

		_, err = NewPostgresWriter(ctx, connStr, opts)
		a.NoError(err)
	}
}

func TestPartitionInterval(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	const ImageName = "docker.io/postgres:17-alpine"
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	r.NoError(err)
	defer func() { a.NoError(pgContainer.Terminate(ctx)) }()

	connStr, _ := pgContainer.ConnectionString(ctx, "sslmode=disable")

	opts := &CmdOpts{
		PartitionInterval: "3 weeks",
		Retention: "14 days",
		MaintenanceInterval: "12 hours",
		BatchingDelay: time.Second,
	}

	pgw, err := NewPostgresWriter(ctx, connStr, opts)
	r.NoError(err)

	conn, err := pgx.Connect(ctx, connStr)
	r.NoError(err)

	m := map[string]map[string]ExistingPartitionInfo{
		"test_metric": {
			"test_db": {
				time.Now(), time.Now().Add(time.Hour),
			},
		},
	}
	err = pgw.EnsureMetricDbnameTime(m, false)
	r.NoError(err)

	var partitionsNum int;
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM pg_partition_tree('test_metric');").Scan(&partitionsNum)
	a.NoError(err)
	// 1 the metric table itself + 1 dbname partition
	// + 4 time partitions (1 we asked for + 3 precreated)
	a.Equal(6, partitionsNum)

	part := partitionMapMetricDbname["test_metric"]["test_db"]
	// partition bounds should have a difference of 3 weeks
	a.Equal(part.StartTime.Add(3 * 7 * 24 * time.Hour), part.EndTime)
}

func TestMaintenance(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// TODO: move postgres container setup to TestMain()
	const ImageName = "docker.io/postgres:17-alpine"
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	r.NoError(err)
	defer func() { a.NoError(pgContainer.Terminate(ctx)) }()

	connStr, _ := pgContainer.ConnectionString(ctx, "sslmode=disable")
	conn, err := pgx.Connect(ctx, connStr)
	r.NoError(err)

	opts := &CmdOpts{
		PartitionInterval: "1 hour",
		Retention: "1 second",
		MaintenanceInterval: "0 days",
		BatchingDelay: time.Hour,
	}

	pgw, err := NewPostgresWriter(ctx, connStr, opts)
	r.NoError(err)

	t.Run("MaintainUniqueSources", func(_ *testing.T) {
		// adds an entry to `admin.all_distinct_dbname_metrics`
		err = pgw.SyncMetric("test", "test_metric_1", AddOp)
		r.NoError(err)

		var numOfEntries int
		err = conn.QueryRow(ctx, "SELECT count(*) FROM admin.all_distinct_dbname_metrics;").Scan(&numOfEntries)
		a.NoError(err)	
		a.Equal(1, numOfEntries)

		// manually call the maintenance routine
		pgw.MaintainUniqueSources()

		// entry should have been deleted, because it has no corresponding entries in `test_metric_1` table.
		err = conn.QueryRow(ctx, "SELECT count(*) FROM admin.all_distinct_dbname_metrics;").Scan(&numOfEntries)
		a.NoError(err)	
		a.Equal(0, numOfEntries)

		message := []metrics.MeasurementEnvelope{
			{
				MetricName: "test_metric_1",
				Data: metrics.Measurements{
					{"number": 1, "string": "test_data"},
				},
				DBName: "test_db",
			},
		} 
		pgw.flush(message)

		// manually call the maintenance routine
		pgw.MaintainUniqueSources()

		// entry should have been added, because there is a corresponding entry in `test_metric_1` table just written.
		err = conn.QueryRow(ctx, "SELECT count(*) FROM admin.all_distinct_dbname_metrics;").Scan(&numOfEntries)
		a.NoError(err)	
		a.Equal(1, numOfEntries)
	})

	t.Run("DeleteOldPartitions", func(_ *testing.T) {
		// Creates a new top level table for `test_metric_2`
		err = pgw.SyncMetric("test", "test_metric_2", AddOp)
		r.NoError(err)

		_, err = conn.Exec(ctx, "CREATE TABLE subpartitions.test_metric_2_dbname PARTITION OF public.test_metric_2 FOR VALUES IN ('test') PARTITION BY RANGE (time)")
		a.NoError(err)

		boundStart := time.Now().Add(-1 * 2 * 24 * time.Hour).Format("2006-01-02")
		boundEnd := time.Now().Add(-1 * 24 * time.Hour).Format("2006-01-02")

		// create a time partition with end bound yesterday 
		_, err = conn.Exec(ctx, 
			fmt.Sprintf(
			`CREATE TABLE subpartitions.test_metric_2_dbname_time 
			PARTITION OF subpartitions.test_metric_2_dbname 
			FOR VALUES FROM ('%s') TO ('%s')`, 
			boundStart, boundEnd),
		)
		a.NoError(err)
		_, err = conn.Exec(ctx, "COMMENT ON TABLE subpartitions.test_metric_2_dbname_time IS $$pgwatch-generated-metric-dbname-time-lvl$$")
		a.NoError(err)

		var partitionsNum int;
		err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM pg_partition_tree('test_metric_2');").Scan(&partitionsNum)
		a.NoError(err)
		a.Equal(3, partitionsNum)

		pgw.opts.Retention = "2 days"
		pgw.DeleteOldPartitions() // 1 day < 2 days, shouldn't delete anything

		err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM pg_partition_tree('test_metric_2');").Scan(&partitionsNum)
		a.NoError(err)
		a.Equal(3, partitionsNum)

		pgw.opts.Retention = "1 hour"
		pgw.DeleteOldPartitions() // 1 day > 1 hour, should delete the partition

		err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM pg_partition_tree('test_metric_2');").Scan(&partitionsNum)
		a.NoError(err)
		a.Equal(2, partitionsNum)
	})

	t.Run("Epcoh to Duration Conversion", func(_ *testing.T) {
		table := map[string]time.Duration{
			"1 hour": time.Hour, 
			"2 hours": 2 * time.Hour,
			"4 days": 4 * 24 * time.Hour, 
			"1 day": 24 * time.Hour,
			"1 year": 365.25 * 24 * time.Hour,
			"1 week": 7 * 24 * time.Hour,
			"3 weeks": 3 * 7 * 24 * time.Hour,
			"2 months": 2 * 30 * 24 * time.Hour,
			"1 month": 30 * 24 * time.Hour,
		}

		for k, v := range table {
			opts := &CmdOpts{
				PartitionInterval: "1 hour",
				Retention: k,
				MaintenanceInterval: k,
				BatchingDelay: time.Hour,
			}

			pgw, err := NewPostgresWriter(ctx, connStr, opts)
			a.NoError(err)
			a.Equal(pgw.retentionInterval, v)
			a.Equal(pgw.maintenanceInterval, v)
		}
	})
}