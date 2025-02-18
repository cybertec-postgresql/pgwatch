---
title: Migrating from v2 to v3
---

## Introduction

This guide will help you migrate from pgwatch2 to pgwatch v3. The migration process is straightforward and should not take long.
Depending on your setup, you may need to migrate the configuration database or the configuration YAML files.
Feel free to skip the steps that do not apply to your setup.

!!! note
    Here and in the following examples under `pgwatch` name we understand v3 related binary, database, and user.
    If we refer to v2 related binary, database, or user, we will use `pgwatch2` name.

## Migrate the Configuration Database

!!! warning
    Before migrating, please, make a backup of your configuration database!

Assuming you have a local PostgreSQL database named `pgwatch2` and a user named `pgwatch2` with
the password `pgwatch2admin`, you can migrate the configuration database to pgwatch3 by following these steps:

1. Connect as superuser to the PostgreSQL instance and rename the `pgwatch2` role and database to `pgwatch`:

    ```terminal
    $ psql -U postgres -h localhost -p 5432 -d postgres
    psql (17.2)

    postgres=# ALTER ROLE pgwatch2 RENAME TO pgwatch;
    ALTER ROLE

    postgres=# ALTER ROLE pgwatch WITH PASSWORD 'pgwatchadmin';
    ALTER ROLE

    postgres=# ALTER DATABASE pgwatch2 RENAME TO pgwatch;
    ALTER DATABASE
    ```

1. Use the `config init` command to create the necessary v3 tables and indexes in the `pgwatch` database:

    ```terminal
    pgwatch --sources=postgresql://pgwatch:pgwatchadmin@localhost/pgwatch config init
    ```

### Migrate the monitored sources

To migrate the monitored sources execute the following query:

```sql
insert into pgwatch.source
select md_unique_name as name,
    format('postgresql://%s:%s@%s:%s/%s', md_user, md_password, md_hostname, md_port, md_dbname) as connstr,
    md_is_superuser as is_superuser,
    md_preset_config_name as preset_config,
    md_config as config,
    md_is_enabled as is_enabled,
    md_last_modified_on as last_modified_on,
    md_dbtype as dbtype,
    md_include_pattern as include_pattern,
    md_exclude_pattern as exclude_pattern,
    md_custom_tags as custom_tags,
    md_group as "group",
    md_host_config as host_config,
    md_only_if_master as only_if_master,
    md_preset_config_name_standby as preset_config_standby,
    md_config_standby as config_standby
from pgwatch2.monitored_db;
```

### Migrate the metrics and presets

!!! note
    You may skip this step if you have not created custom metrics or presets.
    All built-in metrics and presets are already included in pgwatch3.

Exucute the following queries to migrate the metrics and presets:

```sql
insert into pgwatch.preset
select 
    pc_name as name, 
    pc_description as description, 
    pc_config as metrics 
from pgwatch2.preset_config
on conflict do nothing;
```

## Migrate the Configuration YAML Files

To Be Done
