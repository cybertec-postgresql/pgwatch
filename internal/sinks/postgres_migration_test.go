package sinks

import (
	"os"
	"regexp"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationsCountInvariant is a fast, dependency-free unit test that guards against
// the "seed drift" class of bug. The source of truth for "how many migrations exist" is
// migrations() itself, exposed via registeredMigrationsCount(). This test verifies that
// the rows seeded into admin.migration in admin_schema.sql stay in lock-step with the
// registered migrations: both must be updated together whenever a migration is added or
// removed.
func TestMigrationsCountInvariant(t *testing.T) {
	a := assert.New(t)

	registered := registeredMigrationsCount()

	// Count rows seeded into admin.migration in admin_schema.sql. The seed looks like:
	//     INSERT INTO admin.migration (id, version) VALUES
	//         (0, '...'),
	//         (1, '...'),
	//         (2, '...');
	seeded := len(regexp.MustCompile(`(?m)^\s*\(\d+,\s*'`).FindAllString(sqlMetricAdminSchema, -1))
	a.Equal(registered, seeded,
		"registeredMigrationsCount (%d) must equal the number of rows seeded into admin.migration in admin_schema.sql (%d)",
		registered, seeded)
}

// simulatePreV6Migration wipes every row from admin.migration so that the next
// mig.Migrate() replays the full migration chain (01110 → 01180 → 01409 → 01474)
// against the freshly-bootstrapped database. A fresh NewPostgresSinkMigrator
// bootstrap seeds all migrations as applied (the current install path); here
// we emulate an older database that must be upgraded from scratch so every
// migration is exercised in every test that calls this helper.
//
// Callers that need a specific pre-migration object state (e.g. the legacy
// function form of admin.drop_all_metric_tables) must recreate that object
// after this call returns.
func simulatePreV6Migration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	_, err := conn.Exec(ctx, `DELETE FROM admin.migration`)
	require.NoError(t, err)

	var count int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM admin.migration`).Scan(&count))
	require.Equal(t, 0, count, "all migration rows should be wiped after rollback")
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

// TestMigration01474_DropAllMetricTablesProcedure verifies that an older database that
// still has admin.drop_all_metric_tables as a function gets upgraded in place to the
// new procedure by migration 01474, and the upgraded routine actually drops partitions
// partition-by-partition (not in one bulk DROP).
func TestMigration01474_DropAllMetricTablesProcedure(t *testing.T) {
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

	// Simulate a pre-01474 database: drop the procedure seeded by bootstrap and recreate
	// the legacy function; delete the highest migration row so the migrator picks 01474 up.
	_, err = conn.Exec(ctx, `
		DROP PROCEDURE IF EXISTS admin.drop_all_metric_tables();
		CREATE FUNCTION admin.drop_all_metric_tables() RETURNS int AS $$ BEGIN RETURN 0; END $$ LANGUAGE plpgsql;
		DELETE FROM admin.migration WHERE id >= 3;
	`)
	r.NoError(err)

	// Before the migration the routine must be a function (prokind = 'f').
	var prokindBefore string
	r.NoError(conn.QueryRow(ctx, `
		SELECT p.prokind FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'admin' AND p.proname = 'drop_all_metric_tables'
	`).Scan(&prokindBefore))
	a.Equal("f", prokindBefore, "legacy routine must start as a function")

	// Build a tiny dummy metric with one partition so the upgraded procedure has something
	_, err = conn.Exec(ctx, `
		CREATE TABLE public.upgraded_metric (LIKE admin.metrics_template INCLUDING INDEXES) PARTITION BY RANGE (time);
		COMMENT ON TABLE public.upgraded_metric IS 'pgwatch-generated-metric-lvl';
		INSERT INTO admin.all_distinct_dbname_metrics (dbname, metric) VALUES ('db1', 'upgraded_metric');
		CREATE TABLE subpartitions.upgraded_metric_2024w01 PARTITION OF public.upgraded_metric FOR VALUES FROM ('2024-01-01') TO ('2024-01-08');
		COMMENT ON TABLE subpartitions.upgraded_metric_2024w01 IS 'pgwatch-generated-metric-time-lvl';
	`)
	r.NoError(err)

	r.NoError(mig.Migrate())

	// After migration the routine must be a procedure (prokind = 'p').
	var prokindAfter string
	r.NoError(conn.QueryRow(ctx, `
		SELECT p.prokind FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'admin' AND p.proname = 'drop_all_metric_tables'
	`).Scan(&prokindAfter))
	a.Equal("p", prokindAfter, "admin.drop_all_metric_tables must be upgraded to a procedure")

	// The upgraded procedure must actually drop partition-by-partition. The bare
	// conn (not a transaction-wrapped one) is required: CALL ... COMMIT is rejected
	// inside an explicit transaction block.
	_, err = conn.Exec(ctx, "CALL admin.drop_all_metric_tables();")
	r.NoError(err, "upgraded procedure must execute end-to-end")

	var leftover *string
	r.NoError(conn.QueryRow(ctx, "SELECT to_regclass('public.upgraded_metric')::text").Scan(&leftover))
	a.Nil(leftover, "top-level metric table must be dropped by the upgraded procedure")
	var subpartCount int
	r.NoError(conn.QueryRow(ctx, "SELECT count(*) FROM pg_class WHERE relnamespace = 'subpartitions'::regnamespace").Scan(&subpartCount))
	a.Equal(0, subpartCount, "partition must be dropped by the upgraded procedure")
	var listingCount int
	r.NoError(conn.QueryRow(ctx, "SELECT count(*) FROM admin.all_distinct_dbname_metrics").Scan(&listingCount))
	a.Equal(0, listingCount, "listing table must be truncated by the upgraded procedure")

	needs, err := mig.NeedsMigration()
	r.NoError(err)
	a.False(needs, "no migrations should be pending after the upgrade")
}

// TestMigration_AllMigrationsRunFromEmpty is the explicit all-migrations exercise.
// It wipes every row from admin.migration, runs mig.Migrate() against the fresh
// bootstrap, and asserts that every registered migration in the migrator chain
// actually executed and recorded itself in admin.migration — including the new
// 01474 routine. The migrator's "did it run" signal is purely the row count in
// admin.migration, so this test guards against two failure modes at once:
//   - a registered migration that fails silently (count stays below registeredMigrationsCount())
//   - registeredMigrationsCount() drifting above the actual number of registered migrations
func TestMigration_AllMigrationsRunFromEmpty(t *testing.T) {
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

	// After wipe: exactly zero applied migrations.
	var before int
	r.NoError(conn.QueryRow(ctx, "SELECT count(*) FROM admin.migration").Scan(&before))
	a.Equal(0, before)

	r.NoError(mig.Migrate(), "migrate from empty must run the entire chain without error")

	// After migrate: every registered migration row must be present.
	var after int
	r.NoError(conn.QueryRow(ctx, "SELECT count(*) FROM admin.migration").Scan(&after))
	a.Equal(registeredMigrationsCount(), after,
		"every registered migration (incl. 01474) must record itself in admin.migration after a wipe + migrate")

	// The new routine from 01474 must have been created as a procedure.
	var prokind string
	r.NoError(conn.QueryRow(ctx, `
		SELECT p.prokind FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'admin' AND p.proname = 'drop_all_metric_tables'
	`).Scan(&prokind))
	a.Equal("p", prokind, "01474 must install the routine as a procedure")

	// ensure_partition_metric_time must have been (re)installed by 01180.
	var fnExists bool
	r.NoError(conn.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		               WHERE n.nspname = 'admin' AND p.proname = 'ensure_partition_metric_time')
	`).Scan(&fnExists))
	a.True(fnExists, "01180 must install admin.ensure_partition_metric_time")

	// needs-migration must report clean.
	needs, err := mig.NeedsMigration()
	r.NoError(err)
	a.False(needs, "no migrations should be pending after a full replay")

	// Idempotency: a second migrate must be a clean no-op.
	r.NoError(mig.Migrate(), "second migrate after full replay must be a no-op")
}
