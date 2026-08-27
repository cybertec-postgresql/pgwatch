---
title: Docker image variables
---

This page lists the environment variables that are read by the official pgwatch **Docker image wrappers** and consumed by Grafana inside the container. It does **not** list the gatherer daemon's CLI flags — those live in [CLI & environment variables](cli_env.md). Every gatherer flag also accepts an equivalent environment variable.

> Command-line flags always override environment variables.

## Gatherer daemon (within the Docker image)

Every gatherer flag listed in [CLI & environment variables](cli_env.md) can be passed through to the daemon inside the container as an environment variable. The naming convention is `PW_<FLAG_NAME>` (uppercase flag, dashes replaced by underscores). Example: `--web-user` → `PW_WEBUSER`, `--batching-delay` → `PW_BATCHING_DELAY`.

## Docker image wrapper

| Variable | Purpose | Default |
|---|---|---|
| `PW_TESTDB` | When set, the internal Config DB is added to monitoring as a source named `test`. | _unset_ |

## Grafana (inside the image)

| Variable | Purpose | Default |
|---|---|---|
| `PW_GRAFANANOANONYMOUS` | When set, viewing dashboards requires login. | _unset_ (anonymous viewing allowed) |
| `PW_GRAFANAUSER` | Grafana administrative user. | `admin` |
| `PW_GRAFANAPASSWORD` | Grafana administrative user password. | `pgwatchadmin` |
| `PW_GRAFANASSL` | When set, Grafana serves over HTTPS. | _unset_ |
| `PW_GRAFANA_BASEURL` | Public base URL — used for "Query details" links in the Stat Statement Overview dashboard. | `http://0.0.0.0:3000` |

## Web UI

Web UI authentication and TLS are configured through gatherer flags, not image-level variables. See [CLI & environment variables — WebUI](cli_env.md#webui).
