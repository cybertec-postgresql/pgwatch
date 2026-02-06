package sinks

import (
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/prometheus/client_golang/prometheus"
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

// =============================================================================
// Collect Tests
// =============================================================================

// newTestPrometheusWriterWithMetrics creates a PrometheusWriter with initialized internal metrics
func newTestPrometheusWriterWithMetrics(namespace string, gauges map[string][]string) *PrometheusWriter {
	promw := newTestPrometheusWriter(namespace, gauges)
	promw.totalScrapes = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_scrapes"})
	promw.totalScrapeFailures = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_failures"})
	promw.lastScrapeErrors = prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_errors"})
	return promw
}

func TestCollect(t *testing.T) {
	t.Run("empty cache emits only internal metrics", func(t *testing.T) {
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)

		// Should emit totalScrapes, totalScrapeFailures, lastScrapeErrors
		assert.Len(t, ch, 3)
	})

	t.Run("preserves db entries after swap so new writes succeed", func(t *testing.T) {
		// This tests the critical invariant: after Collect swaps the cache,
		// the db entries must be preserved so subsequent Write() calls don't lose data
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)

		// Initialize db
		promw.PromAsyncCacheInitIfRequired("db1", "metric")
		promw.Cache["db1"]["backends"] = metrics.MeasurementEnvelope{
			DBName:     "db1",
			MetricName: "backends",
			Data: metrics.Measurements{
				{metrics.EpochColumnName: time.Now().UnixNano(), "count": int64(10)},
			},
		}

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)

		// After collect, db entry must still exist (empty but present)
		assert.Contains(t, promw.Cache, "db1")

		// New write should succeed (not be silently dropped)
		newMsg := metrics.MeasurementEnvelope{
			DBName:     "db1",
			MetricName: "backends",
			Data: metrics.Measurements{
				{metrics.EpochColumnName: time.Now().UnixNano(), "count": int64(20)},
			},
		}
		err := promw.Write(newMsg)
		assert.NoError(t, err)

		// Verify the write actually stored data
		assert.Equal(t, newMsg, promw.Cache["db1"]["backends"])
	})

	t.Run("collects from multiple dbs with multiple metrics", func(t *testing.T) {
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)

		// Setup: 2 dbs, each with 2 metrics, each metric with 2 fields
		promw.PromAsyncCacheInitIfRequired("db1", "metric")
		promw.PromAsyncCacheInitIfRequired("db2", "metric")

		now := time.Now().UnixNano()
		promw.Cache["db1"]["backends"] = metrics.MeasurementEnvelope{
			DBName: "db1", MetricName: "backends",
			Data: metrics.Measurements{{metrics.EpochColumnName: now, "active": int64(5), "idle": int64(3)}},
		}
		promw.Cache["db1"]["locks"] = metrics.MeasurementEnvelope{
			DBName: "db1", MetricName: "locks",
			Data: metrics.Measurements{{metrics.EpochColumnName: now, "count": int64(2)}},
		}
		promw.Cache["db2"]["backends"] = metrics.MeasurementEnvelope{
			DBName: "db2", MetricName: "backends",
			Data: metrics.Measurements{{metrics.EpochColumnName: now, "active": int64(10), "idle": int64(7)}},
		}
		promw.Cache["db2"]["connections"] = metrics.MeasurementEnvelope{
			DBName: "db2", MetricName: "connections",
			Data: metrics.Measurements{{metrics.EpochColumnName: now, "total": int64(100)}},
		}

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)

		// Expected: 3 internal metrics + (2+1+2+1) = 9 total
		// db1/backends: 2 fields, db1/locks: 1 field, db2/backends: 2 fields, db2/connections: 1 field
		assert.Len(t, ch, 9)

		// Both dbs should still exist after collect
		assert.Contains(t, promw.Cache, "db1")
		assert.Contains(t, promw.Cache, "db2")
	})

	t.Run("successive collects return fresh data each time", func(t *testing.T) {
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("db1", "metric")

		// First collect with some data
		promw.Cache["db1"]["metric1"] = metrics.MeasurementEnvelope{
			DBName: "db1", MetricName: "metric1",
			Data: metrics.Measurements{{metrics.EpochColumnName: time.Now().UnixNano(), "val": int64(1)}},
		}

		ch1 := make(chan prometheus.Metric, 100)
		promw.Collect(ch1)
		firstCollectCount := len(ch1)

		// Second collect without new data - should only have internal metrics
		ch2 := make(chan prometheus.Metric, 100)
		promw.Collect(ch2)
		secondCollectCount := len(ch2)

		assert.Equal(t, 4, firstCollectCount) // 3 internal + 1 metric field
		assert.Equal(t, 3, secondCollectCount) // only 3 internal metrics (no data)
	})

	t.Run("skips change_events metric", func(t *testing.T) {
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)
		promw.PromAsyncCacheInitIfRequired("db1", "change_events")

		promw.Cache["db1"]["change_events"] = metrics.MeasurementEnvelope{
			DBName: "db1", MetricName: "change_events",
			Data: metrics.Measurements{{metrics.EpochColumnName: time.Now().UnixNano(), "value": int64(1)}},
		}
		promw.Cache["db1"]["backends"] = metrics.MeasurementEnvelope{
			DBName: "db1", MetricName: "backends",
			Data: metrics.Measurements{{metrics.EpochColumnName: time.Now().UnixNano(), "count": int64(5)}},
		}

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)

		// Should have 3 internal + 1 (backends), change_events skipped
		assert.Len(t, ch, 4)
	})

	t.Run("handles db with empty metrics map", func(t *testing.T) {
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)

		// Initialize db but don't add any metrics
		promw.PromAsyncCacheInitIfRequired("empty_db", "metric")
		// Don't add any actual metric data

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)

		// Should only have internal metrics, no panic
		assert.Len(t, ch, 3)
		// DB should still exist
		assert.Contains(t, promw.Cache, "empty_db")
	})

	t.Run("totalScrapes increments on each collect", func(t *testing.T) {
		promw := newTestPrometheusWriterWithMetrics("pgwatch", nil)

		ch := make(chan prometheus.Metric, 100)

		// Collect 3 times
		promw.Collect(ch)
		promw.Collect(ch)
		promw.Collect(ch)

		// Drain and check - we should see incrementing totalScrapes
		// Each collect adds 3 metrics, so 9 total
		assert.Len(t, ch, 9)
	})
}
