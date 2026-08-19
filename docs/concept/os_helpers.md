---
title: OS metrics via PL/Python helpers
---

PostgreSQL's SQL surface exposes only a narrow view of the operating system that runs underneath it. For OS-level metrics — CPU load, memory pressure, disk I/O — pgwatch ships a small set of **helper functions** written in PL/Python that read `/proc`, `psutil`, or equivalent system APIs and return their values as ordinary SQL rows. The gatherer then runs those helpers the same way it runs any other metric.

## Why helpers instead of a sidecar agent?

- One connection per source — no need to run a second agent on every database server.
- Metrics land in the same Grafana dashboards as everything else; no second tool to learn.
- Tightly-scoped privilege escalation: each helper is a `SECURITY DEFINER` function that only exposes the value the wrapping SQL reads.

The trade-off is that helpers execute inside the database process. When Postgres is down, no OS metrics can be collected either, and the helpers themselves can be a single point of failure if their definitions get out of sync with the database server's Python version.

## psutil helpers

The most useful helpers (`psutil_cpu`, `psutil_mem`, `psutil_disk`, `psutil_disk_io_total`) rely on the `psutil` Python package. From user reports, `psutil`'s behaviour shifts subtly between Linux distros and kernel versions, so small adjustments to the helpers (e.g. dropping a non-existent column) may be needed. Minimum usable kernel version: 3.3.

When pgwatch runs on the same host as a monitored source, it detects this automatically and fetches the default `psutil*` metrics directly from OS counters, falling back to PL/Python wrappers only if the direct path fails.

## Caveats

- PL/Python is disabled by default on most managed providers (AWS RDS, Google Cloud SQL, etc.) — helper metrics simply will not work there.
- When upgrading PostgreSQL binary-in-place via `pg_upgrade`, helper functions on the cluster being upgraded may need to be dropped and re-installed.
- If you accept the risk and want pgwatch to create all needed helpers on startup, pass `--create-helpers` to the gatherer.

For the workflow to install helpers on a monitored database, see [Tutorial: Preparing databases for monitoring → Metrics initialization](../tutorial/preparing_databases.md). For the source-type catalogue (postgres, patroni, pgbouncer, etc.), see [Reference: Source types](../reference/source_types.md).
