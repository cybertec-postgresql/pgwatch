---
title: Monitor PgBouncer
---

This how-to configures pgwatch to scrape a PgBouncer connection pooler alongside your PostgreSQL databases.

For background, see [Advanced features → Connection poolers](../concept/advanced_features.md#connection-poolers).

## Prerequisites

- A running PgBouncer instance with its `stats_users` configured to allow the monitoring user to run `SHOW STATS`.
- A PgBouncer monitoring user with read-only access to the pooler admin console.

## Steps

1. Add a source of kind `pgbouncer` whose `conn_str` points at PgBouncer's port:

    ```yaml
    - name: pgbouncer-prod
      kind: pgbouncer
      conn_str: postgresql://pgwatch:secret@pgbouncer1:6432/pgbouncer
      preset_metrics: pgbouncer
      is_enabled: true
    ```

2. Set the **DB Name** field on the source to the pool name you want to monitor. Leave it empty to track all pools in the instance — individual pools will then be distinguished by a `database` tag on every measurement.

    > Do **not** put `pgbouncer` in the DB Name field. That special database provides the statistics, but it is not a real pool.

3. Wait for the next refresh cycle (default 120 seconds) and verify pgwatch is collecting metrics:

    ```bash
    curl -H "Token: $TOKEN" http://localhost:8080/source/pgbouncer-prod/metric
    ```

4. Import the PgBouncer Grafana dashboard from the [`grafana/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana) directory of the pgwatch repo and point it at the metrics data source for this source.

## See also

- [Reference: Source types](../reference/source_types.md) — `pgbouncer` kind reference
- [Advanced features → Connection poolers](../concept/advanced_features.md#connection-poolers) — Pgpool-II works analogously
