---
title: Add a custom metric
---

This how-to shows the two supported ways to add a new metric to a pgwatch deployment: through the Web UI on a Config-DB-backed setup, or by editing a YAML file on a YAML-backed setup.

For background on what metrics and presets are and the design rules they must follow, see [Metrics and presets](../concept/metrics_and_presets.md). For the YAML schema, see [Metric definitions](../reference/metric_definitions.md).

## Prerequisites

- A running pgwatch with the configuration store (PostgreSQL or YAML) you intend to edit.
- For the Config-DB workflow: a Web UI admin login.
- For the YAML workflow: read/write access to the `metrics.yaml` file the gatherer was started with (`--metrics`).

## Option A — Add a metric in a Config-DB setup

1. Open the Web UI and go to the **Metrics** page.
2. Click **+ NEW**.
3. Fill the form:
    - Pick a unique metric name.
    - Select the **minimum supported PostgreSQL version**.
    - Paste the query text. The query must return a column named `epoch_ns`; see [Metrics and presets](../concept/metrics_and_presets.md) for the full set of rules.
    - Add any extra fields you need (gauges, instance-level, statement timeout, storage name).
4. Click **ADD METRIC** to store.
5. Activate the metric by including it in a preset:
    - Open the **Presets** page, pick the preset you want to extend, and add the new metric with its fetch interval.
    - Or, on the **Sources** page, edit a single source and add the metric directly to its metric list.

The gatherer picks up the new metric on its next refresh (default every 120 seconds; tune via `--refresh`).

## Option B — Add a metric in a YAML setup

1. Edit the `metrics.yaml` file the gatherer was started with (`--metrics`). If you need a starting point, copy the [default metrics.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/internal/metrics/metrics.yaml) and edit it.
2. Add a new top-level entry following the schema in [Metric definitions](../reference/metric_definitions.md).
3. Optionally add the new metric to an existing or new preset in the same file.
4. Save the file. The gatherer reloads on its next refresh.

## Verifying the metric was added

Confirm the metric is now exposed by querying the gatherer:

```bash
curl -H "Token: $TOKEN" http://localhost:8080/metric | jq '.[] | select(.Name=="my_metric")'
```

You should see your metric definition returned in JSON. For preset membership, query `/preset`.

## See also

- [Tutorial: Preparing databases for monitoring](../tutorial/preparing_databases.md) — install helper functions used by some metrics
- [Reference: Metric definitions](../reference/metric_definitions.md) — full field reference
- [Reference: CLI — Manage metrics](../reference/cli_env.md) — `pgwatch metric list`, `print-init`, `print-sql`
