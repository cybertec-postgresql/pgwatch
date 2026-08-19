---
title: Set up alerting
---

This how-to wires Grafana's built-in alerting to the metrics pgwatch stores. It assumes you already have a Grafana instance with the pgwatch dashboards imported and at least one data source (`pgwatch-metrics` for the Postgres sink or `pgwatch-prometheus` for the Prometheus sink) configured.

For background on why pgwatch pairs with Grafana and the trade-offs of using Grafana alerting at scale, see [Concept: Observability stack](../concept/observability_stack.md).

## Prerequisites

- A running pgwatch deployment (any sink)
- A running Grafana 13+ instance
- The pgwatch dashboards imported (see [Tutorial: Docker installation](docker_installation.md))
- A `pgwatch-metrics` (Postgres) or `pgwatch-prometheus` (Prometheus) data source with the expected UID

## Steps

1. Open Grafana and confirm the pgwatch data source is reachable: **Connections → Data sources → pgwatch-metrics** → **Test**. The button should turn green.

2. Pick the panel you want to alert on. The shipped **Alert Template** dashboard in the [Gallery → Dashboards](../gallery/dashboards.md) shows the panels most teams start with — high connection count, replication lag, long-running transactions, disk-space growth.

3. From the panel's menu choose **More → Alert rule → Create alert rule from panel**.

4. Fill in the alert rule:

    - **Query** — the panel query is pre-filled. Edit it only if you need a different aggregation. **Important:** the query must not use template variables, otherwise Grafana cannot evaluate it as an alert source.
    - **Condition** — pick a threshold and the evaluation window. Start with `IS ABOVE 80` over `5m` and tune from there.
    - **Folder** and **Evaluation group** — keep the defaults.
    - **No data / error handling** — choose what Grafana should do when the query returns no rows or an error.

5. Add a **contact point** for the alert. Go to **Alerting → Contact points → New contact point** and pick a channel: Slack, PagerDuty, email, webhook, etc. Reference the contact point from the rule's **Labels** section.

6. Save the rule and let it evaluate for at least one window. The **Alerting → Alert rules** list shows whether the rule has fired.

## Caveats

- Grafana alert rules only attach to **Graph panels** — other visualisation types cannot host an alert source.
- Rules cannot reference template variables in their queries. If the panel query uses `$datasource` or `$database`, copy and rewrite it as a literal for the alert.
- For high-cardinality environments (hundreds of sources) the per-rule evaluation cost adds up — consider grouping by cluster or using a dedicated alerting system that consumes Prometheus directly.

## See also

- [Concept: Observability stack](../concept/observability_stack.md) — Grafana's role in the pgwatch architecture
- [Reference: REST API](../reference/rest.md) — programmatic configuration of alert rules (via Grafana, not pgwatch itself)
