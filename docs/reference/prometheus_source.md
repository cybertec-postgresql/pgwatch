---
title: Prometheus source
---

When you set `kind: prometheus` on a source, pgwatch scrapes an HTTP endpoint that exposes data in the [Prometheus text exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/) and forwards every metric family to the configured sink. This page documents the connection-string format and the presets that ship out of the box. For the recipe to wire pgwatch to Patroni or `postgres_exporter`, see [How-to: Monitor a Prometheus exporter](../howto/monitor_prometheus_exporter.md).

## Connection string

The `conn_str` field is a plain HTTP or HTTPS URL. Optional query parameters control TLS validation and are stripped before the request is sent to the exporter.

| Query parameter | Description |
|---|---|
| `tlsskipverify=true` | Disable TLS certificate verification (use with caution). |
| `tlsrootcert=<path>` | Absolute path to a PEM-encoded CA certificate file used to verify the server certificate. |

Basic Auth credentials are embedded in the URL in the standard `user:password@host` form.

### Examples

```yaml
# Plain HTTP - no auth
conn_str: "http://patroni-node1:8008/metrics"

# HTTPS with a custom CA certificate
conn_str: "https://patroni-node1:8008/metrics?tlsrootcert=/etc/ssl/certs/my-ca.pem"

# HTTPS with Basic Auth and TLS certificate verification disabled
conn_str: "https://user:secret@patroni-node1:8008/metrics?tlsskipverify=true"
```

## Presets

### `patroni`

Covers the key metric families emitted by Patroni's `GET /metrics` endpoint (default port **8008**):

| Metric family | What it measures | Interval |
|---|---|---|
| `patroni_postgres_running` | 1 if Postgres is running | 30 s |
| `patroni_primary` | 1 if this node is the primary | 30 s |
| `patroni_replica` | 1 if this node is a replica | 30 s |
| `patroni_standby_leader` | 1 if this node is standby leader | 30 s |
| `patroni_cluster_unlocked` | 1 if the cluster has no leader lock | 30 s |
| `patroni_xlog_location` | WAL location on primary | 30 s |
| `patroni_xlog_received_location` | received WAL on replica | 30 s |
| `patroni_xlog_replayed_location` | replayed WAL on replica | 30 s |
| `patroni_dcs_last_seen` | seconds since DCS last contacted | 30 s |
| `patroni_pending_restart` | 1 if node needs a restart | 60 s |
| `patroni_is_paused` | 1 if auto-failover is disabled | 60 s |
| `patroni_postgres_timeline` | Postgres timeline | 60 s |

### `postgres-exporter-basic`

Covers the most important metric families emitted by [`postgres_exporter`](https://github.com/prometheus-community/postgres_exporter):

| Metric family | Interval |
|---|---|
| `pg_stat_activity_count` | 30 s |
| `pg_stat_bgwriter_checkpoints_timed` | 60 s |
| `pg_stat_replication_pg_wal_lsn_diff` | 30 s |

## Custom metrics

Specify individual metric families and their intervals with `custom_metrics`:

```yaml
- name: patroni-prod-node1
  kind: prometheus
  conn_str: "http://patroni-node1:8008/metrics"
  custom_metrics:
    patroni_primary: 30
    patroni_cluster_unlocked: 30
    patroni_dcs_last_seen: 30
  is_enabled: true
```

## Custom tags

Use `custom_tags` to attach arbitrary key-value pairs to every stored data point. This is useful when multiple nodes feed the same sink and you need to distinguish them:

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
