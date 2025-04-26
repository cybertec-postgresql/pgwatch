package reaper

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/stretchr/testify/assert"
)

var (
	initialMetricDefs = metrics.MetricDefs{
		"metric1": metrics.Metric{Description: "metric1"},
	}
	initialPresetDefs = metrics.PresetDefs{
		"preset1": metrics.Preset{Description: "preset1", Metrics: map[string]float64{"metric1": 1.0}},
	}

	newMetricDefs = metrics.MetricDefs{
		"metric2": metrics.Metric{Description: "metric2"},
	}
	newPresetDefs = metrics.PresetDefs{
		"preset2": metrics.Preset{Description: "preset2", Metrics: map[string]float64{"metric2": 2.0}},
	}
)

func TestReaper_FetchStatsDirectlyFromOS(t *testing.T) {
	a := assert.New(t)
	r := &Reaper{}
	md := &sources.SourceConn{}
	for _, m := range directlyFetchableOSMetrics {
		a.True(IsDirectlyFetchableMetric(m), "Expected %s to be directly fetchable", m)
		a.NotPanics(func() {
			_, _ = r.FetchStatsDirectlyFromOS(context.Background(), md, m)
		})
	}
}

func TestConcurrentMetricDefs_Assign(t *testing.T) {
	concurrentDefs := NewConcurrentMetricDefs()
	concurrentDefs.Assign(&metrics.Metrics{
		MetricDefs: initialMetricDefs,
		PresetDefs: initialPresetDefs,
	})

	concurrentDefs.Assign(&metrics.Metrics{
		MetricDefs: newMetricDefs,
		PresetDefs: newPresetDefs,
	})

	assert.Equal(t, newMetricDefs, concurrentDefs.MetricDefs, "MetricDefs should be updated")
	assert.Equal(t, newPresetDefs, concurrentDefs.PresetDefs, "PresetDefs should be updated")
}

func TestConcurrentMetricDefs_RandomAccess(t *testing.T) {
	a := assert.New(t)

	concurrentDefs := NewConcurrentMetricDefs()
	concurrentDefs.Assign(&metrics.Metrics{
		MetricDefs: initialMetricDefs,
		PresetDefs: initialPresetDefs,
	})

	go a.NotPanics(func() {
		for range 1000 {
			_, ok1 := concurrentDefs.GetMetricDef("metric1")
			_, ok2 := concurrentDefs.GetMetricDef("metric2")
			a.True(ok1 || ok2, "Expected metric1 or metric3 to exist at any time")
			_, ok1 = concurrentDefs.GetPresetDef("preset1")
			_, ok2 = concurrentDefs.GetPresetDef("preset2")
			a.True(ok1 || ok2, "Expected preset1 or preset2 to exist at any time")
			m1 := concurrentDefs.GetPresetMetrics("preset1")
			m2 := concurrentDefs.GetPresetMetrics("preset2")
			a.True(m1 != nil || m2 != nil, "Expected preset1 or preset2 metrics to be non-empty")
		}
	})

	go a.NotPanics(func() {
		for range 1000 {
			concurrentDefs.Assign(&metrics.Metrics{
				MetricDefs: newMetricDefs,
				PresetDefs: newPresetDefs,
			})
		}
	})
}
