package sinks_test

import (
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	"github.com/stretchr/testify/assert"
)

type MockWriter struct{}

func (mw *MockWriter) SyncMetric(_, _ string, _ sinks.SyncOp) error {
	return nil
}

func (mw *MockWriter) Write(_ metrics.MeasurementEnvelope) error {
	return nil
}

func TestNewMultiWriter(t *testing.T) {
	input := []struct {
		opts *sinks.CmdOpts
		w    bool // Writer returned
		err  bool // error returned
	}{
		{&sinks.CmdOpts{}, false, true},
		{&sinks.CmdOpts{
			Sinks: []string{"foo"},
		}, false, true},
		{&sinks.CmdOpts{
			Sinks: []string{"jsonfile://test.json"},
		}, true, false},
		{&sinks.CmdOpts{
			Sinks: []string{"jsonfile://test.json", "jsonfile://test1.json"},
		}, true, false},
		{&sinks.CmdOpts{
			Sinks: []string{"prometheus://foo/"},
		}, false, true},
		{&sinks.CmdOpts{
			Sinks: []string{"rpc://foo/"},
		}, false, true},
		{&sinks.CmdOpts{
			Sinks: []string{"postgresql:///baz"},
		}, false, true},
		{&sinks.CmdOpts{
			Sinks: []string{"foo:///"},
		}, false, true},
	}

	for _, i := range input {
		mw, err := sinks.NewSinkWriter(testutil.TestContext, i.opts)
		if i.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		if i.w {
			assert.NotNil(t, mw)
		} else {
			assert.Nil(t, mw)
		}
	}
}

func TestAddWriter(t *testing.T) {
	mw := &sinks.MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	assert.Equal(t, 1, mw.Count())
}

func TestSyncMetrics(t *testing.T) {
	mw := &sinks.MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	err := mw.SyncMetric("db", "metric", sinks.InvalidOp)
	assert.NoError(t, err)
}

func TestWriteMeasurements(t *testing.T) {
	mw := &sinks.MultiWriter{}
	mockWriter := &MockWriter{}
	mw.AddWriter(mockWriter)
	err := mw.Write(metrics.MeasurementEnvelope{})
	assert.NoError(t, err)
}

// mockMigratableWriter implements Writer and Migrator interfaces
type mockMigratableWriter struct {
	migrateErr        error
	needsMigration    bool
	needsMigrationErr error
}

func (m *mockMigratableWriter) SyncMetric(string, string, sinks.SyncOp) error {
	return nil
}

func (m *mockMigratableWriter) Write(metrics.MeasurementEnvelope) error {
	return nil
}

func (m *mockMigratableWriter) Migrate() error {
	return m.migrateErr
}

func (m *mockMigratableWriter) NeedsMigration() (bool, error) {
	return m.needsMigration, m.needsMigrationErr
}

func TestMultiWriterMigrate(t *testing.T) {
	tests := []struct {
		name        string
		writers     []sinks.Writer
		expectError bool
	}{
		{
			name: "no migratable writers",
			writers: []sinks.Writer{
				&MockWriter{},
			},
			expectError: false,
		},
		{
			name: "single migratable writer success",
			writers: []sinks.Writer{
				&mockMigratableWriter{},
			},
			expectError: false,
		},
		{
			name: "single migratable writer error",
			writers: []sinks.Writer{
				&mockMigratableWriter{migrateErr: assert.AnError},
			},
			expectError: true,
		},
		{
			name: "multiple migratable writers success",
			writers: []sinks.Writer{
				&mockMigratableWriter{},
				&mockMigratableWriter{},
			},
			expectError: false,
		},
		{
			name: "multiple writers with one error",
			writers: []sinks.Writer{
				&mockMigratableWriter{},
				&mockMigratableWriter{migrateErr: assert.AnError},
			},
			expectError: true,
		},
		{
			name: "mixed writers with migration error",
			writers: []sinks.Writer{
				&MockWriter{},
				&mockMigratableWriter{migrateErr: assert.AnError},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := &sinks.MultiWriter{}
			for _, w := range tt.writers {
				mw.AddWriter(w)
			}
			err := mw.Migrate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMultiWriterNeedsMigration(t *testing.T) {
	tests := []struct {
		name               string
		writers            []sinks.Writer
		expectNeedsMigrate bool
		expectError        bool
	}{
		{
			name: "no migratable writers",
			writers: []sinks.Writer{
				&MockWriter{},
			},
			expectNeedsMigrate: false,
			expectError:        false,
		},
		{
			name: "single writer needs migration",
			writers: []sinks.Writer{
				&mockMigratableWriter{needsMigration: true},
			},
			expectNeedsMigrate: true,
			expectError:        false,
		},
		{
			name: "single writer no migration needed",
			writers: []sinks.Writer{
				&mockMigratableWriter{needsMigration: false},
			},
			expectNeedsMigrate: false,
			expectError:        false,
		},
		{
			name: "multiple writers one needs migration",
			writers: []sinks.Writer{
				&mockMigratableWriter{needsMigration: false},
				&mockMigratableWriter{needsMigration: true},
			},
			expectNeedsMigrate: true,
			expectError:        false,
		},
		{
			name: "error checking migration",
			writers: []sinks.Writer{
				&mockMigratableWriter{needsMigrationErr: assert.AnError},
			},
			expectNeedsMigrate: false,
			expectError:        true,
		},
		{
			name: "mixed writers one needs migration",
			writers: []sinks.Writer{
				&MockWriter{},
				&mockMigratableWriter{needsMigration: true},
			},
			expectNeedsMigrate: true,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := &sinks.MultiWriter{}
			for _, w := range tt.writers {
				mw.AddWriter(w)
			}
			needs, err := mw.NeedsMigration()
			assert.Equal(t, tt.expectNeedsMigrate, needs)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
