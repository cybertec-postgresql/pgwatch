package reaper

import (
	"context"
	"os"
	"path/filepath"
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
		pausefile := filepath.Join(t.TempDir(), "pausefile")
		require.NoError(t, os.WriteFile(pausefile, []byte("foo"), 0644))
		r := NewReaper(ctx, &cmdopts.Options{Metrics: metrics.CmdOpts{EmergencyPauseTriggerfile: pausefile}})
		assert.NoError(t, r.LoadSources(ctx))
		assert.True(t, len(r.monitoredSources) == 0, "Expected no monitored sources when pause trigger file exists")
	})

	t.Run("Test SyncFromReader errror", func(t *testing.T) {
		reader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return nil, assert.AnError
			},
		}
		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		assert.Error(t, r.LoadSources(ctx))
		assert.Equal(t, 0, len(r.monitoredSources), "Expected no monitored sources after error")
	})

	t.Run("Test SyncFromReader success", func(t *testing.T) {
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres}
		reader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{source1, source2}, nil
			},
		}

		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		assert.NoError(t, r.LoadSources(ctx))
		assert.Equal(t, 2, len(r.monitoredSources), "Expected two monitored sources after successful load")
		assert.NotNil(t, r.monitoredSources.GetMonitoredDatabase(source1.Name))
		assert.NotNil(t, r.monitoredSources.GetMonitoredDatabase(source2.Name))
	})

	t.Run("Test repeated load", func(t *testing.T) {
		source1 := sources.Source{Name: "Source 1", IsEnabled: true, Kind: sources.SourcePostgres}
		source2 := sources.Source{Name: "Source 2", IsEnabled: true, Kind: sources.SourcePostgres}
		reader := &testutil.MockSourcesReaderWriter{
			GetSourcesFunc: func() (sources.Sources, error) {
				return sources.Sources{source1, source2}, nil
			},
		}

		r := NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: reader})
		assert.NoError(t, r.LoadSources(ctx))
		assert.Equal(t, 2, len(r.monitoredSources), "Expected two monitored sources after first load")

		// Load again with the same sources
		assert.NoError(t, r.LoadSources(ctx))
		assert.Equal(t, 2, len(r.monitoredSources), "Expected still two monitored sources after second load")
	})

	t.Run("Test group limited sources", func(t *testing.T) {
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
		assert.NoError(t, r.LoadSources(ctx))
		assert.Equal(t, 3, len(r.monitoredSources), "Expected three monitored sources after load")

		r = NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader, Sources: sources.CmdOpts{Groups: []string{"group1"}}})
		assert.NoError(t, r.LoadSources(ctx))
		assert.Equal(t, 2, len(r.monitoredSources), "Expected two monitored source after group filtering")

		r = NewReaper(ctx, &cmdopts.Options{SourcesReaderWriter: newReader})
		assert.NoError(t, r.LoadSources(ctx))
		assert.Equal(t, 5, len(r.monitoredSources), "Expected five monitored sources after resetting groups")
	})

	t.Run("Test source config changes trigger restart", func(t *testing.T) {
		baseSource := sources.Source{
			Name:                 "TestSource",
			IsEnabled:            true,
			Kind:                 sources.SourcePostgres,
			ConnStr:              "postgres://localhost:5432/testdb",
			Metrics:              map[string]float64{"cpu": 10, "memory": 20},
			MetricsStandby:       map[string]float64{"cpu": 30},
			CustomTags:           map[string]string{"env": "test"},
			Group:                "default",
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
					s.Metrics = map[string]float64{"cpu": 15, "memory": 20}
				},
				expectCancel: true,
			},
			{
				name: "custom metrics add new metric",
				modifySource: func(s *sources.Source) {
					s.Metrics = map[string]float64{"cpu": 10, "memory": 20, "disk": 30}
				},
				expectCancel: true,
			},
			{
				name: "custom metrics remove metric",
				modifySource: func(s *sources.Source) {
					s.Metrics = map[string]float64{"cpu": 10}
				},
				expectCancel: true,
			},
			{
				name: "standby metrics change",
				modifySource: func(s *sources.Source) {
					s.MetricsStandby = map[string]float64{"cpu": 60}
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
				assert.NoError(t, r.LoadSources(ctx))
				assert.Equal(t, 1, len(r.monitoredSources), "Expected one monitored source after initial load")

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
				assert.NoError(t, r.LoadSources(ctx))
				assert.Equal(t, 1, len(r.monitoredSources), "Expected one monitored source after reload")
				assert.Equal(t, modifiedSource, r.monitoredSources[0].Source)

				for metric := range initialSource.Metrics {
					dbMetric := initialSource.Name + "¤¤¤" + metric
					assert.Equal(t, tc.expectCancel, cancelCalled[dbMetric])
					if tc.expectCancel {
						assert.Nil(t, mockConn.ExpectationsWereMet(), "Expected all mock expectations to be met")
						_, exists := r.cancelFuncs[dbMetric]
						assert.False(t, exists, "Expected cancel func to be removed from map after cancellation")
					}
				}
			})
		}
	})

	t.Run("Test only changed source cancelled in multi-source setup", func(t *testing.T) {
		source1 := sources.Source{
			Name:      "Source1",
			IsEnabled: true,
			Kind:      sources.SourcePostgres,
			ConnStr:   "postgres://localhost:5432/db1",
			Metrics:   map[string]float64{"cpu": 10},
		}
		source2 := sources.Source{
			Name:      "Source2",
			IsEnabled: true,
			Kind:      sources.SourcePostgres,
			ConnStr:   "postgres://localhost:5432/db2",
			Metrics:   map[string]float64{"memory": 20},
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
		assert.NoError(t, r.LoadSources(ctx))

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

		assert.NoError(t, r.LoadSources(ctx))

		assert.True(t, source1Cancelled, "Source1 should be cancelled due to config change")
		assert.False(t, source2Cancelled, "Source2 should NOT be cancelled as it was not modified")
		assert.Nil(t, mockConn1.ExpectationsWereMet(), "Expected all mock expectations to be met")
	})
}
