---
title: Metric definitions
---

## What is metric?

Metrics are named SQL queries that return a timestamp and
anything else you find helpful. Most metrics have different query
text versions for different target PostgreSQL versions, also optionally
considering primary and/or replica states.

``` sql
-- a sample metric
SELECT
  (extract(epoch from now()) * 1e9)::int8 as epoch_ns,
  extract(epoch from (now() - pg_postmaster_start_time()))::int8 as postmaster_uptime_s,
  case when pg_is_in_recovery() then 1 else 0 end as in_recovery_int;
```

The correct version of the metric definition will be chosen automatically by
regularly connecting to the target database and checking the Postgres
version, recovery state, and if the monitoring user is a superuser or
not.

## Built-in metrics and presets

There's a good set of pre-defined metrics & metric configs provided by
the pgwatch project to cover all typical needs, but when monitoring
hundreds of hosts you'd typically want to develop some custom *Preset
Configs* or at least adjust the metric fetching intervals according to
your monitoring goals.

Some things to note about the built-in metrics:

- Only half of them are included in the *Preset configs* and are
ready for direct usage. The rest need extra extensions or
privileges, OS-level tool installations, etc. To see what's possible,
just browse the sample metrics.
- Some built-in metrics are marked to be only executed when a server is a
primary or, conversely, a standby. One can inspect the flags and set them
on the Web UI Metrics tab or in the YAML file, changing the metric
definition.
- Some unique preset metrics have some
non-standard behavior attached to them, e.g., `change_events`, `recommendations`,
`server_log_event_counts`, `instance_up`, etc.

### change_events

The "change_events" built-in metric tracks DDL & config
changes. Internally, it uses some other `*_hashes` metrics that
are not meant to be used independently. Such metrics
should not be removed.

### recommendations

When enabled, this metric will find all
other metrics starting with `reco_*` and execute those
queries. The metric targets performance,
security, and other "best practices" violations. Users can add
new `reco_*` queries freely.

### server_log_event_counts

This metric enables the Postgres server log "tailing" for errors. It can't
be used for remote setups, though, unless the DB logs are
somehow mounted or copied over, as real file access is needed.
For details, see the [Log parsing](../reference/advanced_features.md#log-parsing) chapter.

### instance_up

For standard metrics there will be no data rows stored when the DB
is not reachable, but for this one, there will be a zero stored for
the "is_up" column that, under normal operations, would always
be 1. This metric can be used to calculate some "uptime" SLA
indicator, for example.

### archiver

This metric retrieves key statistics from the PostgreSQL `pg_stat_archiver` view
providing insights into the status of WAL file archiving.
It returns the total number of successfully archived files and failed archiving attempts.
Additionally, it identifies if the most recent attempt
resulted in a failure and calculates how many seconds have passed since the last failure.
The metric only considers data if WAL archiving is
enabled in the system, helping administrators monitor and diagnose issues related to the archiving process.

### backends

This metric gathers detailed information from the PostgreSQL `pg_stat_activity` view, providing an overview of the database's current session and activity
state. It tracks the total number of client backends, active sessions, idle sessions, sessions waiting on locks, and background
workers. The metric also calculates statistics on blocked sessions, most extended waiting times, average and longest session durations, transaction times,
and query durations. Additionally, it monitors autovacuum worker activity and provides the age of the oldest transaction (measured by `xmin`). This
metric helps administrators monitor session states, detect bottlenecks, and ensure the system is within its connection limits, providing visibility
into database performance and contention.

### bgwriter

This metric retrieves statistics from the `pg_stat_bgwriter` view, providing information about the background writer process in PostgreSQL. It reports the number of buffers cleaned (written to disk) by the background writer, how many times buffers were written because the background writer reached the maximum limit (`maxwritten_clean`), and the total number of buffers allocated. Additionally, it calculates the time in seconds since the last reset of these statistics. This metric helps monitor the efficiency and behavior of PostgreSQL's background writer, which plays a crucial role in managing I/O by writing modified buffers to disk, thus helping to ensure smooth database performance.

### blocking_locks

This metric provides information about lock contention in PostgreSQL by identifying sessions waiting for locks and the sessions holding those locks.
It captures details from the `pg_locks` view and the `pg_stat_activity` view to highlight the interactions between the waiting and blocking sessions.
The result helps identify which queries are causing delays due to lock contention, the type of locks involved, and the users or sessions responsible for holding or
 waiting on locks. This metric helps diagnose performance bottlenecks related to database lock.

### checkpointer

This metric provides insights into the activity and performance of PostgreSQL's checkpointer process, which ensures that modified data pages are regularly written to disk to maintain consistency. It tracks the number of checkpoints triggered either by the system's timing or specific requests, as well as how many restart points have been completed in standby environments. Additionally, it measures the time spent writing and synchronizing buffers to disk, the total number of buffers written, and how long it has been since the last reset of these statistics. This metric helps administrators understand how efficiently the system handles checkpoints and whether there might be I/O performance issues related to the frequency or duration of checkpoint operations.

### db_stats

This metric provides a comprehensive overview of various performance and health statistics for the current PostgreSQL database. It tracks key metrics such as the number of active database connections (`numbackends`), transaction statistics (committed, rolled back), block I/O (blocks read and hit in the cache), and tuple operations (rows returned, fetched, inserted, updated, deleted). Additionally, it monitors conflicts, temporary file usage, deadlocks, and block read/write times.

The metric also includes system uptime by calculating how long the PostgreSQL `postmaster` process has been running and tracks checksum failures and the time since the last checksum failure. It identifies if the database is in recovery mode, retrieves the system identifier, and tracks session-related statistics such as total session time, active time, idle-in-transaction time, and abandoned, fatal, or killed sessions.

Lastly, it monitors the number of invalid indexes not being rebuilt. This metric helps database administrators gain insights into overall database performance, transaction behavior, session activity, and potential index-related issues, which are critical for efficient database management and troubleshooting.

### wal

This metric tracks key information about the PostgreSQL system's write-ahead logging (WAL) and recovery state. It calculates the current WAL location, showing how far the system has progressed in terms of WAL writing or replaying if in recovery mode. The metric also indicates whether the database is in recovery, monitors the system's uptime since the `postmaster` process started, and provides the system's unique identifier. Additionally, it retrieves the current timeline, which is essential for tracking the state of the WAL log and recovery process. This metric helps administrators monitor database health, especially regarding recovery and WAL operations.

### locks

This metric identifies lock contention in the PostgreSQL database by tracking sessions waiting for locks and the corresponding sessions holding those locks. It examines active queries in the current database and captures detailed information about waiting and blocking sessions. For each waiting session, it records the lock type, user, lock mode, the query being executed, and the table involved. Similarly, for the session holding the lock, it captures the same details. This helps database administrators identify queries causing delays due to lock contention, enabling them to troubleshoot performance issues and optimize query execution.

### kpi

This metric provides a detailed overview of PostgreSQL database performance and activity. It tracks the current WAL (Write-Ahead Log) location, the number of active and blocked backends, and the oldest transaction time. It calculates the total transaction rate (TPS) by summing committed and rolled-back transactions and specific statistics on table and index performance, such as the number of sequential scans on tables larger than 10MB and the number of function calls.

Additionally, the metric tracks block read and write times, the amount of temporary bytes used, deadlocks, and whether the database is in recovery mode. Finally, it calculates the uptime of the PostgreSQL `postmaster` process. This information helps administrators monitor and manage system performance, detect potential bottlenecks, and optimize query and transaction behavior.

### stat_statements

This metric provides detailed statistics about the performance and resource usage of SQL queries executed on the PostgreSQL database. It collects data from the `pg_stat_statements` view, focusing on queries executed more than five times and having significant execution time (greater than 5 milliseconds). It aggregates essential performance metrics for each query, such as:

- **Execution metrics**: Total number of executions (`calls`), total execution time, and total planning time.
- **I/O metrics**: Blocks read and written (both shared and temporary), blocks dirtied, and associated read/write times.
- **WAL metrics**: WAL (Write-Ahead Log) bytes generated and the number of WAL full-page images (FPI).
- **User activity**: The users who executed the queries and a sample of the query text.

The metric ranks queries based on different performance factors, including execution time, number of calls, block reads/writes, and temporary block usage, and it limits the results to the top 100 queries in each category. This helps administrators identify resource-intensive queries, optimize database performance, and improve query efficiency by focusing on those that consume the most I/O or take the longest to execute.

### table_stats

This metric collects and summarizes detailed information about table sizes, table activity, and maintenance operations in PostgreSQL. It tracks both individual tables and partitioned tables, including their root partitions. The metric calculates the size of each table (in bytes) and other key statistics like sequential scans, index scans, tuples inserted, updated, or deleted, and the number of live and dead tuples. It also tracks maintenance operations like a vacuum and analyze runs, as well as whether autovacuum is disabled for specific tables.

For partitioned tables, the metric aggregates the statistics across all partitions and summarizes the partitioned table as a whole, marking it as the root partition. Additionally, it calculates the time since the last vacuum and analyze operations, and captures the transaction freeze age for each table, which helps monitor when a table might need a vacuum to prevent transaction wraparound.

By focusing on tables larger than 10MB and ignoring temporary and system tables, this metric helps database administrators monitor the largest and most active tables in their database, ensuring that maintenance operations like vacuum and analyze are running effectively and identifying tables that may be contributing to performance bottlenecks due to size or activity.

## Custom metrics

To work with custom metrics definitions, you should adhere to a couple of basic
concepts:

- Every metric query should have an `epoch_ns` (nanoseconds since
epoch column) to record the metrics reading time. If the column is
not there, things will still work, but the server timestamp of the
metrics gathering daemon will be used, so a slight loss (assuming
intra-datacenter monitoring with little lag) of precision occurs.

- Queries should only return text, integer, boolean, or floating point
(a.k.a. double precision) Postgres data types. Note that columns
with NULL values are not stored in the data layer, as it's a
bit bothersome to work with NULLs!

- Column names should be descriptive enough so that they're
self-explanatory, but not too long as it costs storage.

- Metric queries should execute fast - at least below the selected
*Statement timeout* (default 5s).

- Columns can be optionally "tagged" by prefixing them with
`tag_`. By doing this, the column data will be indexed by Postgres, providing the sophisticated auto-discovery support for indexed keys/values when building charts with Grafana and faster queries on those columns.

- All fetched metric rows can be "prettified" with custom
static key-value data per host. Use the "Custom tags" Web UI field for the monitored DB entry or the "custom_tags" YAML
field. Note that this works per host and applies to all metrics.

- For Prometheus the numerical columns are by default mapped to a
Value Type of "Counter" (as most Statistics Collector columns are
cumulative), but when this is not the case and the column is a
"Gauge" then according column attributes should be declared. See the section on column attributes for details.

- For Prometheus all text fields will be turned into tags/labels as
only floats can be stored!

## Adding and using a custom metric

### For *Config DB* based setups

1. Go to the Web UI "METRICS" page and press the "+ NEW" button.
1. Fill out the template - pick a name for your metric, select the minimum
 supported PostgreSQL version, and insert the query text and any
 extra attributes, if any (see below for options). Hit the "ADD METRIC"
 button to store.
1. Activate the newly added metric by including it in some existing
    *Preset Config* in the "PRESETS" page or add it directly to the monitored DB,
 together with an interval, into the "METRICS" tab when editing a source on the "SOURCES" page.

### For *YAML* based setups

1. Edit or create a copy of the `metrics.yaml` file shipped with the installation.
1. Add a new entry to the `metrics` array and optionally add a new metric
in some existing or new *Preset*.

Here is the structure of a metric definition in YAML format:

```yaml
    metric_name:
        description: "A short description of the metric"
        init_sql: |
            CREATE EXTENSION IF NOT EXISTS some_extension;
            CREATE OR REPLACE FUNCTION get_some_stat(OUT some_stat int)
            ...
        sqls:
            11: |
                select /* pgwatch_generated */
                (extract(epoch from now()) * 1e9)::int8 as epoch_ns,
                ...
            14: |
                select /* pgwatch_generated */
                (extract(epoch from now()) * 1e9)::int8 as epoch_ns,
                ...
        gauges:
            - some_column1
            - some_column2
            - * # for all columns
        is_instance_level: true
        node_status: primary/standby
        statement_timeout_seconds: 300
        metric_storage_name: some_other_metric_name
```

- *init_sql*

    Optional SQL code that must be executed before the metric query
    itself. DBA can use it to create helper functions, install
    extensions, etc. Usually executed during the [preparation database](../tutorial/preparing_databases.md)
    for monitoring with superuser privileges.

    As mentioned in [Metrics initialization](../tutorial/preparing_databases.md#metrics-initialization)
    section, Postgres knows very little about the operating system that it's running on, so in some cases, it might be
    advantageous to also monitor some basic OS statistics together with the
    PostgreSQL ones to get a better head start when troubleshooting
    performance problems. But as setup of such OS tools and linking the
    gathered data is not always trivial, pgwatch has a system of *helpers*
    for fetching such data.

    One can invent and install such *helpers* on the monitored databases
    freely to expose any information needed via Python,
    or any other procedure language supported by Postgres.

- *sqls*

    The actual metric query texts. The key is the minimum supported
    PostgreSQL version. The query text should be a valid SQL query
    that returns a timestamp column named `epoch_ns` and any other
    columns you want to store. The `pgwatch_generated` comment is
    good for debugging purposes and filtering out the
    queries from the logs.

    !!! Notice
        Note the "minimally supported" part - i.e.
        if your query will work from version v11.x to v17.x, then you only
        need one entry called "11". If there was a breaking change in
        the internal catalogs at v13 so that the query stopped working,
        you need a new entry named "13" that will be used for all
        versions above v13.

- *gauges*

    List of columns that should be treated as gauges. By default, all
    columns are treated as counters. Gauges are metrics that can go
    up and down, like the number of active connections. Counters are
    metrics that only go up, like the number of queries executed. This
    property is only relevant for Prometheus output.

- *is_instance_level*

    Enables caching, i.e., sharing of metric data between various
    databases of a single instance to reduce the load on the monitored
    server.

- *node_status*

    If set to "primary" or "standby," the metric will only be executed
    when the server is in that state. This property is helpful for only relevant metrics for primary or standby servers.

- *statement_timeout_seconds*

    The maximum time in seconds the metric query is allowed to run
    before it's killed. The default is 5 seconds.

- *metric_storage_name*

    Enables dynamic "renaming" of metrics at the storage level, i.e.
    declaring almost similar metrics with different names, but the data
    will be stored under one metric. Currently used (for out-of-the-box
    metrics) only for the `stat_statements_no_query_text` metric to
    not store actual query texts from the "pg_stat_statements"
    extension for more security-sensitive instances.
