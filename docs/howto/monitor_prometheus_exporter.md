---
title: Monitor a Prometheus exporter
---

This how-to wires pgwatch to a Prometheus text-format HTTP exporter. A common use case is [Patroni](https://patroni.readthedocs.io/), which exposes cluster-health metrics (node role, WAL position, DCS connectivity, …) on `GET /metrics`. The same recipe works for [`postgres_exporter`](https://github.com/prometheus-community/postgres_exporter) and any other endpoint that emits the Prometheus exposition format.

For the connection-string format, query parameters, and full preset/metric tables, see [Reference: Prometheus source](../reference/prometheus_source.md).

## Prerequisites

- A reachable Prometheus text exposition endpoint
- Network access from the pgwatch host to the exporter port
- (Optional) A CA certificate file if the exporter is served over HTTPS with a non-public CA

## Steps

1. Add a source of kind `prometheus`. Point `conn_str` at the exporter URL and pick the right preset.

    For Patroni:

    ```yaml
    - name: patroni-prod-node1
      kind: prometheus
      conn_str: "http://patroni-node1:8008/metrics"
      preset_metrics: patroni
      is_enabled: true
    ```

    For `postgres_exporter`, pick `postgres-exporter-basic` instead.

2. For HTTPS endpoints with a private CA, pass the CA file path via a query parameter:

    ```yaml
    conn_str: "https://patroni-node1:8008/metrics?tlsrootcert=/etc/ssl/certs/my-ca.pem"
    ```

3. (Optional) Attach `custom_tags` so multi-node metrics can be distinguished in Grafana:

    ```yaml
    - name: patroni-prod-node1
      kind: prometheus
      conn_str: "http://patroni-node1:8008/metrics"
      preset_metrics: patroni
      custom_tags:
        cluster: prod
        node: node1
      is_enabled: true
    ```

4. Wait for the next refresh cycle (default 120 seconds). pgwatch fetches the URL, parses the Prometheus text, and forwards every metric family to the sink.

## Verifying metrics are flowing

List the metrics the source is producing:

```bash
curl -H "Token: $TOKEN" http://localhost:8080/source/patroni-prod-node1/metric | jq '.[].Name'
```

You should see the metric families from the chosen preset (or your `custom_metrics` list).

## See also

- [Reference: Prometheus source](../reference/prometheus_source.md) — URI format, query parameters, preset/metric tables
- [Reference: Source types](../reference/source_types.md) — `prometheus` vs `postgres` vs `patroni`
