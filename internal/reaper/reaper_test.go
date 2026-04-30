package reaper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		r := NewReaper(ctx, &cmdopts.Options{Metrics: metrics.CmdOpts{EmergencyPauseTriggerfile: pausefile}})
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
		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
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

		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
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

		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
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

		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader, Sources: sources.CmdOpts{Groups: []string{"group1", "group2"}}})
		a.NoError(r.LoadSources(ctx))
		a.Equal(3, len(r.monitoredSources), "Expected three monitored sources after load")

		r = NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader, Sources: sources.CmdOpts{Groups: []string{"group1"}}})
		a.NoError(r.LoadSources(ctx))
		a.Equal(2, len(r.monitoredSources), "Expected two monitored source after group filtering")

		r = NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader})
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

				r := NewReaper(ctx, &cmdopts.Options{
					SourcesReaderWriter: initialReader,
					SinksWriter:         &sinks.MultiWriter{},
				})
				a.NoError(r.LoadSources(ctx))
				a.Equal(1, len(r.monitoredSources), "Expected one monitored source after initial load")

				mockConn, err := pgxmock.NewPool()
				require.NoError(t, err)
				mockConn.ExpectClose()
				r.monitoredSources[0].Conn = mockConn

				// Add a mock cancel function for a metric gatherer
				cancelCalled := make(map[string]bool)
				for metric := range initialSource.Metrics {
					dbMetric := initialSource.Name + "¤¤¤" + metric
					r.cancelFuncs[dbMetric] = func() {
						cancelCalled[dbMetric] = true
					}
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
				a.Equal(modifiedSource, r.monitoredSources[0].Source)

				for metric := range initialSource.Metrics {
					dbMetric := initialSource.Name + "¤¤¤" + metric
					a.Equal(tc.expectCancel, cancelCalled[dbMetric])
					if tc.expectCancel {
						a.Nil(mockConn.ExpectationsWereMet(), "Expected all mock expectations to be met")
						_, exists := r.cancelFuncs[dbMetric]
						a.False(exists, "Expected cancel func to be removed from map after cancellation")
					}
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

		r := NewReaper(ctx, &cmdopts.Options{
			SourcesReaderWriter: initialReader,
			SinksWriter:         &sinks.MultiWriter{},
		})
		a.NoError(r.LoadSources(ctx))

		// Set mock connections for both sources to avoid nil pointer on Close()
		mockConn1, err := pgxmock.NewPool()
		require.NoError(t, err)
		mockConn1.ExpectClose()
		r.monitoredSources[0].Conn = mockConn1

		source1Cancelled := false
		source2Cancelled := false
		r.cancelFuncs[source1.Name+"¤¤¤"+"cpu"] = func() { source1Cancelled = true }
		r.cancelFuncs[source2.Name+"¤¤¤"+"memory"] = func() { source2Cancelled = true }

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

func newFetchMetricReaper() *Reaper {
	return &Reaper{
		Options: &cmdopts.Options{
			Metrics: metrics.CmdOpts{},
			Sinks:   sinks.CmdOpts{},
		},
		measurementCache: NewInstanceMetricCache(),
	}
}

func TestReaper_FetchMetric(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("metric not found in definitions", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		env, err := r.FetchMetric(ctx, md, "nonexistent_metric_xyz")
		a.ErrorIs(err, metrics.ErrMetricNotFound)
		a.Nil(env)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("primary-only metric skipped on standby", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["primary_only_metric"] = metrics.Metric{
			SQLs:       metrics.SQLs{0: "SELECT 1"},
			NodeStatus: "primary",
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		md.IsInRecovery = true

		env, err := r.FetchMetric(ctx, md, "primary_only_metric")
		a.NoError(err)
		a.Nil(env)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("standby-only metric skipped on primary", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["standby_only_metric"] = metrics.Metric{
			SQLs:       metrics.SQLs{0: "SELECT 1"},
			NodeStatus: "standby",
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		md.IsInRecovery = false

		env, err := r.FetchMetric(ctx, md, "standby_only_metric")
		a.NoError(err)
		a.Nil(env)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("default metric with no SQL for version returns nil", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["no_sql_metric"] = metrics.Metric{
			SQLs: metrics.SQLs{}, // no SQL defined
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		env, err := r.FetchMetric(ctx, md, "no_sql_metric")
		a.NoError(err)
		a.Nil(env)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("default metric query success returns envelope", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["test_metric"] = metrics.Metric{
			SQLs: metrics.SQLs{0: "SELECT 1"},
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		md.Name = "mydb"
		md.CustomTags = map[string]string{"env": "prod"}

		rows := pgxmock.NewRows([]string{"epoch_ns", "value"}).
			AddRow(time.Now().UnixNano(), int64(42))
		mock.ExpectQuery("SELECT 1").WillReturnRows(rows)

		env, err := r.FetchMetric(ctx, md, "test_metric")
		require.NoError(t, err)
		require.NotNil(t, env)
		a.Equal("mydb", env.DBName)
		a.Equal("test_metric", env.MetricName)
		a.Len(env.Data, 1)
		a.Equal(map[string]string{"env": "prod"}, env.CustomTags)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("default metric query error returns error", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["error_metric"] = metrics.Metric{
			SQLs: metrics.SQLs{0: "SELECT fail"},
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		mock.ExpectQuery("SELECT fail").WillReturnError(assert.AnError)

		env, err := r.FetchMetric(ctx, md, "error_metric")
		a.Error(err)
		a.Nil(env)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("default metric query returns empty rows", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["empty_metric"] = metrics.Metric{
			SQLs: metrics.SQLs{0: "SELECT empty"},
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		mock.ExpectQuery("SELECT empty").WillReturnRows(pgxmock.NewRows([]string{"epoch_ns"}))

		env, err := r.FetchMetric(ctx, md, "empty_metric")
		a.NoError(err)
		a.Nil(env)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("storage name used as metric name in envelope", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs["logical_metric"] = metrics.Metric{
			SQLs:        metrics.SQLs{0: "SELECT 1"},
			StorageName: "physical_metric",
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()

		rows := pgxmock.NewRows([]string{"epoch_ns", "v"}).
			AddRow(time.Now().UnixNano(), int64(1))
		mock.ExpectQuery("SELECT 1").WillReturnRows(rows)

		env, err := r.FetchMetric(ctx, md, "logical_metric")
		require.NoError(t, err)
		require.NotNil(t, env)
		a.Equal("physical_metric", env.MetricName)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("instance_up special metric returns envelope via GetInstanceUpMeasurement", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs[specialMetricInstanceUp] = metrics.Metric{
			SQLs: metrics.SQLs{0: "SELECT 1"},
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		mock.ExpectPing()

		env, err := r.FetchMetric(ctx, md, specialMetricInstanceUp)
		require.NoError(t, err)
		require.NotNil(t, env)
		a.Equal(specialMetricInstanceUp, env.MetricName)
		a.Len(env.Data, 1)
		a.Equal(1, env.Data[0][specialMetricInstanceUp])
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("change_events special metric returns nil when no changes detected", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		metricDefs.MetricDefs[specialMetricChangeEvents] = metrics.Metric{
			SQLs: metrics.SQLs{0: "SELECT 1"},
		}
		// Remove all hash metric definitions so detection functions return early
		delete(metricDefs.MetricDefs, "sproc_hashes")
		delete(metricDefs.MetricDefs, "table_hashes")
		delete(metricDefs.MetricDefs, "index_hashes")
		delete(metricDefs.MetricDefs, "configuration_hashes")
		delete(metricDefs.MetricDefs, "privilege_hashes")

		md, mock := createTestSourceConn(t)
		defer mock.Close()

		env, err := r.FetchMetric(ctx, md, specialMetricChangeEvents)
		a.NoError(err)
		a.Nil(env, "expected nil envelope when no changes detected")
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("cache hit serves data without querying DB", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		r.Metrics.InstanceLevelCacheMaxSeconds = 30

		metricDefs.MetricDefs["cached_metric"] = metrics.Metric{
			SQLs:            metrics.SQLs{0: "SELECT 1"},
			IsInstanceLevel: true,
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		md.Metrics = metrics.MetricIntervals{"cached_metric": 10}

		// Pre-populate the cache
		cachedData := metrics.Measurements{
			metrics.Measurement{
				metrics.EpochColumnName: time.Now().UnixNano(),
				"value":                 int64(99),
			},
		}
		cacheKey := md.GetClusterIdentifier() + ":cached_metric"
		r.measurementCache.Put(cacheKey, cachedData)

		// No DB query expected
		env, err := r.FetchMetric(ctx, md, "cached_metric")
		require.NoError(t, err)
		require.NotNil(t, env)
		a.Equal("cached_metric", env.MetricName)
		a.Len(env.Data, 1)
		a.NoError(mock.ExpectationsWereMet())
	})

	t.Run("sysinfo fields added to measurements", func(t *testing.T) {
		a := assert.New(t)
		r := newFetchMetricReaper()
		r.Sinks.RealDbnameField = "real_dbname"
		r.Sinks.SystemIdentifierField = "sys_id"
		metricDefs.MetricDefs["sysinfo_metric"] = metrics.Metric{
			SQLs: metrics.SQLs{0: "SELECT sysinfo"},
		}
		md, mock := createTestSourceConn(t)
		defer mock.Close()
		md.RealDbname = "realdb"
		md.SystemIdentifier = "42"

		rows := pgxmock.NewRows([]string{"epoch_ns", "v"}).
			AddRow(time.Now().UnixNano(), int64(1))
		mock.ExpectQuery("SELECT sysinfo").WillReturnRows(rows)

		env, err := r.FetchMetric(ctx, md, "sysinfo_metric")
		require.NoError(t, err)
		require.NotNil(t, env)
		a.Equal("realdb", env.Data[0]["real_dbname"])
		a.Equal("42", env.Data[0]["sys_id"])
		a.NoError(mock.ExpectationsWereMet())
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
	r := NewReaper(ctx, &cmdopts.Options{
		SinksWriter: err,
	})
	go r.WriteMeasurements(ctx)
	r.WriteInstanceDown("foo")
}

func TestReaper_Ready(t *testing.T) {
	a := assert.New(t)
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := NewReaper(ctx, &cmdopts.Options{})
	a.False(r.Ready())
	r.ready.Store(true)
	a.True(r.Ready())
}

func TestReaper_WriteInstanceDown(t *testing.T) {
	a := assert.New(t)
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := NewReaper(ctx, &cmdopts.Options{})
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
		r := &Reaper{
			Options: &cmdopts.Options{
				Sinks: sinks.CmdOpts{
					RealDbnameField:       "real_dbname",
					SystemIdentifierField: "sys_id",
				},
			},
		}
		md := &sources.SourceConn{
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
		r := &Reaper{Options: &cmdopts.Options{}}
		md := &sources.SourceConn{
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
		r := &Reaper{
			Options: &cmdopts.Options{
				Sinks: sinks.CmdOpts{
					RealDbnameField:       "real_dbname",
					SystemIdentifierField: "sys_id",
				},
			},
		}
		md := &sources.SourceConn{}
		data := metrics.Measurements{metrics.Measurement{}}
		r.AddSysinfoToMeasurements(data, md)
		a.NotContains(data[0], "real_dbname")
		a.NotContains(data[0], "sys_id")
	})
}

func TestReaper_ShutdownOldWorkers(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("cancels worker for DB removed from config", func(t *testing.T) {
		a := assert.New(t)
		r := NewReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		dbMetric := "testdb" + dbMetricJoinStr + "cpu"
		r.cancelFuncs[dbMetric] = func() { cancelCalled = true }

		r.ShutdownOldWorkers(ctx, map[string]bool{})

		a.True(cancelCalled)
		a.NotContains(r.cancelFuncs, dbMetric)
	})

	t.Run("cancels worker for whole DB shutdown", func(t *testing.T) {
		a := assert.New(t)
		r := NewReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		dbMetric := "testdb" + dbMetricJoinStr + "cpu"
		r.cancelFuncs[dbMetric] = func() { cancelCalled = true }

		r.ShutdownOldWorkers(ctx, map[string]bool{"testdb": true})

		a.True(cancelCalled)
		a.NotContains(r.cancelFuncs, dbMetric)
	})

	t.Run("cancels worker for metric removed from preset", func(t *testing.T) {
		a := assert.New(t)
		r := NewReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		dbMetric := "testdb" + dbMetricJoinStr + "cpu"
		r.cancelFuncs[dbMetric] = func() { cancelCalled = true }
		r.monitoredSources = sources.SourceConns{
			{Source: sources.Source{Name: "testdb", Metrics: metrics.MetricIntervals{"memory": 10}}},
		}

		r.ShutdownOldWorkers(ctx, map[string]bool{})

		a.True(cancelCalled)
		a.NotContains(r.cancelFuncs, dbMetric)
	})

	t.Run("keeps worker when metric is still active", func(t *testing.T) {
		a := assert.New(t)
		r := NewReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		dbMetric := "testdb" + dbMetricJoinStr + "cpu"
		r.cancelFuncs[dbMetric] = func() { cancelCalled = true }
		r.monitoredSources = sources.SourceConns{
			{Source: sources.Source{Name: "testdb", Metrics: metrics.MetricIntervals{"cpu": 10}}},
		}

		r.ShutdownOldWorkers(ctx, map[string]bool{})

		a.False(cancelCalled)
		a.Contains(r.cancelFuncs, dbMetric)
	})

	t.Run("cancels all workers when context is cancelled", func(t *testing.T) {
		a := assert.New(t)
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		r := NewReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		dbMetric := "testdb" + dbMetricJoinStr + "cpu"
		r.cancelFuncs[dbMetric] = func() { cancelCalled = true }
		r.monitoredSources = sources.SourceConns{
			{Source: sources.Source{Name: "testdb", Metrics: metrics.MetricIntervals{"cpu": 10}}},
		}

		r.ShutdownOldWorkers(cancelledCtx, map[string]bool{})

		a.True(cancelCalled)
	})
}

func TestReaper_CreateSourceHelpers(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("skips already initialized source", func(t *testing.T) {
		r := NewReaper(ctx, &cmdopts.Options{})
		md := &sources.SourceConn{Source: sources.Source{Name: "existing"}}
		r.prevLoopMonitoredDBs = sources.SourceConns{md}
		// Conn is nil — would panic if used, proving early return
		r.CreateSourceHelpers(ctx, r.logger, md)
	})

	t.Run("skips non-postgres source", func(t *testing.T) {
		r := NewReaper(ctx, &cmdopts.Options{})
		md := &sources.SourceConn{Source: sources.Source{Name: "pgbouncer", Kind: sources.SourcePgBouncer}}
		r.CreateSourceHelpers(ctx, r.logger, md)
	})

	t.Run("skips source in recovery", func(t *testing.T) {
		r := NewReaper(ctx, &cmdopts.Options{})
		md := &sources.SourceConn{
			Source:      sources.Source{Name: "standby"},
			RuntimeInfo: sources.RuntimeInfo{IsInRecovery: true},
		}
		r.CreateSourceHelpers(ctx, r.logger, md)
	})

	t.Run("creates extensions when configured", func(t *testing.T) {
		a := assert.New(t)
		r := NewReaper(ctx, &cmdopts.Options{
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
		r := NewReaper(ctx, &cmdopts.Options{
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
	r := NewReaper(ctx, &cmdopts.Options{})
	assert.NotPanics(t, r.PrintMemStats)
}
