---
title: Source types
---

When you add a new "to be monitored" entry to pgwatch you select a **source kind**. Each kind changes how the gatherer discovers and connects to its targets. The table below lists every supported kind; the prose after it adds notes for the kinds that need them.

| Kind | Target | Connection string |
|---|---|---|
| `postgres` | A single database on a Postgres instance | Standard Postgres URI |
| `postgres-continuous-discovery` | All (or a filtered subset of) databases on a Postgres instance | Standard Postgres URI; uses regex include/exclude patterns |
| `pgbouncer` | A PgBouncer pooler | Postgres URI to the PgBouncer port |
| `pgpool` | A Pgpool-II instance | Postgres URI to the Pgpool port |
| `patroni` | A Patroni HA cluster (members discovered from DCS) | URI to the DCS — see below |
| `prometheus` | An HTTP endpoint exposing Prometheus text exposition format | `http(s)://host:port/path` |

Internally, monitoring always happens **per database, not per cluster** — even when the kind discovers an entire cluster.

## Continuous-discovery modes

For `postgres-continuous-discovery` and `patroni`, the gatherer periodically rescans the target and adds or removes databases automatically based on what it finds. (`pgpool` is not a discovery kind — it represents a single Pgpool-II instance and does not enumerate databases.)

All continuous modes need a connection that can `SELECT FROM pg_database`; the per-row visibility is gated by `datallowconn`, `not datistemplate`, and `has_database_privilege(datname, 'CONNECT')`, so the user does not need any elevated role on databases it lacks CONNECT on.

## Patroni connection string

When `kind: patroni` is selected, `conn_str` must point at the DCS:

```
etcd://host:port[,host:port..]/namespace/scope
```

Example: `etcd://localhost:2379/service/batman`.

Omit the scope to resolve every database in the namespace; set it to limit resolution to one Patroni cluster.

For etcd, TLS material and credentials are encoded into the URI:

```
etcd://username:password@host:2379/service/batman?ca_file=/path/to/ca.crt&cert_file=/path/to/client.crt&key_file=/path/to/client.key
```

## See also

- [Concept: Installation options](../concept/installation_options.md) — choosing the configuration store
- [How-to: Monitor a Patroni cluster](../howto/monitor_patroni_cluster.md) — Patroni source recipe
- [How-to: Monitor PgBouncer](../howto/monitor_pgbouncer.md) — PgBouncer source recipe
- [How-to: Monitor a Prometheus exporter](../howto/monitor_prometheus_exporter.md) — Prometheus source recipe
