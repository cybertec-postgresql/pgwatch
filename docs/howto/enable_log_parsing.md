---
title: Enable log parsing
---

This how-to turns on pgwatch's server log parsing so that PostgreSQL server log severities are counted per database and per instance (`server_log_event_counts`), and log records are counted per message pattern (`server_log_patterns`).

For background, see [Advanced features → PostgreSQL server log parsing](../concept/advanced_features.md#postgresql-server-log-parsing).

## Prerequisites

- `logging_collector = on` in `postgresql.conf`. This one is not optional: with the collector off, PostgreSQL writes to the postmaster's stderr and there are no log files in `log_directory` to read at all.
- Any `log_destination`. **`csvlog`, `jsonlog` and `stderr` all work**, so the PostgreSQL default needs no change. `log_destination` is a list, and where more than one is set pgwatch reads `csvlog` first, then `jsonlog`, then `stderr` — it reads one of them, never two, so a server writing both does not have its events counted twice.
- For `stderr`, pgwatch reads the server's own `log_line_prefix` and parses accordingly. A prefix that includes `%d` (or `%q%u@%d`) is worth having: without a database in the line, per-database counts cannot be attributed and only the instance-wide `_total` columns are meaningful.
- For **local** parsing mode: pgwatch runs on the same host as the database server and the OS user running pgwatch has read access to the log directory, plus the `pg_read_all_settings` role.
- For **remote** parsing mode: the monitoring user has the `pg_monitor` role and `EXECUTE` on `pg_read_file(text, bigint, bigint)`.

## Steps

1. Activate `server_log_event_counts`, `server_log_patterns`, or both on the source. The simplest way is to pick a preset that includes them (the built-in `full` and `debug` presets do) or to add them directly to the source's metrics list:

    ```yaml
    - name: my-postgres
      kind: postgres
      conn_str: postgresql://pgwatch@db1/mydb
      custom_metrics:
        server_log_event_counts: 60
        server_log_patterns: 60
      is_enabled: true
    ```

    Both metrics share one log parser per source, and each is sent at its own interval.

2. Choose the parsing mode. pgwatch picks automatically:

    - **Local mode** — used when the monitoring connection comes through a Unix socket or when the `data_directory` setting on the server matches what `pg_control_system()` reports.
    - **Remote mode** — used otherwise; reads log blocks through `pg_read_file()`.

    Local mode is preferred where possible — it avoids round-tripping log bytes through the SQL connection.

3. Tune the fetch interval. In remote mode each call reads up to 10 MB from a single log file. The interval also sets how often counts are emitted: an interval elapses and an envelope is sent whether or not anything was logged, so a quiet server reports zeroes rather than a gap.

4. Verify pgwatch is recording counts:

    ```bash
    curl -H "Token: $TOKEN" http://localhost:8080/source/my-postgres/metric
    ```

    Look for rows in the `server_log_event_counts` metric family grouped by severity, and in `server_log_patterns` grouped by pattern.

## What `server_log_patterns` stores

Each record is reduced to a template by [pglogwatch](https://github.com/pashagolub/pglogwatch): numbers, quoted literals, `key=value` values, paths and SQL literals become `$1`, `$2`, and so on, while words and identifiers stay. Records with the same severity, SQLSTATE and template share a pattern id. One row per pattern and database is stored each interval:

| Column | Meaning |
|---|---|
| `tag_pattern_id` | Pattern id, 16 hex characters, or `other` |
| `tag_severity` | Severity in English, lowercase (`error`, `log`, …) |
| `tag_sqlstate` | Five-character SQLSTATE, or empty |
| `tag_database` | The record's database, or empty for background processes |
| `template` | The masked message; null on the `other` row |
| `rules_version` | Version of the masking rules; a change gives new pattern ids |
| `count` | Records in the interval |
| `duration_sum_ms`, `duration_max_ms` | Sum and maximum of `duration: … ms` values; null when the pattern had none |

Rows count the whole instance, not only the source's database. Per severity, the 100 patterns with the highest counts are stored; the rest are summed into one `other` row, so the counts of a severity add up to its `_total` column in `server_log_event_counts`. An interval with no records stores no rows. With `log_min_duration_statement` or `log_connections` set, the `log` severity shows which statements are slow and who connects.

In the Prometheus sink the `tag_` columns become labels; `template` is a text column and is not exposed.

## Notes

- The feature stores **counts** and value-free **templates**, never log lines or the values in them. `server_log_event_counts` stores counts only. Templates keep identifiers such as table names, and the role name of a failed password authentication; leave `server_log_patterns` off where even those must not leave the server.
- Parsing resumes by **byte offset** per file, so a pgwatch restart continues where it left off instead of re-counting or skipping. On first sight of a file that already has content, parsing starts at its END: a fresh pgwatch pointed at a server with months of retained logs counts what happens next, not the backlog.
- Localised servers are handled: `lc_messages` is read from the server and severities are normalised to English before counting, so the column names do not change with the server's locale.
- For binary PG upgrades via `pg_upgrade`, helper functions that wrap log access may need to be re-installed afterwards.
