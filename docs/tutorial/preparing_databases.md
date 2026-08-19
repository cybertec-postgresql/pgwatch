---
title: Preparing databases for monitoring
---

This tutorial walks you through the steps you take on each database you want pgwatch to monitor. For background on why helper functions are sometimes needed and which metrics rely on them, see [Concept: OS metrics via PL/Python helpers](../concept/os_helpers.md). For the catalogue of source kinds you can pick on the Sources page, see [Reference: Source types](../reference/source_types.md).

## What you'll do

1. Create a monitoring role and grant it the right privileges.
2. Enable `pg_stat_statements` and `track_io_timing`.
3. (Only for metrics that need OS-level data) install the helper functions exposed by the metrics.

## Step 1 — Create a monitoring role

The recommended role is `pgwatch` with the `pg_monitor` privilege granted:

```sql
CREATE ROLE pgwatch WITH LOGIN PASSWORD 'secret';
-- For critical databases it might make sense to ensure that the user account
-- used for monitoring can only open a limited number of connections
-- (there are according checks in code, but multiple instances might be launched)
ALTER ROLE pgwatch CONNECTION LIMIT 5;
GRANT pg_monitor TO pgwatch;
GRANT CONNECT ON DATABASE mydb TO pgwatch;
GRANT EXECUTE ON FUNCTION pg_stat_file(text) to pgwatch; -- for wal_size metric
GRANT EXECUTE ON FUNCTION pg_stat_file(text, boolean) TO pgwatch;
```

If you pick a different role name, adjust the helper-creation SQL scripts accordingly: they grant `EXECUTE` to `pgwatch` by default.

## Step 2 — Enable `pg_stat_statements`

`pg_stat_statements` powers the *Stat statements Top* dashboard and many panels across other dashboards. Without it, those panels will be empty.

1. Install the Postgres `contrib` package:

    - Debian/Ubuntu: `apt install postgresql-contrib`
    - RedHat/CentOS: `yum install -y postgresqlXY-contrib`

2. Add the extension to `shared_preload_libraries` and enable I/O timing, then restart the server:

    ```ini
    shared_preload_libraries = 'pg_stat_statements'
    track_io_timing = on
    ```

3. Activate the extension in the database (requires superuser):

    ```bash
    psql -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements"
    ```

## Step 3 — Install helper functions (only if you need OS-level metrics)

Some built-in metrics — `cpu_load`, `psutil_*`, `wal_size`, and a few others — depend on helper functions that don't ship with vanilla Postgres. If you don't enable these metrics you can skip this step.

1. Find out which helpers a metric needs:

    ```bash
    pgwatch metric print-init cpu_load
    ```

    The output is a SQL transaction you can review.

2. Run the init SQL as a superuser on each monitored database. The simplest path is to pipe it through `psql`:

    ```bash
    export PGUSER=superuser
    pgwatch metric print-init cpu_load psutil_mem psutil_disk | psql -d mydb
    ```

    !!! hint
        If many databases will be created on this instance over time, install the helpers in `template1` so every new database inherits them.

3. (Optional) If you'd rather have pgwatch create the helpers on startup, pass `--create-helpers` to the gatherer. This is **not** the default — pgwatch runs with the least-privilege principle.

## Defaults to be aware of

- The gatherer's default statement timeout for metric queries is **5 seconds**.
- For most preset workloads, metric collection adds only a few milliseconds of overhead per source per tick.

## Upgrades

When you do a binary-in-place PostgreSQL upgrade (`pg_upgrade`), helper functions on the cluster being upgraded may need to be dropped and re-installed afterwards — run the relevant `pgwatch metric print-init | psql` for each helper you rely on.
