---
title: Enable log parsing
---

This how-to turns on pgwatch's CSVLOG parsing so that PostgreSQL server log severities are counted per database and per instance.

For background, see [Advanced features → PostgreSQL server log parsing](../concept/advanced_features.md#postgresql-server-log-parsing).

## Prerequisites

- The target PostgreSQL instance is configured to write logs in **CSVLOG** format (`log_destination = 'csvlog'` in `postgresql.conf`).
- For **local** parsing mode: pgwatch runs on the same host as the database server and the OS user running pgwatch has read access to the log directory, plus the `pg_read_all_settings` role.
- For **remote** parsing mode: the monitoring user has the `pg_monitor` role and `EXECUTE` on `pg_read_file(text, bigint, bigint)`.

## Steps

1. Activate the `server_log_event_counts` metric on the source. The simplest way is to pick a preset that includes it (the built-in `full` preset does) or to add it directly to the source's metrics list:

    ```yaml
    - name: my-postgres
      kind: postgres
      conn_str: postgresql://pgwatch@db1/mydb
      custom_metrics:
        server_log_event_counts: 60
      is_enabled: true
    ```

2. Choose the parsing mode. pgwatch picks automatically:

    - **Local mode** — used when the monitoring connection comes through a Unix socket or when the `data_directory` setting on the server matches what `pg_control_system()` reports.
    - **Remote mode** — used otherwise; reads log blocks through `pg_read_file()`.

    Local mode is preferred where possible — it avoids round-tripping log bytes through the SQL connection.

3. Tune the fetch interval. Each call reads up to 10 MB from a single log file. With high log volume, raise the interval to keep up; with low volume, lower it for finer-grained event counts.

4. Verify pgwatch is recording counts:

    ```bash
    curl -H "Token: $TOKEN" http://localhost:8080/source/my-postgres/metric
    ```

    Look for rows in the `server_log_event_counts` metric family grouped by severity.

## Notes

- The feature only stores **counts**, not log lines or usernames — safe for security-sensitive deployments.
- For binary PG upgrades via `pg_upgrade`, helper functions that wrap log access may need to be re-installed afterwards.
