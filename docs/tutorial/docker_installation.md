---
title: Installing using Docker
---

This tutorial walks you through launching pgwatch with the official Docker image, getting a Grafana dashboard up, and adding your first source. It uses the batteries-included `cybertecpostgresql/pgwatch-demo` image.

If you want to drive pgwatch with YAML files instead of the bundled Config DB, see [How-to: Configure pgwatch with YAML files](../howto/yaml_configuration.md). For the slim gatherer-only image, see [Reference: Docker images](../reference/docker_images.md).

## Step 1 — Pull the image

```bash
docker pull cybertecpostgresql/pgwatch-demo:latest
```

## Step 2 — Run the container

Expose Grafana on port 3000 and (optionally) the admin Web UI on 8080. The `--restart=unless-stopped` flag brings the container back automatically after a reboot.

```bash
docker run -d --restart=unless-stopped \
  -p 3000:3000 -p 8080:8080 \
  --name pgwatch-demo cybertecpostgresql/pgwatch-demo:latest
```

After a few seconds, open Grafana at `http://localhost:3000` (default credentials `admin` / `pgwatchadmin`) — you should see the *Health-check* dashboard populated with metrics from the internal Postgres config DB.

## Step 3 — Add a source to monitor

Open the admin Web UI at `http://localhost:8080` and go to **SOURCES**. Click **+ NEW**, fill in the connection details of the database you want to monitor, and pick a preset (`minimal`, `basic`, or `exhaustive`). Save the source.

> It can take up to 2 minutes for a newly added source to start producing metrics. Tune this via `PW_REFRESH`.

That's the happy path. From here you usually want one of:

- **A more durable setup** with named volumes — see below.
- **A custom install** with full control over every component — see [Tutorial: Custom installation](custom_installation.md).
- **Production hardening** — see [How-to: Harden a Docker deployment](../howto/harden_docker_deployment.md).

## More future-proof setup

For setups you intend to keep running, mount named Docker volumes for every component so upgrades don't lose state:

```bash
for v in pg grafana pgwatch ; do docker volume create $v ; done

docker run -d --restart=unless-stopped --name pgwatch \
    -p 3000:3000 -p 127.0.0.1:5432:5432 -p 192.168.1.XYZ:8080:8080 \
    -v pg:/var/lib/postgresql \
    -v grafana:/var/lib/grafana \
    -v pgwatch:/pgwatch/persistent-config \
    cybertecpostgresql/pgwatch-demo:latest
```

The Postgres port (5432) is bound to localhost only — this lets you run native backup tools without exposing the database to the network.

## Compose-based setup with YAML sources and dual sinks

For an example that wires pgwatch through Docker Compose with a YAML sources file and **two** sinks (Postgres + Prometheus), see the [`docker/docker-compose.yml`](https://github.com/cybertec-postgresql/pgwatch/blob/master/docker/docker-compose.yml) file in the repository. The accompanying `sources.yaml` in the same directory configures a single `demo` source.

## See also

- [Reference: Docker images](../reference/docker_images.md) — image catalogue
- [Reference: Ports](../reference/ports.md) — what every port is for
- [How-to: Configure pgwatch with YAML files](../howto/yaml_configuration.md) — when you outgrow the Config-DB image
