package reaper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"strings"
	"fmt"
	"github.com/sirupsen/logrus/hooks/test"
	"errors"

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

func TestReaper_Ready(t *testing.T) {
	ctx := context.Background()
	r := NewReaper(ctx, &cmdopts.Options{})

	assert.False(t, r.Ready())

	r.ready.Store(true)
	assert.True(t, r.Ready())
}
func TestReaper_PrintMemStats(t *testing.T) {
	ctx := log.WithLogger(context.Background(), log.NewNoopLogger())
	r := NewReaper(ctx, &cmdopts.Options{})

	assert.NotPanics(t, func() {
		r.PrintMemStats()
	})
}


// MockSinkWriter simulates the Sinks interface
type MockSinkWriter struct {
	WriteCalled  bool
	LastMsg      metrics.MeasurementEnvelope
	SyncCalled   bool
	DeleteCalled bool
	WriteError   error
}

func (m *MockSinkWriter) Write(msg metrics.MeasurementEnvelope) error {
	m.WriteCalled = true
	m.LastMsg = msg
	return m.WriteError
}

func (m *MockSinkWriter) SyncMetric(_, _ string, _ sinks.SyncOp) error {
	m.SyncCalled = true
	return nil
}

func TestWriteMeasurements(t *testing.T) {
    tests := []struct {
        name       string
        writeError error
    }{
        {
            name: "Happy Path - Successful Write",
            writeError: nil,
        },
        {
            name: "Error Path - Write Fails",
            writeError: errors.New("something went wrong"),
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockSink := &MockSinkWriter{
                WriteError: tt.writeError,
            }
            opts := &cmdopts.Options{}
            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            r := NewReaper(ctx, opts)
            r.SinksWriter = mockSink

            go r.WriteMeasurements(ctx)

            dummyMsg := metrics.MeasurementEnvelope{
                DBName:     "test_db",
                MetricName: "test_metric",
                Data:       metrics.Measurements{{"value": 1}},
            }
            r.measurementCh <- dummyMsg

            // Allow brief time for channel processing
            time.Sleep(50 * time.Millisecond)

            assert.True(t, mockSink.WriteCalled, "Sink Write should have been called")
            assert.Equal(t, "test_db", mockSink.LastMsg.DBName)
            assert.Equal(t, "test_metric", mockSink.LastMsg.MetricName)
            
            // Clean up
            cancel() 
        })
    }
}

func TestWriteMonitoredSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := NewReaper(ctx, &cmdopts.Options{})

	// Add a fake monitored source
	r.monitoredSources = append(r.monitoredSources, &sources.SourceConn{
		Source: sources.Source{
			Name:         "test_db_source",
			Group:        "test_group",
			OnlyIfMaster: true,
			CustomTags: map[string]string{"env": "prod"},
		},
	})

	go r.WriteMonitoredSources(ctx)

	// Listen to the channel to verify output
	select {
	case msg := <-r.measurementCh:
		assert.Equal(t, "test_db_source", msg.DBName)
		assert.Equal(t, monitoredDbsDatastoreSyncMetricName, msg.MetricName)

		assert.NotEmpty(t, msg.Data)
		row := msg.Data[0]

		assert.Equal(t, "test_group", row["tag_group"])
		assert.Equal(t, true, row["master_only"])
		assert.Equal(t, "prod", row["tag_env"])

	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for WriteMonitoredSources to produce data")
	}
}

func TestAddSysinfoToMeasurements(t *testing.T) {
	opts := &cmdopts.Options{}
	opts.Sinks.RealDbnameField = "real_dbname"
	opts.Sinks.SystemIdentifierField = "sys_id"

	r := NewReaper(context.Background(), opts)

	md := &sources.SourceConn{}
	md.RealDbname = "postgres_prod"
	md.SystemIdentifier = "123456789"

	data := metrics.Measurements{
		{"value": 10},
		{"value": 20},
	}

	r.AddSysinfoToMeasurements(data, md)

	for _, row := range data {
		assert.Equal(t, "postgres_prod", row["real_dbname"])
		assert.Equal(t, "123456789", row["sys_id"])
	}

	assert.Equal(t, 10, data[0]["value"])
	assert.Equal(t, 20, data[1]["value"])
}

func TestFetchMetric_CacheHit(t *testing.T) {
	ctx := context.Background()
	opts := &cmdopts.Options{}
	opts.Metrics.InstanceLevelCacheMaxSeconds = 60

	r := NewReaper(ctx, opts)
	metricName := "cached_metric"

	md := &sources.SourceConn{
		Source: sources.Source{
			Name: "db1",
		},
	}
	md.SystemIdentifier = "sys_id_123"
	md.Metrics = map[string]float64{metricName: 10}

	// Setup Metric Definition
	metricDefs.Lock()
	metricDefs.MetricDefs[metricName] = metrics.Metric{
		SQLs: map[int]string{0: "SELECT 1"},
		IsInstanceLevel: true,
	}
	metricDefs.Unlock()

	// Pre-populate Cache
	cacheKey := fmt.Sprintf("%s:%s", md.GetClusterIdentifier(), metricName)
	cachedData := metrics.Measurements{{"cached_val": 999}}

	r.measurementCache.Put(cacheKey, cachedData)

	envelope, err := r.FetchMetric(ctx, md, metricName)

	assert.NoError(t, err)

	if envelope == nil {
		t.Fatal("Cache Miss! FetchMetric returned nil envelope. Check GetClusterIdentifier() logic.")
	}

	assert.Equal(t, metricName, envelope.MetricName)
	assert.Equal(t, 999, envelope.Data[0]["cached_val"])
}

func TestFetchMetric_NotFound(t *testing.T) {
	ctx := context.Background()
	r := NewReaper(ctx, &cmdopts.Options{})
	md := &sources.SourceConn{Source: sources.Source{Name: "db1"}}

	// Execute with non-existent metric
	envelope, err := r.FetchMetric(ctx, md, "ghost_metric")

	assert.Error(t, err)
	assert.Equal(t, metrics.ErrMetricNotFound, err)
	assert.Nil(t, envelope)
}

func TestFetchMetric_EmptySQL_Ignored(t *testing.T) {
	ctx := context.Background()
	r := NewReaper(ctx, &cmdopts.Options{})
	md := &sources.SourceConn{Source: sources.Source{Name: "db1"}}

	metricName := "empty_sql_metric"

	metricDefs.Lock()
	metricDefs.MetricDefs[metricName] = metrics.Metric{
		SQLs: map[int]string{},
	}
	metricDefs.Unlock()

	envelope, err := r.FetchMetric(ctx, md, metricName)

	assert.NoError(t, err)
	assert.Nil(t, envelope)
}

func TestFetchMetric_SwitchCases_PgxMock(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		metricName   string
		setupMetric  metrics.Metric
		isInRecovery bool
		mockDB       func(mock pgxmock.PgxPoolIface)
		expectNilEnv bool
		expectErr    bool
	}{
		{
			name: "Case: Skip PrimaryOnly metric on Standby",
			metricName: "test_primary_metric",
			setupMetric: metrics.Metric{
				NodeStatus: "primary",
			},
			isInRecovery: true,
			mockDB: func(_ pgxmock.PgxPoolIface) {
				// We expect no queries because the function should exit early
			},
			expectNilEnv: true, // Should return nil, nil
			expectErr:    false,
		},
		{
			name: "Case: Skip StandbyOnly metric on Primary",
			metricName: "test_standby_metric",
			setupMetric: metrics.Metric{
				NodeStatus: "standby",
			},
			isInRecovery: false,
			mockDB: func(_ pgxmock.PgxPoolIface) {
				// We expect no queries because the function should exit early
			},
			expectNilEnv: true,
			expectErr:    false,
		},
		{
			name: "Case: specialMetricInstanceUp",
			metricName: specialMetricInstanceUp,
			setupMetric: metrics.Metric{
				IsInstanceLevel: false,
			},
			mockDB: func(mock pgxmock.PgxPoolIface) {
				// The function executes a Ping to check if the DB is up
				mock.ExpectPing() 
			},
			expectNilEnv: false,
			expectErr:    false,
		},
{
			name: "Case: specialMetricChangeEvents",
			metricName: specialMetricChangeEvents,
			setupMetric: metrics.Metric{
				IsInstanceLevel: false,
			},
			mockDB: func(mock pgxmock.PgxPoolIface) {
				// Inject dummy metric definitions so the Detect functions have SQL to run
				metricDefs.Lock()
				metricDefs.MetricDefs["sproc_hashes"] = metrics.Metric{SQLs: map[int]string{0: "SELECT dummy_sproc"}}
				metricDefs.MetricDefs["table_hashes"] = metrics.Metric{SQLs: map[int]string{0: "SELECT dummy_table"}}
				metricDefs.MetricDefs["index_hashes"] = metrics.Metric{SQLs: map[int]string{0: "SELECT dummy_index"}}
				metricDefs.MetricDefs["configuration_hashes"] = metrics.Metric{SQLs: map[int]string{0: "SELECT dummy_config"}}
				metricDefs.MetricDefs["privilege_changes"] = metrics.Metric{SQLs: map[int]string{0: "SELECT dummy_priv"}}
				metricDefs.Unlock()

				// Expect all 5 queries in exact order. 
				mock.ExpectQuery("SELECT dummy_sproc").
					WillReturnRows(pgxmock.NewRows([]string{"tag_sproc", "tag_oid", "md5"}))

				mock.ExpectQuery("SELECT dummy_table").
					WillReturnRows(pgxmock.NewRows([]string{"tag_table", "md5"}))

				mock.ExpectQuery("SELECT dummy_index").
					WillReturnRows(pgxmock.NewRows([]string{"tag_index", "table", "md5", "is_valid"}))

				mock.ExpectQuery("SELECT dummy_config").
					WillReturnRows(pgxmock.NewRows([]string{"epoch", "objIdent", "objValue"}))

				mock.ExpectQuery("SELECT dummy_priv").
					WillReturnRows(pgxmock.NewRows([]string{"object_type", "tag_role", "tag_object", "privilege_type"}))
			},
			expectNilEnv: true, 
			expectErr:    false,
		},
		{
			name: "Case: Default with Valid SQL",
			metricName: "default_valid_sql",
			setupMetric: metrics.Metric{
				SQLs: map[int]string{0: "SELECT 1 AS val"},
				IsInstanceLevel: false,
			},
			mockDB: func(mock pgxmock.PgxPoolIface) {
				// Mocking the QueryMeasurements function
				columns := []string{"val"}
				mock.ExpectQuery("SELECT 1 AS val").
					WillReturnRows(pgxmock.NewRows(columns).AddRow(99))
			},
			expectNilEnv: false,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			assert.NoError(t, err)
			defer mock.Close()

			md := &sources.SourceConn{
				Source: sources.Source{
					Name: "db_test",
				},
				Conn: mock,
			}
			md.SystemIdentifier = "sys_id_test"
			md.Metrics = map[string]float64{tt.metricName: 10}
			md.ChangeState = make(map[string]map[string]string)
			md.IsInRecovery = tt.isInRecovery
			
			if tt.mockDB != nil {
				tt.mockDB(mock)
			}

			// Setup Reaper and Metrics
			opts := &cmdopts.Options{}
			r := NewReaper(ctx, opts)

			metricDefs.Lock()
			if metricDefs.MetricDefs == nil {
				metricDefs.MetricDefs = make(map[string]metrics.Metric)
			}
			metricDefs.MetricDefs[tt.metricName] = tt.setupMetric
			metricDefs.Unlock()

			envelope, err := r.FetchMetric(ctx, md, tt.metricName)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNilEnv {
				assert.Nil(t, envelope)
			} else {
				assert.NotNil(t, envelope)
				assert.Equal(t, tt.metricName, envelope.MetricName)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCreateSourceHelpers(t *testing.T) {
	logger, hook := test.NewNullLogger()
	ctx := context.Background()

	tests := []struct {
		name              string
		sourceName        string
		sourceType        sources.Kind
		inRecovery        bool
		inPrevLoop        bool
		createHelpers     bool
		tryCreateExts     string

		expectedLogMsgs   []string
		unexpectedLogMsgs []string
	}{
		{
			name:          "Skip if Non-Postgres Source",
			sourceName:    "PgBouncer",
			sourceType:    sources.SourcePgBouncer,
			inRecovery:    false,
			createHelpers: true,
			unexpectedLogMsgs: []string{"trying to create helper objects"},
		},
		{
			name:          "Skip if In Recovery",
			sourceName:    "standby_db",
			sourceType:    sources.SourcePostgres,
			inRecovery:    true,
			createHelpers: true,
			unexpectedLogMsgs: []string{"trying to create helper objects"},
		},
		{
			name:          "Skip if Already Created",
			sourceName:    "existing_db",
			sourceType:    sources.SourcePostgres,
			inRecovery:    false,
			inPrevLoop:    true,
			createHelpers: true,
			tryCreateExts: "plpythonu",
			unexpectedLogMsgs: []string{
				"trying to create helper objects",
				"trying to create extensions",
			},
		},
		{
			name:          "Happy Path: Create Helpers",
			sourceName:    "fresh_primary",
			sourceType:    sources.SourcePostgres,
			inRecovery:    false,
			inPrevLoop:    false,
			createHelpers: true,
			expectedLogMsgs: []string{"trying to create helper objects if missing"},
		},
		{
			name:          "Happy Path: Create Extensions",
			sourceName:    "fresh_primary_ext",
			sourceType:    sources.SourcePostgres,
			inRecovery:    false,
			inPrevLoop:    false,
			tryCreateExts: "pg_stat_statements",
			expectedLogMsgs: []string{"trying to create extensions if missing"},
		},
		{
			name:          "Happy Path: Create Both",
			sourceName:    "fresh_primary_full",
			sourceType:    sources.SourcePostgres,
			inRecovery:    false,
			inPrevLoop:    false,
			createHelpers: true,
			tryCreateExts: "pg_stat_statements",
			expectedLogMsgs: []string{
				"trying to create helper objects if missing",
				"trying to create extensions if missing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook.Reset()

			opts := &cmdopts.Options{}
			r := &Reaper{
				Options:              opts,
				logger:               logger,
				prevLoopMonitoredDBs: make(sources.SourceConns, 0),
			}

			r.Sources.CreateHelpers = tt.createHelpers
			r.Sources.TryCreateListedExtsIfMissing = tt.tryCreateExts

			src := &sources.SourceConn{
				Source: sources.Source{
					Name: tt.sourceName,
					Kind: tt.sourceType,
				},
			}
			src.IsInRecovery = tt.inRecovery

			if tt.inPrevLoop {
				r.prevLoopMonitoredDBs = append(r.prevLoopMonitoredDBs, src)
			}

            mock, err := pgxmock.NewPool()
            if err != nil {
                t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
            }
            defer mock.Close()

            src.Conn = mock 

            // Setup Expectations for "Happy Path"
            if strings.Contains(tt.name, "Happy Path") {
                // Determine what to expect based on the test case
                if strings.Contains(tt.name, "Extensions") || strings.Contains(tt.name, "Both") {
					rows := pgxmock.NewRows([]string{"name"}).AddRow("pg_stat_statements").AddRow("plpythonu")
                    mock.ExpectQuery("select name::text from pg_available_extensions").WillReturnRows(rows)

					mock.ExpectQuery("create extension .*").WillReturnRows(pgxmock.NewRows([]string{}))}
            }

			srcL := logger.WithField("source", src.Name)

			assert.NotPanics(t, func() {
				r.CreateSourceHelpers(ctx, srcL, src)
			})

			entries := hook.AllEntries()

			// Check Expected Messages
			for _, exp := range tt.expectedLogMsgs {
				found := false
				for _, e := range entries {
					if strings.Contains(e.Message, exp) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected log message not found: %s", exp)
			}

			// Check Unexpected Messages
			for _, unexp := range tt.unexpectedLogMsgs {
				found := false
				for _, e := range entries {
					if strings.Contains(e.Message, unexp) {
						found = true
						break
					}
				}
				assert.False(t, found, "Found unexpected log message: %s", unexp)
			}
		})
	}
}
