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

	t.Run("handles non-existent db gracefully", func(_ *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		// Should not panic
		promw.PurgeMetricsFromPromAsyncCacheIfAny("non_existent", "")
		promw.PurgeMetricsFromPromAsyncCacheIfAny("non_existent", "metric")
	})
}

// =============================================================================
// SyncMetric Tests
// =============================================================================

func TestPrometheusSyncMetric(t *testing.T) {
	t.Run("AddOp initializes cache", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		err := promw.SyncMetric("test_db", "test_metric", AddOp)

		assert.NoError(t, err)
		assert.Contains(t, promw.Cache, "test_db")
	})

	t.Run("DeleteOp removes metric", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "test_metric")
		promw.Cache["test_db"]["test_metric"] = metrics.MeasurementEnvelope{}

		err := promw.SyncMetric("test_db", "test_metric", DeleteOp)

		assert.NoError(t, err)
		assert.NotContains(t, promw.Cache["test_db"], "test_metric")
	})

	t.Run("DeleteOp with empty metric removes entire db", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "metric")

		err := promw.SyncMetric("test_db", "", DeleteOp)

		assert.NoError(t, err)
		assert.NotContains(t, promw.Cache, "test_db")
	})
}

// =============================================================================
// Write Tests
// =============================================================================

func TestPrometheusWrite(t *testing.T) {
	t.Run("writes data to cache", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "test_metric")

		msg := metrics.MeasurementEnvelope{
			DBName:     "test_db",
			MetricName: "test_metric",
			Data: metrics.Measurements{
				{metrics.EpochColumnName: time.Now().UnixNano(), "value": int64(100)},
			},
		}

		err := promw.Write(msg)

		assert.NoError(t, err)
		assert.Equal(t, msg, promw.Cache["test_db"]["test_metric"])
	})

	t.Run("ignores empty data", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "test_metric")

		msg := metrics.MeasurementEnvelope{
			DBName:     "test_db",
			MetricName: "test_metric",
			Data:       metrics.Measurements{},
		}

		err := promw.Write(msg)

		assert.NoError(t, err)
		assert.NotContains(t, promw.Cache["test_db"], "test_metric")
	})

	t.Run("ignores nil data", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("test_db", "test_metric")

		msg := metrics.MeasurementEnvelope{
			DBName:     "test_db",
			MetricName: "test_metric",
			Data:       nil,
		}

		err := promw.Write(msg)

		assert.NoError(t, err)
	})
}

// =============================================================================
// DefineMetrics Tests
// =============================================================================

func TestDefineMetrics(t *testing.T) {
	t.Run("sets up gauges from metric definitions", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", nil)

		m := &metrics.Metrics{
			MetricDefs: map[string]metrics.Metric{
				"backends":   {Gauges: []string{"active", "idle"}},
				"locks":      {Gauges: []string{"*"}},
				"wal":        {Gauges: []string{}},
				"table_size": {},
			},
		}

		err := promw.DefineMetrics(m)

		assert.NoError(t, err)
		assert.Equal(t, []string{"active", "idle"}, promw.gauges["backends"])
		assert.Equal(t, []string{"*"}, promw.gauges["locks"])
		assert.Equal(t, []string{}, promw.gauges["wal"])
		assert.Nil(t, promw.gauges["table_size"])
	})

	t.Run("overwrites existing gauges", func(t *testing.T) {
		promw := newTestPrometheusWriter("pgwatch", map[string][]string{
			"old_metric": {"old_gauge"},
		})

		m := &metrics.Metrics{
			MetricDefs: map[string]metrics.Metric{
				"new_metric": {Gauges: []string{"new_gauge"}},
			},
		}

		err := promw.DefineMetrics(m)

		assert.NoError(t, err)
		assert.NotContains(t, promw.gauges, "old_metric")
		assert.Contains(t, promw.gauges, "new_metric")
	})
}
