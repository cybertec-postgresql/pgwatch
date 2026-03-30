package reaper

import (
	"sync"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
)

// TestLoadMetrics_DataRace reproduces a data race between LoadMetrics writing
// md.Metrics / md.MetricsStandby and a concurrent reader calling
// md.GetMetricInterval. Without the md.Lock() guard in the LoadMetrics loop
// this test will fail under `go test -race`.
func TestLoadMetrics_DataRace(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	presetMetrics := metrics.MetricIntervals{"cpu_load": 60}
	defs := &metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"cpu_load": {Description: "CPU load"},
		},
		PresetDefs: metrics.PresetDefs{
			"basic": {Description: "basic preset", Metrics: presetMetrics},
		},
	}

	r := NewReaper(ctx, &cmdopts.Options{
		MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
			GetMetricsFunc: func() (*metrics.Metrics, error) {
				return defs, nil
			},
		},
	})

	md := sources.NewSourceConn(sources.Source{
		Name:          "race-test-db",
		PresetMetrics: "basic",
	})
	r.monitoredSources = sources.SourceConns{md}

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range iterations {
			if err := r.LoadMetrics(); err != nil {
				t.Errorf("LoadMetrics: %v", err)
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for range iterations {
			md.GetMetricInterval("cpu_load")
		}
	}()

	wg.Wait()
}
