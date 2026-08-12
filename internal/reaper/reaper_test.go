package reaper

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
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

func TestReaper_FilterSource(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	newMd := func(kind sources.Kind, isInRecovery, onlyIfMaster bool, approxDbSizeBytes int64) *sources.DbConn {
		md := sources.NewDbConn(sources.Source{Name: "testdb", Kind: kind, OnlyIfMaster: onlyIfMaster})
		md.IsInRecovery = isInRecovery
		md.ApproxDbSize = approxDbSizeBytes
		return md
	}

	t.Run("primary with onlyIfMaster: not filtered", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		r.cancelFuncs["testdb"] = func() {}

		a.False(r.FilterSource(ctx, newMd(sources.SourcePostgres, false, true, 0)))
		_, exists := r.cancelFuncs["testdb"]
		a.True(exists, "worker should not be shut down for primary")
	})

	t.Run("standby without onlyIfMaster: not filtered", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})

		a.False(r.FilterSource(ctx, newMd(sources.SourcePostgres, true, false, 0)))
	})

	t.Run("standby with onlyIfMaster, postgres: worker shut down", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }

		a.True(r.FilterSource(ctx, newMd(sources.SourcePostgres, true, true, 0)))
		a.True(cancelCalled)
		_, exists := r.cancelFuncs["testdb"]
		a.False(exists)
	})

	t.Run("below size threshold: worker shut down", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{
			Sources:     sources.CmdOpts{MinDbSizeMB: 500},
			SinksWriter: &sinks.MultiWriter{},
		})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }
		md := newMd(sources.SourcePostgres, false, false, 100*1048576) // 100 MB
		r.monitoredSources = sources.SourceConns{md}

		a.True(r.FilterSource(ctx, md))
		a.True(cancelCalled)
		_, exists := r.cancelFuncs["testdb"]
		a.False(exists)
	})

	t.Run("above size threshold: not filtered", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{Sources: sources.CmdOpts{MinDbSizeMB: 500}})

		a.False(r.FilterSource(ctx, newMd(sources.SourcePostgres, false, false, 600*1048576)))
	})

	t.Run("equal to size threshold: not filtered", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{Sources: sources.CmdOpts{MinDbSizeMB: 100}})

		a.False(r.FilterSource(ctx, newMd(sources.SourcePostgres, false, false, 100*1048576)))
	})

	t.Run("zero ApproxDbSize bypasses size check", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{Sources: sources.CmdOpts{MinDbSizeMB: 500}})

		a.False(r.FilterSource(ctx, newMd(sources.SourcePostgres, false, false, 0)))
	})

	t.Run("no min size configured: never size-filtered", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})

		a.False(r.FilterSource(ctx, newMd(sources.SourcePostgres, false, false, 1*1048576)))
	})
}

func TestReaper_TrackRecoveryStatus(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	newPgConn := func(kind sources.Kind, isInRecovery bool, standby metrics.MetricIntervals) *sources.DbConn {
		md := sources.NewDbConn(sources.Source{Name: "testdb", Kind: kind})
		md.IsInRecovery = isInRecovery
		md.MetricsStandby = standby
		return md
	}

	t.Run("no role change: cache updated silently", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		r.srcRecoveryStatus["testdb"] = false
		md := newPgConn(sources.SourcePostgres, false, nil)

		r.TrackRecoveryStatus(ctx, md)

		a.False(r.srcRecoveryStatus["testdb"])
	})

	t.Run("primary→standby with standby config: cache updated", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		r.srcRecoveryStatus["testdb"] = false
		md := newPgConn(sources.SourcePostgres, true, metrics.MetricIntervals{"cpu": 10})

		r.TrackRecoveryStatus(ctx, md)

		a.True(r.srcRecoveryStatus["testdb"])
	})

	t.Run("standby→primary: cache updated", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		r.srcRecoveryStatus["testdb"] = true
		md := newPgConn(sources.SourcePostgres, false, nil)

		r.TrackRecoveryStatus(ctx, md)

		a.False(r.srcRecoveryStatus["testdb"])
	})

	t.Run("primary→standby without standby config: cache updated, no shutdown", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		r.srcRecoveryStatus["testdb"] = false
		md := newPgConn(sources.SourcePostgres, true, nil)

		r.TrackRecoveryStatus(ctx, md)

		a.True(r.srcRecoveryStatus["testdb"])
	})

	t.Run("pgbouncer: cache updated", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		md := newPgConn(sources.SourcePgBouncer, false, nil)

		r.TrackRecoveryStatus(ctx, md)

		a.False(r.srcRecoveryStatus["testdb"])
	})

	t.Run("patroni discovery: cache updated", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		md := newPgConn(sources.SourcePatroniDiscovery, true, nil)

		r.TrackRecoveryStatus(ctx, md)

		a.True(r.srcRecoveryStatus["testdb"])
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

	newMd := func(config metrics.MetricIntervals) *sources.DbConn {
		md := sources.NewDbConn(sources.Source{Name: "mydb"})
		md.Metrics = config
		return md
	}

	t.Run("syncs known metrics using metric name", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		r.SyncMetricsToSinks(ctx, newMd(metrics.MetricIntervals{"cpu": 10}))

		require.Len(t, sw.synced, 1)
		a.Equal("mydb", sw.synced[0].source)
		a.Equal("cpu", sw.synced[0].metric)
	})

	t.Run("uses StorageName when set and metric is not special", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		r.SyncMetricsToSinks(ctx, newMd(metrics.MetricIntervals{"memory": 30}))

		require.Len(t, sw.synced, 1)
		a.Equal("mem_storage", sw.synced[0].metric)
	})

	t.Run("skips unknown metric definitions", func(t *testing.T) {
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		r.SyncMetricsToSinks(ctx, newMd(metrics.MetricIntervals{"unknown_metric": 60}))

		assert.Empty(t, sw.synced)
	})

	t.Run("logs sink error but continues", func(t *testing.T) {
		sw := &mockSyncWriter{err: errors.New("sink error")}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		assert.NotPanics(t, func() {
			r.SyncMetricsToSinks(ctx, newMd(metrics.MetricIntervals{"cpu": 10}))
		})
	})

	t.Run("empty config results in no syncs", func(t *testing.T) {
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		r.SyncMetricsToSinks(ctx, newMd(metrics.MetricIntervals{}))

		assert.Empty(t, sw.synced)
	})

	t.Run("standby config used when in recovery", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})
		md := sources.NewDbConn(sources.Source{Name: "mydb"})
		md.Metrics = metrics.MetricIntervals{"cpu": 10}
		md.MetricsStandby = metrics.MetricIntervals{"memory": 20}
		md.IsInRecovery = true

		r.SyncMetricsToSinks(ctx, md)

		require.Len(t, sw.synced, 1)
		a.Equal("mem_storage", sw.synced[0].metric) // standby config used
	})

	t.Run("multiple metrics all synced", func(t *testing.T) {
		a := assert.New(t)
		sw := &mockSyncWriter{}
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: sw})

		r.SyncMetricsToSinks(ctx, newMd(metrics.MetricIntervals{"cpu": 10, "memory": 20}))

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

func TestReaper_StartWorker(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	// minimal Reaper implementation that records whether Reap was called
	type fakeReaper struct{}
	newFake := func() *fakeReaper { return &fakeReaper{} }

	t.Run("starts worker and registers cancel func", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		reaped := make(chan struct{}, 1)
		fake := reaperFunc(func(context.Context) { reaped <- struct{}{} })

		r.StartWorker(ctx, "testdb", fake)

		_, exists := r.cancelFuncs["testdb"]
		a.True(exists, "cancel func should be registered")
		<-reaped      // blocks until Reap is called
		_ = newFake() // suppress unused warning
	})

	t.Run("no-op when worker already running", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{})
		callCount := 0
		fake := reaperFunc(func(context.Context) { callCount++ })
		r.cancelFuncs["testdb"] = func() {}

		r.StartWorker(ctx, "testdb", fake)

		a.Equal(0, callCount, "Reap should not be called when worker already exists")
	})
}

// reaperFunc is a function that implements the Reaper interface.
type reaperFunc func(ctx context.Context)

func (f reaperFunc) Reap(ctx context.Context) { f(ctx) }

func TestReaper_ShutdownWorker(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("cancels and removes the named worker", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }

		r.ShutdownWorker(ctx, "testdb")

		a.True(cancelCalled)
		a.NotContains(r.cancelFuncs, "testdb")
	})

	t.Run("no-op when source has no running worker", func(*testing.T) {
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		// must not panic
		r.ShutdownWorker(ctx, "nonexistent")
	})
}

func TestReaper_CleanupRemovedWorkers(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())

	t.Run("cancels worker for DB removed from config", func(t *testing.T) {
		a := assert.New(t)
		r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})
		cancelCalled := false
		r.cancelFuncs["testdb"] = func() { cancelCalled = true }
		// monitoredSources is empty — testdb was removed

		r.CleanupRemovedWorkers(ctx)

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

		r.CleanupRemovedWorkers(ctx)

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

		r.CleanupRemovedWorkers(cancelledCtx)

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
		r.CreateSourceHelpers(ctx, md)
	})

	t.Run("skips non-postgres source", func(*testing.T) {
		r := newReaper(ctx, &cmdopts.Options{})
		md := &sources.DbConn{Source: sources.Source{Name: "pgbouncer", Kind: sources.SourcePgBouncer}}
		r.CreateSourceHelpers(ctx, md)
	})

	t.Run("skips source in recovery", func(*testing.T) {
		r := newReaper(ctx, &cmdopts.Options{})
		md := &sources.DbConn{
			Source:      sources.Source{Name: "standby"},
			RuntimeInfo: sources.RuntimeInfo{IsInRecovery: true},
		}
		r.CreateSourceHelpers(ctx, md)
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

		r.CreateSourceHelpers(ctx, md)
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

		r.CreateSourceHelpers(ctx, md)
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
			r.CreateSourceHelpers(ctx, md)
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

// fakeSourceConn is a SourceConn with a programmable Connect. It is neither
// *sources.DbConn nor *sources.PromConn, so after a successful Connect the
// sweep does nothing further for it — Connect behavior is the only observable.
type fakeSourceConn struct {
	src     sources.Source
	connect func(ctx context.Context) error
}

func (f *fakeSourceConn) Connect(ctx context.Context, _ sources.CmdOpts) error {
	return f.connect(ctx)
}
func (f *fakeSourceConn) Ping(context.Context) error                      { return nil }
func (f *fakeSourceConn) IsPostgresSource() bool                          { return false }
func (f *fakeSourceConn) GetSource() sources.Source                       { return f.src }
func (f *fakeSourceConn) GetMetricInterval(string) time.Duration          { return 0 }
func (f *fakeSourceConn) SetMetricIntervals(_, _ metrics.MetricIntervals) {}
func (f *fakeSourceConn) Close()                                          {}

// captureWriter is a sinks.Writer that records every Write envelope.
type captureWriter struct {
	mu   sync.Mutex
	envs []metrics.MeasurementEnvelope
	ch   chan metrics.MeasurementEnvelope
}

func newCaptureWriter(size int) *captureWriter {
	return &captureWriter{ch: make(chan metrics.MeasurementEnvelope, size)}
}

func (w *captureWriter) SyncMetric(string, string, sinks.SyncOp) error { return nil }

func (w *captureWriter) Write(env metrics.MeasurementEnvelope) error {
	w.mu.Lock()
	w.envs = append(w.envs, env)
	w.mu.Unlock()
	w.ch <- env
	return nil
}

// count returns how many envelopes were written for the named source.
func (w *captureWriter) count(name string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, e := range w.envs {
		if e.DBName == name {
			n++
		}
	}
	return n
}

// waitEnvelope waits for an envelope for the named source or fails the test
// after the timeout.
func (w *captureWriter) waitEnvelope(t *testing.T, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case env := <-w.ch:
			if env.DBName == name {
				return
			}
		case <-deadline:
			t.Fatalf("timed out after %s waiting for an envelope for source %q", timeout, name)
		}
	}
}

// newSweepReaper builds a reaper whose source and metric readers fail, so the
// main loop keeps the seeded monitoredSources untouched. The refresh interval
// is huge, so exactly one sweep runs per Reap invocation.
func newSweepReaper(ctx context.Context, sink sinks.Writer, srcs ...sources.SourceConn) *reaper {
	r := newReaper(ctx, &cmdopts.Options{
		Sources: sources.CmdOpts{Refresh: 3600},
		SourcesReaderWriter: &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) { return nil, errors.New("no sources") },
		},
		MetricsReaderWriter: &testutil.MockMetricsReaderWriter{
			GetMetricsFunc: func() (*metrics.Metrics, error) { return nil, errors.New("no metrics") },
		},
		SinksWriter: sink,
	})
	r.monitoredSources = sources.SourceConns(srcs)
	return r
}

// TestReaper_SweepSourceIsolation verifies that one stalled source cannot
// delay the processing of the sources behind it in the list.
func TestReaper_SweepSourceIsolation(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	sink := newCaptureWriter(16)
	fail := func(context.Context) error { return errors.New("connect failed") }
	stall := func(ctx context.Context) error {
		select {
		case <-time.After(2 * time.Second):
			return errors.New("connect failed")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r := newSweepReaper(ctx, sink,
		&fakeSourceConn{src: sources.Source{Name: "s1"}, connect: fail},
		&fakeSourceConn{src: sources.Source{Name: "s2"}, connect: stall},
		&fakeSourceConn{src: sources.Source{Name: "s3"}, connect: fail},
	)
	reapCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()
	go r.Reap(reapCtx)

	// s3 sits behind the stalled s2 in the source list. A sequential sweep
	// cannot report s3 before s2's 2s stall elapses; allow 1/4 of the stall.
	sink.waitEnvelope(t, "s3", 500*time.Millisecond)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	cancel()
}

// TestReaper_SweepBoundedParallelism verifies that sources are processed
// concurrently but never more than maxConcurrentSourceConnects at a time.
func TestReaper_SweepBoundedParallelism(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	const total = 64
	const sleep = 50 * time.Millisecond
	sink := newCaptureWriter(total)

	var inFlight, maxSeen atomic.Int64
	srcs := make([]sources.SourceConn, 0, total)
	for i := range total {
		srcs = append(srcs, &fakeSourceConn{
			src: sources.Source{Name: fmt.Sprintf("db%02d", i)},
			connect: func(context.Context) error {
				cur := inFlight.Add(1)
				defer inFlight.Add(-1)
				for {
					if m := maxSeen.Load(); cur <= m || maxSeen.CompareAndSwap(m, cur) {
						break
					}
				}
				time.Sleep(sleep)
				return errors.New("connect failed")
			},
		})
	}
	r := newSweepReaper(ctx, sink, srcs...)
	reapCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()
	go r.Reap(reapCtx)

	// Every processed source reports exactly one instance_up=0 envelope.
	seen := make(map[string]struct{}, total)
	deadline := time.After(10 * time.Second)
	var elapsed time.Duration
	for len(seen) < total {
		select {
		case env := <-sink.ch:
			seen[env.DBName] = struct{}{}
			elapsed = time.Since(start)
		case <-deadline:
			t.Fatalf("only %d of %d sources processed", len(seen), total)
		}
	}
	cancel()

	assert.Len(t, seen, total, "every source must be processed")
	assert.Greater(t, maxSeen.Load(), int64(1), "sources must be processed concurrently")
	assert.LessOrEqual(t, maxSeen.Load(), int64(maxConcurrentSourceConnects), "concurrency must be bounded")
	// A sequential sweep needs at least total*sleep (3.2s); allow half of that.
	assert.Less(t, elapsed, total*sleep/2, "sweep must be faster than sequential")
}

// TestReaper_WorkerChurn exercises StartWorker/ShutdownWorker under
// concurrency: exactly one worker per name, reusable names, no deadlocks.
func TestReaper_WorkerChurn(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})

	const contenders = 16
	var started atomic.Int64
	blocking := func() Reaper {
		return reaperFunc(func(ctx context.Context) {
			started.Add(1)
			<-ctx.Done()
		})
	}

	// Concurrent StartWorker calls for the same name start exactly one worker.
	var wg sync.WaitGroup
	for range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.StartWorker(ctx, "hot", blocking())
		}()
	}
	wg.Wait()
	require.Eventually(t, func() bool { return started.Load() == 1 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, int64(1), started.Load(), "exactly one worker must run for a name")

	// After ShutdownWorker the name is free and a new worker starts.
	r.ShutdownWorker(ctx, "hot")
	r.StartWorker(ctx, "hot", blocking())
	require.Eventually(t, func() bool { return started.Load() == 2 }, 5*time.Second, time.Millisecond)
	r.ShutdownWorker(ctx, "hot")

	// Churn: overlapping Start/Shutdown on several names must not deadlock.
	names := []string{"a", "b", "c", "d"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var churn sync.WaitGroup
		for i := range 64 {
			churn.Add(1)
			go func() {
				defer churn.Done()
				name := names[i%len(names)]
				r.StartWorker(ctx, name, blocking())
				r.ShutdownWorker(ctx, name)
			}()
		}
		churn.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker churn deadlocked")
	}
	for _, name := range names {
		r.ShutdownWorker(ctx, name)
	}
	assert.Empty(t, r.cancelFuncs)
}

// TestReaper_InstanceUpWrittenOncePerSweep verifies that a failing source
// produces exactly one instance_up=0 envelope per sweep.
func TestReaper_InstanceUpWrittenOncePerSweep(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	sink := newCaptureWriter(16)
	r := newSweepReaper(ctx, sink,
		&fakeSourceConn{src: sources.Source{Name: "db1"},
			connect: func(context.Context) error { return errors.New("down") }},
	)
	reapCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go r.Reap(reapCtx)

	sink.waitEnvelope(t, "db1", 5*time.Second)
	// Drain for a bounded window; no duplicate failure envelope may appear.
	time.Sleep(200 * time.Millisecond)
	cancel()
	assert.Equal(t, 1, sink.count("db1"), "instance_up=0 must be written exactly once per sweep")
}

// TestReaper_StartWorkerDuplicateIsNoOp verifies that a second StartWorker
// call for an existing name leaves the running worker untouched.
func TestReaper_StartWorkerDuplicateIsNoOp(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	r := newReaper(ctx, &cmdopts.Options{SinksWriter: &sinks.MultiWriter{}})

	firstStarted := make(chan struct{})
	firstStopped := make(chan struct{})
	first := reaperFunc(func(ctx context.Context) {
		close(firstStarted)
		<-ctx.Done()
		close(firstStopped)
	})
	var secondRuns atomic.Int64
	second := reaperFunc(func(context.Context) { secondRuns.Add(1) })

	r.StartWorker(ctx, "db", first)
	<-firstStarted
	r.StartWorker(ctx, "db", second)

	assert.Equal(t, int64(0), secondRuns.Load(), "second StartWorker must never start its worker")
	select {
	case <-firstStopped:
		t.Fatal("first worker must keep running after a duplicate StartWorker")
	default:
	}

	r.ShutdownWorker(ctx, "db")
	<-firstStopped
	_, exists := r.cancelFuncs["db"]
	assert.False(t, exists, "worker must be deregistered after shutdown")
}

// TestReaper_SweepGoroutineHygiene simulates a brownout in which every source
// wedges inside Connect, then verifies that cancelling the Reap context lets
// the process return to its baseline goroutine count: the sweep fan-out is
// joined via g.Wait() and WriteMeasurements exits on ctx.Done(), so no sweep
// or writer goroutine may outlive Reap.
func TestReaper_SweepGoroutineHygiene(t *testing.T) {
	ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
	sink := newCaptureWriter(16)

	// 8 wedged sources stay well under maxConcurrentSourceConnects, so the
	// whole fan-out is inside Connect at the same time.
	const numSources = 8
	var inside atomic.Int64
	allInside := make(chan struct{})
	wedge := func(ctx context.Context) error {
		if inside.Add(1) == numSources {
			close(allInside)
		}
		<-ctx.Done()
		return ctx.Err()
	}
	srcs := make([]sources.SourceConn, 0, numSources)
	for i := range numSources {
		srcs = append(srcs, &fakeSourceConn{
			src:     sources.Source{Name: fmt.Sprintf("wedge%02d", i)},
			connect: wedge,
		})
	}
	r := newSweepReaper(ctx, sink, srcs...)

	// Baseline is taken after every fixture exists but before Reap starts its
	// writer and sweep goroutines.
	baseline := runtime.NumGoroutine()

	reapCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		r.Reap(reapCtx)
		close(done)
	}()

	// Brownout: wait until every source is wedged inside Connect.
	select {
	case <-allInside:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for all sources to enter Connect")
	}

	// Brownout clears: the wedged Connect calls must observe the cancellation
	// and Reap must unwind.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Reap did not return after context cancellation")
	}

	// Goroutine exit is observed asynchronously by the runtime, so poll until
	// the count settles. Tolerance of 2 absorbs runtime background goroutines
	// (timers, GC workers) that may start independently of the reaper; a real
	// leak of the sweep fan-out would be one goroutine per wedged source (8
	// here) plus the writer, far above the tolerance.
	const tolerance = 2
	deadline := time.Now().Add(5 * time.Second)
	for {
		if n := runtime.NumGoroutine(); n <= baseline+tolerance {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("goroutine leak: baseline %d, still %d goroutines after Reap returned", baseline, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
