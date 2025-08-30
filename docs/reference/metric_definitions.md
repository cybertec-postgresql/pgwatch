---
title: Metric definitions
---

## What is a metric?

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

## What is a preset?

Presets in pgwatch are named collections of `metric_name: time interval` pairs,
defined once and reused across multiple monitoring targets for convenience and consistency.

## Built-in metrics and presets

There's a good set of pre-defined metrics, metrics configs, and presets
provided by the pgwatch project to cover all typical needs.  
However, when monitoring hundreds of hosts, you'd typically want to define **custom metrics and/or presets**
or at least adjust the metric fetching intervals according to your monitoring goals.

You can find the full list in pgwatch's default [metrics.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/internal/metrics/metrics.yaml) file,
for a more user-friendly experience, consider browsing them via the [Web UI](../gallery/webui.md)

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
non-standard behavior attached to them, e.g., `change_events`, `server_log_event_counts`,
`instance_up`, etc.

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

!!! Info
    The default `metrics.yml` file is loaded into the config DB on bootstrap
    no need to add it manually.

1. Go to the [Web UI](../gallery/webui.md) "METRICS" page and press the "+ NEW" button.
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
        storage_name: some_other_metric_name
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

- *storage_name*

    Enables dynamic "renaming" of metrics at the storage level, i.e.
    declaring almost similar metrics with different names, but the data
    will be stored under one metric. Currently used (for out-of-the-box
    metrics) only for the `stat_statements_no_query_text` metric to
    not store actual query texts from the "pg_stat_statements"
    extension for more security-sensitive instances.
