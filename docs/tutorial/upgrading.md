---
title: Upgrading
---

The pgwatch daemon code doesn't need too much maintenance itself (if
you're not interested in new features), but the preset metrics,
dashboards and the other components that pgwatch relies on, like Grafana,
are under very active development and get updates quite regularly so
already purely from the security standpoint it would make sense to stay
up to date.

We also regularly include new component versions in the Docker images
after verifying that they work. If using Docker, you could also choose
to build your own images any time some new component versions are
released, just increment the version numbers in the Dockerfile.

## Updating to a newer Docker version

### Without volumes

If pgwatch container was started in the simplest way possible without
volumes, and if previously gathered metrics are not of great importance,
and there are no user modified metric or dashboard changes that should
be preserved, then the easiest way to get the latest components would be
just to launch new container and import the old monitoring config:

    # let's backup up the monitored hosts
    psql -p5432 -U pgwatch -d pgwatch -c "\copy monitored_db to 'monitored_db.copy'"

    # stop the old container and start a new one ...
    docker stop ... && docker run ....

    # import the monitored hosts
    psql -p5432 -U pgwatch -d pgwatch -c "\copy monitored_db from 'monitored_db.copy'"

If metrics data and other settings like custom dashboards need to be
preserved then some more steps are needed, but basically it's about
pulling Postgres backups and restoring them into the new container.

A tip: to make the restore process easier it would already make sense to
mount the host folder with the backups in it on the new container with
`"-v \~/pgwatch_backups:/pgwatch_backups:rw,z"` when starting the
Docker image. Otherwise, one needs to set up SSH or use something like S3
for example. Also note that port 5432 need to be exposed to take backups
outside of Docker for Postgres respectively.

### With volumes

To make updates a bit easier, the preferred way to launch pgwatch
should be to use Docker volumes for each individual component - see the
[Installing using Docker](docker_installation.md)
chapter for details. Then one can just stop the old
container and start a new one, re-using the volumes.

With some releases though, updating to newer version might additionally
still require manual rollout of Config DB *schema migrations scripts*,
so always check the release notes for hints on that or just go to the
`"pgwatch/sql/migrations"` folder and execute all SQL scripts that have
a higher version than the old pgwatch container. Error messages like
will "missing columns" or "wrong datatype" will also hint at that,
after launching with a new image. FYI - such SQL "patches" are
generally not provided for metric updates, nor dashboard changes, and
they need to be updated separately.

## Updating without Docker

For a custom installation there's quite some freedom in doing updates -
as components (Grafana, PostgreSQL) are loosely coupled, they can be
updated any time without worrying too much about the other components.
Only "tightly coupled" components are the pgwatch metrics collector,
config DB and the optional Web UI - if the pgwatch config is kept in
the database. If [YAML based approach](../concept/installation_options.md) is used, then things
are even more simple - the pgwatch daemon can be updated any time as
YAML schema has default values for everything and there are no other
"tightly coupled" components like the Web UI.

### Updating Grafana

The update process for Grafana looks pretty much like the installation
so take a look at the according
[chapter](custom_installation.md#detailed-steps-for-the-configurartion-database-approach-with-postgres-sink).
If using Grafana package repository it should happen automatically along
with other system packages. Grafana has a built-in database schema
migrator, so updating the binaries and restarting is enough.

### Updating Grafana dashboards

There are no update or migration scripts for the built-in Grafana
dashboards as it would break possible user applied changes. If you know
that there are no user changes, then one can just delete or rename the
existing ones in bulk and import the latest JSON definitions.
See [here](../concept/long_term_installations.md) for
some more advice on how to manage dashboards.

### Updating the config / metrics DB version

Database updates can be quite complex, with many steps, so it makes
sense to follow the manufacturer's instructions here.

For PostgreSQL one should distinguish between minor version updates and
major version upgrades. Minor updates are quite straightforward and
problem-free, consisting of running something like:

    apt update && apt install postgresql
    sudo systemctl restart postgresql

For PostgreSQL major version upgrades one should read through the
according release notes (e.g.
[here](https://www.postgresql.org/docs/17/release-17.html#id-1.11.6.5.4))
and be prepared for the unavoidable downtime.

### Updating the pgwatch schema

This is the pgwatch specific part, with some coupling between the
following components - Config DB SQL schema, metrics collector, and the
optional Web UI.

Here one should check from the
[CHANGELOG](https://github.com/cybertec-postgresql/pgwatch/releases)
if pgwatch schema needs updating. If yes, then manual applying of
schema diffs is required before running the new gatherer or Web UI. If
no, i.e. no schema changes, all components can be updated independently
in random order.

    pgwatch --config=postgresql://localhost/pgwatch --upgrade

### Updating the metrics collector

Compile or install the gatherer from RPM / DEB / tarball packages. See
the [Custom installation](custom_installation.md)  chapter for details.

If using a SystemD service file to auto-start the collector then you
might want to also check for possible updates on the template there -
`/etc/pgwatch/startup-scripts/pgwatch.service`.

### Updating metric definitions

In the YAML mode you always get new SQL definitions for the built-in
metrics automatically when refreshing the sources via GitHub or
pre-built packages, but with Config DB approach one needs to do it
manually. Given that there are no user added metrics, it's simple
enough though - just delete all old ones and re-insert everything from
the latest metric definition SQL file.

    pg_dump -t pgwatch.metric pgwatch > old_metric.sql  # a just-in-case backup
    psql  -c "truncate pgwatch.metric" pgwatch
    psql -f /etc/pgwatch/sql/config_store/metric_definitions.sql pgwatch

!!! Warning
    If you have added some own custom metrics be sure not to delete or truncate them!
