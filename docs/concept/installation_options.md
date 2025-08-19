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
**sources to be monitored** and **metrics and presets definitions** 
defined in simple YAML files. In that case there is 
no need for the central Postgres configuration database. See
[sample.sources.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/contrib/sample.sources.yaml)
And
[sample.metrics.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/contrib/sample.metrics.yaml)
config files for example.

!!! Note 
    You can use folder of YAML files instead of a single file 
    so that you can easily manage things via *Ansible*, for example. 

!!! Note 
    Environment variables can be used inside sources YAML files.  
    example:
    ```yaml
    - name: ...
      conn_str: $MY_VERY_SECRET_CONN_STR
      ...
    ```