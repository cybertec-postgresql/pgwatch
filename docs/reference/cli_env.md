---
title: Command-Line Options & Environment Variables
---

## General Usage

```terminal
  pgwatch [OPTIONS] [config | metric | source]
```

When no command is specified, pgwatch starts the monitoring process.
It reads the configuration from the specified sources and metrics, then begins collecting measurements from the resolved databases.

## Options

### Sources

- `-s`, `--sources=`

    Postgres URI, file or folder of YAML files containing info on which DBs to monitor.  
    ENV: `$PW_SOURCES`

    Examples: `postgresql://pgwatch@localhost:5432/pgwatch`,  
    `/etc/sources.yaml`,  
    `/etc/pgwatch/sources/`

- `--refresh=`

    How frequently to resync sources and metrics (default: 120).  
    ENV: `$PW_REFRESH`

- `-g`, `--group=`

    Groups for filtering which databases to monitor. By default all are monitored.  
    ENV: `$PW_GROUP`

- `--min-db-size-mb=`

    Smaller size databases will be ignored and not monitored until they reach the threshold (default: 0).  
    ENV: `$PW_MIN_DB_SIZE_MB`

- `--max-parallel-connections-per-db=`

    Max parallel metric fetches per monitored database. Note the multiplication effect on multi-DB instances (default: 4).  
    ENV: `$PW_MAX_PARALLEL_CONNECTIONS_PER_DB`

- `--try-create-listed-exts-if-missing=`

    Try creating the listed extensions (comma sep.) on first connect for all monitored DBs when missing. Main usage - pg_stat_statements.  
    ENV: `$PW_TRY_CREATE_LISTED_EXTS_IF_MISSING`

    Example: `pg_stat_statements,pg_hint_plan`

- `--create-helpers`

    Create helper database objects from metric definitions.  
    ENV: `$PW_CREATE_HELPERS`

### Metrics

- `-m`, `--metrics=`

    Postgres URI, YAML file or folder of YAML files with metrics definitions.  
    ENV: `$PW_METRICS`

- `--instance-level-cache-max-seconds=`

    Max allowed staleness for instance level metric data shared between DBs of an instance. Set to 0 to disable (default: 30).  
    ENV: `$PW_INSTANCE_LEVEL_CACHE_MAX_SECONDS`

- `--emergency-pause-triggerfile=`

    When the file exists no metrics will be temporarily fetched / scraped (default: /tmp/pgwatch-emergency-pause).  
    ENV: `$PW_EMERGENCY_PAUSE_TRIGGERFILE`

### Sinks

- `--sink=`

    URI where metrics will be stored, can be used multiple times.  
    ENV: `$PW_SINK`

    Examples: `postgresql://pgwatch@localhost:5432/metrics`,  
    `prometheus://localhost:9090`,  
    `jsonfile:///tmp/metrics.json`,  
    `grpc://user:pwd@localhost:5000/?sslrootca=/home/user/ca.crt`

    See [Sinks Options & Parameters](sinks_options.md) for more details.

- `--batching-delay=`

    Sink-specific batching flush delay; may be ignored by some sinks (default: 950ms).  
    ENV: `$PW_BATCHING_DELAY`

- `--partition-interval=`

    Time range for PostgreSQL sink table partitions. Must be a valid PostgreSQL interval. (default: 1 week)  
    ENV: `$PW_PARTITION_INTERVAL`

    Example:
    `--partition-inteval="3 weeks 4 days"`,

- `--retention=`

    Delete metrics older than this. Must be a valid PostgreSQL interval. (default: 14 days)  
    ENV: `$PW_RETENTION`

- `--maintenance-interval=`

    Run pgwatch maintenance tasks on sinks with this interval e.g., deleting old metrics; Set to zero to disable. Must be a valid PostgreSQL interval. (default: 12 hours)  
    ENV: `$PW_MAINTENANCE_INTERVAL`

- `--real-dbname-field=`

    Tag key for real database name (default: real_dbname).  
    ENV: `$PW_REAL_DBNAME_FIELD`

- `--system-identifier-field=`

    Tag key for system identifier value (default: sys_id).  
    ENV: `$PW_SYSTEM_IDENTIFIER_FIELD`

### Logging

- `--log-level=[debug|info|error]`

    Verbosity level for stdout and log file (default: info)

- `--log-file=`

    File name to store logs

- `--log-file-format=[json|text]`

    Format of file logs (default: json)

- `--log-file-rotate`

    Rotate log files

- `--log-file-size=`

    Maximum size in MB of the log file before it gets rotated (default: 100)

- `--log-file-age=`

    Number of days to retain old log files, 0 means forever (default: 0)

- `--log-file-number=`

    Maximum number of old log files to retain, 0 to retain all (default: 0)

### WebUI

- `--web-disable=[all|ui]`

    Disable REST API and/or web UI.  
    ENV: `$PW_WEBDISABLE`

- `--web-addr=`

    TCP address in the form 'host:port' to listen on (default: :8080).  
    ENV: `$PW_WEBADDR`

- `--web-base-path=`

    Base path for web UI and API endpoints (e.g., 'pgwatch' for reverse proxy setups). When set, all web endpoints will be served under this path.  
    ENV: `$PW_WEBBASEPATH`

    Example: `--web-base-path=/pgwatch`

- `--web-user=`

    Admin username.  
    ENV: `$PW_WEBUSER`

- `--web-password=`

    Admin password.  
    ENV: `$PW_WEBPASSWORD`

### Help Options

- `-h`, `--help`

    Show this help message

## Available commands

### Manage configurations

```terminal
  pgwatch [OPTIONS] config <init | upgrade>
```

!!! info
    To use `config` command, you need to specify at least one of the following: `-s`, `--sources`, `-m`, `--metrics`, or `--sink` options.

- `init`

    Initialize the configuration and/or sink database with the required tables and functions. If file is used, it will
    be created in the specified location and filled with built-in defaults. Works with PostgreSQL-based sources,
    metrics, and sink databases.

- `upgrade`

    Upgrade the configuration and/or sink database to the latest version by applying all pending migrations.
    File or folder based configurations are not supported. The command will automatically detect which
    databases (sources, metrics, sinks) require migrations and apply them.

### Manage metrics

```terminal
  pgwatch [OPTIONS] metric <print-init | print-sql | list>
```

!!! info
    To use `metric` command, you need to specify the `-m`, `--metrics` option.

- `list [name...]`

    Export metric and/or preset definitions in YAML format. When no names are provided, outputs all built-in metrics and presets.
    When names are specified, outputs only the requested metrics and/or presets. For presets, both the preset definition
    and all its component metrics are included.

    Examples:

    ```terminal
    # Export all metrics and presets
    pgwatch metric list > custom-metrics.yaml

    # Export a specific metric
    pgwatch metric list cpu_load

    # Export specific presets with their metrics
    pgwatch metric list minimal standard

    # Export a mix of metrics and presets
    pgwatch metric list cpu_load db_size minimal
    ```

    This command is useful for:

    - Creating custom metrics.yaml files by redirecting output
    - Inspecting metric definitions before deploying
    - Copying specific metrics to customize for your needs
    - Understanding what metrics are included in presets

- `print-init`

    Get and print init SQL for a given metric(s) or preset(s)

    Examples:

    ```terminal
    # Print init SQL for a specific metric
    pgwatch metric print-init bgwriter cpu_load

    # Print init SQL for a specific preset
    pgwatch metric print-init exhaustive
    ```

- `print-sql`

    Get and print SQL for a given metric. Optional parameter `-v, --version=` specifies
    PostgreSQL version to get SQL for.

    Examples:

    ```terminal
    # Print SQL for a specific metric
    pgwatch metric print-sql bgwriter

    # Print SQL for a specific metric and PostgreSQL version
    pgwatch metric print-sql bgwriter -v 14
    ```

### Manage sources

```terminal
    pgwatch [OPTIONS] source <ping | resolve>
```

!!! info
    To use `source` command, you need to specify the `-s`, `--sources` option.

- `ping`

    Try to connect to configured sources, report errors if any and then exit.  

- `resolve`

    Connect to the configured source(s) and return resolved connection strings for the monitoring targets discovered.

    Example (say we have a source pg-stage1 defined as postgres-continuous-discovery):

    ```terminal
        pgwatch --sources=postgresql://pgwatch:pgwatchadmin@localhost/pgwatch source resolve

        pg-stage1_postgres=postgresql://pgwatch:pgwatch@localhost:5432/postgres
        pg-stage1_pagila=postgresql://pgwatch:pgwatch@localhost:5432/pagila
        pg-stage1_test=postgresql://pgwatch:pgwatch@localhost:5432/test
        pg-stage1_timetable=postgresql://pgwatch:pgwatch@localhost:5432/timetable
        pg-stage1_test602=postgresql://pgwatch:pgwatch@localhost:5432/test602
    ```
