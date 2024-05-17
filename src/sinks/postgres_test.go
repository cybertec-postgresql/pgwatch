package sinks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/sinks"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

func TestReadMetricSchemaType(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	pgw := sinks.PostgresWriter{
		Ctx:    ctx,
		SinkDb: conn,
	}

	conn.ExpectQuery("SELECT schema_type").
		WillReturnError(errors.New("expected"))
	assert.Error(t, pgw.ReadMetricSchemaType())

	conn.ExpectQuery("SELECT schema_type").
		WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	assert.NoError(t, pgw.ReadMetricSchemaType())
	assert.Equal(t, sinks.DbStorageSchemaTimescale, pgw.MetricSchema)
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

	opts := &config.MeasurementOpts{BatchingDelay: time.Hour, Retention: 356}
	pgw, err := sinks.NewWriterFromPostgresConn(ctx, conn, opts, metrics.GetDefaultMetrics())
	assert.NoError(t, err)
	assert.NotNil(t, pgw)

	assert.NoError(t, conn.ExpectationsWereMet())
}
