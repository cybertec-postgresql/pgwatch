---
title: Ports
---

Ports exposed by pgwatch components. Bind to `127.0.0.1` (loopback) for components you do not want reachable from the network.

| Port | Component | Image | Notes |
|---|---|---|---|
| `3000` | Grafana | `cybertecpostgresql/pgwatch-demo` | Dashboarding UI. Auth is anonymous by default; flip with `PW_GRAFANANOANONYMOUS=1`. |
| `8080` | Web UI / REST API | both | Admin UI for managing sources, metrics, presets. Disable with `--web-disable=all`. |
| `5432` | Postgres (config DB + metrics sink) | `cybertecpostgresql/pgwatch-demo` | Not exposed by default — must be published with `-p`. Bind to `127.0.0.1` for backups. |
| `9090` | Prometheus sink | `cybertec/pgwatch` (when `--sink=prometheus://:9090`) | Listen port for Prometheus scrape. |
| `9187` | Prometheus sink (alternate) | `cybertec/pgwatch` | Any port is configurable via the `--sink=prometheus://:<port>/<namespace>` URI. |
