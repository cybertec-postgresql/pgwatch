---
title: Monitoring managed cloud databases
---

Although all cloud service providers offer some kind of built-in
instrumentation and graphs, they're mostly rather conservative in this
are not to consume extra server resources and not to overflow and
confuse beginners with too much information. So for advanced
troubleshooting it might make sense to gather some additional metrics on
your own, especially given that you can also easily add custom business
metrics to pgwatch using plain SQL, for example to track the amount of
incoming sales orders. Also with pgwatch / Grafana you have more
freedom on the visual representation side and access to around 30
prebuilt dashboards and a lot of freedom creating custom alerting rules.

The common denominator for all managed cloud services is that they
remove / disallow dangerous or potentially dangerous functionalities
like file system access and untrusted PL-languages like Python - so
you'll lose a small amount of metrics and "helper functions" compared
to a standard on-site setup described in the
`previous chapter <preparing_databases>`{.interpreted-text role="ref"}.
This also means that you will get some errors displayed on some preset
dashboards like "DB overview" and thus will be better off using a
dashboard called "DB overview Unprivileged" tailored specially for
such a use case.

pgwatch has been tested to work with the following managed database
services:

## Google Cloud SQL for PostgreSQL

- No Python / OS helpers possible. OS metrics can be integrated in
    Grafana though using the
    [Stackdriver](https://grafana.com/docs/grafana/latest/datasources/google-cloud-monitoring/)
    data source.
- `pg_monitor` system role available.
- pgwatch default preset name: `gce`.
- Documentation: <https://cloud.google.com/sql/docs/postgres>

To get most out pgwatch on GCE you need some additional clicks in the
GUI / Cloud Console "Flags" section to enable some common PostgreSQL
monitoring parameters like `track_io_timing` and `track_functions`.

## Amazon RDS for PostgreSQL

- No Python / OS helpers possible. OS metrics can be integrated in
    Grafana though using the
    [CloudWatch](https://grafana.com/docs/grafana/latest/datasources/cloudwatch/)
    data source

- `pg_monitor` system role available.

- pgwatch default preset names: `rds`

- Documentation:

    <https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_PostgreSQL.html>
    <https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Aurora.AuroraPostgreSQL.html>

Note that the AWS Aurora PostgreSQL-compatible engine is missing some
additional metrics compared to normal RDS.

## Azure Database for PostgreSQL

- No Python / OS helpers possible. OS metrics can be integrated in
    Grafana though using the [Azure
    Monitor](https://grafana.com/docs/grafana/latest/datasources/azuremonitor/)
    data source
- `pg_monitor` system role available.
- pgwatch default preset name: `azure`
- Documentation: <https://docs.microsoft.com/en-us/azure/postgresql/>

Surprisingly on Azure some file access functions are whitelisted, thus
one can for example use the `wal_size` metrics.

!!! Note
    By default Azure has **pg_stat_statements** not fully activated by
    default, so you need to enable it manually or via the API.
    Documentation: <https://docs.microsoft.com/en-us/azure/postgresql/howto-optimize-query-stats-collection>

## Aiven for PostgreSQL

The [Aiven developer
documentation](https://aiven.io/docs/products/postgresql/howto/monitor-with-pgwatch2)
contains information on how to monitor PostgreSQL instances running on
the Aiven platform with pgwatch.
