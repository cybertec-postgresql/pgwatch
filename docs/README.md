---
title: Introduction
---

pgwatch is a flexible PostgreSQL-specific monitoring solution, relying
on Grafana dashboards for the UI part. It supports monitoring of almost
all metrics for Postgres versions 9.0 to 13 out of the box and can be
easily extended to include custom metrics. At the core of the solution
is the metrics gathering daemon written in Go, with many options to
configure the details and aggressiveness of monitoring, types of metrics
storage and the display the metrics.

# Quick start with Docker

For the fastest setup experience Docker images are provided via Docker
Hub (if new to Docker start
[here](https://docs.docker.com/get-started/)). For custom setups see the
[Custom installations](custom_installation.md) paragraph below or turn to the pre-built DEB / RPM / Tar
packages on the Github Releases
[page](https://github.com/cybertec-postgresql/pgwatch/releases).

Launching the latest pgwatch Docker image with built-in Postgres
metrics storage DB:

    # run the latest Docker image, exposing Grafana on port 3000 and the administrative web UI on 8080
    docker run -d -p 3000:3000 -p 8080:8080 -e PW_TESTDB=true --name pw3 cybertec/pgwatch

After some minutes you could for example open the [DB
overview](http://127.0.0.1:3000/dashboard/db/db-overview) dashboard
and start looking at metrics in Grafana. For defining your own
dashboards or making changes you need to log in as admin (default
user/password: `admin/pgwatchadmin`).

If you don't want to add the `"test"` database (the pgwatch
configuration DB holding connection strings to monitored DBs and metric
definitions) to monitoring, remove the `PW_TESTDB` env variable.

Also note that for long term production usage with Docker it's highly
recommended to use separate *volumes* for each pgwatch component - see
[here](docker_installation.md) for a better launch example.

# Typical "pull" architecture

To get an idea how pgwatch is typically deployed a diagram of the
standard Docker image fetching metrics from a set of Postgres databases
configured via a configuration DB:

[![pgwatch typical deployment architecture diagram](screenshots/pgwatch_architecture.png)](screenshots/pgwatch_architecture.png)

# Typical "push" architecture

A better fit for very dynamic (Cloud) environments might be a more
de-centralized "push" approach or just exposing the metrics over a
port for remote scraping. In that case the only component required would
be the pgwatch metrics collection daemon.

[![pgwatch "push" deployment architecture diagram](screenshots/pgwatch_architecture_push.png)](screenshots/pgwatch_architecture_push.png)
