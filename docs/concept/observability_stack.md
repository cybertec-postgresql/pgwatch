---
title: Observability stack
---

pgwatch does not include a dashboarding or alerting engine of its own. The metrics it stores are accessible through whatever sink you chose — Postgres, TimescaleDB, Prometheus, JSON file, or gRPC — and you are free to point any tool that speaks the corresponding protocol at that data. In practice, almost every pgwatch deployment uses Grafana on top, because the project ships a curated set of dashboards specifically designed for PostgreSQL.

## Why Grafana

- First-class support for PostgreSQL, TimescaleDB, and Prometheus as data sources — the three sinks pgwatch most commonly targets.
- The shipped dashboards (see the [Gallery → Dashboards](../gallery/dashboards.md)) expect the standard Postgres function syntax (window functions for counter resets, percentile aggregates for latency panels, etc.).
- Built-in alerting with a graphical rule editor covers most common operational use cases.

Grafana's alerting is convenient for lighter deployments but has limits at scale: alert rules can only attach to Graph panels, and queries with template variables do not work. For enterprise-scale setups, many teams graduate to a dedicated alerting system that consumes Prometheus metrics directly.

## Storage recommendations

Pick a sink based on what already exists in your environment.

| Sink | When to use it |
|---|---|
| **Postgres** (with or without TimescaleDB) | Default. The shipped dashboards and SQL metric queries target this substrate; TimescaleDB adds automatic retention and continuous aggregates. |
| **Prometheus** | When an external Prometheus server is already the system of record for monitoring data. pgwatch can [expose metrics in the Prometheus text exposition format](../howto/enable_prometheus_scrape.md) instead of writing to a database. |
| **gRPC** | When you want to stream metrics into a system pgwatch does not natively support — an external time-series store, alerting system, or analytics pipeline. You implement the receiving server using the protobuf contract under `api/pb/`. |
| **JSON file** | Testing, CI, and local development. Writes one file per measurement batch; not intended for production retention. |

Multiple sinks can run side-by-side (see [Reference: Sinks options](../reference/sinks_options.md) for the `MultiWriter` mechanism), so the choice is not exclusive — Postgres for dashboards, Prometheus for an existing alerting stack, gRPC for a custom data lake, etc.

## Alerting

pgwatch does not have strong opinions about where alerts should fire. What the project provides:

- A pre-built *Alert Template* dashboard in the [dashboards gallery](../gallery/dashboards.md) that suggests which metrics are worth alerting on.
- The same Grafana alerting workflow that works with any other PostgreSQL data source.

For the concrete recipe to add alert rules in Grafana, see [How-to: Set up alerting](../howto/set_up_alerting.md). For larger setups, point the gRPC or Prometheus sink at a dedicated alerting system (Alertmanager, Grafana Mimir + Grafana, SaaS, etc.) instead of running Grafana alert rules.
