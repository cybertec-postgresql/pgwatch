---
title: Dashboarding and alerting
---

This page is a quick orientation to the dashboarding and alerting tooling that pairs with pgwatch. For background and trade-offs see [Concept: Observability stack](../concept/observability_stack.md). For the concrete alert-setup recipe see [How-to: Set up alerting](set_up_alerting.md).

## Where the dashboards live

pgwatch ships a set of pre-defined Grafana dashboards covering most of the metrics the built-in presets collect. Browse them in the [Gallery → Dashboards](../gallery/dashboards.md) page or in the [`grafana/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana) folder of the repository. The dashboards assume one of these data sources is configured:

- `pgwatch-metrics` — for Postgres / TimescaleDB sinks
- `pgwatch-prometheus` — for the Prometheus sink

The default Docker image wires both for you.

## Customising dashboards

Almost every deployment ends up tweaking the built-in dashboards (colours, units, panel layout). Two practices keep that work portable across upgrades:

- Use **Save as** to put customised dashboards in a separate folder rather than overwriting the originals.
- Treat the built-in dashboards as code you can pull and re-import after pgwatch upgrades.

See [Concept: Long-term installations → Dashboard maintenance](../concept/long_term_installations.md#dashboard-maintenance) for the long-term playbook.

## Alerting

Grafana's built-in alerting is the path of least resistance and is fully compatible with pgwatch's stored metrics. The recipe lives at [How-to: Set up alerting](set_up_alerting.md).
