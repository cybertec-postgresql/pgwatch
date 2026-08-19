---
title: Managed PostgreSQL provider support
---

pgwatch ships a preset per managed PostgreSQL provider that uses only the metrics the platform's low-privilege monitoring user can see. Pick the preset that matches your platform; the gatherer will then keep the dashboards populated and the log quiet.

## Provider comparison

| Provider | Preset | Python / OS helpers | `pg_monitor` role | Notes |
|---|---|---|---|---|
| Google Cloud SQL for PostgreSQL | `gce` | No | Yes | OS metrics available via [Grafana Stackdriver data source](https://grafana.com/docs/grafana/latest/datasources/google-cloud-monitoring/). Set `track_io_timing` and `track_functions` in the Cloud Console Flags section. |
| Amazon RDS for PostgreSQL | `rds` | No | Yes | OS metrics available via [Grafana CloudWatch data source](https://grafana.com/docs/grafana/latest/datasources/cloudwatch/). |
| Amazon Aurora PostgreSQL-compatible | `rds` | No | Yes | Some PostgreSQL metrics are missing compared to standard RDS. |
| Azure Database for PostgreSQL | `azure` | No | Yes | OS metrics available via [Grafana Azure Monitor data source](https://grafana.com/docs/grafana/latest/datasources/azuremonitor/). Some file-access functions (e.g. for `wal_size`) are whitelisted. `pg_stat_statements` is **not** activated by default — enable it manually or via the API. |
| Aiven for PostgreSQL | _provider-specific setup_ | No | Yes | See the [Aiven developer documentation](https://aiven.io/docs/products/postgresql/howto/monitor-with-pgwatch2). |

## Common caveats across providers

- PL/Python and other untrusted procedural languages are disabled on every managed platform listed above — Python-based OS helper metrics will fail. Use the platform's OS-metrics pipeline instead.
- The `DB overview` dashboard may show errors for unprivileged users; switch to the `DB overview Unprivileged` dashboard when running with a non-superuser role.
- Set `track_io_timing` and `track_functions` wherever the platform exposes them — many metrics rely on these being on.
