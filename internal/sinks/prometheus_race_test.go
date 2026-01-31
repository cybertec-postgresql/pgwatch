package sinks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestCollect_RaceCondition_Real(_ *testing.T) {
	// 1. Initialize the real PrometheusWriter
	// Note: In the current buggy code, this shares the global 'promAsyncMetricCache'
	promw, _ := NewPrometheusWriter(context.Background(), "127.0.0.1:0/pgwatch")

	// 2. Register a metric so Write() actually puts data into the map
	_ = promw.SyncMetric("race_db", "test_metric", AddOp)

	var wg sync.WaitGroup
	done := make(chan struct{})

	// --- The Writer (Simulating Database Updates) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				// Call the REAL Write method
				_ = promw.Write(metrics.MeasurementEnvelope{
					DBName:     "race_db",
					MetricName: "test_metric",
					Data: metrics.Measurements{
						{
							metrics.EpochColumnName: time.Now().UnixNano(),
							"value":                 int64(100),
						},
					},
				})
				// No sleep here -> hammer the map as fast as possible
			}
		}
	}()

	// --- The Collector (Simulating Prometheus Scrapes) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Prometheus provides a channel to receive metrics
		ch := make(chan prometheus.Metric, 10000)

		// Scrape 50 times (more than enough to trigger a race in a tight loop)
		for i := 0; i < 50; i++ {
			// Call the REAL Collect method
			promw.Collect(ch)

			// Drain the channel so it doesn't block
		drainLoop:
			for {
				select {
				case <-ch:
				default:
					break drainLoop
				}
			}
		}
		close(done) // Tell the writer to stop
	}()

	wg.Wait()
}
