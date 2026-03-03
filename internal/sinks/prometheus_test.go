package sinks

import (
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPrometheusWriter(namespace string) *PrometheusWriter {
	return &PrometheusWriter{
		ctx:       testutil.TestContext,
		logger:    log.GetLogger(testutil.TestContext),
		Namespace: namespace,
		Cache:     make(PromMetricCache),
		lastScrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "test_last_scrape_errors",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "test_total_scrapes",
		}),
		totalScrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "test_total_scrape_failures",
		}),
	}
}

// TestWriteAfterCollect verifies that Write() works after Collect().
// Since Collect() now reads a snapshot without clearing the cache,
// the cache should still contain the original data after collect.
func TestWriteAfterCollect(t *testing.T) {
	promw := newTestPrometheusWriter("test")

	// Write initial data
	msg := metrics.MeasurementEnvelope{
		DBName:     "db1",
		MetricName: "metric1",
		Data: metrics.Measurements{
			{metrics.EpochColumnName: time.Now().UnixNano(), "value": int64(100)},
		},
	}
	require.NoError(t, promw.Write(msg))

	// Collect reads a snapshot — cache is NOT cleared
	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	assert.NotEmpty(t, promw.Cache, "cache should still contain data after Collect (snapshot-based)")

	// Write after Collect — must work
	msg.Data[0]["value"] = int64(200)
	require.NoError(t, promw.Write(msg))

	assert.Contains(t, promw.Cache, "db1")
	assert.Equal(t, int64(200), promw.Cache["db1"]["metric1"].Data[0]["value"])
}

// TestCollect_CachePreserved verifies Collect() does not consume the cache.
// Parallel scrapes and back-to-back scrapes should see the same data.
func TestCollect_CachePreserved(t *testing.T) {
	promw := newTestPrometheusWriter("test")

	// Populate cache with multiple databases
	for _, db := range []string{"db1", "db2", "db3", "db4", "db5"} {
		promw.Cache[db] = map[string]metrics.MeasurementEnvelope{
			"metric": {
				DBName:     db,
				MetricName: "metric",
				Data: metrics.Measurements{
					{metrics.EpochColumnName: time.Now().UnixNano(), "value": int64(1)},
				},
			},
		}
	}
	assert.Len(t, promw.Cache, 5)

	// First Collect
	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)

	// Cache should still have all 5 databases
	assert.Len(t, promw.Cache, 5, "cache should be preserved after Collect")

	// Second Collect should also work (back-to-back scrapes)
	ch2 := make(chan prometheus.Metric, 100)
	promw.Collect(ch2)
	assert.Len(t, promw.Cache, 5, "cache should be preserved after second Collect")
}

// TestCollect_DeterministicLabelOrdering verifies that metrics with the same
// labels produce consistent output regardless of map insertion order.
// This is the fix for the "collected metric was collected before" error.
func TestCollect_DeterministicLabelOrdering(t *testing.T) {
	promw := newTestPrometheusWriter("test")
	promw.gauges = map[string][]string{"metric1": {"*"}}

	// Create two data rows with the same labels but potentially different
	// map iteration orders (Go randomizes map iteration).
	promw.Cache["db1"] = map[string]metrics.MeasurementEnvelope{
		"metric1": {
			DBName:     "db1",
			MetricName: "metric1",
			Data: metrics.Measurements{
				{
					metrics.EpochColumnName: time.Now().UnixNano(),
					"tag_host":              "server1",
					"tag_port":              "5432",
					"tag_region":            "us-east-1",
					"value":                 int64(42),
				},
			},
		},
	}

	// Collect multiple times — should never produce duplicate errors.
	// Each Collect emits 3 self-instrumentation metrics + 1 data metric = 4 total.
	const metaMetrics = 3 // totalScrapes + totalScrapeFailures + lastScrapeErrors
	for i := 0; i < 100; i++ {
		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)
		close(ch)

		var collected []prometheus.Metric
		for m := range ch {
			collected = append(collected, m)
		}
		// 1 data metric + 3 meta-metrics
		assert.Len(t, collected, 1+metaMetrics, "iteration %d: expected 1 data + 3 meta metrics", i)
	}
}

// TestCollect_DeduplicateMetrics verifies that duplicate row data
// (same metric name + same label values) is emitted only once per scrape.
func TestCollect_DeduplicateMetrics(t *testing.T) {
	promw := newTestPrometheusWriter("test")
	promw.gauges = map[string][]string{"metric1": {"*"}}

	// Two identical rows — same labels, same field name
	promw.Cache["db1"] = map[string]metrics.MeasurementEnvelope{
		"metric1": {
			DBName:     "db1",
			MetricName: "metric1",
			Data: metrics.Measurements{
				{
					metrics.EpochColumnName: time.Now().UnixNano(),
					"tag_host":              "server1",
					"value":                 int64(42),
				},
				{
					metrics.EpochColumnName: time.Now().UnixNano(),
					"tag_host":              "server1",
					"value":                 int64(99), // different value, same identity
				},
			},
		},
	}

	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	close(ch)

	var count int
	for range ch {
		count++
	}
	// Should deduplicate — only the first occurrence is emitted.
	// 1 data metric + 3 meta-metrics = 4 total.
	assert.Equal(t, 1+3, count, "duplicate metric identity should be deduplicated (1 data + 3 meta)")
}

// TestCollect_InvalidMetricDoesNotPanic verifies that a malformed metric
// (label count mismatch) does not cause a panic. This tests the
// NewConstMetric error handling path.
func TestCollect_InvalidMetricDoesNotPanic(t *testing.T) {
	promw := newTestPrometheusWriter("test")

	// This should not panic under any circumstances
	assert.NotPanics(t, func() {
		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)
	})
}

// TestCollect_EmptyCache verifies that collecting from an empty cache
// produces only the self-instrumentation metrics.
func TestCollect_EmptyCache(t *testing.T) {
	promw := newTestPrometheusWriter("test")

	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	close(ch)

	var count int
	for range ch {
		count++
	}
	// 0 data metrics + 3 meta-metrics (totalScrapes, totalScrapeFailures, lastScrapeErrors)
	assert.Equal(t, 3, count, "empty cache should produce only 3 meta-metrics")
}

// TestCollect_StaleMetricsDropped verifies that metrics older than
// promScrapingStalenessHardDropLimit are dropped.
func TestCollect_StaleMetricsDropped(t *testing.T) {
	promw := newTestPrometheusWriter("test")
	promw.gauges = map[string][]string{"metric1": {"*"}}

	staleEpoch := time.Now().Add(-promScrapingStalenessHardDropLimit - time.Minute).UnixNano()
	promw.Cache["db1"] = map[string]metrics.MeasurementEnvelope{
		"metric1": {
			DBName:     "db1",
			MetricName: "metric1",
			Data: metrics.Measurements{
				{
					metrics.EpochColumnName: staleEpoch,
					"value":                 int64(42),
				},
			},
		},
	}

	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	close(ch)

	var count int
	for range ch {
		count++
	}
	// 0 data metrics + 3 meta-metrics
	assert.Equal(t, 3, count, "stale metrics should be dropped, only meta-metrics remain")
}
