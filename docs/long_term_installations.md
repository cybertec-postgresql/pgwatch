---
title: Long term installations
---

For long term pgwatch3 setups the main challenge is to keep the software
up-to-date to guarantee stable operation and also to make sure that all
DB-s are under monitoring.

## Keeping inventory in sync

Adding new DBs to monitoring and removing those shut down, can become a
problem if teams are big, databases are many, and it's done per hand
(common for on-premise, non-orchestrated deployments). To combat that,
the most typical approach would be to write some script or Cronjob that
parses the company's internal inventory database, files or endpoints
and translate changes to according CRUD operations on the
*pgwatch3.monitored_db* table directly.

One could also use the *REST API* for that purpose.

If pgwatch3 configuration is kept in YAML files, it should be also
relatively easy to automate the maintenance as the configuration can be
organized so that one file represent a single monitoring entry, i.e. the
`--config` parameter can also refer to a folder of YAML files.

## Updating the pgwatch3 collector

The pgwatch3 metrics gathering daemon is the core component of the
solution alas the most critical one. So it's definitely recommended to
update it at least once per year or minimally when some freshly released
Postgres major version instances are added to monitoring. New Postgres
versions don't necessary mean that something will break, but you'll be
missing some newly added metrics, plus the occasional optimizations. See
the [upgrading chapter](upgrading.md) for details, but basically
the process is very similar to initial installation as the collector
doesn't have any state on its own - it's just on binary program.

## Metrics maintenance

Metric definition SQL-s are regularly corrected as suggestions /
improvements come in and also new ones are added to cover latest
Postgres versions, so would make sense to refresh them 1-2x per year.

If using a YAML based config, just installing newer pre-built RPM / DEB
packages will do the trick automatically (built-in metrics at
*/etc/pgwatch3/metrics* will be refreshed) but for Config DB based
setups you'd need to follow a simple process described
[here](upgrading.md#updating-metric-definitions).

## Dashboard maintenance

Same as with metrics, also the built-in Grafana dashboards are being
actively updates, so would make sense to refresh them occasionally also.
The bulk delete / import scripts can be found
[here](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana)
or you could also manually just re-import some dashboards of interest
from JSON files in [/etc/pgwatch3/grafana-dashboards] folder
or from
[Github](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana).

!!! Info
    Notable new dashboards are usually listed also in [release
    notes](https://github.com/cybertec-postgresql/pgwatch/blob/master/docs/CHANGELOG.md)
    and most dashboards also have a sample
    [screenshots](https://github.com/cybertec-postgresql/pgwatch/tree/master/docs/screenshots)
    available.

## Storage monitoring

In addition to all that you should at least initially periodically
monitor the metrics DB size\...as it can grow quite a lot (especially
when using Postgres for storage) when the monitored databases have
hundreds of tables / indexes and if a lot of unique SQL-s are used and
*pg_stat_statements* monitoring is enabled. If the storage grows too
fast, one can increase the metric intervals (especially for
"table_stats", "index_stats" and "stat_statements") or decrease
the data retention periods via *\--pg-retention-days* or
*\--iretentiondays* params.
