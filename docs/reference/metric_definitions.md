---
title: Metric definitions
---

This page describes the YAML schema for a metric definition. For background on what metrics and presets are and the design rules they must follow, see [Metrics and presets](../concept/metrics_and_presets.md). For the workflow to add a metric, see [How-to: Add a custom metric](../howto/add_custom_metric.md).

## Metric definition YAML schema

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

## Field reference

### `description`

Free-text description shown in the Web UI and exported by `pgwatch metric list`.

### `init_sql`

Optional SQL block executed before the metric query itself. Typically used to install extensions, create helper functions, or seed schema objects. Requires a connection with enough privilege to run the statements; see [Tutorial: Preparing databases for monitoring — Metrics initialization](../tutorial/preparing_databases.md) for the helper-installation workflow.

### `sqls`

The actual metric queries. The key is the **minimum** PostgreSQL major version the query is valid for. The gatherer picks the highest keyed entry that is less than or equal to the target database's version.

A query must return a column named `epoch_ns` and may return any number of additional columns. The `pgwatch_generated` comment tag helps with log filtering and debugging.

!!! note
    If a query works unchanged from v14 through v18, only the `14` entry is needed. Add a new keyed entry only when an internal catalog or syntax change breaks the query at a specific version.

### `gauges`

List of columns that should be treated as gauges. By default, all numeric columns are treated as counters (cumulative). This field is only relevant for Prometheus output.

A trailing `*` entry marks every column as a gauge.

### `is_instance_level`

Boolean. When `true`, the metric result is cached and shared between all databases of a single instance to reduce load on the monitored server.

### `node_status`

Optional: `primary` or `standby`. When set, the metric is only executed on nodes in the matching role.

### `statement_timeout_seconds`

Maximum wall-clock time the query is allowed to run before it is killed. Defaults to 5 seconds.

### `storage_name`

Optional storage-side name override. Used to coalesce data from multiple metric definitions into a single stored metric. The built-in `stat_statements_no_query_text` metric is the canonical example: it stores data under the same key as `stat_statements` but without the query text column, so dashboards that reference the original metric keep working on more security-sensitive instances.
