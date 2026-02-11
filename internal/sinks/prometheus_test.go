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

// TestLazyInitialization_WriteAfterCollect verifies that Write() works after
// Collect() clears the cache. Collect() no longer pre-creates maps, so Write()
// must create them lazily.
func TestLazyInitialization_WriteAfterCollect(t *testing.T) {
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

	// Collect clears the cache
	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)
	assert.Empty(t, promw.Cache, "cache should be empty after Collect")

	// Write after Collect - must work via lazy initialization
	msg.Data[0]["value"] = int64(200)
	require.NoError(t, promw.Write(msg))

	assert.Contains(t, promw.Cache, "db1")
	assert.Equal(t, int64(200), promw.Cache["db1"]["metric1"].Data[0]["value"])
}

// TestCollect_NoPreallocation verifies Collect() creates an empty cache
// without pre-allocating maps for each database (O(1) instead of O(N)).
func TestCollect_NoPreallocation(t *testing.T) {
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

	// Collect
	ch := make(chan prometheus.Metric, 100)
	promw.Collect(ch)

	// New cache should be empty - no pre-allocated maps
	assert.Empty(t, promw.Cache)
}
