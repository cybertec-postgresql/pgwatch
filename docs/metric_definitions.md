---
title: Metric definitions
---

## Whats is metric?

Metrics are named SQL queries that return a timestamp and pretty much
anything else you find useful. Most metrics have many different query
text versions for different target PostgreSQL versions, also optionally
taking into account primary / replica state and as of v1.8 also versions
of installed extensions.

``` sql
-- a sample metric
SELECT
  (extract(epoch from now()) * 1e9)::int8 as epoch_ns,
  extract(epoch from (now() - pg_postmaster_start_time()))::int8 as postmaster_uptime_s,
  case when pg_is_in_recovery() then 1 else 0 end as in_recovery_int;
```

Correct version of the metric definition will be chosen automatically by
regularly connecting to the target database and checking the Postgres
version, recovery state, and if the monitoring user is a superuser or
not. 

## Built-in metrics and presets

There's a good set of pre-defined metrics & metric configs provided by
the pgwatch3 project to cover all typical needs, but when monitoring
hundreds of hosts you'd typically want to develop some custom *Preset
Configs* or at least adjust the metric fetching intervals according to
your monitoring goals.

Some things to note about the built-in metrics:

-   Only a half of them are included in the *Preset configs* and are
    ready for direct usage. The rest need some extra extensions or
    privileges, OS level tool installations etc. To see what's possible
    just browse the sample metrics.
-   Some builtin metrics are marked to be only executed when server is a
    primary or conversely, a standby. The flags can be inspected / set
    on the Web UI Metrics tab or in YAML mode by suffixing the metric
    definition with "standby" or "master". 
-   There are a couple of special preset metrics that have some
    non-standard behaviour attached to them:

    - *change_events*

        The "change_events" built-in metric, tracking DDL & config
        changes, uses internally some other "*_hashes" metrics which
        are not meant to be used on their own. Such metrics are
        described also accordingly on the Web UI /metrics page and they
        should not be removed.

    - *recommendations*

        When enabled (i.e. `interval > 0`), this metric will find all
        other metrics starting with "reco_\*" and execute those
        queries. The purpose of the metric is to spot some performance,
        security and other "best practices" violations. Users can add
        new "reco_\*" queries freely.

    - *server_log_event_counts*

        This enables Postgres server log "tailing" for errors. Can't
        be used for "pull" setups though unless the DB logs are
        somehow mounted / copied over, as real file access is needed.
        See the [Log parsing](advanced_features.md#log-parsing) chapter for details.

    - *instance_up*

        For normal metrics there will be no data rows stored if the DB
        is not reachable, but for this one there will be a 0 stored for
        the "is_up" column that under normal operations would always
        be 1. This metric can be used to calculate some "uptime" SLA
        indicator for example.

## Custom metrics

For defining metrics definitions you should adhere to a couple of basic
concepts:

-   Every metric query should have an `epoch_ns` (nanoseconds since
    epoch column to record the metrics reading time. If the column is
    not there, things will still work but server timestamp of the
    metrics gathering daemon will be used, some a small loss (assuming
    intra-datacenter monitoring with little lag) of precision occurs.

-   Queries should only return text, integer, boolean or floating point
    (a.k.a. double precision) Postgres data types. Note that columns
    with NULL values are not stored at all in the data layer as it's a
    bit bothersome to work with NULLs!

-   Column names should be descriptive enough so that they're
    self-explanatory, but not over long as it costs also storage

-   Metric queries should execute fast - at least below the selected
    *Statement timeout* (default 5s)

-   Columns can be optionally "tagged" by prefixing them with
    `tag_`. By doing this, the column data will be indexed by the
    Postgres giving following advantages:
    -   Sophisticated auto-discovery support for indexed keys/values,
        when building charts with Grafana.
    -   Faster queries for queries on those columns.

-   All fetched metric rows can also be "prettyfied" with any custom
    static key-value data, per host. To enable use the "Custom tags"
    Web UI field for the monitored DB entry or "custom_tags" YAML
    field. Note that this works per host and applies to all metrics.

-   For Prometheus the numerical columns are by default mapped to a
    Value Type of "Counter" (as most Statistics Collector columns are
    cumulative), but when this is not the case and the column is a
    "Gauge" then according column attributes should be declared. See
    below section on column attributes for details.

-   For Prometheus all text fields will be turned into tags / labels as
    only floats can be stored!

## Adding and using a custom metric

### For *Config DB* based setups:

1.  Go to the Web UI "Metric definitions" page and scroll to the
    bottom.
1.  Fill the template - pick a name for your metric, select minimum
    supported PostgreSQL version and insert the query text and any
    extra attributes if any (see below for options). Hit the "New"
    button to store.
1.  Activate the newly added metric by including it in some existing
    *Preset Config* (listed on top of the page) or add it directly in
    JSON form, together with an interval, into the "Custom metrics
    config" filed on the "DBs" page.

### For *YAML* based setups:

1.  Create a new folder for the metric under
    `/etc/pgwatch3/metrics`. The folder name will be the metric's
    name, so choose wisely.
1.  Create a new subfolder for each "minimally supported Postgres
    version and insert the metric's SQL definition into a file
    named "metric.sql". 
    
    !!! Notice
        Note the "minimally supported" part - i.e.
        if your query will work from version v11.0 to v17 then you only
        need one entry called "11". If there was a breaking change in
        the internal catalogs at v13 so that the query stopped working,
        you need a new entry named "13" that will be used for all
        versions above v13.

1.  Activate the newly added metric by including it in some existing
    *Preset Config* or add
    it directly to the YAML config "custom_metrics" section.

## Metric attributes

The ehaviour of plain metrics can be extended with a set of
attributes that will modify the gathering in some way. The attributes
are stored in YAML files called *metric_attrs.yaml" in a metrics root
directory or in the `metric_attribute` Config DB table.

Currently supported attributes are:

- *is_instance_level*

    Enables caching, i.e. sharing of metric data between various
    databases of a single instance to reduce load on the monitored
    server.

    ```yaml
            wal:
                sqls:
                    11: |
                        select /* pgwatch3_generated */
                        ...
                gauges:
                    - '*'
                is_instance_level: true
    ```

- *statement_timeout_seconds*

    Enables to override the default 'per monitored DB' statement
    timeouts on metric level.

- *metric_storage_name*

    Enables dynamic "renaming" of metrics at storage level, i.e.
    declaring almost similar metrics with different names but the data
    will be stored under one metric. Currently used (for out-of-the box
    metrics) only for the `stat_statements_no_query_text` metric, to
    not to store actual query texts from the "pg_stat_statements"
    extension for more security sensitive instances.

- *extension_version_based_overrides*
    
    Enables to "switch out" the query text from some other metric
    based on some specific extension version. See 'reco_add_index' for
    an example definition.

- *disabled_days*
    
    Enables to "pause" metric gathering on specified days. See
    `metric_attrs.yaml` for "wal" for an example.

- *disabled_times*
    
    Enables to "pause" metric gathering on specified time intervals.
    e.g. "09:00-17:00" for business hours. Note that if time zone is
    not specified the server time of the gather daemon is used.
    disabled_days / disabled_times can also be defined both on metric
    and host (host_attrs) level.


## Column attributes

Besides the *\_tag* column prefix modifier, it's also possible to
modify the output of certain columns via a few attributes. It's only
relevant for Prometheus output though currently, to set the correct data
types in the output description, which is generally considered a
nice-to-have thing anyways. For YAML based setups this means adding a
"column_attrs.yaml" file in the metric's top folder and for Config DB
based setup an according "column_attrs" JSON column should be filled
via the Web UI.

Supported column attributes:

- *gauges*

    Describe the mentioned output columns as of TYPE *gauge*, i.e. the
    value can change any time in any direction. Default TYPE for
    pgwatch3 is *counter*.

    ```yaml
        table_stats_approx:
            sqls:
                11: |
                    ...
            gauges:
                - table_size_b
                - total_relation_size_b
                - toast_size_b
                - seconds_since_last_vacuum
                - seconds_since_last_analyze
                - n_live_tup
                - n_dead_tup
            metric_storage_name: table_stats
    ```

# Adding metric fetching helpers

As mentioned in [Helper Functions](preparing_databases.md#rolling-out-helper-functions)
section, Postgres knows very little about the Operating
System that it's running on, so in some (most) cases it might be
advantageous to also monitor some basic OS statistics together with the
PostgreSQL ones, to get a better head start when troubleshooting
performance problems. But as setup of such OS tools and linking the
gathered data is not always trivial, pgwatch3 has a system of *helpers*
for fetching such data.

One can invent and install such *helpers* on the monitored databases
freely to expose any information needed (backup status etc) via Python,
or any other PL-language supported by Postgres, and then add according
metrics similarly to any normal Postgres-native metrics.
