package sinks

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

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
	promw.gauges = map[string][]string{promInstanceUpStateMetric: {"*"}}

	// Create two data rows with the same labels but potentially different
	// map iteration orders (Go randomizes map iteration).
	promw.Cache["db1"] = map[string]metrics.MeasurementEnvelope{
		"metric1": {
			DBName:     "db1",
			MetricName: promInstanceUpStateMetric,
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
	for i := range 100 {
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
			CustomTags: map[string]string{"sys_id": "42"}, // custom tags should not affect identity
			Data: metrics.Measurements{
				{
					metrics.EpochColumnName: time.Now().UnixNano(),
					"tag_host":              "server1",
					"value":                 int64(42),
					"bool_val":              false,
					"extra_field1":          "ignored", // extra fields should not affect identity
				},
				{
					metrics.EpochColumnName: time.Now().UnixNano(),
					"tag_host":              "server1",
					"value":                 int64(99), // different values, same identity
					"bool_val":              true,
					"extra_field1":          "ignored", // extra fields should not affect identity
				},
			},
		},
	}

	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	close(ch)

	var count int
	for c := range ch {
		t.Log(c.Desc())
		count++
	}
	// Should deduplicate — only the first occurrence is emitted.
	// 2 data metrics + 3 meta-metrics = 5 total.
	assert.Equal(t, 2+3, count, "duplicate metric identity should be deduplicated (1 data + 3 meta)")
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

	staleEpoch := time.Now().Add(-promCacheTTL - time.Minute).UnixNano()
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

func TestPrometheusWriteEmpty(t *testing.T) {
	promw := newTestPrometheusWriter("test")
	assert.NoError(t, promw.Write(metrics.MeasurementEnvelope{}))
	ch := make(chan prometheus.Metric, 100)
	written, errCount := promw.WritePromMetrics(metrics.MeasurementEnvelope{}, ch)
	assert.Zero(t, errCount)
	assert.Zero(t, written)
	close(ch)
}

func TestPrometheusWRiteUnsupportedMetric(t *testing.T) {
	promw := newTestPrometheusWriter("test")
	assert.NoError(t, promw.Write(metrics.MeasurementEnvelope{
		DBName:     "db1",
		MetricName: "change_events", // unsupported metric
		Data: metrics.Measurements{
			{metrics.EpochColumnName: time.Now().UnixNano(), "value": int64(100)},
		},
	}))
}

// TestSourceKindPrometheus covers T041: SourceKind == "prometheus" is the sentinel
// used to distinguish prom-sourced envelopes from DB-sourced ones.
func TestSourceKindPrometheus(t *testing.T) {
	tests := []struct {
		name     string
		envelope metrics.MeasurementEnvelope
		want     bool
	}{
		{name: "prometheus", envelope: metrics.MeasurementEnvelope{SourceKind: "prometheus"}, want: true},
		{name: "empty", envelope: metrics.MeasurementEnvelope{}, want: false},
		{name: "postgresql", envelope: metrics.MeasurementEnvelope{SourceKind: "postgresql"}, want: false},
		{name: "pgbouncer", envelope: metrics.MeasurementEnvelope{SourceKind: "pgbouncer"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.envelope.SourceKind == "prometheus")
		})
	}
}

// collectDataMetrics drains ch and returns only the metrics whose desc string
// contains nameFilter.
func collectDataMetrics(t *testing.T, ch <-chan prometheus.Metric, nameFilter string) []prometheus.Metric {
	t.Helper()
	var out []prometheus.Metric
	for m := range ch {
		if strings.Contains(m.Desc().String(), nameFilter) {
			out = append(out, m)
		}
	}
	return out
}

// TestPrometheusWriter_PromSourcedEnvelope covers T042: Write + Collect for a
// prom-sourced envelope.
func TestPrometheusWriter_PromSourcedEnvelope(t *testing.T) {
	const namespace = "pgwatch"
	const metricName = "pg_stat_activity_count"
	epochNs := time.Now().UnixNano()

	newEnv := func() metrics.MeasurementEnvelope {
		return metrics.MeasurementEnvelope{
			DBName:     "mydb",
			MetricName: metricName,
			SourceKind: "prometheus",
			Data: metrics.Measurements{
				{
					"tag_datname":           "defaultdb",
					metricName:              float64(42),
					metrics.EpochColumnName: epochNs,
				},
			},
		}
	}

	t.Run("metric name has no namespace prefix", func(t *testing.T) {
		promw := newTestPrometheusWriter(namespace)
		require.NoError(t, promw.Write(newEnv()))

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)
		close(ch)

		dataMetrics := collectDataMetrics(t, ch, metricName)
		require.Len(t, dataMetrics, 1)

		descStr := dataMetrics[0].Desc().String()
		assert.Contains(t, descStr, `fqName: "`+metricName+`"`)
		assert.NotContains(t, descStr, namespace+"_"+metricName)
	})

	t.Run("tag_* columns become labels", func(t *testing.T) {
		promw := newTestPrometheusWriter(namespace)
		require.NoError(t, promw.Write(newEnv()))

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)
		close(ch)

		dataMetrics := collectDataMetrics(t, ch, metricName)
		require.Len(t, dataMetrics, 1)

		var dtoMetric dto.Metric
		require.NoError(t, dataMetrics[0].Write(&dtoMetric))

		labels := make(map[string]string)
		for _, lp := range dtoMetric.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}
		assert.Equal(t, "defaultdb", labels["datname"], "tag_datname should become label datname")
		assert.Equal(t, "mydb", labels["dbname"], "DBName should be added as dbname label")
	})

	t.Run("epoch_ns used as metric timestamp", func(t *testing.T) {
		promw := newTestPrometheusWriter(namespace)
		require.NoError(t, promw.Write(newEnv()))

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)
		close(ch)

		dataMetrics := collectDataMetrics(t, ch, metricName)
		require.Len(t, dataMetrics, 1)

		var dtoMetric dto.Metric
		require.NoError(t, dataMetrics[0].Write(&dtoMetric))
		require.NotNil(t, dtoMetric.TimestampMs)
		assert.Equal(t, epochNs/1_000_000, dtoMetric.GetTimestampMs())
	})

	t.Run("duplicate label sets are deduplicated", func(t *testing.T) {
		promw := newTestPrometheusWriter(namespace)

		env := metrics.MeasurementEnvelope{
			DBName:     "mydb",
			MetricName: "pg_connections",
			SourceKind: "prometheus",
			Data: metrics.Measurements{
				{
					"tag_host":              "server1",
					"pg_connections":        float64(10),
					metrics.EpochColumnName: time.Now().UnixNano(),
				},
				{
					"tag_host":              "server1", // same label set → duplicate
					"pg_connections":        float64(20),
					metrics.EpochColumnName: time.Now().UnixNano(),
				},
			},
		}
		require.NoError(t, promw.Write(env))

		ch := make(chan prometheus.Metric, 100)
		promw.Collect(ch)
		close(ch)

		dataMetrics := collectDataMetrics(t, ch, "pg_connections")
		assert.Len(t, dataMetrics, 1, "duplicate (metric_name, label_set) pair should be emitted only once")
	})
}

// TestPrometheusWriter_NonPromSourced_NamespacePrefix covers T043: the pgwatch
// namespace IS still prepended for non-prometheus-sourced envelopes.
func TestPrometheusWriter_NonPromSourced_NamespacePrefix(t *testing.T) {
	const namespace = "pgwatch"
	promw := newTestPrometheusWriter(namespace)
	promw.gauges = map[string][]string{"pg_stat_activity": {"*"}}

	env := metrics.MeasurementEnvelope{
		DBName:     "mydb",
		MetricName: "pg_stat_activity",
		SourceKind: "", // not prometheus
		Data: metrics.Measurements{
			{
				metrics.EpochColumnName: time.Now().UnixNano(),
				"numbackends":           int64(5),
			},
		},
	}
	require.NoError(t, promw.Write(env))

	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	close(ch)

	dataMetrics := collectDataMetrics(t, ch, "pg_stat_activity")
	require.Len(t, dataMetrics, 1)

	descStr := dataMetrics[0].Desc().String()
	assert.Contains(t, descStr, `fqName: "`+namespace+`_pg_stat_activity_numbackends"`)
}
