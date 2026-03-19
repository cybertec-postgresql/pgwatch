package reaper

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDefinerWriter implements sinks.Writer and sinks.MetricsDefiner for testing.
type mockDefinerWriter struct {
	defineErr    error
	defineCalled bool
	receivedDefs *metrics.Metrics
}

func (m *mockDefinerWriter) SyncMetric(string, string, sinks.SyncOp) error { return nil }
func (m *mockDefinerWriter) Write(metrics.MeasurementEnvelope) error       { return nil }
func (m *mockDefinerWriter) DefineMetrics(defs *metrics.Metrics) error {
	m.defineCalled = true
	m.receivedDefs = defs
	return m.defineErr
}

var (
	initialMetricDefs = metrics.MetricDefs{
		"metric1": metrics.Metric{Description: "metric1"},
	}
	initialPresetDefs = metrics.PresetDefs{
		"preset1": metrics.Preset{Description: "preset1", Metrics: metrics.MetricIntervals{"metric1": 1.0}},
	}

	newMetricDefs = metrics.MetricDefs{
		"metric2": metrics.Metric{Description: "metric2"},
	}
	newPresetDefs = metrics.PresetDefs{
		"preset2": metrics.Preset{Description: "preset2", Metrics: metrics.MetricIntervals{"metric2": 2.0}},
	}
)

func TestReaper_FetchStatsDirectlyFromOS(t *testing.T) {
	a := assert.New(t)
	r := &Reaper{}
	t.Run("metrics directly fetchable when on same host", func(*testing.T) {
		conn, _ := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
		expq := conn.ExpectQuery("SELECT COALESCE(inet_client_addr(), inet_server_addr()) IS NULL")
		expq.Times(uint(len(directlyFetchableOSMetrics)))
		md := &sources.SourceConn{Conn: conn}
		for _, m := range directlyFetchableOSMetrics {
			expq.WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true))
			a.True(IsDirectlyFetchableMetric(md, m), "Expected %s to be directly fetchable", m)
			a.NotPanics(func() {
				_, _ = r.FetchStatsDirectlyFromOS(context.Background(), md, m)
			})
		}
	})

	t.Run("cpu_load not directly fetchable when not on same host", func(*testing.T) {
		remoteConn, _ := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
		remoteConn.ExpectQuery("SELECT COALESCE(inet_client_addr(), inet_server_addr()) IS NULL").
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))
		remoteMd := &sources.SourceConn{Conn: remoteConn}
		a.False(IsDirectlyFetchableMetric(remoteMd, metricCPULoad),
			"cpu_load should not be directly fetchable when pgwatch is not on the same host as PostgreSQL")
	})
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

func TestReaper_LoadMetrics(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("returns error from GetMetrics", func(t *testing.T) {
		r := NewReaper(ctx, &cmdopts.Options{
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) {
					return nil, assert.AnError
				},
			},
		})
		assert.ErrorIs(t, r.LoadMetrics(), assert.AnError)
	})

	t.Run("updates metricDefs on success", func(t *testing.T) {
		defs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{"m1": {Description: "M1"}},
			PresetDefs: metrics.PresetDefs{"p1": {Description: "P1", Metrics: metrics.MetricIntervals{"m1": 1.0}}},
		}
		r := NewReaper(ctx, &cmdopts.Options{
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) { return defs, nil },
			},
		})
		assert.NoError(t, r.LoadMetrics())

		m, ok := metricDefs.GetMetricDef("m1")
		assert.True(t, ok)
		assert.Equal(t, "M1", m.Description)

		p, ok := metricDefs.GetPresetDef("p1")
		assert.True(t, ok)
		assert.Equal(t, "P1", p.Description)
	})

	t.Run("calls DefineMetrics on MetricsDefiner sink", func(t *testing.T) {
		defs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{"m2": {Description: "M2"}},
			PresetDefs: metrics.PresetDefs{},
		}
		mock := &mockDefinerWriter{}
		r := NewReaper(ctx, &cmdopts.Options{
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) { return defs, nil },
			},
			SinksWriter: mock,
		})
		assert.NoError(t, r.LoadMetrics())
		assert.True(t, mock.defineCalled, "DefineMetrics should be called on MetricsDefiner sink")
		assert.Equal(t, defs, mock.receivedDefs)
	})

	t.Run("DefineMetrics error is logged not returned", func(t *testing.T) {
		defs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{"m3": {Description: "M3"}},
			PresetDefs: metrics.PresetDefs{},
		}
		mock := &mockDefinerWriter{defineErr: assert.AnError}
		r := NewReaper(ctx, &cmdopts.Options{
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) { return defs, nil },
			},
			SinksWriter: mock,
		})
		assert.NoError(t, r.LoadMetrics(), "DefineMetrics error should not propagate")
		assert.True(t, mock.defineCalled)
	})

	t.Run("resolves preset metrics for monitored sources", func(t *testing.T) {
		defs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{
				"m1": {Description: "M1"},
				"m2": {Description: "M2"},
			},
			PresetDefs: metrics.PresetDefs{
				"preset1":  {Metrics: metrics.MetricIntervals{"m1": 10.0}},
				"standby1": {Metrics: metrics.MetricIntervals{"m2": 20.0}},
			},
		}
		r := NewReaper(ctx, &cmdopts.Options{
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) { return defs, nil },
			},
		})
		r.monitoredSources = sources.SourceConns{
			sources.NewSourceConn(sources.Source{
				Name:                 "src1",
				PresetMetrics:        "preset1",
				PresetMetricsStandby: "standby1",
			}),
		}
		assert.NoError(t, r.LoadMetrics())

		sc := r.monitoredSources[0]
		assert.Equal(t, metrics.MetricIntervals{"m1": 10.0}, sc.Metrics)
		assert.Equal(t, metrics.MetricIntervals{"m2": 20.0}, sc.MetricsStandby)
	})

	t.Run("skips preset resolution for sources without presets", func(t *testing.T) {
		customMetrics := metrics.MetricIntervals{"cpu": 5.0}
		defs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{"cpu": {Description: "CPU"}},
			PresetDefs: metrics.PresetDefs{},
		}
		r := NewReaper(ctx, &cmdopts.Options{
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) { return defs, nil },
			},
		})
		r.monitoredSources = sources.SourceConns{
			sources.NewSourceConn(sources.Source{
				Name:    "src2",
				Metrics: customMetrics,
			}),
		}
		assert.NoError(t, r.LoadMetrics())
		assert.Equal(t, customMetrics, r.monitoredSources[0].Metrics, "custom metrics should be unchanged")
	})

	// Regression test for https://github.com/cybertec-postgresql/pgwatch/issues/1091
	// When a source config change (e.g. custom_tags) triggers a gatherer restart AND the preset
	// interval is updated simultaneously, the new interval must be picked up after reload.
	t.Run("preset interval update is applied after source config change", func(t *testing.T) {
		ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
		initialDefs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{"test_metric": {}},
			PresetDefs: metrics.PresetDefs{
				"test_preset": {Metrics: metrics.MetricIntervals{"test_metric": 1}},
			},
		}
		src := sources.Source{
			Name:          "test_source",
			IsEnabled:     true,
			Kind:          sources.SourcePostgres,
			ConnStr:       "postgres://localhost:5432/testdb",
			CustomTags:    map[string]string{"version": "1.0"},
			PresetMetrics: "test_preset",
		}
		getMetricsFn := func() (*metrics.Metrics, error) { return initialDefs, nil }
		r := NewReaper(ctx, &cmdopts.Options{
			SourcesReaderWriter: &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) { return sources.Sources{src}, nil },
			},
			MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
				GetMetricsFunc: func() (*metrics.Metrics, error) { return getMetricsFn() },
			},
			SinksWriter: &sinks.MultiWriter{},
		})
		require.NoError(t, r.LoadSources(ctx))
		require.NoError(t, r.LoadMetrics())
		assert.Equal(t, metrics.MetricIntervals{"test_metric": 1}, r.monitoredSources[0].Metrics)

		// Attach a mock connection so CloseResourcesForRemovedMonitoredDBs doesn't panic
		// when the custom_tags change triggers a full source restart.
		mockConn, err := pgxmock.NewPool()
		require.NoError(t, err)
		mockConn.ExpectClose()
		r.monitoredSources[0].Conn = mockConn

		// Simulate what happens between two Reap iterations:
		// 1. source custom_tags change triggers restart
		// 2. preset interval also changes
		src.CustomTags = map[string]string{"version": "2.0"}
		updatedDefs := &metrics.Metrics{
			MetricDefs: metrics.MetricDefs{"test_metric": {}},
			PresetDefs: metrics.PresetDefs{
				"test_preset": {Metrics: metrics.MetricIntervals{"test_metric": 2}},
			},
		}
		getMetricsFn = func() (*metrics.Metrics, error) { return updatedDefs, nil }

		require.NoError(t, r.LoadSources(ctx))
		require.NoError(t, r.LoadMetrics())
		assert.Equal(t, metrics.MetricIntervals{"test_metric": 2}, r.monitoredSources[0].Metrics,
			"preset interval should be updated after source config change triggered a restart")
	})
}

func TestChangeDetectionResults(t *testing.T) {
	a := assert.New(t)

	cdr := &ChangeDetectionResults{
		Target:  "test",
		Created: 2,
		Altered: 3,
		Dropped: 1,
	}

	a.Equal(6, cdr.Total(), "Total should be sum of Created, Altered, and Dropped")
	a.Equal("test: 2/3/1", cdr.String(), "String representation should match expected format")
}
