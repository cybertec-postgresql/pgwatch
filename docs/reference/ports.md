---
title: Listening ports and endpoints
---

Ports pgwatch itself opens. The defaults are baked into the pgwatch binary's CLI flags; the Dockerfiles just `EXPOSE` the same numbers, and the compose files in `docker/` publish them to the host. Bind any of these to `127.0.0.1` if you don't want them reachable from the network.

Ports exposed by the bundled Grafana / Postgres / Prometheus containers live in the docker-compose stacks — see [Tutorial: Installing using Docker](../tutorial/docker_installation.md) for `3000`, `5432`, and the Prometheus-server `9090`.

| Port | Component | Default from | Default endpoint(s) |
|---|---|---|---|
| `8080` | Web UI and REST API | `--web-addr=:8080` (`PW_WEBADDR`), default in `internal/webserver/cmdopts.go` | `/` (UI), `/login`, `/source`, `/source/{name}`, `/metric`, `/metric/{name}`, `/preset`, `/preset/{name}`, `/test-connect`, `/liveness`, `/readiness`. Disable the UI entirely with `--web-disable=all`. |
| `9090` | Prometheus scrape endpoint | chosen by the operator via `--sink=prometheus://:9090/<namespace>` (the URI is the source of truth) | `/metrics` (Prometheus text exposition format). The `docker/compose.pgwatch.yml` stack uses this default. |
