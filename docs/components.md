---
title: Components
---

The main development idea around pgwatch3 was to do the minimal work
needed and not to reinvent the wheel - meaning that pgwatch3 is mostly
just about gluing together already some proven pieces of software for
metrics storage and using Grafana for dashboarding. So here a listing of
components that can be used to build up a monitoring setup around the
pgwatch3 metrics collector. Note that most components are not mandatory
and for tasks like metrics storage there are many components to choose
from.

## The metrics gathering daemon

The metrics collector, written in Go, is the only mandatory and most
critical component of the whole solution. The main task of the pgwatch3
collector / daemon is pretty simple - reading the configuration and
metric defintions, fetching the metrics from the configured databases
using the configured connection info and finally storing the metrics to
some other database, or just exposing them over a port for scraping in
case of Prometheus mode.

## Configuration

The configuration says which databases, how often and with which metrics
(SQL-s queries) are to be gathered. There are 3 options to store the
configuration:

- A PostgreSQL database holding a simple schema with 5 tables;
- File based approach - YAML config file.

## Measurements storage

Many options here so that one can for example go for maximum storage
effectiveness or pick something where they already know the query
language:

### [PostgreSQL](https://www.postgresql.org/) 
PostgreSQL is a world's most advanced Open Source RDBMS.

Postgres storage is based on the JSONB datatype so minimally
version 9.4+ is required, but for bigger setups where partitioning
is a must, v11+ is needed. Any already existing Postgres database
will do the trick, see the [Bootstrapping the Metrics DB](metrics_db_bootstrap) section for2 details.

### [TimescaleDB](https://www.timescale.com/) 
TimescaleDB is a time-series extension for PostgreSQL.

Although technically a plain extension it's often mentioned as a
separate database system as it brings custom data compression to
the table, enabling huge disk savings over standard Postgres. Note
that pgwatch3 does not use Timescale's built-in *retention*
management but a custom version.

### [Prometheus](https://prometheus.io/) 
Prometheus is a time series database and monitoring system.

Though Prometheus is not a traditional database system, it's a
good choice for monitoring Cloud-like environments as the
monitoring targets don't need to know too much about how actual
monitoring will be carried out later and also Prometheus has a
nice fault-tolerant alerting system for enterprise needs. By
default Prometheus is not set up for long term metrics storage!

### JSON files
Plain text files for testing / special use cases.

## The Web UI

The second homebrewn component of the pgwatch3 solution is an optional
and relatively simple Web UI for administering details of the monitoring
configuration like which databases should be monitored, with which
metrics and intervals. Besides that there are some basic overview tables
to analyze the gathered data and also possibilities to delete unneeded
metric data (when removing a test host for example).


!!! Note
    Note that the Web UI can only be used if storing the configuration in
    the database (Postgres).

## Metrics representation

Standard pgwatch3 setup uses [Grafana](http://grafana.org/) for
analyzing the gathered metrics data in a visual, point-and-click way.
For that a rich set of predefined dashboards for Postgres is provided,
that should cover the needs of most users - advanced users would mostly
always want to customize some aspects though, so it's not meant as a
one-size-fits-all solution. Also as metrics are stored in a DB, they can
be visualized or processed in any other way.

## Component diagram

Component diagram of a typical setup:

[![pgwatch3 typical deployment architecture diagram](screenshots/pgwatch3_architecture.png)](screenshots/pgwatch3_architecture.png)

## Component reuse

All components are *loosely coupled*, thus for non-pgwatch3 components
(pgwatch3 components are only the metrics collector and the optional Web
UI) you can decide to make use of an already existing installation of
Postgres, Grafana or Prometheus and run additionally just the pgwatch3
collector.

### To use an existing Postgres DB for storing the monitoring config

Create a new pgwatch3 DB, preferrably also an accroding role who
owns it. Then roll out the schema
(pgwatch3/sql/config_store/config_store.sql) and set the following
parameters when running the image: `PW_PGHOST`, `PW_PGPORT`,
`PW_PGDATABASE`, `PW_PGUSER`, `PW_PGPASSWORD`, `PW_PGSSL` (optional).

### To use an existing Grafana installation

Load the pgwatch3 dashboards from *grafana_dashboard* folder if
needed (one can totally define their own) and set the following
paramater: `PW_GRAFANA_BASEURL`. This parameter only provides correct
links to Grafana dashboards from the Web UI. Grafana is the most
loosely coupled component for pgwatch3 and basically doesn't have
to be used at all. One can make use of the gathered metrics directly
over the Postgres (or Graphite) API-s.

### To use an existing Postgres DB for storing metrics

1.  Roll out the metrics storage schema according to instructions
    from [here](preparing_databases.md).
1.  Following parameters need to be set for the gatherer:
    -   `--datastore=postgres` or `PW_DATASTORE=postgres`
    -   `--pg-metric-store-conn-str="postgresql://user:pwd@host:port/db"`
        or `PW_PG_METRIC_STORE_CONN_STR="..."`
    -   optionally also adjust the `--pg-retention-days` parameter. By
        default 14 days for Postgres are kept

1.  If using the Web UI also set the datastore parameters
    `--datastore` and `--pg-metric-store-conn-str` if wanting to
    have an option to be able to clean up data also via the UI in a
    more targeted way.

When using Postgres metrics storage, the schema rollout script
activates "asynchronous commiting" feature for the *pgwatch3* role
in the metrics storage DB by default! If this is not wanted (no
metrics can be lost in case of a crash), then re-enstate normal
(synchronous) commiting with below query and restart the pgwatch3
agent:
```sql
ALTER ROLE pgwatch3 IN DATABASE $MY_METRICS_DB SET synchronous_commit TO on;
```