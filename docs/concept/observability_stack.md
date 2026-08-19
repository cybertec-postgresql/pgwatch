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

For storage, the project recommends Postgres (with or without TimescaleDB) for most users — the dashboards and SQL metric queries are written against that substrate. Prometheus is a fine choice when an external Prometheus server is already the system of record for monitoring data; pgwatch can expose metrics in the [Prometheus text exposition format](../howto/enable_prometheus_scrape.md) instead of writing to a database.

JSON file and gRPC sinks exist for special-purpose integrations — testing, custom pipelines — and are not the path most users take.

## What this means for alerting

Alerting is its own discipline and pgwatch does not have strong opinions about where alerts should fire. What the project does provide:

- A pre-built *Alert Template* dashboard in the [dashboards gallery](../gallery/dashboards.md) that suggests which metrics are worth alerting on.
- Compatibility with the same Grafana alerting workflow used for any other PostgreSQL data source.

For the concrete recipe to add alert rules in Grafana, see [How-to: Set up alerting](../howto/set_up_alerting.md).
