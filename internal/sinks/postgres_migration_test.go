package sinks

import (
	"os"
	"regexp"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsCountInvariant is a fast, dependency-free unit test that guards against the
// "stale MigrationsCount" class of bug. It asserts that MigrationsCount matches:
//   - the number of migrations actually registered in migrations(), and
//   - the number of rows seeded into admin.migration in admin_schema.sql.
//
// Whenever a migration is added, all three must be bumped together, so this test fails loudly
// if any of them drifts out of sync.
func TestMigrationsCountInvariant(t *testing.T) {
	a := assert.New(t)

	// 1. Count migrations actually registered in migrations().
	m, err := migrator.New(migrations())
	require.NoError(t, err)
	registered := m.Count()
	a.Equal(MigrationsCount, registered,
		"MigrationsCount (%d) must equal the number of migrations registered in migrations() (%d)",
		MigrationsCount, registered)

	// 2. Count rows seeded into admin.migration in admin_schema.sql. The seed looks like:
	//     INSERT INTO admin.migration (id, version) VALUES
	//         (0, '...'),
	//         (1, '...'),
	//         (2, '...');
	seeded := len(regexp.MustCompile(`(?m)^\s*\(\d+,\s*'`).FindAllString(sqlMetricAdminSchema, -1))
	a.Equal(MigrationsCount, seeded,
		"MigrationsCount (%d) must equal the number of rows seeded into admin.migration in admin_schema.sql (%d)",
		MigrationsCount, seeded)
}

// simulatePreV6Migration rolls the seeded admin.migration table back to a pre-01409 state so
// that the migrator considers the "01409 Switch to time-only partitioning" migration pending.
// A fresh NewPostgresSinkMigrator bootstrap seeds all migrations as applied (the current install
// path); here we emulate an older database that must still be upgraded.
//
// The migrator determines pending migrations purely from the row count in admin.migration, so we
// delete the last-applied migration row, leaving MigrationsCount-1 rows behind.
func simulatePreV6Migration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	_, err := conn.Exec(ctx, `DELETE FROM admin.migration WHERE id = (SELECT max(id) FROM admin.migration)`)
	require.NoError(t, err)

	var count int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM admin.migration`).Scan(&count))
	require.Equal(t, MigrationsCount-1, count, "exactly one migration should be pending after rollback")
}

// oldSchemaMetricTable creates a metric table using the pre-v6 (dbname -> time) two-level
// partitioning layout that the "01409 Switch to time-only partitioning" migration converts from:
//
//	public.<metric>                      PARTITION BY LIST (dbname)      -- top level
//	  subpartitions.<metric>_<dbname>    PARTITION BY RANGE (time)       -- dbname level
//	    subpartitions.<metric>_<...>_<w> leaf partition                  -- time level
//
// It seeds a couple of rows so the test can assert data survives the migration.
func oldSchemaMetricTable(t *testing.T, conn *pgx.Conn, metric string) {
	t.Helper()
	_, err := conn.Exec(ctx, `
		CREATE TABLE public.`+metric+` (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY LIST (dbname);
		COMMENT ON TABLE public.`+metric+` IS 'pgwatch-generated-metric-lvl';

		CREATE TABLE subpartitions.`+metric+`_db1 PARTITION OF public.`+metric+`
			FOR VALUES IN ('db1') PARTITION BY RANGE (time);

		CREATE TABLE subpartitions.`+metric+`_db1_2024w01 PARTITION OF subpartitions.`+metric+`_db1
			FOR VALUES FROM ('2024-01-01') TO ('2024-01-08');
		COMMENT ON TABLE subpartitions.`+metric+`_db1_2024w01 IS 'pgwatch-generated-metric-time-lvl';

		INSERT INTO public.`+metric+` (time, dbname, data) VALUES
			('2024-01-03 10:00:00+00', 'db1', '{"x": 1}'::jsonb),
			('2024-01-04 11:00:00+00', 'db1', '{"x": 2}'::jsonb);
	`)
	require.NoError(t, err)
}

// isRangePartitioned reports whether the given relation is now top-level RANGE partitioned.
func isRangePartitioned(t *testing.T, conn *pgx.Conn, metric string) bool {
	t.Helper()
	var ok bool
	err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_partitioned_table WHERE partrelid = to_regclass($1) AND partstrat = 'r')`,
		metric).Scan(&ok)
	require.NoError(t, err)
	return ok
}

// rowCount returns the number of rows currently stored in the (partitioned) metric table.
func rowCount(t *testing.T, conn *pgx.Conn, metric string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM public.`+metric).Scan(&n))
	return n
}

// TestMigration01409_TimeOnlyPartitioning is an end-to-end test against a real PostgreSQL
// container. It builds the old (dbname -> time) partitioned layout, runs the sink migrations,
// and asserts that the table is converted to time-only RANGE partitioning while preserving data.
// It also runs the migration a second time to verify idempotency / re-run safety.
func TestMigration01409_TimeOnlyPartitioning(t *testing.T) {
	if os.Getenv("PGWATCH_TEST_SKIP_MIGRATION") != "" {
		t.Skip("migration integration test skipped via PGWATCH_TEST_SKIP_MIGRATION")
	}
	r := require.New(t)
	a := assert.New(t)

	pgContainer, pgTearDown, err := testutil.SetupPostgresContainer()
	r.NoError(err)
	defer pgTearDown()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	r.NoError(err)

	// Bootstrap the admin schema (creates admin.*, subpartitions, metrics_template, etc.).
	// A fresh bootstrap seeds admin.migration with ALL migrations already applied, so we
	// delete the 01409 row to simulate an older database that predates this migration.
	mig, err := NewPostgresSinkMigrator(ctx, connStr)
	r.NoError(err)

	conn, err := pgx.Connect(ctx, connStr)
	r.NoError(err)
	defer conn.Close(ctx)

	simulatePreV6Migration(t, conn)

	const metric = "old_style_metric"
	oldSchemaMetricTable(t, conn, metric)

	// Sanity check: before migration the table is LIST (dbname) partitioned, not RANGE.
	a.False(isRangePartitioned(t, conn, metric), "table should start as LIST(dbname) partitioned")
	a.Equal(2, rowCount(t, conn, metric), "seeded rows should be present before migration")

	// Run the migrations (this executes 01110, 01180 and the 01409 conversion).
	r.NoError(mig.Migrate())

	// After migration the top-level table must be RANGE(time) partitioned and keep its data.
	a.True(isRangePartitioned(t, conn, metric), "table should be converted to RANGE(time) partitioning")
	a.Equal(2, rowCount(t, conn, metric), "data must be preserved across the migration")

	// The temporary *_before_v6_migration table must have been cleaned up.
	var leftover bool
	r.NoError(conn.QueryRow(ctx,
		`SELECT to_regclass($1) IS NOT NULL`, metric+"_before_v6_migration").Scan(&leftover))
	a.False(leftover, "the *_before_v6_migration scratch table should be dropped")

	// Idempotency: running the migrations again must not error and must not lose data.
	needs, err := mig.NeedsMigration()
	r.NoError(err)
	a.False(needs, "no migrations should be pending immediately after a successful migrate")

	r.NoError(mig.Migrate(), "re-running Migrate() must be a no-op and not error")
	a.True(isRangePartitioned(t, conn, metric))
	a.Equal(2, rowCount(t, conn, metric), "data must remain intact after a second migrate")
}

// TestMigration01409_EmptyTable verifies the migration handles a metric table with no rows:
// MIN(time) is NULL server-side, so it should create a single empty time partition without error.
func TestMigration01409_EmptyTable(t *testing.T) {
	if os.Getenv("PGWATCH_TEST_SKIP_MIGRATION") != "" {
		t.Skip("migration integration test skipped via PGWATCH_TEST_SKIP_MIGRATION")
	}
	r := require.New(t)
	a := assert.New(t)

	pgContainer, pgTearDown, err := testutil.SetupPostgresContainer()
	r.NoError(err)
	defer pgTearDown()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	r.NoError(err)

	mig, err := NewPostgresSinkMigrator(ctx, connStr)
	r.NoError(err)

	conn, err := pgx.Connect(ctx, connStr)
	r.NoError(err)
	defer conn.Close(ctx)

	simulatePreV6Migration(t, conn)

	const metric = "empty_old_metric"
	_, err = conn.Exec(ctx, `
		CREATE TABLE public.`+metric+` (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY LIST (dbname);
		COMMENT ON TABLE public.`+metric+` IS 'pgwatch-generated-metric-lvl';
		CREATE TABLE subpartitions.`+metric+`_db1 PARTITION OF public.`+metric+`
			FOR VALUES IN ('db1') PARTITION BY RANGE (time);
		CREATE TABLE subpartitions.`+metric+`_db1_2024w01 PARTITION OF subpartitions.`+metric+`_db1
			FOR VALUES FROM ('2024-01-01') TO ('2024-01-08');
		COMMENT ON TABLE subpartitions.`+metric+`_db1_2024w01 IS 'pgwatch-generated-metric-time-lvl';
	`)
	r.NoError(err)

	r.NoError(mig.Migrate())

	a.True(isRangePartitioned(t, conn, metric), "empty table should still be converted to RANGE(time)")
	a.Equal(0, rowCount(t, conn, metric))

	// at least one (empty) time partition should have been created
	var leaves int
	r.NoError(conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_partition_tree($1) WHERE isleaf`, metric).Scan(&leaves))
	a.GreaterOrEqual(leaves, 1, "a single empty time partition should exist")
}
