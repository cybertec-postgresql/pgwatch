---
title: Listening ports and endpoints
---

Ports that pgwatch itself opens. All of them are configurable; the defaults come from the `cybertec/pgwatch` and `cybertecpostgresql/pgwatch-demo` images. Bind to `127.0.0.1` for anything you don't want reachable from the network.

Ports exposed by the bundled Grafana / Postgres / Prometheus containers live in the docker-compose stacks — see [Tutorial: Installing using Docker](../tutorial/docker_installation.md) for `3000`, `5432`, and the Prometheus `9090` server.

| Port | Component | Flag / URI | Default endpoint(s) |
|---|---|---|---|
| `8080` | Web UI and REST API | `--web-addr=:8080` (`PW_WEBADDR`) | `/` (UI), `/login`, `/source`, `/source/{name}`, `/metric`, `/metric/{name}`, `/preset`, `/preset/{name}`, `/test-connect`, `/liveness`, `/readiness`. Disable the UI entirely with `--web-disable=all`. |
| `9090` | Prometheus scrape endpoint | `--sink=prometheus://:9090/<namespace>` | `/metrics` (Prometheus text exposition format). Any port is configurable via the URI. |
| `9187` | Prometheus scrape endpoint (Compose default) | `--sink=prometheus://:9187/pgwatch` (set in `docker/compose.pgwatch.yml`) | Same `/metrics` endpoint; published at `localhost:9187/metrics`. |
