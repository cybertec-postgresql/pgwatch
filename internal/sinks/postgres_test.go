package sinks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	jsoniter "github.com/json-iterator/go"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
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
	conn.ExpectQuery("SELECT EXISTS").WithArgs("admin").WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	conn.ExpectQuery("SELECT schema_type").WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	for _, m := range metrics.GetDefaultBuiltInMetrics() {
		conn.ExpectExec("select admin.ensure_dummy_metrics_table").WithArgs(m).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	}

	opts := &CmdOpts{BatchingDelay: time.Hour, Retention: 356}
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
	conn.ExpectExec("insert into admin\\.all_distinct_dbname_metrics").WithArgs(dbUnique, metricName).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	conn.ExpectExec("select admin\\.ensure_dummy_metrics_table").WithArgs(metricName).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	err = pgw.SyncMetric(dbUnique, metricName, op)
	assert.NoError(t, err)
	assert.NoError(t, conn.ExpectationsWereMet())

	op = invalidOp
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

func TestPostgresWriter_EnsureMetricTime(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)
	pgw := PostgresWriter{
		ctx:    ctx,
		sinkDb: conn,
	}

	TestPartBounds := map[string]ExistingPartitionInfo{"test_metric_realtime": {time.Now(), time.Now()}}
	conn.ExpectQuery(`select part_available_from, part_available_to`).
		WithArgs("test_metric_realtime", TestPartBounds["test_metric_realtime"].StartTime).
		WillReturnRows(pgxmock.NewRows([]string{"part_available_from", "part_available_to"}).
			AddRow(time.Now(), time.Now()))

	conn.ExpectQuery(`select part_available_from, part_available_to`).
		WithArgs("test_metric_realtime", TestPartBounds["test_metric_realtime"].EndTime).
		WillReturnRows(pgxmock.NewRows([]string{"part_available_from", "part_available_to"}).
			AddRow(time.Now(), time.Now()))

	err = pgw.EnsureMetricTime(TestPartBounds, true)
	assert.NoError(t, err)
	assert.NoError(t, conn.ExpectationsWereMet())
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
