---
title: Installation options
---

Besides freedom of choosing from a set of metric measurements storage options one can
also choose how is the monitoring configuration
(connect strings, metric sets and intervals) going to be stored.

## Configuration database based operation

This is the original central pull mode depicted on the
[architecture diagram](components.md#component-diagram).
It requires a small schema to be rolled out on any Postgres
database accessible to the metrics gathering daemon, which will hold the
connect strings, metric definition SQLs and preset configurations and
some other more minor attributes. For rollout details see the
[custom installation](../tutorial/custom_installation.md) chapter.

The default Docker demo image `cybertecpostgresql/pgwatch-demo` uses this approach.

## File based operation

One can deploy the gatherer daemon(s) decentralized with
*sources to be monitored* defined in simple YAML files. In that case there
is no need for the central Postgres configuration database. See the
[sample.sources.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/contrib/sample.sources.yaml)
config file for an example.

!!! Note
    In this mode you also may want, but not forced, to point out the path to
    metric definition YAML file when starting the
    gatherer. Also note that the configuration system supports multiple
    YAML files in a folder so that you could easily programmatically manage
    things via *Ansible*, for example, and you can also use environment
    variables inside YAML files.
