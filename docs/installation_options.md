---
title: Installation options
---

Besides freedom of choosing from a set of metric storage options one can
also choose how they're going to retrieve metrics from databases - in a
"pull" or "push" way and how is the monitoring configuration
(connect strings, metric sets and intervals) going to be stored.

## Config DB based operation

This is the original central pull mode depicted on the
[architecture diagram](screenshots/pgwatch_architecture.png). 
It requires a small schema to be rolled out on any Postgres
database accessible to the metrics gathering daemon, which will hold the
connect strings, metric definition SQL-s and preset configurations and
some other more minor attributes. For rollout details see the
[custom installation](custom_installation.md) chapter.

The default Docker images use this approach.

## File based operation

From v1.4.0 one can deploy the gatherer daemon(s) decentrally with
*hosts to be monitored* defined in simple YAML files. In that case there
is no need for the central Postgres "config DB". See the sample
[instances.yaml](https://github.com/cybertec-postgresql/pgwatch/blob/master/src/sources/sample.sources.yaml)
config file for an example. Note that in this mode you also need to
point out the path to metric definition SQL files when starting the
gatherer. Also note that the configuration system also supports multiple
YAML files in a folder so that you could easily programmatically manage
things via *Ansible* for example and you can also use Env. vars in
sideYAML files.

Relevant Gatherer env. vars / flags: `--config, --metrics-folder` or
`PW_CONFIG / PW_METRICS_FOLDER`.

## Prometheus mode

In v1.6.0 was added support for Prometheus - being one of the most
popular modern metrics gathering / alerting solutions. When the
`--datastore / PW_DATASTORE` parameter is set to *prometheus* then the
pgwatch metrics collector doesn't do any normal interval-based
fetching but listens on port *9187* (changeable) for scrape requests
configured and performed on Prometheus side. Returned metrics belong to
the "pgwatch" namespace (a prefix basically) which is changeable via
the `--prometheus-namespace` flag if needed.

Also important to note - in this mode the pgwatch agent should not be
run centrally but on all individual DB hosts. While technically possible
though to run centrally, it would counter the core idea of Prometheus
and would make scrapes also longer and risk getting timeouts as all DBs
are scanned sequentially for metrics.

FYI -- the functionality has some overlap with the existing
[postgres_exporter](https://github.com/wrouesnel/postgres_exporter)
project, but also provides more flexibility in metrics configuration and
all config changes are applied "online".

Also note that Prometheus can only store numerical metric data values -
so metrics output for example PostgreSQL storage and Prometheus are not
100% compatile. Due to that there's also a separate "preset config"
named "prometheus".
