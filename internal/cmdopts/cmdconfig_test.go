package cmdopts

import (
	"bytes"
	"errors"
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
		a.Error(err)
		a.True(c.CommandCompleted)
		a.Equal(ExitCodeConfigError, c.ExitCode)
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
			Metrics:      metrics.CmdOpts{Metrics: "/tmp/metrics.yaml"},
			Sources:      sources.CmdOpts{Sources: "/tmp/sources.yaml", Refresh: 120, MaxParallelConnectionsPerDb: 1},
			Sinks:        sinks.CmdOpts{BatchingDelay: time.Second},
			OutputWriter: t.Output(),
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
	})

	t.Run("init metrics reader fails", func(*testing.T) {
		opts := &Options{
			Metrics:      metrics.CmdOpts{Metrics: "postgresql://invalid@host/db"},
			Sources:      sources.CmdOpts{Sources: "postgresql://invalid@host/db", Refresh: 120, MaxParallelConnectionsPerDb: 1},
			Sinks:        sinks.CmdOpts{BatchingDelay: time.Second},
			OutputWriter: t.Output(),
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
	})
}

func TestConfigUpgradeCommand_Execute_Coverage(t *testing.T) {
	a := assert.New(t)

	t.Run("no components specified - returns error", func(*testing.T) {
		opts := &Options{
			OutputWriter: io.Discard,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "at least one of --sources, --metrics, or --sink must be specified")
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

	t.Run("yaml sources/metrics - logs warning", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Metrics:      metrics.CmdOpts{Metrics: "/tmp/metrics.yaml"},
			Sources:      sources.CmdOpts{Sources: "/tmp/sources.yaml"},
			OutputWriter: &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
		a.Contains(output.String(), "[WARN] configuration storage does not support upgrade, skipping")
	})

	t.Run("successful sink upgrade only - with pre-initialized writer", func(*testing.T) {
		opts := &Options{
			Sinks:        sinks.CmdOpts{Sinks: []string{"postgresql://localhost/db"}},
			SinksWriter:  &mockMigratableSinksWriter{},
			OutputWriter: io.Discard,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.NoError(err)
		a.Equal(ExitCodeOK, opts.ExitCode)
	})

	t.Run("sink upgrade with migration error - pre-initialized writer", func(*testing.T) {
		opts := &Options{
			Sinks:        sinks.CmdOpts{Sinks: []string{"postgresql://localhost/db"}},
			SinksWriter:  &mockMigratableSinksWriter{migrateErr: errors.New("sink migration failed")},
			OutputWriter: io.Discard,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "sink migration failed")
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

	t.Run("non-migratable sink - logs warning", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Sinks:        sinks.CmdOpts{Sinks: []string{"jsonfile://test.json"}},
			SinksWriter:  &mockNonMigratableSinksWriter{},
			OutputWriter: &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
		a.Contains(output.String(), "[WARN] sink storage does not support upgrade, skipping")
	})

	t.Run("yaml sources/metrics with postgres sink - upgrades sink only", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Metrics:      metrics.CmdOpts{Metrics: "/tmp/metrics.yaml"},
			Sources:      sources.CmdOpts{Sources: "/tmp/sources.yaml"},
			Sinks:        sinks.CmdOpts{Sinks: []string{"postgresql://localhost/db"}},
			SinksWriter:  &mockMigratableSinksWriter{},
			OutputWriter: &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.NoError(err)
		a.Equal(ExitCodeOK, opts.ExitCode)
		a.Contains(output.String(), "[WARN] configuration storage does not support upgrade, skipping")
	})

	t.Run("postgres sources/metrics with non-postgres sink - config upgrades, sink warns", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Metrics:             metrics.CmdOpts{Metrics: "postgresql://localhost/db"},
			Sources:             sources.CmdOpts{Sources: "postgresql://localhost/db"},
			Sinks:               sinks.CmdOpts{Sinks: []string{"jsonfile://test.json"}},
			MetricsReaderWriter: &mockMigratableMetricsReader{},
			SinksWriter:         &mockNonMigratableSinksWriter{},
			OutputWriter:        &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.NoError(err)
		a.Equal(ExitCodeOK, opts.ExitCode)
		a.Contains(output.String(), "[WARN] sink storage does not support upgrade, skipping")
	})

	t.Run("only metrics specified as yaml - logs warning", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Metrics:      metrics.CmdOpts{Metrics: "/tmp/metrics.yaml"},
			OutputWriter: &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
		a.Contains(output.String(), "[WARN] configuration storage does not support upgrade, skipping")
	})

	t.Run("only sources specified as yaml - logs warning", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Sources:      sources.CmdOpts{Sources: "/tmp/sources.yaml"},
			OutputWriter: &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
		a.Contains(output.String(), "[WARN] configuration storage does not support upgrade, skipping")
	})

	t.Run("both metrics and sources specified, only metrics is postgres", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Metrics:      metrics.CmdOpts{Metrics: "postgresql://localhost/db"},
			Sources:      sources.CmdOpts{Sources: "/tmp/sources.yaml"},
			OutputWriter: &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
		a.Contains(output.String(), "[WARN] configuration storage does not support upgrade, skipping")
	})

	t.Run("pre-initialized non-migratable metrics reader", func(*testing.T) {
		var output bytes.Buffer
		opts := &Options{
			Metrics:             metrics.CmdOpts{Metrics: "postgresql://localhost/db"},
			Sources:             sources.CmdOpts{Sources: "postgresql://localhost/db"},
			MetricsReaderWriter: &mockNonMigratableMetricsReader{},
			OutputWriter:        &output,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "no components support upgrade")
		a.Contains(output.String(), "[WARN] configuration storage does not support upgrade, skipping")
	})

	t.Run("pre-initialized migratable metrics reader - succeeds", func(*testing.T) {
		opts := &Options{
			Metrics:             metrics.CmdOpts{Metrics: "postgresql://localhost/db"},
			Sources:             sources.CmdOpts{Sources: "postgresql://localhost/db"},
			MetricsReaderWriter: &mockMigratableMetricsReader{},
			OutputWriter:        io.Discard,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.NoError(err)
		a.Equal(ExitCodeOK, opts.ExitCode)
	})

	t.Run("pre-initialized metrics reader with migration error", func(*testing.T) {
		opts := &Options{
			Metrics:             metrics.CmdOpts{Metrics: "postgresql://localhost/db"},
			Sources:             sources.CmdOpts{Sources: "postgresql://localhost/db"},
			MetricsReaderWriter: &mockMigratableMetricsReader{migrateErr: errors.New("migration failed")},
			OutputWriter:        io.Discard,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.Error(err)
		a.ErrorContains(err, "migration failed")
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

	t.Run("both metrics and sink upgradeable - both succeed", func(*testing.T) {
		opts := &Options{
			Metrics:             metrics.CmdOpts{Metrics: "postgresql://localhost/db"},
			Sources:             sources.CmdOpts{Sources: "postgresql://localhost/db"},
			Sinks:               sinks.CmdOpts{Sinks: []string{"postgresql://localhost/db"}},
			MetricsReaderWriter: &mockMigratableMetricsReader{},
			SinksWriter:         &mockMigratableSinksWriter{},
			OutputWriter:        io.Discard,
		}
		cmd := ConfigUpgradeCommand{owner: opts}
		err := cmd.Execute(nil)
		a.NoError(err)
		a.Equal(ExitCodeOK, opts.ExitCode)
	})
}

// Additional mock types for non-migratable readers/writers
type mockNonMigratableMetricsReader struct{}

func (m *mockNonMigratableMetricsReader) GetMetrics() (*metrics.Metrics, error) {
	return &metrics.Metrics{}, nil
}
func (m *mockNonMigratableMetricsReader) WriteMetrics(*metrics.Metrics) error       { return nil }
func (m *mockNonMigratableMetricsReader) DeleteMetric(string) error                 { return nil }
func (m *mockNonMigratableMetricsReader) UpdateMetric(string, metrics.Metric) error { return nil }
func (m *mockNonMigratableMetricsReader) CreateMetric(string, metrics.Metric) error { return nil }
func (m *mockNonMigratableMetricsReader) DeletePreset(string) error                 { return nil }
func (m *mockNonMigratableMetricsReader) UpdatePreset(string, metrics.Preset) error { return nil }
func (m *mockNonMigratableMetricsReader) CreatePreset(string, metrics.Preset) error { return nil }

type mockNonMigratableSinksWriter struct{}

func (m *mockNonMigratableSinksWriter) SyncMetric(string, string, sinks.SyncOp) error { return nil }
func (m *mockNonMigratableSinksWriter) Write(metrics.MeasurementEnvelope) error       { return nil }
