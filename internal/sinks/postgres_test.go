package sinks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
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
	pgw, err := NewWriterFromPostgresConn(ctx, conn, opts, metrics.GetDefaultMetrics())
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
	op := "add"
	conn.ExpectExec("insert into admin\\.all_distinct_dbname_metrics").WithArgs(dbUnique, metricName).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	conn.ExpectExec("select admin\\.ensure_dummy_metrics_table").WithArgs(metricName).WillReturnResult(pgxmock.NewResult("EXECUTE", 1))
	err = pgw.SyncMetric(dbUnique, metricName, op)
	assert.NoError(t, err)
	assert.NoError(t, conn.ExpectationsWereMet())

	op = "foo"
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
	messages := []metrics.MeasurementEnvelope{
		{
			MetricName: "test_metric",
			Data: metrics.Measurements{
				{"number": 1, "string": "test_data"},
			},
			DBName:     "test_db",
			CustomTags: map[string]string{"foo": "boo"},
		},
	}

	highLoadTimeout = 0
	err = pgw.Write(messages)
	assert.NoError(t, err, "messages skipped due to high load")

	highLoadTimeout = time.Second * 5
	pgw.input = make(chan []metrics.MeasurementEnvelope, cacheLimit)
	err = pgw.Write(messages)
	assert.NoError(t, err, "write successful")

	cancel()
	err = pgw.Write(messages)
	assert.Error(t, err, "context canceled")
}
