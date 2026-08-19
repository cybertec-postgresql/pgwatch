---
title: Advanced features
---

Beyond the core metric-fetching path, pgwatch ships a number of capabilities for monitoring PostgreSQL's surrounding ecosystem: high-availability clusters, connection poolers, log files, and managed cloud databases. This page explains what each capability does and when you would reach for it. The concrete configuration recipes live in dedicated how-to guides; see the cross-links below.

## Patroni clusters

Patroni is a popular PostgreSQL-specific HA-cluster manager. Because Patroni clusters are dynamic — nodes can come and go at any time — monitoring them through ordinary per-DB sources is awkward. pgwatch reads the cluster membership directly from a Distributed Consensus Store (etcd, ZooKeeper, or Consul), so cluster changes are picked up automatically without any external orchestration. The `patroni` source kind drives this discovery; see [How-to: Monitor a Patroni cluster](../howto/monitor_patroni_cluster.md).

## PostgreSQL server log parsing

pgwatch can ingest PostgreSQL server logs in CSVLOG format and count event severities (errors, warnings, etc.) per database and per instance. Only counts are stored — never the raw log lines, usernames, or query bodies — so the feature is safe to enable in security-sensitive environments. The gatherer picks between local and remote parsing automatically based on whether it can read the log directory directly. See [How-to: Enable log parsing](../howto/enable_log_parsing.md).

## Connection poolers

PgBouncer and Pgpool-II expose their own operational metrics that complement PostgreSQL's. pgwatch knows how to talk to both poolers' `SHOW STATS` (and `SHOW POOL_NODES`/`SHOW POOL_PROCESSES` for Pgpool-II) and ships dedicated metrics, presets, and Grafana dashboards for each. See [How-to: Monitor PgBouncer](../howto/monitor_pgbouncer.md).

## Prometheus scraping

pgwatch can expose its collected metrics over an HTTP endpoint in the Prometheus text exposition format, which lets an external Prometheus server scrape them instead of pgwatch writing to a database. This mode keeps the rest of the gatherer intact — every metric that has a positive fetch interval is exposed — but drops metrics that need cross-scrape state, such as `change_events`. See [How-to: Expose metrics for Prometheus scraping](../howto/enable_prometheus_scrape.md).

## Managed cloud providers

Managed PostgreSQL services (AWS RDS, Azure Database for PostgreSQL, Google Cloud SQL) restrict what a low-privilege monitoring user can see. pgwatch ships a preset per provider — `aws`, `azure`, `gce` — that uses only the metrics available on each platform, so the dashboards stay populated and the gatherer logs stay quiet. See [Reference: Managed PostgreSQL provider support](../reference/managed_postgres_support.md).
