package reaper

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sync"
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

func TestReaper_LoadSources(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())

	t.Run("Test pause trigger file", func(t *testing.T) {
		a := assert.New(t)
		pausefile := filepath.Join(t.TempDir(), "pausefile")
		require.NoError(t, os.WriteFile(pausefile, []byte("foo"), 0644))
		r := newReaper(ctx, &cmdopts.Options{Metrics: metrics.CmdOpts{EmergencyPauseTriggerfile: pausefile}})
		a.NoError(r.LoadSources(ctx))
		a.True(len(r.monitoredSources) == 0, "Expected no monitored sources when pause trigger file exists")
	})

	t.Run("Test SyncFromReader errror", func(t *testing.T) {
		a := assert.New(t)
		reader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return nil, assert.AnError
			},
		}
		r := newReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		a.Error(r.LoadSources(ctx))
		a.Equal(0, len(r.monitoredSources), "Expected no monitored sources after error")
	})

	t.Run("Test SyncFromReader success", func(t *testing.T) {
		a := assert.New(t)
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres}
		reader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{source1, source2}, nil
			},
		}

		r := newReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		a.NoError(r.LoadSources(ctx))
		a.Equal(2, len(r.monitoredSources), "Expected two monitored sources after successful load")
		a.NotNil(r.monitoredSources.GetMonitoredDatabase(source1.Name))
		a.NotNil(r.monitoredSources.GetMonitoredDatabase(source2.Name))
	})

	t.Run("Test repeated load", func(t *testing.T) {
		a := assert.New(t)
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres}
		reader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{source1, source2}, nil
			},
		}

		r := newReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		a.NoError(r.LoadSources(ctx))
		a.Equal(2, len(r.monitoredSources), "Expected two monitored sources after first load")

		// Load again with the same sources
		a.NoError(r.LoadSources(ctx))
		a.Equal(2, len(r.monitoredSources), "Expected still two monitored sources after second load")
	})

	t.Run("Test group limited sources", func(t *testing.T) {
		a := assert.New(t)
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres, Group: ""}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres, Group: "group1"}
		source3 := sources.Source{Name: "Source 3", IsEnabled: true, Kind: sources.SourcePostgres, Group: "group1"}
		source4 := sources.Source{Name: "Source 4", IsEnabled: true, Kind: sources.SourcePostgres, Group: "group2"}
		source5 := sources.Source{Name: "Source 5", IsEnabled: true, Kind: sources.SourcePostgres, Group: "default"}
		newReader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{source1, source2, source3, source4, source5}, nil
			},
		}

		r := newReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader, Sources: sources.CmdOpts{Groups: []string{"group1", "group2"}}})
		a.NoError(r.LoadSources(ctx))
		a.Equal(3, len(r.monitoredSources), "Expected three monitored sources after load")

		r = newReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader, Sources: sources.CmdOpts{Groups: []string{"group1"}}})
		a.NoError(r.LoadSources(ctx))
		a.Equal(2, len(r.monitoredSources), "Expected two monitored source after group filtering")

		r = newReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader})
		a.NoError(r.LoadSources(ctx))
		a.Equal(5, len(r.monitoredSources), "Expected five monitored sources after resetting groups")
	})

	t.Run("Test source config changes trigger restart", func(t *testing.T) {
		baseSource := sources.Source{
			Name:           "TestSource",
			IsEnabled:      true,
			Kind:           sources.SourcePostgres,
			ConnStr:        "postgres://localhost:5432/testdb",
			Metrics:        metrics.MetricIntervals{"cpu": 10, "memory": 20},
			MetricsStandby: metrics.MetricIntervals{"cpu": 30},
			CustomTags:     map[string]string{"env": "test"},
			Group:          "default",
		}

		testCases := []struct {
			name         string
			modifySource func(s *sources.Source)
			expectCancel bool
		}{
			{
				name: "custom tags change",
				modifySource: func(s *sources.Source) {
					s.CustomTags = map[string]string{"env": "production"}
				},
				expectCancel: true,
			},
			{
				name: "custom tags add new tag",
				modifySource: func(s *sources.Source) {
					s.CustomTags = map[string]string{"env": "test", "region": "us-east"}
				},
				expectCancel: true,
			},
			{
				name: "custom tags remove tag",
				modifySource: func(s *sources.Source) {
					s.CustomTags = map[string]string{}
				},
				expectCancel: true,
			},
			{
				name: "preset metrics change",
				modifySource: func(s *sources.Source) {
					s.PresetMetrics = "exhaustive"
				},
				expectCancel: true,
			},
			{
				name: "preset standby metrics change",
				modifySource: func(s *sources.Source) {
					s.PresetMetricsStandby = "standby-preset"
				},
				expectCancel: true,
			},
			{
				name: "connection string change",
				modifySource: func(s *sources.Source) {
					s.ConnStr = "postgres://localhost:5433/newdb"
				},
				expectCancel: true,
			},
			{
				name: "custom metrics change interval",
				modifySource: func(s *sources.Source) {
					s.Metrics = metrics.MetricIntervals{"cpu": 15, "memory": 20}
				},
				expectCancel: true,
			},
			{
				name: "custom metrics add new metric",
				modifySource: func(s *sources.Source) {
					s.Metrics = metrics.MetricIntervals{"cpu": 10, "memory": 20, "disk": 30}
				},
				expectCancel: true,
			},
			{
				name: "custom metrics remove metric",
				modifySource: func(s *sources.Source) {
					s.Metrics = metrics.MetricIntervals{"cpu": 10}
				},
				expectCancel: true,
			},
			{
				name: "standby metrics change",
				modifySource: func(s *sources.Source) {
					s.MetricsStandby = metrics.MetricIntervals{"cpu": 60}
				},
				expectCancel: true,
			},
			{
				name: "group change",
				modifySource: func(s *sources.Source) {
					s.Group = "new-group"
				},
				expectCancel: true,
			},
			{
				name: "kind change",
				modifySource: func(s *sources.Source) {
					s.Kind = sources.SourcePgBouncer
				},
				expectCancel: true,
			},
			{
				name: "only if master change",
				modifySource: func(s *sources.Source) {
					s.OnlyIfMaster = true
				},
				expectCancel: true,
			},
			{
				name: "no change - same config",
				modifySource: func(_ *sources.Source) {
					// No modifications - source stays the same
				},
				expectCancel: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				a := assert.New(t)
				initialSource := *baseSource.Clone()
				initialReader := &testutil.MockSourcesReaderWriter{
					GetSourcesFunc: func() (sources.Sources, error) {
						return sources.Sources{initialSource}, nil
					},
				}

				r := newReaper(ctx, &cmdopts.Options{
					SourcesReaderWriter: initialReader,
					SinksWriter:         &sinks.MultiWriter{},
				})
				a.NoError(r.LoadSources(ctx))
				a.Equal(1, len(r.monitoredSources), "Expected one monitored source after initial load")

				mockConn, err := pgxmock.NewPool()
				require.NoError(t, err)
				mockConn.ExpectClose()
				r.monitoredSources[0].(*sources.DbConn).Conn = mockConn

				// Add a mock cancel function for the source reaper
				cancelCalled := make(map[string]bool)
				r.cancelFuncs[initialSource.Name] = func() {
					cancelCalled[initialSource.Name] = true
				}

				// Create modified source
				modifiedSource := *baseSource.Clone()
				tc.modifySource(&modifiedSource)

				modifiedReader := &testutil.MockSourcesReaderWriter{
					GetSourcesFunc: func() (sources.Sources, error) {
						return sources.Sources{modifiedSource}, nil
					},
				}
				r.SourcesReaderWriter = modifiedReader

				// Reload sources
				a.NoError(r.LoadSources(ctx))
				a.Equal(1, len(r.monitoredSources), "Expected one monitored source after reload")
				a.Equal(modifiedSource, r.monitoredSources[0].GetSource())

				assert.Equal(t, tc.expectCancel, cancelCalled[initialSource.Name])
				if tc.expectCancel {
					assert.Nil(t, mockConn.ExpectationsWereMet(), "Expected all mock expectations to be met")
					_, exists := r.cancelFuncs[initialSource.Name]
					assert.False(t, exists, "Expected cancel func to be removed from map after cancellation")
				}
			})
		}
	})

	t.Run("Test only changed source cancelled in multi-source setup", func(t *testing.T) {
		a := assert.New(t)
		source1 := sources.Source{
			Name:      "Source1",
			IsEnabled: true,
			Kind:      sources.SourcePostgres,
			ConnStr:   "postgres://localhost:5432/db1",
			Metrics:   metrics.MetricIntervals{"cpu": 10},
		}
		source2 := sources.Source{
			Name:      "Source2",
			IsEnabled: true,
			Kind:      sources.SourcePostgres,
			ConnStr:   "postgres://localhost:5432/db2",
			Metrics:   metrics.MetricIntervals{"memory": 20},
		}

		initialReader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{source1, source2}, nil
			},
		}

		r := newReaper(ctx, &cmdopts.Options{
			SourcesReaderWriter: initialReader,
			SinksWriter:         &sinks.MultiWriter{},
		})
		a.NoError(r.LoadSources(ctx))

		// Set mock connections for both sources to avoid nil pointer on Close()
		mockConn1, err := pgxmock.NewPool()
		require.NoError(t, err)
		mockConn1.ExpectClose()
		r.monitoredSources[0].(*sources.DbConn).Conn = mockConn1

		source1Cancelled := false
		source2Cancelled := false
		r.cancelFuncs[source1.Name] = func() { source1Cancelled = true }
		r.cancelFuncs[source2.Name] = func() { source2Cancelled = true }

		// Only modify source1
		modifiedSource1 := *source1.Clone()
		modifiedSource1.ConnStr = "postgres://localhost:5433/db1_new"

		modifiedReader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{modifiedSource1, source2}, nil
			},
		}
		r.SourcesReaderWriter = modifiedReader

		a.NoError(r.LoadSources(ctx))

		a.True(source1Cancelled, "Source1 should be cancelled due to config change")
		a.False(source2Cancelled, "Source2 should NOT be cancelled as it was not modified")
		a.Nil(mockConn1.ExpectationsWereMet(), "Expected all mock expectations to be met")
	})
}

type mockErr string

func (m mockErr) SyncMetric(string, string, sinks.SyncOp) error {
	return errors.New(string(m))
}

func (m mockErr) Write(metrics.MeasurementEnvelope) error {
	return errors.New(string(m))
}

func TestWriteMeasurements(t *testing.T) {
	ctx, cancel := context.WithCancel(log.WithLogger(t.Context(), log.NewNoopLogger()))
	defer cancel()
	var err mockErr = "write error"
	r := newReaper(ctx, &cmdopts.Options{
		SinksWriter: err,
	})
	go r.WriteMeasurements(ctx)
	r.WriteInstanceDown("foo")
}

func TestReaper_Ready(t *testing.T) {
	a := assert.New(t)
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := newReaper(ctx, &cmdopts.Options{})
	a.False(r.Ready())
	r.ready.Store(true)
	a.True(r.Ready())
}

func TestReaper_WriteInstanceDown(t *testing.T) {
	a := assert.New(t)
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := newReaper(ctx, &cmdopts.Options{})
	r.WriteInstanceDown("testdb")
	select {
	case msg := <-r.measurementCh:
		a.Equal("testdb", msg.DBName)
		a.Equal(specialMetricInstanceUp, msg.MetricName)
		require.Len(t, msg.Data, 1)
		a.Equal(0, msg.Data[0][specialMetricInstanceUp])
	default:
		t.Error("expected message in measurementCh")
	}
}

func TestReaper_AddSysinfoToMeasurements(t *testing.T) {
	t.Run("adds real dbname and system identifier fields", func(t *testing.T) {
		a := assert.New(t)
		r := &reaper{
			Options: &cmdopts.Options{
				Sinks: sinks.CmdOpts{
					RealDbnameField:       "real_dbname",
					SystemIdentifierField: "sys_id",
				},
			},
		}
		md := &sources.DbConn{
			RuntimeInfo: sources.RuntimeInfo{
				RealDbname:       "realdb",
				SystemIdentifier: "12345",
			},
		}
		data := metrics.Measurements{metrics.Measurement{}}
		r.AddSysinfoToMeasurements(data, md)
		a.Equal("realdb", data[0]["real_dbname"])
		a.Equal("12345", data[0]["sys_id"])
	})

	t.Run("skips fields when config field names are empty", func(t *testing.T) {
		a := assert.New(t)
		r := &reaper{Options: &cmdopts.Options{}}
		md := &sources.DbConn{
			RuntimeInfo: sources.RuntimeInfo{
				RealDbname:       "realdb",
				SystemIdentifier: "12345",
			},
		}
		data := metrics.Measurements{metrics.Measurement{}}
		r.AddSysinfoToMeasurements(data, md)
		a.NotContains(data[0], "real_dbname")
		a.NotContains(data[0], "sys_id")
	})

	t.Run("skips fields when md values are empty", func(t *testing.T) {
		a := assert.New(t)
		r := &reaper{
			Options: &cmdopts.Options{
				Sinks: sinks.CmdOpts{
					RealDbnameField:       "real_dbname",
					SystemIdentifierField: "sys_id",
				},
			},
		}
		md := &sources.DbConn{}
		data := metrics.Measurements{metrics.Measurement{}}
		r.AddSysinfoToMeasurements(data, md)
		a.NotContains(data[0], "real_dbname")
		a.NotContains(data[0], "sys_id")
	})
}

// mockSyncWriter records SyncMetric calls for assertions.
type mockSyncWriter struct {
	synced []struct{ source, metric string }
	err    error
}

func (m *mockSyncWriter) SyncMetric(sourceName, metricName string, _ sinks.SyncOp) error {
	if m.err != nil {
		return m.err
	}
	m.synced = append(m.synced, struct{ source, metric string }{sourceName, metricName})
	return nil
}

func (m *mockSyncWriter) Write(metrics.MeasurementEnvelope) error { return nil }

func TestReaper_SyncMetricsToSinks(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	srcL := log.GetLogger(ctx)

	// Populate the package-level metricDefs used by SyncMetricsToSinks.
	origDefs := metricDefs
	metricDefs = NewConcurrentMetricDefs()
	t.Cleanup(func() { metricDefs = origDefs })

	metricDefs.Assign(&metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"cpu":    metrics.Metric{},
			"memory": metrics.Metric{StorageName: "mem_storage"},
		},
		PresetDefs: metrics.PresetDefs{},
	})

	t.Run("syncs known metrics using metric name", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})
		config := metrics.MetricIntervals{"cpu": 10}

		r.SyncMetricsToSinks(srcL, "mydb", config)

		require.Len(t, sw.synced, 1)
		a.Equal("mydb", sw.synced[0].source)
		a.Equal("cpu", sw.synced[0].metric)
	})

	t.Run("uses StorageName when set and metric is not special", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})
		config := metrics.MetricIntervals{"memory": 30}

		r.SyncMetricsToSinks(srcL, "mydb", config)

		require.Len(t, sw.synced, 1)
		a.Equal("mem_storage", sw.synced[0].metric)
	})

	t.Run("skips unknown metric definitions", func(t *testing.T) {
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})
		config := metrics.MetricIntervals{"unknown_metric": 60}

		r.SyncMetricsToSinks(srcL, "mydb", config)

		assert.Empty(t, sw.synced)
	})

	t.Run("logs sink error but continues", func(t *testing.T) {
		sw := &mockSyncWriter{err: errors.New("sink error")}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})
		config := metrics.MetricIntervals{"cpu": 10}

		// Should not panic even when SyncMetric returns an error.
		assert.NotPanics(t, func() {
			r.SyncMetricsToSinks(srcL, "mydb", config)
		})
	})

	t.Run("empty config results in no syncs", func(t *testing.T) {
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		r.SyncMetricsToSinks(srcL, "mydb", metrics.MetricIntervals{})

		assert.Empty(t, sw.synced)
	})

	t.Run("multiple metrics all synced", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})
		config := metrics.MetricIntervals{"cpu": 10, "memory": 20}

		r.SyncMetricsToSinks(srcL, "mydb", config)

		a.Len(sw.synced, 2)
		synced := maps.Collect(func(yield func(string, bool) bool) {
			for _, s := range sw.synced {
				if !yield(s.metric, true) {
					return
				}
			}
		})
		a.True(synced["cpu"])
		a.True(synced["mem_storage"])
	})
}

func TestReaper_ShutdownOldWorkers(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("cancels worker for DB removed from config", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }

		r.ShutdownOldWorkers(ctx, map[string]bool{})

		a.True(cancelCalled)
		a.NotContains(r.cancelFuncs, "testdb")
	})

	t.Run("cancels worker for whole DB shutdown", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }

		r.ShutdownOldWorkers(ctx, map[string]bool{"testdb": true})

		a.True(cancelCalled)
		a.NotContains(r.cancelFuncs, "testdb")
	})

	t.Run("keeps worker when source is still active", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }
		r.monitoredSources = sources.SourceConns{
			sources.NewDbConn(sources.Source{Name: "testdb", Metrics: metrics.MetricIntervals{"cpu": 10}}),
		}

		r.ShutdownOldWorkers(ctx, map[string]bool{})

		a.False(cancelCalled)
		a.Contains(r.cancelFuncs, "testdb")
	})

	t.Run("cancels all workers when context is cancelled", func(t *testing.T) {
		a := assert.New(t)
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }
		r.monitoredSources = sources.SourceConns{
			sources.NewDbConn(sources.Source{Name: "testdb", Metrics: metrics.MetricIntervals{"cpu": 10}}),
		}

		r.ShutdownOldWorkers(cancelledCtx, map[string]bool{})

		a.True(cancelCalled)
	})
}

func TestReaper_CreateSourceHelpers(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("skips already initialized source", func(*testing.T) {
		r := newReaper(ctx, &cmdopts.Options{})
		md := &sources.DbConn{Source: sources.Source{Name: "existing"}}
		r.prevLoopMonitoredDBs = sources.SourceConns{md}
		// Conn is nil — would panic if used, proving early return
		r.CreateSourceHelpers(ctx, r.logger, md)
	})

	t.Run("skips non-postgres source", func(*testing.T) {
		r := newReaper(ctx, &cmdopts.Options{})
		md := &sources.DbConn{Source: sources.Source{Name: "pgbouncer", Kind: sources.SourcePgBouncer}}
		r.CreateSourceHelpers(ctx, r.logger, md)
	})

	t.Run("skips source in recovery", func(*testing.T) {
		r := newReaper(ctx, &cmdopts.Options{})
		md := &sources.DbConn{
			Source:      sources.Source{Name: "standby"},
			RuntimeInfo: sources.RuntimeInfo{IsInRecovery: true},
		}
		r.CreateSourceHelpers(ctx, r.logger, md)
	})

	t.Run("creates extensions when configured", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{
			Sources: sources.CmdOpts{TryCreateListedExtsIfMissing: "pg_stat_statements"},
		})
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		mock.ExpectQuery("pg_available_extensions").
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("pg_stat_statements"))
		mock.ExpectExec(`create extension if not exists`).
			WillReturnResult(pgxmock.NewResult("CREATE", 1))

		r.CreateSourceHelpers(ctx, r.logger, md)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("creates metric helpers when configured", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{
			Sources: sources.CmdOpts{CreateHelpers: true},
		})
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		const helperMetric = "test_helper_metric"
		metricDefs.MetricDefs[helperMetric] = metrics.Metric{
			InitSQL: "CREATE OR REPLACE FUNCTION test_helper() RETURNS void LANGUAGE sql AS ''",
		}
		t.Cleanup(func() { delete(metricDefs.MetricDefs, helperMetric) })
		md.Metrics = metrics.MetricIntervals{helperMetric: 10}

		mock.ExpectExec("CREATE OR REPLACE FUNCTION").
			WillReturnResult(pgxmock.NewResult("CREATE", 1))

		r.CreateSourceHelpers(ctx, r.logger, md)
		a.NoError(mock.ExpectationsWereMet())
	})
}

func TestReaper_PrintMemStats(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := newReaper(ctx, &cmdopts.Options{})
	assert.NotPanics(t, r.PrintMemStats)
}

// TestRace_AddSysinfoToMeasurements verifies no data race between concurrent
// RuntimeInfo writes (simulating FetchRuntimeInfo) and AddSysinfoToMeasurements reads.
func TestRace_AddSysinfoToMeasurements(*testing.T) {
	r := &reaper{
		Options: &cmdopts.Options{
			Sinks: sinks.CmdOpts{
				RealDbnameField:       "real_dbname",
				SystemIdentifierField: "sys_id",
			},
		},
	}
	md := sources.NewDbConn(sources.Source{Name: "race-test"})

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: simulate FetchRuntimeInfo updating RuntimeInfo fields under Lock.
	go func() {
		defer wg.Done()
		for range iterations {
			md.Lock()
			md.RealDbname = "realdb"
			md.SystemIdentifier = "12345"
			md.Unlock()
		}
	}()

	// Reader: AddSysinfoToMeasurements must RLock before reading.
	go func() {
		defer wg.Done()
		data := metrics.Measurements{metrics.Measurement{}}
		for range iterations {
			r.AddSysinfoToMeasurements(data, md)
		}
	}()

	wg.Wait()
}

// TestRace_CreateSourceHelpers verifies no data race between concurrent
// RuntimeInfo writes and CreateSourceHelpers reading IsInRecovery.
func TestRace_CreateSourceHelpers(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := newReaper(ctx, &cmdopts.Options{})
	md := sources.NewDbConn(sources.Source{
		Name: "race-test",
		Kind: sources.SourcePostgres,
	})

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range iterations {
			md.Lock()
			md.IsInRecovery = !md.IsInRecovery
			md.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		for range iterations {
			r.CreateSourceHelpers(ctx, r.logger, md)
		}
	}()

	wg.Wait()
}

// TestRace_MainLoopRuntimeInfoSnapshot verifies that the main reaper loop's
// RLock snapshot of IsInRecovery, VersionStr, ApproxDbSize, Metrics, and
// MetricsStandby does not race against concurrent FetchRuntimeInfo writes.
func TestRace_MainLoopRuntimeInfoSnapshot(*testing.T) {
	md := sources.NewDbConn(sources.Source{
		Name: "race-test",
		Kind: sources.SourcePostgres,
	})

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: simulate FetchRuntimeInfo updating RuntimeInfo fields under Lock.
	go func() {
		defer wg.Done()
		for range iterations {
			md.Lock()
			md.IsInRecovery = !md.IsInRecovery
			md.VersionStr = "PostgreSQL 16.0"
			md.ApproxDbSize = 1024
			md.Metrics = metrics.MetricIntervals{"cpu": 30}
			md.MetricsStandby = metrics.MetricIntervals{"cpu": 60}
			md.Unlock()
		}
	}()

	// Reader: snapshot under RLock exactly as the main loop does after FetchRuntimeInfo.
	go func() {
		defer wg.Done()
		for range iterations {
			md.RLock()
			_ = md.IsInRecovery
			_ = md.VersionStr
			_ = md.ApproxDbSize
			if md.Metrics != nil {
				_ = maps.Clone(md.Metrics)
			}
			if md.MetricsStandby != nil {
				_ = maps.Clone(md.MetricsStandby)
			}
			md.RUnlock()
		}
	}()

	wg.Wait()
}
