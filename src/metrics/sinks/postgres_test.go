package sinks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics/sinks"
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
