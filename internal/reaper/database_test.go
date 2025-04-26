package reaper

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
)

func TestTryCreateMetricsFetchingHelpers(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	assert.NoError(t, err)
	defer mock.Close()

	metricDefs.MetricDefs["metric1"] = metrics.Metric{
		InitSQL: "CREATE FUNCTION metric1",
	}

	md := &sources.SourceConn{
		Conn: mock,
		Source: sources.Source{
			Name:           "testdb",
			Metrics:        map[string]float64{"metric1": 42, "nonexistent": 0},
			MetricsStandby: map[string]float64{"metric1": 42},
		},
	}

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec("CREATE FUNCTION metric1").WillReturnResult(pgxmock.NewResult("CREATE", 1))

		err = TryCreateMetricsFetchingHelpers(ctx, md)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error on exec", func(t *testing.T) {
		mock.ExpectExec("CREATE FUNCTION metric1").WillReturnError(assert.AnError)

		err = TryCreateMetricsFetchingHelpers(ctx, md)
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

}
