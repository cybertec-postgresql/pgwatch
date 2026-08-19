---
title: Quickstart
---

This is the fastest path from zero to "I see metrics in Grafana". You will run the single-container `cybertecpostgresql/pgwatch-demo` image, open the bundled Grafana, and add one source.

If anything here feels too brief, see [Tutorial: Installing using Docker](docker_installation.md) for the full Docker walk-through, [Tutorial: Custom installation](custom_installation.md) for a non-Docker install, or [Tutorial: Preparing databases for monitoring](preparing_databases.md) for the steps to take on each database you want to monitor.

## Step 1 — Pull and run

```bash
docker pull cybertecpostgresql/pgwatch-demo:latest
docker run -d --restart=unless-stopped \
  -p 3000:3000 -p 8080:8080 \
  -e PW_TESTDB=true \
  --name pgwatch-demo cybertecpostgresql/pgwatch-demo:latest
```

Two ports are exposed: **3000** for Grafana, **8080** for the admin Web UI.

## Step 2 — Open Grafana

Browse to `http://localhost:3000` and log in with `admin` / `pgwatchadmin`. The *Health check* dashboard is already populated with metrics from the bundled test database.

## Step 3 — Add a source

Browse to `http://localhost:8080`. Go to **SOURCES**, click **+ NEW**, fill in:

- **Name**: any unique label
- **Kind**: `postgres` (or whichever matches your target — see [Reference: Source types](../reference/source_types.md))
- **Connection string**: e.g. `postgresql://user:pass@db-host:5432/dbname`
- **Preset metrics**: `exhaustive` is a good starting point for monitoring

Save the source.

## Step 4 — Watch metrics flow

It can take up to **2 minutes** for a new source to start producing data (the default refresh interval). After that, the dashboards in Grafana will populate with metrics from your database.

## Next steps

- For a more durable setup with named volumes, see [Tutorial: Installing using Docker → More future-proof setup](docker_installation.md#more-future-proof-setup).
- For production hardening, see [How-to: Harden a Docker deployment](../howto/harden_docker_deployment.md).
- For Kubernetes, see [How-to: Deploy to Kubernetes](../howto/deploy_to_kubernetes.md).
