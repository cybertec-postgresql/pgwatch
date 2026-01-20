package cmdopts

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigInitCommand_Execute(t *testing.T) {
	a := assert.New(t)
	t.Run("subcommand is missing", func(*testing.T) {
		os.Args = []string{0: "config_test", "config"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("subcommand is invalid", func(*testing.T) {
		os.Args = []string{0: "config_test", "config", "invalid"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("sources and metrics are empty", func(*testing.T) {
		os.Args = []string{0: "config_test", "config", "init"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("metrics is a proper file name", func(*testing.T) {
		fname := t.TempDir() + "/metrics.yaml"
		os.Args = []string{0: "config_test", "--metrics=" + fname, "config", "init"}
		_, err := New(io.Discard)
		a.NoError(err)
		a.FileExists(fname)
		fi, err := os.Stat(fname)
		require.NoError(t, err)
		a.True(fi.Size() > 0)
	})

	t.Run("sources is a proper file name", func(*testing.T) {
		fname := t.TempDir() + "/sources.yaml"
		os.Args = []string{0: "config_test", "--sources=" + fname, "config", "init"}
		_, err := New(io.Discard)
		a.NoError(err)
		a.FileExists(fname)
		fi, err := os.Stat(fname)
		require.NoError(t, err)
		a.True(fi.Size() > 0)
	})

	t.Run("metrics is an invalid file name", func(*testing.T) {
		os.Args = []string{0: "config_test", "--metrics=/", "config", "init"}
		opts, err := New(io.Discard)
		a.Error(err)
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

	t.Run("metrics is proper postgres connectin string", func(*testing.T) {
		os.Args = []string{0: "config_test", "--metrics=postgresql://foo@bar/baz", "config", "init"}
		opts, err := New(io.Discard)
		a.Error(err)
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

}

func TestConfigUpgradeCommand_Execute(t *testing.T) {
	a := assert.New(t)

	t.Run("sources and metrics are empty", func(*testing.T) {
		os.Args = []string{0: "config_test", "config", "upgrade"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("metrics is a proper file name but files are not upgradable", func(*testing.T) {
		fname := t.TempDir() + "/metrics.yaml"
		os.Args = []string{0: "config_test", "--metrics=" + fname, "config", "upgrade"}
		c, err := New(io.Discard)
		a.NoError(err)
		a.True(c.CommandCompleted)
		a.Equal(ExitCodeOK, c.ExitCode)
	})

}

// Mock types for testing Migrator interface with proper interface implementations

type mockMigratableSourcesReader struct {
	migrateErr        error
	needsMigration    bool
	needsMigrationErr error
}

func (m *mockMigratableSourcesReader) Migrate() error { return m.migrateErr }
func (m *mockMigratableSourcesReader) NeedsMigration() (bool, error) {
	return m.needsMigration, m.needsMigrationErr
}
func (m *mockMigratableSourcesReader) GetSources() (sources.Sources, error) {
	return sources.Sources{}, nil
}
func (m *mockMigratableSourcesReader) WriteSources(sources.Sources) error { return nil }
func (m *mockMigratableSourcesReader) DeleteSource(string) error          { return nil }
func (m *mockMigratableSourcesReader) UpdateSource(sources.Source) error  { return nil }
func (m *mockMigratableSourcesReader) CreateSource(sources.Source) error  { return nil }

type mockMigratableMetricsReader struct {
	migrateErr        error
	needsMigration    bool
	needsMigrationErr error
}

func (m *mockMigratableMetricsReader) Migrate() error { return m.migrateErr }
func (m *mockMigratableMetricsReader) NeedsMigration() (bool, error) {
	return m.needsMigration, m.needsMigrationErr
}
func (m *mockMigratableMetricsReader) GetMetrics() (*metrics.Metrics, error) {
	return &metrics.Metrics{}, nil
}
func (m *mockMigratableMetricsReader) WriteMetrics(*metrics.Metrics) error       { return nil }
func (m *mockMigratableMetricsReader) DeleteMetric(string) error                 { return nil }
func (m *mockMigratableMetricsReader) UpdateMetric(string, metrics.Metric) error { return nil }
func (m *mockMigratableMetricsReader) CreateMetric(string, metrics.Metric) error { return nil }
func (m *mockMigratableMetricsReader) DeletePreset(string) error                 { return nil }
func (m *mockMigratableMetricsReader) UpdatePreset(string, metrics.Preset) error { return nil }
func (m *mockMigratableMetricsReader) CreatePreset(string, metrics.Preset) error { return nil }

type mockMigratableSinksWriter struct {
	migrateErr        error
	needsMigration    bool
	needsMigrationErr error
}

func (m *mockMigratableSinksWriter) Migrate() error { return m.migrateErr }
func (m *mockMigratableSinksWriter) NeedsMigration() (bool, error) {
	return m.needsMigration, m.needsMigrationErr
}
func (m *mockMigratableSinksWriter) SyncMetric(string, string, sinks.SyncOp) error { return nil }
func (m *mockMigratableSinksWriter) Write(metrics.MeasurementEnvelope) error       { return nil }

func TestNeedsSchemaUpgrade(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*Options)
		expectUpgrade bool
		expectError   bool
	}{
		{
			name: "sources needs migration",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigration: true}
			},
			expectUpgrade: true,
			expectError:   false,
		},
		{
			name: "metrics needs migration",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigration: false}
				opts.MetricsReaderWriter = &mockMigratableMetricsReader{needsMigration: true}
			},
			expectUpgrade: true,
			expectError:   false,
		},
		{
			name: "sinks needs migration",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigration: false}
				opts.MetricsReaderWriter = &mockMigratableMetricsReader{needsMigration: false}
				opts.SinksWriter = &mockMigratableSinksWriter{needsMigration: true}
			},
			expectUpgrade: true,
			expectError:   false,
		},
		{
			name: "no migration needed",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigration: false}
				opts.MetricsReaderWriter = &mockMigratableMetricsReader{needsMigration: false}
				opts.SinksWriter = &mockMigratableSinksWriter{needsMigration: false}
			},
			expectUpgrade: false,
			expectError:   false,
		},
		{
			name: "error checking sources migration",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigrationErr: assert.AnError}
			},
			expectUpgrade: false,
			expectError:   true,
		},
		{
			name: "error checking metrics migration",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigration: false}
				opts.MetricsReaderWriter = &mockMigratableMetricsReader{needsMigrationErr: assert.AnError}
			},
			expectUpgrade: false,
			expectError:   true,
		},
		{
			name: "error checking sinks migration",
			setupMocks: func(opts *Options) {
				opts.SourcesReaderWriter = &mockMigratableSourcesReader{needsMigration: false}
				opts.MetricsReaderWriter = &mockMigratableMetricsReader{needsMigration: false}
				opts.SinksWriter = &mockMigratableSinksWriter{needsMigrationErr: assert.AnError}
			},
			expectUpgrade: false,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &Options{}
			if tt.setupMocks != nil {
				tt.setupMocks(opts)
			}

			upgrade, err := opts.NeedsSchemaUpgrade()

			assert.Equal(t, tt.expectUpgrade, upgrade)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigInitCommand_InitSources(t *testing.T) {
	a := assert.New(t)

	t.Run("yaml file creation", func(*testing.T) {
		fname := t.TempDir() + "/sources.yaml"
		opts := &Options{
			Sources: sources.CmdOpts{Sources: fname},
		}
		cmd := ConfigInitCommand{owner: opts}
		err := cmd.InitSources()
		a.NoError(err)
		a.FileExists(fname)
	})

	t.Run("postgres connection - error without setup", func(*testing.T) {
		opts := &Options{
			Sources: sources.CmdOpts{Sources: "postgresql://user@host/db"},
		}
		cmd := ConfigInitCommand{owner: opts}
		err := cmd.InitSources()
		a.Error(err)
	})
}

func TestConfigInitCommand_InitMetrics(t *testing.T) {
	a := assert.New(t)

	t.Run("yaml file creation with default metrics", func(*testing.T) {
		fname := t.TempDir() + "/metrics.yaml"
		opts := &Options{
			Metrics: metrics.CmdOpts{Metrics: fname},
		}
		cmd := ConfigInitCommand{owner: opts}
		err := cmd.InitMetrics()
		a.NoError(err)
		a.FileExists(fname)
	})

	t.Run("postgres connection - error without setup", func(*testing.T) {
		opts := &Options{
			Metrics: metrics.CmdOpts{Metrics: "postgresql://user@host/db"},
		}
		cmd := ConfigInitCommand{owner: opts}
		err := cmd.InitMetrics()
		a.Error(err)
	})
}

func TestConfigInitCommand_InitSinks(t *testing.T) {
	a := assert.New(t)

	t.Run("postgres connection - error without setup", func(*testing.T) {
		opts := &Options{
			Sinks: sinks.CmdOpts{Sinks: []string{"postgresql://user@host/db"}},
		}
		cmd := ConfigInitCommand{owner: opts}
		err := cmd.InitSinks()
		a.Error(err)
	})
}

func TestConfigUpgradeCommand_Errors(t *testing.T) {
	a := assert.New(t)

	t.Run("non-postgres configuration not supported", func(*testing.T) {
		opts := &Options{
			Metrics: metrics.CmdOpts{Metrics: "/tmp/metrics.yaml"},
			Sources: sources.CmdOpts{Sources: "/tmp/sources.yaml", Refresh: 120, MaxParallelConnectionsPerDb: 1},
			Sinks:   sinks.CmdOpts{BatchingDelay: time.Second},
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "does not support upgrade")
	})

	t.Run("init metrics reader fails", func(*testing.T) {
		opts := &Options{
			Metrics: metrics.CmdOpts{Metrics: "postgresql://invalid@host/db"},
			Sources: sources.CmdOpts{Sources: "postgresql://invalid@host/db", Refresh: 120, MaxParallelConnectionsPerDb: 1},
			Sinks:   sinks.CmdOpts{BatchingDelay: time.Second},
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
	})
}
