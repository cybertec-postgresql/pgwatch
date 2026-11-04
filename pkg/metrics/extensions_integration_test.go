package metrics_test

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v7/internal/testutil"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestExtensionsMetricAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pgContainer, tearDown, err := testutil.SetupPostgresContainer()
	require.NoError(t, err)
	defer tearDown()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	conn, err := pgx.Connect(ctx, connStr)
	require.NoError(t, err)
	defer conn.Close(ctx)

	sql := metrics.GetDefaultMetrics().MetricDefs["extensions"].SQLs[14]
	require.NotEmpty(t, sql)

	type row struct {
		name, installed string
		def             *string
	}
	collect := func() []row {
		rows, err := conn.Query(ctx, "select tag_extension, tag_installed_version, tag_default_version from ("+sql+") x")
		require.NoError(t, err)
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			require.NoError(t, rows.Scan(&r.name, &r.installed, &r.def))
			out = append(out, r)
		}
		require.NoError(t, rows.Err())
		return out
	}

	// AC-002
	got := collect()
	require.Len(t, got, 1)
	require.Equal(t, "plpgsql", got[0].name)

	// AC-001
	_, err = conn.Exec(ctx, "create extension pg_stat_statements")
	require.NoError(t, err)
	var found *row
	for _, r := range collect() {
		if r.name == "pg_stat_statements" {
			found = &r
		}
	}
	require.NotNil(t, found)
	require.NotEmpty(t, found.installed)
	require.NotNil(t, found.def)
}
