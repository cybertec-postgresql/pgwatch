---
title: Bootstrapping the Configuration Database
---

For background on the two configuration store formats (Postgres database vs YAML files) and their trade-offs, see [Concept: Installation options](../concept/installation_options.md). For the full CLI flag and environment-variable reference, see [Reference: CLI & environment variables](../reference/cli_env.md).

## Choosing a Database

pgwatch supports any database that supports the PostgreSQL wire protocol. This includes:

- PostgreSQL
- TimescaleDB
- CitusDB
- CockroachDB
- many more

We will use PostgreSQL in this guide. But the steps are similar for other databases. It's up to you to choose the database that best fits your needs and set it up accordingly.

## Creating the Database

First, we need to create a database for storing the configuration. We will use the `psql` command-line tool to create the database. You can also use a GUI tool like pgAdmin to create the database.

Let's assume we want to create a database named `pgwatch` on a completely fresh PostgreSQL installation.
It is wise to use a special role for the configuration database, so we will create a role named `pgwatch` and assign it to the `pgwatch` database.

```terminal
$ psql -U postgres -h localhost -p 5432 -d postgres
psql (17.2)

postgres=# CREATE ROLE pgwatch WITH LOGIN PASSWORD 'pgwatchadmin';
CREATE ROLE

postgres=# CREATE DATABASE pgwatch OWNER pgwatch;
CREATE DATABASE
```

That's it! We have created a database named `pgwatch` with the owner `pgwatch`. Now we can proceed to the next step.

## Init (optional)

pgwatch will automatically create the necessary tables and indexes in the database when it starts. But in case
you want to create the schema as a separate step, you can use the `config init` command. The only thing you
need is to provide the connection string to the database.

```terminal
pgwatch --sources=postgresql://pgwatch:pgwatchadmin@localhost/pgwatch config init
```

Or you can use the `config init` command with the `--metrics` flag, since metrics and sources share the same database.

```terminal
pgwatch --metrics=postgresql://pgwatch:pgwatchadmin@localhost/pgwatch config init
```

If you're using a PostgreSQL sink for storing measurements, you can also initialize the sink database schema:

```terminal
pgwatch --sink=postgresql://pgwatch:pgwatchadmin@localhost/measurements config init
```

## Usage

You can now configure pgwatch to use the `pgwatch` database as the configuration database for storing monitored sources,
metric definitions and presets.

```terminal
$ pgwatch --sources=postgresql://pgwatch:pgwatchadmin@localhost/pgwatch --sink=postgresql://pgwatch@10.0.0.42/measurements
...
[INFO] [metrics:75] [presets:17] [sources:2] sources and metrics refreshed
...
```

!!! info
    Even though configuration database can hold both sources and metrics definitions,
    you are free to use any combination of configurations. For example, you can use a database
    for metrics and YAML file for sources, or vice versa. The only combination that is
    rejected is two different Postgres connection strings for `--sources` and `--metrics`;
    when both are Postgres URIs they must be identical.

If now you want to see the tables created by pgwatch in the configuration database, you can connect to the database
using the `psql` command-line tool and list the tables.

```terminal
$ psql postgresql://pgwatch:pgwatchadmin@localhost/pgwatch
psql (17.2)

pgwatch=# \dt pgwatch.*
           List of relations
 Schema  |   Name    | Type  |  Owner
---------+-----------+-------+---------
 pgwatch | metric    | table |  owner
 pgwatch | migration | table |  owner
 pgwatch | preset    | table |  owner
 pgwatch | source    | table |  owner
(4 rows)
```

You may examine these tables to understand how pgwatch stores metrics and presets definitions, as well as what sources and how to monitor in the database.
