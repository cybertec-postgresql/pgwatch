---
title: Postgres Sink Internals
---

When pgwatch uses a PostgreSQL database as a measurement sink, it automatically creates and manages a rich schema of tables, partitions, and helper functions. This page is a complete reference to every database object, algorithm, and runtime behavior involved.

**Who is this for?** Operators inspecting their measurements DB, developers extending pgwatch, and DBAs tuning or debugging partition behavior.

**Prerequisites:**

- [Bootstrapping the Measurements Database (Sink)](../howto/metrics_db_bootstrap.md) for initial database setup.
- [Sinks Options](sinks_options.md) for the `--sink` connection URI format.
- [CLI & Environment Variables](cli_env.md) for all referenced flags.

pgwatch supports two storage backends behind this sink: **native PostgreSQL partitioning** and **TimescaleDB**. Both are covered here, with the native Postgres path treated as the primary focus.

## Automatic Schema Bootstrap

On first startup, pgwatch checks for the existence of the `admin` schema. If it does not exist, all embedded SQL files are executed in order:

1. `admin_schema.sql` — schemas, extension, tables, template, triggers.
2. `admin_functions.sql` — all PL/pgSQL helper and maintenance functions.
3. `ensure_partition_postgres.sql` — the native partition-ensure function.
4. `ensure_partition_timescale.sql` — the TimescaleDB partition-ensure function.
5. `change_chunk_interval.sql` — TimescaleDB chunk interval tuning function.
6. `change_compression_interval.sql` — TimescaleDB compression interval tuning function.

The `--init` flag can be used to trigger this bootstrap without starting the monitoring loop, which is useful for pre-provisioning the measurements database before any sources are configured.

### Startup Validation

Three CLI parameters are validated via a single SQL query during initialization:

| Flag | Env Var | Default | Constraint |
|------|---------|---------|------------|
| `--retention` | `PW_RETENTION` | `14 days` | >= 1 hour or exactly `0` (disabled) |
| `--maintenance-interval` | `PW_MAINTENANCE_INTERVAL` | `12 hours` | >= 0 |
| `--partition-interval` | `PW_PARTITION_INTERVAL` | `1 week` | >= 1 hour |

All three values are cast to `::interval` **server-side**, so they must be valid PostgreSQL interval syntax (e.g. `'14 days'`, `'2 weeks'`, `'1 month'`). However, they are not all processed the same way:

- `--retention` and `--maintenance-interval` are converted to numeric durations via `extract(epoch from $1::interval)` and stored as Go `time.Duration` values (the epoch in seconds is multiplied by `time.Second` to get nanoseconds).
- `--partition-interval` is **only validated** as a boolean (`$3::interval >= '1h'::interval`). Its raw string value is passed directly to the `admin.ensure_partition_metric_dbname_time()` SQL function on every flush. This flag applies only to **native PostgreSQL partitioning** — the TimescaleDB path ignores it and uses the `timescale_chunk_interval` key in `admin.config` instead.

### Storage Schema Type Detection

After the bootstrap, pgwatch reads `admin.storage_schema_type` to determine whether to use native Postgres or TimescaleDB code paths.

The value is auto-detected at schema creation time:

- If the `timescaledb` extension is installed in the measurements database, the value is `'timescale'`.
- Otherwise, it defaults to `'postgres'`.

Internally this is represented as `DbStorageSchemaPostgres` or `DbStorageSchemaTimescale`. Changing this value after initialization is not supported without re-creating the schema.

## The Three Database Schemas

pgwatch creates three schemas in the measurements database. Each has a distinct role:

### `admin` — Metadata and Maintenance

Houses all configuration tables, the row template, maintenance functions, and the migration tracker. Created by `CREATE SCHEMA IF NOT EXISTS "admin"`.

### `public` — Top-Level Metric Tables

One table per distinct metric name (e.g. `public.cpu_load`, `public.wal_stats`). These are the entry point for all queries and `COPY` writes. In the native Postgres path, these tables hold no data directly — they are partitioned parents. In the TimescaleDB path, these are hypertables with automatic chunking.

### `subpartitions` — Child Partitions (Native Postgres Only)

Contains all level-2 (source/dbname) and level-3 (time-range) partitions. Created by `CREATE SCHEMA IF NOT EXISTS "subpartitions"`. Not used in the TimescaleDB path.

## The `admin` Schema in Detail

### `admin.storage_schema_type`

A single-row table that stores the active backend mode.

| Column | Type | Notes |
|--------|------|-------|
| `schema_type` | `text NOT NULL` | `'postgres'` or `'timescale'` |
| `initialized_on` | `timestamptz NOT NULL` | Defaults to `now()` |

A `CHECK` constraint enforces the two allowed values. A unique index on the constant expression `((1))` prevents more than one row from ever existing. The default value comes from `admin.get_default_storage_type()`, a SQL function that checks `pg_extension` for `timescaledb`.

### `admin.metrics_template`

The universal row shape for all metric tables:

| Column | Type | Notes |
|--------|------|-------|
| `time` | `timestamptz NOT NULL` | Defaults to `now()` |
| `dbname` | `text NOT NULL` | Source database name |
| `data` | `jsonb NOT NULL` | Metric measurements |
| `tag_data` | `jsonb` | Tag key-value pairs (nullable) |

`CHECK (false)` prevents direct inserts — this table is used only as a template via `LIKE admin.metrics_template INCLUDING INDEXES`. The default index is a B-tree on `(dbname, time)`.

!!! tip
    For very large deployments generating huge amounts of metrics data, consider replacing the default B-tree index with BRIN. The source SQL file (`admin_schema.sql`) contains commented-out alternatives for both a BRIN index on `(dbname, time)` and a GIN index on `tag_data`.

### `admin.config`

A key-value store for runtime-tunable parameters.

| Column | Type | Notes |
|--------|------|-------|
| `key` | `text NOT NULL` | Primary key |
| `value` | `text NOT NULL` | Configuration value |
| `created_on` | `timestamptz NOT NULL` | Defaults to `now()` |
| `last_modified_on` | `timestamptz` | Auto-set by trigger |

Default rows seeded at creation:

| Key | Default Value |
|-----|---------------|
| `timescale_chunk_interval` | `2 days` |
| `timescale_compress_interval` | `1 day` |

A `BEFORE UPDATE` trigger (`config_modified`) automatically sets `last_modified_on = now()` on every update via the `trg_config_modified()` function.

!!! warning
    Do not update `admin.config` rows directly for chunk or compression intervals. Use `admin.timescale_change_chunk_interval()` or `admin.timescale_change_compress_interval()` instead, as they also propagate the new value to all existing hypertables.

### `admin.all_distinct_dbname_metrics`

A lookup table for Grafana variable dropdowns.

| Column | Type | Notes |
|--------|------|-------|
| `dbname` | `text NOT NULL` | Source database name |
| `metric` | `text NOT NULL` | Metric name |
| `created_on` | `timestamptz NOT NULL` | Defaults to `now()` |

Primary key: `(dbname, metric)`.

This table is populated in two ways:

1. **Eagerly by pgwatch**: an `INSERT ... WHERE NOT EXISTS` is issued every time a new source+metric pair is encountered via `SyncMetric()`.
2. **Reconciled periodically**: `admin.maintain_unique_sources()` runs on the maintenance schedule, scanning actual data in metric tables and adding or removing stale rows.

Grafana dashboards query this table for fast `SELECT DISTINCT dbname FROM admin.all_distinct_dbname_metrics WHERE metric = '...'` instead of scanning potentially massive metric tables with millions of rows.

### `admin.migration`

Tracks applied schema migrations for the `pgx-migrator` integration.

| Column | Type | Notes |
|--------|------|-------|
| `id` | `bigint` | Primary key |
| `version` | `text NOT NULL` | Migration description |

Seeded with two rows at creation:

| ID | Version |
|----|---------|
| 0 | `01110 Apply postgres sink schema migrations` |
| 1 | `01180 Apply admin functions migrations for v5` |

The `pgwatch config upgrade` CLI command invokes the migration path.

### Extension: `btree_gin`

Installed automatically via `CREATE EXTENSION IF NOT EXISTS btree_gin`. This enables GIN indexes on scalar types, which can be useful if you opt into a GIN index on the `tag_data` column.

## The 3-Level Partitioning Hierarchy (Native Postgres)

This section applies only when `admin.storage_schema_type` is set to `'postgres'`. The TimescaleDB path is covered [below](#timescaledb-storage-path).

pgwatch uses three levels of PostgreSQL declarative partitioning:

```
public.<metric>                                    -- Level 1: PARTITION BY LIST (dbname)
  └── subpartitions.<metric>_<dbname>              -- Level 2: PARTITION BY RANGE (time)
        ├── subpartitions.<metric>_<dbname>_20250414   -- Level 3: leaf (data lives here)
        ├── subpartitions.<metric>_<dbname>_20250421
        └── subpartitions.<metric>_<dbname>_20250428
```

This design provides the fastest query performance when filtering by a single source name and a time range, because the planner can **prune at both the list (dbname) and range (time) levels**.

All three levels are created by the `admin.ensure_partition_metric_dbname_time()` function, which is invoked by the Go sink code before every batch of writes.

### Level 1 — Metric Table (`public.<metric>`)

One table per distinct metric name. Created with:

```sql
CREATE TABLE public.<metric>
  (LIKE admin.metrics_template INCLUDING INDEXES)
  PARTITION BY LIST (dbname);
COMMENT ON TABLE public.<metric> IS 'pgwatch-generated-metric-lvl';
```

The existence check uses `to_regclass('public.' || quote_ident(metric)) IS NULL`. The table inherits its column definitions and indexes from `admin.metrics_template`.

### Level 2 — Source Partition (`subpartitions.<metric>_<dbname>`)

One child per monitored source (database) within each metric:

```sql
CREATE TABLE subpartitions.<metric>_<dbname>
  PARTITION OF public.<metric>
  FOR VALUES IN ('<dbname>')
  PARTITION BY RANGE (time);
COMMENT ON TABLE subpartitions.<metric>_<dbname>
  IS 'pgwatch-generated-metric-dbname-lvl';
```

The `FOR VALUES IN (...)` clause uses the **literal dbname string**, not the (potentially hashed) table name. This means queries against the partition always use the original source name.

### Level 3 — Time Partition (`subpartitions.<metric>_<dbname>_<suffix>`)

Leaf partitions where actual measurement rows are stored:

```sql
CREATE TABLE subpartitions.<metric>_<dbname>_<suffix>
  PARTITION OF subpartitions.<metric>_<dbname>
  FOR VALUES FROM ('<start>') TO ('<end>');
COMMENT ON TABLE subpartitions.<metric>_<dbname>_<suffix>
  IS 'pgwatch-generated-metric-dbname-time-lvl';
```

The time suffix format depends on the partition period:

| Partition Period | Suffix Format | Example |
|-----------------|---------------|---------|
| >= 1 day | `YYYYMMDD` | `cpu_load_mydb_20250419` |
| >= 1 hour and < 1 day | `YYYYMMDD_HH24` | `cpu_load_mydb_20250419_14` |

The partition period minimum is **1 hour**. Anything less triggers an exception.

### First-Partition Alignment

When no existing partitions are found for a metric+dbname pair, the starting boundary is aligned using `date_trunc`:

| Period Size | Alignment |
|-------------|-----------|
| >= 1 week | `date_trunc('week', metric_timestamp)` |
| >= 1 day | `date_trunc('day', metric_timestamp)` |
| < 1 day | `date_trunc('hour', metric_timestamp)` |

This ensures clean, predictable partition boundaries.

### Extending Existing Partitions

If partitions already exist, the function reads the **maximum upper bound** from the `pg_catalog` by extracting the `TO (...)` clause from `pg_get_expr(relpartbound, ...)`. New partitions are then created starting from that point, maintaining continuity without gaps.

### Pre-Creation of Future Partitions

The `partitions_to_precreate` parameter (default: **3**) controls how many future partitions are created ahead of the current time. A loop `FOR i IN 0..partitions_to_precreate` creates the current partition plus N future ones. This avoids DDL during normal write operations when time advances into a new partition window.

### Return Values

The function returns two `OUT` parameters: `part_available_from` and `part_available_to`, representing the time bounds of the partition containing the given `metric_timestamp`. These values are cached by the Go side to avoid re-calling the function when subsequent data still fits within known bounds.

If the function cannot determine the containing partition from the creation loop (e.g. the timestamp falls within an already-existing partition), it falls back to a catalog query matching `metric_timestamp >= lower AND metric_timestamp < upper`.

## Table Name Generation and the 63-Character Limit

### The Problem

PostgreSQL's `max_identifier_length` (compile-time default: **63**) silently truncates object names that exceed it. With the naming pattern `<metric>_<dbname>_<time_suffix>`, long metric or database names can easily exceed this limit, leading to collisions or errors.

### How It Works

The ensure function reads `current_setting('max_identifier_length')::int` into a constant at runtime and checks every generated name against it.

**Level 1 names** (`public.<metric>`) use the raw metric name. No truncation logic is applied at this level.

**Level 2 names** follow this algorithm:

1. Start with the default: `metric || '_' || dbname`.
2. If `char_length(name) > max_identifier_length`:
    - Calculate the available budget: `ideal_length = max_identifier_length - char_length(metric) - 1` (the `-1` accounts for the underscore separator).
    - Replace the dbname portion with `substring(md5(dbname) from 1 for ideal_length)`.
    - Result: `<metric>_<md5_prefix_of_dbname>`.

**Level 3 names** follow a similar algorithm:

1. Start with the default: `<metric>_<dbname>_<time_suffix>`.
2. If `char_length(name) > max_identifier_length`:
    - Calculate the available budget: `ideal_length = max_identifier_length - char_length(metric) - char_length(time_suffix) - 2` (two underscores).
    - Replace the dbname portion with `substring(md5(dbname) from 1 for ideal_length)`.
    - Result: `<metric>_<md5_prefix_of_dbname>_<time_suffix>`.
    - The time suffix is **never** truncated.

### Practical Implications

- The MD5 fallback produces a **prefix** of the 32-character hex digest, not the full hash. The prefix length is calculated to exactly fill the remaining identifier budget.
- All `FOR VALUES IN (...)` clauses still use the **original** dbname string regardless of the table name. Queries and data integrity are unaffected by the hashing — it is purely cosmetic in the object names.
- If you see partition names containing hex fragments (e.g. `cpu_load_a1b2c3d4e5_20250419`) in `subpartitions`, this indicates the source database name was long enough to trigger the MD5 fallback.

## Concurrency Control: Advisory Locks

### Why Advisory Locks?

Multiple pgwatch instances or concurrent flush goroutines may try to create the same partition simultaneously. PostgreSQL DDL is not idempotent by default — concurrent `CREATE TABLE` for the same name can raise errors. Advisory locks serialize DDL without blocking normal DML.

### Lock Key Derivation (Per-Metric)

Used by `ensure_partition_metric_dbname_time`, `ensure_partition_timescale`, and `drop_source_partitions`. The algorithm:

1. Compute `md5(metric)` — a 32-character hex string.
2. Strip all non-digit characters: `regexp_replace(md5(metric), E'\\D', '', 'g')`.
3. Truncate to 10 characters: `::varchar(10)`.
4. Cast to `bigint`: `::int8`.

Called as:

```sql
PERFORM pg_advisory_xact_lock(<derived_key>);
```

This serializes all DDL (level-1, level-2, level-3 creation) for the **same metric** across all sessions. Different metrics can proceed concurrently. The lock is released automatically when the transaction commits or rolls back (the `xact` variant).

### Global Maintenance Lock

Used by `admin.maintain_unique_sources()`. A fixed key `1571543679778230000` is used with the **try** variant:

```sql
IF NOT pg_try_advisory_xact_lock(1571543679778230000) THEN
  RETURN 0;
END IF;
```

If another session already holds the lock, the function returns `0` immediately instead of blocking. This ensures only one maintenance run executes at a time across all pgwatch instances sharing the same measurements database.

## The Write Path (Go Side)

### Pipeline Overview

The Go sink processes measurements through a three-stage pipeline:

```
Write(msg) ──► poll() ──► flush(msgs) ──► COPY to PostgreSQL
  channel       batch       sort, ensure     pgx.CopyFrom
  (buffer=256)  by count    partitions,
                or timer    bulk insert
```

### `Write()` — Enqueueing

Measurements arrive as `MeasurementEnvelope` structs containing a `DBName`, `MetricName`, optional `CustomTags`, and a `Data` slice of measurement maps.

- Non-blocking send to a buffered channel (capacity: **256**).
- If the channel is full, waits up to **5 seconds** (`highLoadTimeout`), then **drops** the message silently.
- After sending (or dropping), checks the `lastError` channel for any error from the most recent flush — returns it to the caller if present.
- If the context is already cancelled, returns immediately.

### `poll()` — Batching

A dedicated goroutine accumulates messages into a local slice and flushes when **either**:

- The batch reaches **256** messages (`cacheLimit`), or
- The `BatchingDelay` ticker fires (default: **950ms**, configurable via `--batching-delay` / `PW_BATCHING_DELAY`).

After each flush, the slice is reset to zero length (capacity is retained to avoid re-allocation). The goroutine exits when the context is cancelled.

### `flush()` — Partition Ensure + COPY

This is the core function that translates in-memory measurements into on-disk PostgreSQL rows.

**Step 1: Sort by Metric Name.** Messages are sorted lexicographically by `MetricName` using `slices.SortFunc`. This ensures all data for the same metric is contiguous, enabling the `CopyFrom` iterator to process one metric at a time.

**Step 2: Compute Timestamp Bounds.** The function scans every data row in the batch to find the earliest and latest epoch timestamps:

- For the **Postgres** schema: a `map[metric]map[dbname]{StartTime, EndTime}` is built.
- For the **TimescaleDB** schema: a `map[metric]{StartTime, EndTime}` is built (dbname is not relevant for chunking).

**Step 3: Ensure Partitions Exist.**

- **Postgres path** (`EnsureMetricDbnameTime`): iterates every `(metric, dbname)` pair and calls `admin.ensure_partition_metric_dbname_time($1, $2, $3, $4)`. The function may be called **twice** per pair — once for `StartTime` and once for `EndTime` — if the data spans beyond the currently cached partition bounds. Results are cached in `partitionMapMetricDbname` to avoid redundant SQL calls.
- **TimescaleDB path** (`EnsureMetricTimescale`): calls `admin.ensure_partition_timescale($1)` once per new metric. Once a hypertable is known to exist, the SQL call is never repeated.
- After the ensure step, the `forceRecreatePartitions` flag is unconditionally reset to `false`.

**Step 4: Bulk Insert via `COPY`.** Data is written using `pgx.CopyFrom` with the `copyFromMeasurements` iterator:

- **Target table**: `pgx.Identifier{<metric_name>}` — always the `public.<metric>` table. PostgreSQL's partition routing handles inserting into the correct leaf partition.
- **Target columns**: `time`, `dbname`, `data`, `tag_data` (fixed order).
- The iterator walks the sorted envelopes, yielding rows one measurement at a time. When it encounters a different metric name, it signals the end of the current batch so `CopyFrom` can be called again for the next metric's table.
- `context.Background()` is used for `CopyFrom` (not the writer's own context), so in-flight copies are not cancelled by a shutdown signal.

### Data Serialization

Each measurement row is split into two JSON objects at write time:

- **`data`**: all key-value pairs from the measurement map.
- **`tag_data`**: keys with the `tag_` prefix are extracted from `data` (prefix stripped) and merged with any `CustomTags` from the envelope.

Both are serialized with `jsoniter.ConfigFastest.MarshalToString`. The `time` column is derived from the measurement's `epoch_ns` field as `time.Unix(0, epoch_nanos)`.

### Error Recovery: Check Constraint Violation

If `CopyFrom` fails with PostgreSQL error code **`23514`** (check_violation), this typically means a partition was dropped or is missing for the data's time range. On this error:

1. `forceRecreatePartitions` is set to `true`.
2. A warning is logged: *"Some metric partitions might have been removed..."*.
3. On the **next** `flush()` call, the partition cache is bypassed and the ensure functions are re-invoked for all `(metric, dbname)` pairs, effectively re-creating any missing partitions.

## Dummy Metric Tables

### Purpose

Grafana dashboards query `public.<metric>` tables for dropdowns and panels. If a table doesn't exist, Grafana shows error notifications. pgwatch proactively creates **empty** top-level tables for all known metrics to suppress these errors.

### `admin.ensure_dummy_metrics_table(metric)`

- If the table already exists, returns `false` (no-op).
- If the `postgres` schema type is active: creates a `PARTITION BY LIST (dbname)` table from the template.
- If the `timescale` schema type is active: delegates to `admin.ensure_partition_timescale(metric)` to create a full hypertable.
- Always adds the `pgwatch-generated-metric-lvl` comment.

### Built-in Metrics

On startup, dummy tables are created for all default built-in metrics: `sproc_changes`, `table_changes`, `index_changes`, `privilege_changes`, `object_changes`, and `configuration_changes`.

### `SyncMetric()` — Runtime Metric Sync

Called when a new source+metric pair is added at runtime. Performs two operations:

1. Inserts the `(dbname, metric)` pair into `admin.all_distinct_dbname_metrics`.
2. Ensures a dummy table exists for the metric.

Only fires on `AddOp`. `DeleteOp` and `DefineOp` are currently no-ops.

## `admin.get_top_level_metric_tables()`

A helper function used internally by several maintenance functions. Returns all table names in the `public` schema that satisfy three conditions:

1. The table is a regular table or partitioned table (`relkind IN ('r', 'p')`).
2. It has a column named `time`.
3. It has the comment `pgwatch-generated-metric-lvl`.

This prevents pgwatch from accidentally operating on user-created tables in the `public` schema.

## Data Retention and Cleanup

### Background Maintenance Schedule

A goroutine runs two functions on a timer controlled by `--maintenance-interval` (default: **12 hours**):

1. `DeleteOldPartitions()` — drops expired time partitions or chunks.
2. `MaintainUniqueSources()` — reconciles the Grafana listing table.

If `--maintenance-interval` is set to `0`, the goroutine is not started and no automatic maintenance occurs.

### Finding Old Partitions: `admin.get_old_time_partitions(older_than, schema_type)`

Returns partitions eligible for deletion.

**Postgres path**: queries `pg_class` joined with `pg_inherits` in the `subpartitions` schema. Extracts the upper bound from the partition expression using the regex pattern `TO \('...')`. Selects partitions where this bound is earlier than `now() - older_than`. Matches both the current comment (`pgwatch-generated-metric-dbname-time-lvl`) and the legacy comment (`pgwatch-generated-metric-time-lvl`) for backward compatibility.

**TimescaleDB path**: uses `show_chunks(hypertable_name, older_than)` from the TimescaleDB information views.

The `schema_type` parameter defaults to an empty string, in which case the function reads the value from `admin.storage_schema_type`.

### Dropping Old Partitions: `admin.drop_old_time_partitions(older_than, schema_type)`

**Postgres path**: for each partition returned by `get_old_time_partitions`:

1. `ALTER TABLE <parent> DETACH PARTITION <child>` — removes the partition from the hierarchy without dropping it yet.
2. `DROP TABLE IF EXISTS <child>` — physically removes the table.

The detach-then-drop pattern avoids holding exclusive locks on the parent table for the duration of the drop. Returns the count of dropped partitions.

**TimescaleDB path**: calls `drop_chunks(hypertable_name, older_than)` and returns the count.

Called from Go via `DeleteOldPartitions()` with the `--retention` value as the `older_than` parameter.

### Removing a Source: `admin.drop_source_partitions(p_source_name)`

Removes **all data** for a decommissioned source/database across every metric table.

1. Finds level-2 partitions in `subpartitions` whose `FOR VALUES IN (...)` clause matches `p_source_name` (via regex on `pg_get_expr`). Only partitions whose parent has the `pgwatch-generated-metric-lvl` comment are considered.
2. For each matching partition:
    - Acquires the **same per-metric advisory lock** used by the ensure function to prevent concurrent DDL conflicts.
    - `ALTER TABLE <public.metric> DETACH PARTITION <subpartitions.child>`.
    - `DROP TABLE IF EXISTS <subpartitions.child>`.
3. After all partitions are dropped: `DELETE FROM admin.all_distinct_dbname_metrics WHERE dbname = p_source_name`.
4. Returns the number of partitions dropped.

This function is **not called automatically** by the pgwatch daemon — it is intended for manual invocation when decommissioning a monitored source.

!!! tip
    To cleanly remove all data for a decommissioned database:
    ```sql
    SELECT admin.drop_source_partitions('my-old-database');
    ```

### Dropping All Metric Tables: `admin.drop_all_metric_tables()`

A nuclear option that drops every top-level metric table (cascading to all partitions) and truncates `admin.all_distinct_dbname_metrics`. Iterates `admin.get_top_level_metric_tables()`, executing `DROP TABLE` for each. Returns the count of tables dropped.

### Truncating All Metric Tables: `admin.truncate_all_metric_tables()`

Clears all data but **preserves table structures**. Iterates `admin.get_top_level_metric_tables()`, executing `TRUNCATE TABLE` for each. Also truncates `admin.all_distinct_dbname_metrics`. Returns the count of tables truncated.

### Reconciling the Listing Table: `admin.maintain_unique_sources()`

Reconciles `admin.all_distinct_dbname_metrics` with actual data across all metric tables. Protected by the global try-lock (`pg_try_advisory_xact_lock(1571543679778230000)`) — skips if another instance is already running.

**Per metric table, the function:**

1. Uses a **recursive CTE** to efficiently find distinct `dbname` values (faster than `SELECT DISTINCT` for large tables):

```sql
WITH RECURSIVE t(dbname) AS (
  SELECT MIN(dbname) FROM <metric>
  UNION
  SELECT (SELECT MIN(dbname) FROM <metric> WHERE dbname > t.dbname) FROM t
)
SELECT array_agg(dbname) FROM t WHERE dbname IS NOT NULL
```

2. Deletes rows from `all_distinct_dbname_metrics` for dbnames no longer present in the metric table.
3. Inserts rows for dbnames present in the metric table but missing from the listing.

**After processing all metrics:** deletes listing rows whose `metric` column references a table that no longer exists (dropped metric tables).

Returns the total number of rows affected across all operations.

## TimescaleDB Storage Path

When TimescaleDB is the active backend, the partitioning model is significantly simpler: only Level 1 tables exist in `public`, and chunk management is delegated entirely to the TimescaleDB extension.

### `admin.ensure_partition_timescale(metric)`

Creates the metric hypertable only if it does not already exist.

1. Acquires the same per-metric advisory lock as the native path.
2. Reads chunk and compression intervals from `admin.config`:
    - `timescale_chunk_interval` — default `2 days`.
    - `timescale_compress_interval` — default `1 day`.
3. Creates the table from the template: `CREATE TABLE IF NOT EXISTS public.<metric> (LIKE admin.metrics_template INCLUDING INDEXES)`.
4. Adds the standard `pgwatch-generated-metric-lvl` comment.
5. Converts it to a hypertable: `SELECT create_hypertable(...)` with the configured `chunk_time_interval`.
6. Enables compression segmented by `dbname`: `ALTER TABLE ... SET (timescaledb.compress, timescaledb.compress_segmentby = 'dbname')`.
7. Adds an automatic compression policy: `SELECT add_compression_policy(...)` with the configured compression interval.

The `subpartitions` schema is not used in this path.

### Changing the Chunk Interval: `admin.timescale_change_chunk_interval(new_interval)`

Upserts the `timescale_chunk_interval` key in `admin.config`, then iterates all existing hypertables in the `public` schema (from `_timescaledb_catalog.hypertable`) and calls `set_chunk_time_interval(metric, new_interval)` for each. Affects both existing and future tables.

### Changing the Compression Interval: `admin.timescale_change_compress_interval(new_interval)`

Upserts the `timescale_compress_interval` key in `admin.config`, then for each existing hypertable:

1. Updates the chunk time interval via `set_chunk_time_interval`.
2. Adjusts the compression policy based on the TimescaleDB version:
    - **>= 2.0**: `remove_compression_policy` followed by `add_compression_policy`.
    - **< 2.0**: `remove_compress_chunks_policy` followed by `add_compress_chunks_policy`.

### Retention with TimescaleDB

`admin.drop_old_time_partitions` delegates to `drop_chunks(...)` — the native TimescaleDB chunk removal mechanism — rather than the manual detach/drop loop used for native Postgres partitions.

## Migrations

### Migration Framework

pgwatch uses the [`pgx-migrator`](https://github.com/cybertec-postgresql/pgx-migrator) library for schema versioning. The migration table is `admin.migration`.

- `NeedsMigration()` checks if any pending migrations exist.
- `Migrate()` applies all pending migrations in order.
- The CLI command `pgwatch config upgrade` triggers the migration path.

### Current Migrations

| ID | Name | Action |
|----|------|--------|
| 0 | `01110 Apply postgres sink schema migrations` | No-op (the migration table itself is bootstrapped) |
| 1 | `01180 Apply admin functions migrations for v5` | Drops legacy functions (`ensure_partition_metric_time`, `get_old_time_partitions(integer, text)`, `drop_old_time_partitions(integer, boolean, text)`) and re-applies `ensure_partition_postgres.sql` and `admin_functions.sql` |

### Adding New Migrations

When adding a new migration, two things must be updated:

1. Add a new `&migrator.Migration{}` entry in the Go source (`internal/sinks/postgres.go`).
2. Add a corresponding row to the `INSERT INTO admin.migration` statement in `admin_schema.sql` and update `sinkSchema` in `cmd/pgwatch/version.go`.

## Grafana Integration

`admin.all_distinct_dbname_metrics` is the primary data source for dashboard variable queries:

```sql
SELECT DISTINCT dbname
FROM admin.all_distinct_dbname_metrics
WHERE metric = 'cpu_load'
ORDER BY 1;
```

This avoids expensive full-table scans on metric tables that may contain millions of rows.

For security-sensitive environments, create a read-only Grafana user and grant access to all relevant schemas:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE pgwatch IN SCHEMA public
  GRANT SELECT ON TABLES TO pgwatch_grafana;
ALTER DEFAULT PRIVILEGES FOR ROLE pgwatch IN SCHEMA subpartitions
  GRANT SELECT ON TABLES TO pgwatch_grafana;

GRANT USAGE ON SCHEMA public TO pgwatch_grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO pgwatch_grafana;

GRANT USAGE ON SCHEMA admin TO pgwatch_grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA admin TO pgwatch_grafana;

GRANT USAGE ON SCHEMA subpartitions TO pgwatch_grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA subpartitions TO pgwatch_grafana;
```

## CLI Parameters Reference

All sink-specific flags that affect the behavior described on this page:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--sink` | `PW_SINK` | *(none)* | URI where metrics will be stored; can be used multiple times |
| `--batching-delay` | `PW_BATCHING_DELAY` | `950ms` | Sink-specific batching flush delay; may be ignored by some sinks |
| `--partition-interval` | `PW_PARTITION_INTERVAL` | `1 week` | Time range for native PostgreSQL sink time partitions. Not used by TimescaleDB (which uses `admin.config` instead). Must be a valid PostgreSQL interval |
| `--retention` | `PW_RETENTION` | `14 days` | Delete metrics older than this; `0` to disable. Must be a valid PostgreSQL interval |
| `--maintenance-interval` | `PW_MAINTENANCE_INTERVAL` | `12 hours` | Run maintenance tasks (retention, listing cleanup) at this interval; `0` to disable. Must be a valid PostgreSQL interval |
| `--real-dbname-field` | `PW_REAL_DBNAME_FIELD` | `real_dbname` | Tag key for real database name |
| `--system-identifier-field` | `PW_SYSTEM_IDENTIFIER_FIELD` | `sys_id` | Tag key for system identifier value |

## Object Comment Tags Reference

pgwatch identifies its own tables in the PostgreSQL catalog using `COMMENT ON TABLE`. These tags are used by maintenance functions to distinguish pgwatch-managed objects from user tables.

| Comment String | Applied To | Meaning |
|----------------|-----------|---------|
| `pgwatch-generated-metric-lvl` | `public.<metric>` | Top-level metric table |
| `pgwatch-generated-metric-dbname-lvl` | `subpartitions.<metric>_<dbname>` | Level-2 source partition |
| `pgwatch-generated-metric-dbname-time-lvl` | `subpartitions.<metric>_<dbname>_<suffix>` | Level-3 time partition (leaf) |
| `pgwatch-generated-metric-time-lvl` | *(legacy)* | Old-style time partition; still recognized by retention for backward compatibility |

## Quick Reference: All `admin` Schema Objects

### Tables

| Object | Purpose |
|--------|---------|
| `admin.storage_schema_type` | Single-row table storing `'postgres'` or `'timescale'` |
| `admin.metrics_template` | Row shape template (`CHECK (false)`, never holds data) |
| `admin.config` | Key-value configuration (TimescaleDB chunk/compression intervals) |
| `admin.all_distinct_dbname_metrics` | Grafana dropdown listing of `(dbname, metric)` pairs |
| `admin.migration` | Schema migration version tracker |

### Functions

| Function | Purpose |
|----------|---------|
| `admin.get_default_storage_type()` | Auto-detect `postgres` vs `timescale` based on installed extensions |
| `admin.ensure_dummy_metrics_table(metric)` | Create an empty top-level metric table to suppress Grafana errors |
| `admin.ensure_partition_metric_dbname_time(...)` | 3-level partition ensure for native PostgreSQL partitioning |
| `admin.ensure_partition_timescale(metric)` | Hypertable ensure for TimescaleDB |
| `admin.get_top_level_metric_tables()` | List all pgwatch-managed metric tables in `public` |
| `admin.get_old_time_partitions(older_than, schema_type)` | Find expired time partitions or chunks |
| `admin.drop_old_time_partitions(older_than, schema_type)` | Drop expired time partitions or chunks |
| `admin.drop_source_partitions(p_source_name)` | Remove all data for a decommissioned source |
| `admin.drop_all_metric_tables()` | Drop every metric table and all partitions |
| `admin.truncate_all_metric_tables()` | Truncate every metric table (preserves structure) |
| `admin.maintain_unique_sources()` | Reconcile the Grafana listing table with actual data |
| `admin.timescale_change_chunk_interval(interval)` | Update TimescaleDB chunk interval for all hypertables |
| `admin.timescale_change_compress_interval(interval)` | Update TimescaleDB compression interval for all hypertables |
| `trg_config_modified()` | Trigger function: sets `last_modified_on` on `admin.config` updates |
