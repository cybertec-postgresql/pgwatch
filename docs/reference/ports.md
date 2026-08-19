---
title: Ports and endpoints
---

# pgwatch-served ports

These are the ports pgwatch itself listens on. All of them are configurable; the defaults come from the `cybertec/pgwatch` and `cybertecpostgresql/pgwatch-demo` images. Bind to `127.0.0.1` for anything you don't want reachable from the network.

| Port | Component | Flag / URI | Default endpoint(s) |
|---|---|---|---|
| `8080` | Web UI and REST API | `--web-addr=:8080` (`PW_WEBADDR`) | `/` (UI), `/login`, `/source`, `/source/{name}`, `/metric`, `/metric/{name}`, `/preset`, `/preset/{name}`, `/test-connect`, `/liveness`, `/readiness`. Disable the UI entirely with `--web-disable=all`. |
| `9090` | Prometheus scrape endpoint | `--sink=prometheus://:9090/<namespace>` | `/metrics` (Prometheus text exposition format). Any port is configurable. |
| `9187` | Prometheus scrape endpoint (Docker demo default) | `--sink=prometheus://:9187/pgwatch` (set in `docker/compose.pgwatch.yml`) | Same `/metrics` endpoint; published at `localhost:9187/metrics`. |

# Demo / compose ports

These ports are **only** exposed when running the bundled Docker Compose stacks under `docker/` — they belong to Grafana, the optional Postgres container that hosts the config DB and metrics sink, and the optional Prometheus server.

| Port | Component | Stack | Notes |
|---|---|---|---|
| `3000` | Grafana | `docker/compose.grafana.yml` | Dashboarding UI. Anonymous access by default; flip with `PW_GRAFANANOANONYMOUS=1`. |
| `5432` | Postgres | `docker/compose.postgres.yml` (and `compose.timescaledb.yml`) | Hosts both the configuration DB (`pgwatch`) and the metrics sink (`pgwatch_metrics`). Not exposed by default — must be published with `-p 5432:5432`. Bind to `127.0.0.1` for backups. |
| `9090` | Prometheus server | `docker/compose.prometheus.yml` | Web UI at `localhost:9090`, scrapes pgwatch at `pgwatch:9187`. |
