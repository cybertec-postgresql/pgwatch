---
title: Docker images
---

The pgwatch project publishes two Docker images on [Docker Hub](https://hub.docker.com/u/cybertecpostgresql):

| Image | Includes | Use it when |
|---|---|---|
| `cybertecpostgresql/pgwatch-demo` | Gatherer daemon, Postgres (config DB + metrics sink), Grafana, Web UI | You want a single-container demo or all-in-one deployment with the bundled Config DB. |
| `cybertecpostgresql/pgwatch` | Gatherer daemon only | You already run Grafana, Prometheus, and/or a Postgres instance elsewhere and want pgwatch to plug into that stack. |

## cybertecpostgresql/pgwatch-demo

The original pgwatch "batteries-included" image. Add connection details to a monitored database through the admin Web UI (or by inserting directly into the bundled Postgres Config DB) and use the pre-defined Grafana dashboards to analyse the metrics.

This image is the one used by the [Docker installation tutorial](../tutorial/docker_installation.md) and by the [Harden a Docker deployment](../howto/harden_docker_deployment.md) recipe.

## cybertecpostgresql/pgwatch

A lightweight image that contains only the gatherer daemon. It can be wired into an existing monitoring stack by:

- mounting a YAML file or directory at a known path and passing `--sources=/path/to/dir/`,
- connecting to an external Postgres Config DB via `--sources=postgresql://...`,
- passing metric definitions via `--metrics=/path/to/metrics.yaml`.

For an end-to-end Compose example, see [`docker/docker-compose.yml`](https://github.com/cybertec-postgresql/pgwatch/blob/master/docker/docker-compose.yml) in the repository.

## Building custom images

For custom tweaks, stricter security defaults, or pinning specific component versions, build the images yourself with the [`docker/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/docker) Dockerfiles in the repository.
