---
title: Installing using Docker
---

## Simple setup steps

The simplest real-life pgwatch setup should look something like that:

1. Decide which metrics storage engine you want to use -
    *cybertecpostgresql/pgwatch-demo* uses PostgreSQL.
    When only Prometheus sink is used (exposing a
    port for remote scraping), one should use the slimmer
    *cybertecpostgresql/pgwatch* image which doesn't have any built in
    databases.

1. Find the latest pgwatch release version by going to the project's
    GitHub *Releases* page or use the public API with something like
    that:

```bash
curl -so- https://api.github.com/repos/cybertec-postgresql/pgwatch/releases/latest | jq .tag_name | grep -oE '[0-9\.]+'
```

1. Pull the image:

```bash
docker pull cybertecpostgresql/pgwatch-demo:X.Y.Z
```

1. Run the Docker image, exposing minimally the Grafana port served on
    port 3000 internally. In a relatively secure environment you'd
    usually also include the administrative web UI served on port 8080:

```bash
docker run -d --restart=unless-stopped -p 3000:3000 -p 8080:8080 \
--name pw3 cybertecpostgresql/pgwatch-demo:X.Y.Z
```

Note that we're setting the container to be automatically restarted
in case of a reboot/crash - which is highly recommended if not using
some container management framework to run pgwatch.

## More future-proof setup steps

Although the above simple setup example will do for more temporal setups
/ troubleshooting sessions, for permanent setups it's highly
recommended to create separate volumes for all software components in
the container, so that it would be easier to
[update](upgrading.md) to newer pgwatch
Docker images and pull file system based backups, and also it might be a
good idea to expose all internal ports at least on *localhost* for
possible troubleshooting and making possible to use native backup tools
more conveniently for Postgres.

Note that, for maximum flexibility, security and update simplicity it's
best to do a custom setup though - see the next
[chapter](custom_installation.md) for that.

So in short, for plain Docker setups would be best to do something like:

```bash
# let's create volumes for Postgres, Grafana and pgwatch marker files / SSL certificates
for v in pg  grafana pw3 ; do docker volume create $v ; done

# launch pgwatch with fully exposed Grafana, Web UI, Postgres
docker run -d --restart=unless-stopped --name pw3 \
    -p 3000:3000 -p 127.0.0.1:5432:5432 -p 192.168.1.XYZ:8080:8080 \
    -v pg:/var/lib/postgresql/data -v grafana:/var/lib/grafana -v pw3:/pgwatch/persistent-config \
    cybertecpostgresql/pgwatch-demo:X.Y.Z
```

Note that in non-trusted environments it's a good idea to specify more
sensitive ports together with some explicit network interfaces for
additional security - by default Docker listens on all network devices!

Also note that one can configure many aspects of the software components
running inside the container via ENV - for a complete list of all
supported Docker environment variables see the [ENV_VARIABLES.md](../reference/env_variables.md) file.

## Available Docker images

Following images are regularly pushed to [Docker
Hub](https://hub.docker.com/u/cybertecpostgresql):

### cybertecpostgresql/pgwatch-demo

The original pgwatch “batteries-included” image with PostgreSQL measurements
storage. Just insert connect infos to your database via the admin Web UI (or
directly into the Config DB) and then turn to the pre-defined Grafana
dashboards to analyze DB health and performance.

### cybertecpostgresql/pgwatch

A light-weight image containing only the metrics collection daemon /
agent, that can be integrated into the monitoring setup over
configuration specified either via ENV, mounted YAML files or a
PostgreSQL Config DB. See the [Component reuse](custom_installation.md) chapter for
wiring details.

## Building custom Docker images

For custom tweaks, more security, specific component versions, etc. one
could easily build the images themselves, just a Docker installation is
needed.

## Interacting with the Docker container

- If launched with the `PW_TESTDB=1` env. parameter then the
    pgwatch configuration database running inside Docker is added to
    the monitoring, so that you should immediately see some metrics at
    least on the *Health-check* dashboard.

- To add new databases / instances to monitoring open the
    administration Web interface on port 8080 (or some other port, if
    re-mapped at launch) and go to the *SOURCES* page. Note that the Web UI
    is an optional component, and one can manage monitoring entries
    directly in the Postgres Config DB via `INSERT` / `UPDATE` into
    `"pgwatch.monitored_db"` table. Default user/password are again
    `pgwatch/pgwatchadmin`, database name - `pgwatch`. In both
    cases note that it can take up to 2min (default main loop time,
    changeable via `PW_SERVERS_REFRESH_LOOP_SECONDS`) before you see
    any metrics for newly inserted databases.

- One can edit existing or create new Grafana dashboards, change
    Grafana global settings, create users, alerts, etc. after logging in
    as `pgwatch/pgwatchadmin` (by default, changeable at launch
    time).

- Metrics and their intervals that are to be gathered can be
    customized for every database separately via a custom JSON config
    field or more conveniently by using *Preset Configs*, like
    "minimal", "basic" or "exhaustive" (`monitored_db.preset_config`
    table), where the name should already hint at the amount of metrics
    gathered. For privileged users the "exhaustive" preset is a good
    starting point, and "unprivileged" for simple developer accounts.

- To add a new metrics yourself (which are simple SQL queries
    returning any values and a timestamp) head to
    <http://127.0.0.1:8080/metrics>. The queries should always include a
    `"epoch_ns"` column and `"tag\_"` prefix can be used for columns
    that should be quickly searchable/groupable, and thus will be
    indexed with the PostgreSQL metric stores. See to the bottom of the
    "metrics" page for more explanations or the documentation chapter
    on metrics.

- For a quickstart on dashboarding, a list of available metrics
    together with some instructions are presented on the
    "Documentation" dashboard.

- Some built-in metrics like `"cpu_load"` and others, that gather
    privileged or OS statistics, require installing *helper functions*
    (looking like
    [that](https://github.com/cybertec-postgresql/pgwatch/blob/master/docs/tutorial/preparing_databases.md?plain=1#L111)),
    so it might be normal to see some blank panels or fetching errors in
    the logs. On how to prepare databases for monitoring see the
    [Monitoring preparations](preparing_databases.md) chapter.

- For effective graphing you want to familiarize yourself with the
    query language of the database system that was selected for metrics
    storage. Some tips to get going:

  - For PostgreSQL/TimescaleDB - some knowledge of [Window
        functions](https://www.postgresql.org/docs/current/tutorial-window.html)
        is a must if looking at longer time periods of data as the
        statistics could have been reset in the meantime by user
        request or the server might have crashed, so that simple
        `max() - min()` aggregates on cumulative counters (most data
        provided by Postgres is cumulative) would lie.

- For possible troubleshooting needs, logs of the components running
    inside Docker are by default (if not disabled on container launch)
    visible under:
    <http://127.0.0.1:8080/logs/%5Bpgwatch%7Cpostgres%7Cwebui%7Cgrafana>.
    It's of course also possible to log into the container and look at
    log files directly - they're situated under
    `/var/logs/supervisor/`.

    FYI - `docker logs ...` command is not really useful after a
    successful container startup in pgwatch case.

## Ports used

- *5432* - Postgres configuration or metrics storage DB (when using the
    cybertec/pgwatch image)
- *8080* - Management Web UI (monitored hosts, metrics, metrics
    configurations)
- *3000* - Grafana dashboarding

## Docker Compose

As mentioned in the [Components](../concept/components.md) chapter, remember that the pre-built Docker images are just
one example how your monitoring setup around the pgwatch metrics
collector could be organized. For another example how various components
(as Docker images here) can work together, see a *Docker Compose*
[example](https://github.com/cybertec-postgresql/pgwatch/blob/master/docker-compose.yml)
with loosely coupled components.

## Example of advanced setup using YAML files and dual sinks

pgwatch service in file `docker/docker-compose.yml` can look like this:

```yaml
  pgwatch:
    image: cybertecpostgresql/pgwatch:latest
    command:
      - "--web-disable=true"
      - "--sources=/sources.yaml"
      - "--sink=postgresql://pgwatch@postgres:5432/pgwatch_metrics"
      - "--sink=prometheus://:8080"
    volumes:
      - "./sources.yaml:/sources.yaml"
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
```

Source file `sources.yaml` in the same directory:

```yaml
- name: demo
  conn_str: postgresql://pgwatch:pgwatchadmin@postgres/pgwatch'
  preset_metrics: exhaustive
  is_enabled: true
  group: default
```

Running this setup you get pgwatch that uses sources from YAML file and
outputs measurements to postgres DB and exposes them for Prometheus
to scrape on port 8080 instead of WebUI (which is disabled by `--web-disable`).
Metrics definition are built-in, you can examine definition in [`internal/metrics/metrics.yaml`](https://github.com/cybertec-postgresql/pgwatch/blob/master/internal/metrics/metrics.yaml).
