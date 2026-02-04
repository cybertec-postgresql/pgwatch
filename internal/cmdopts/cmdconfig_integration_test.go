package cmdopts

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigUpgrade_VerifyNoCircularDependency tests that config upgrade can be run even when the schema
// needs migrations, proving there's no circular dependency
func TestConfigUpgrade_VerifyNoCircularDependency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create a PostgreSQL container with empty database (no schema)
	pgContainer, tearDown, err := testutil.SetupPostgresContainerWithInitScripts()
	require.NoError(t, err)
	defer tearDown()

	connStr, err := pgContainer.ConnectionString(testutil.TestContext)
	require.NoError(t, err)

	ctx := context.Background()

	// Test with only metrics and sinks (sources reader doesn't implement Migrator)
	opts := &Options{
		Metrics:      metrics.CmdOpts{Metrics: connStr},
		Sinks:        sinks.CmdOpts{Sinks: []string{connStr}, RetentionInterval: "30 days", BatchingDelay: time.Second, PartitionInterval: "1 week", MaintenanceInterval: "12 hours"},
		OutputWriter: io.Discard,
	}

	// Config upgrade should work on metrics and sinks
	cmd := ConfigUpgradeCommand{owner: opts}
	err = cmd.Execute(nil)
	assert.NoError(t, err)
	assert.Equal(t, ExitCodeOK, opts.ExitCode)

	// After successful upgrade, InitConfigReaders should succeed
	opts2 := &Options{
		Metrics: metrics.CmdOpts{Metrics: connStr},
		Sources: sources.CmdOpts{Sources: connStr, Refresh: 120, MaxParallelConnectionsPerDb: 1},
		Sinks:   sinks.CmdOpts{Sinks: []string{connStr}},
	}
	err = opts2.InitConfigReaders(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, opts2.MetricsReaderWriter)
	assert.NotNil(t, opts2.SourcesReaderWriter)
}
