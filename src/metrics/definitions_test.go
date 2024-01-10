package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

func TestGetMetricSchemaType(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	conn.ExpectQuery("SELECT schema_type").
		WillReturnError(errors.New("expected"))
	_, err = metrics.GetMetricSchemaType(ctx, conn)
	assert.Error(t, err)

	conn.ExpectQuery("SELECT schema_type").
		WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	schemaType, err := metrics.GetMetricSchemaType(context.Background(), conn)
	assert.NoError(t, err)
	assert.Equal(t, metrics.MetricSchemaTimescale, schemaType)
}
