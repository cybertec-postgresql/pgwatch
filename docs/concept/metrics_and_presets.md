---
title: Metrics and presets
---

This page explains what metrics and presets are in pgwatch and the design rules that custom metrics must follow. For the YAML schema, see [Metric definitions](../reference/metric_definitions.md). For the workflow to add a metric, see [How-to: Add a custom metric](../howto/add_custom_metric.md).

## What is a metric?

A metric is a named SQL query that the gatherer runs against a monitored database. The query must return a column named `epoch_ns` (nanoseconds since the Unix epoch) plus any number of additional columns you want stored. Most metrics ship multiple query versions — one per minimum supported PostgreSQL major version — and may also differ for primary versus standby roles.

The gatherer selects the right version of a metric at runtime by inspecting the target database: it checks the PostgreSQL major version, the recovery state, and whether the monitoring user has superuser rights.

```sql
-- a sample metric
SELECT
  (extract(epoch from now()) * 1e9)::int8 as epoch_ns,
  extract(epoch from (now() - pg_postmaster_start_time()))::int8 as postmaster_uptime_s,
  case when pg_is_in_recovery() then 1 else 0 end as in_recovery_int;
```

## What is a preset?

A preset is a named collection of `metric_name: time_interval` pairs. Presets let you define a monitoring profile once — for example `basic` or `exhaustive` — and apply it consistently across many monitored databases. Sources can override individual intervals without leaving the preset.

## Built-in metrics and presets

The pgwatch project ships a set of pre-defined metrics and presets that cover most common needs. For deployments at scale, you will usually want to extend this set with custom metrics or adjust fetch intervals to match your monitoring goals. The full list lives in the [default metrics.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/internal/metrics/metrics.yaml) file in the pgwatch repository, and the Web UI exposes them under the Metrics and Presets pages (see the [Web UI gallery](../gallery/webui.md)).

A few things to know about the built-in set:

- Roughly half of the built-in metrics are referenced by one of the shipped presets and are ready to use out of the box. The rest need extra Postgres extensions, OS-level tooling, or elevated privileges.
- A handful of built-in metrics are restricted to a specific node role — primary-only or standby-only — and the gatherer skips them on nodes in the opposite role.
- Some metrics have non-standard behavior. `change_events`, `server_log_event_counts`, and `instance_up` are the most prominent examples.

## Custom-metric design rules

Custom metric definitions must obey a small set of rules so the gatherer can store, label, and serve them correctly:

- Every metric query must return an `epoch_ns` column (nanoseconds since epoch). If the column is missing, the gatherer falls back to its own clock; this works but loses precision when there is clock skew between the database server and the gatherer host.
- Returned columns must be of type `text`, `integer`, `boolean`, or `double precision`. Columns whose value is `NULL` are dropped before storage — design with that in mind.
- Column names should be descriptive enough to be self-explanatory on a Grafana panel but short enough not to bloat storage.
- Queries must execute within the configured `statement_timeout_seconds` (default 5 seconds).
- A column can be promoted to a **tag** by prefixing its name with `tag_`. Tagged columns are indexed and become available for fast filtering in Grafana.
- All rows produced for a source can be enriched with static **custom tags** via the source's `custom_tags` field (Web UI or YAML). Custom tags are added to every metric emitted by that source.
- For Prometheus output, numeric columns are mapped to a Counter value type by default (because most Statistics Collector columns are cumulative). Columns that go up **and** down — connection counts, queue depths, and the like — must be listed under `gauges` in the metric definition.
- In Prometheus output all `text` columns are turned into labels; only numeric values can be exposed as samples.

For the field-by-field YAML schema that realises these rules, see [Metric definitions](../reference/metric_definitions.md).
