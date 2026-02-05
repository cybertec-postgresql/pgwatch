package sinks

import (
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
)

// newTestPrometheusWriter creates a PrometheusWriter for testing without starting HTTP server
func newTestPrometheusWriter(namespace string, gauges map[string][]string) *PrometheusWriter {
	return &PrometheusWriter{
		ctx:       testutil.TestContext,
		logger:    log.GetLogger(testutil.TestContext),
		Namespace: namespace,
		gauges:    gauges,
		Cache:     make(PromMetricCache),
	}
}

// =============================================================================
// Cache Operations Tests
// =============================================================================

func TestPromAsyncCacheInitIfRequired(t *testing.T) {
	t.Run("initializes new db entry", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		promw.PromAsyncCacheInitIfRequired("test_db", "some_metric")

		assert.Contains(t, promw.Cache, "test_db")
		assert.NotNil(t, promw.Cache["test_db"])
	})

	t.Run("does not overwrite existing db entry", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		// Initialize and add some data
		promw.PromAsyncCacheInitIfRequired("test_db", "metric1")
		promw.Cache["test_db"]["metric1"] = metrics.MeasurementEnvelope{DBName: "test_db"}

		// Call again - should not overwrite
		promw.PromAsyncCacheInitIfRequired("test_db", "metric2")

		assert.Contains(t, promw.Cache["test_db"], "metric1")
	})

	t.Run("initializes multiple dbs independently", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		promw.PromAsyncCacheInitIfRequired("db1", "metric")
		promw.PromAsyncCacheInitIfRequired("db2", "metric")

		assert.Contains(t, promw.Cache, "db1")
		assert.Contains(t, promw.Cache, "db2")
	})
}

func TestPromAsyncCacheAddMetricData(t *testing.T) {
	t.Run("adds data to initialized db", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "test_metric")

		msg := metrics.MeasurementEnvelope{
			DBName:     "test_db",
			MetricName: "test_metric",
			Data: metrics.Measurements{
				{metrics.EpochColumnName: time.Now().UnixNano(), "value": int64(42)},
			},
		}

		promw.PromAsyncCacheAddMetricData("test_db", "test_metric", msg)

		assert.Contains(t, promw.Cache["test_db"], "test_metric")
		assert.Equal(t, msg, promw.Cache["test_db"]["test_metric"])
	})

	t.Run("ignores data for uninitialized db", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		msg := metrics.MeasurementEnvelope{
			DBName:     "unknown_db",
			MetricName: "test_metric",
		}

		// Should not panic and should not add data
		promw.PromAsyncCacheAddMetricData("unknown_db", "test_metric", msg)

		assert.NotContains(t, promw.Cache, "unknown_db")
	})

	t.Run("overwrites existing metric data", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "test_metric")

		msg1 := metrics.MeasurementEnvelope{DBName: "test_db", MetricName: "test_metric"}
		msg2 := metrics.MeasurementEnvelope{DBName: "test_db", MetricName: "test_metric", Data: metrics.Measurements{{metrics.EpochColumnName: int64(123)}}}

		promw.PromAsyncCacheAddMetricData("test_db", "test_metric", msg1)
		promw.PromAsyncCacheAddMetricData("test_db", "test_metric", msg2)

		assert.Equal(t, msg2, promw.Cache["test_db"]["test_metric"])
	})
}

func TestPurgeMetricsFromPromAsyncCacheIfAny(t *testing.T) {
	t.Run("removes entire db when metric is empty", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "metric")

		promw.PurgeMetricsFromPromAsyncCacheIfAny("test_db", "")

		assert.NotContains(t, promw.Cache, "test_db")
	})

	t.Run("removes specific metric only", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "metric1")
		promw.Cache["test_db"]["metric1"] = metrics.MeasurementEnvelope{}
		promw.Cache["test_db"]["metric2"] = metrics.MeasurementEnvelope{}

		promw.PurgeMetricsFromPromAsyncCacheIfAny("test_db", "metric1")

		assert.NotContains(t, promw.Cache["test_db"], "metric1")
		assert.Contains(t, promw.Cache["test_db"], "metric2")
	})

	t.Run("handles non-existent db gracefully", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		// Should not panic
		promw.PurgeMetricsFromPromAsyncCacheIfAny("non_existent", "")
		promw.PurgeMetricsFromPromAsyncCacheIfAny("non_existent", "metric")
	})
}
