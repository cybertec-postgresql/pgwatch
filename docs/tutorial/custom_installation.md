---
title: Custom installation
---

This tutorial walks you through installing pgwatch manually, one component at a time, on a Linux host. It uses the **PostgreSQL configuration database** as the source of truth for monitored sources and metric definitions, and stores metric measurements in a second PostgreSQL database. This is the recommended path for production deployments.

If you prefer to drive pgwatch with YAML files instead, see [How-to: Configure pgwatch with YAML files](../howto/yaml_configuration.md). For a one-step containerised alternative, see [Tutorial: Installing using Docker](docker_installation.md).

## Overview

pgwatch has four components:

1. **Metrics collector** — the `pgwatch` daemon, written in Go
2. **Configuration store** — a PostgreSQL database holding sources, metrics, and presets
3. **Metrics storage (sink)** — a PostgreSQL database that holds historical metric measurements
4. **Visualisation** — Grafana with the pgwatch dashboards

For background, see [Concept: Components](../concept/components.md).

## Requirements

- PostgreSQL 14 or newer (latest major recommended)
- Grafana 10 or newer (for visualisation)
- A user account on every database you want to monitor

## Step 1 — Install the pgwatch binary

On Debian/Ubuntu:

```bash
sudo apt update && sudo apt install pgwatch
```

On RPM-based distros, install the latest `.rpm` from the [GitHub releases page](https://github.com/cybertec-postgresql/pgwatch/releases).

To build from source instead, see the project's `README.md`.

## Step 2 — Create the configuration database

```bash
sudo -u postgres psql -c "create user pgwatch password 'your_password'"
sudo -u postgres psql -c "create database pgwatch owner pgwatch"
```

pgwatch creates the schema in this database on first start. To do it explicitly:

```bash
pgwatch --sources=postgresql://pgwatch:your_password@localhost:5432/pgwatch config init
```

## Step 3 — Create the metrics measurements database

```bash
sudo -u postgres psql -c "create database pgwatch_metrics owner pgwatch"
```

pgwatch creates the metrics schema here automatically as soon as it runs.

## Step 4 — Prepare each database you want to monitor

For every database you want pgwatch to watch, create a dedicated role with the `pg_monitor` privilege:

```sql
CREATE USER pgwatch WITH PASSWORD 'your_password';
GRANT pg_monitor TO pgwatch;
```

For the full set of preparation steps (extensions, helper functions, etc.), see [Tutorial: Preparing databases for monitoring](preparing_databases.md).

## Step 5 — Start the gatherer

```bash
pgwatch \
  --sources=postgresql://pgwatch:your_password@localhost:5432/pgwatch \
  --sink=postgresql://pgwatch:your_password@localhost:5432/pgwatch_metrics
```

Wait a few seconds — you should see `sources and metrics refreshed` on stdout.

### Running as a systemd service

Create `/etc/systemd/system/pgwatch.service`:

```ini
[Unit]
Description=pgwatch
After=network-online.target

[Service]
Type=exec
User=pgwatch
ExecStart=/usr/bin/pgwatch --sources=postgresql://pgwatch:your_password@localhost:5432/pgwatch --sink=postgresql://pgwatch:your_password@localhost:5432/pgwatch_metrics
Restart=on-failure
TimeoutStartSec=0
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl start pgwatch
sudo systemctl enable pgwatch
```

## Step 6 — Add a source to monitor

Open the admin Web UI at `http://localhost:8080` and go to **SOURCES**. Click **+ NEW**, fill in the connection details of the database you want to monitor, and pick a preset (`minimal`, `basic`, or `exhaustive`). Save the source.

Or use the [REST API](../reference/rest.md), or insert directly into the `pgwatch.source` table.

> It can take up to 2 minutes for a newly added source to start producing metrics. Tune this via `--refresh`.

## Step 7 — Install Grafana and import dashboards

Follow the [official Grafana installation guide](https://grafana.com/docs/grafana/latest/setup-grafana/installation/) for your OS.

Then add the `pgwatch-metrics` (Postgres) or `pgwatch-prometheus` (Prometheus) data source — these UIDs are what the built-in dashboards expect — and import the dashboards from the [`grafana/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana) folder of the pgwatch repository.

!!! note
    Starting from Grafana 12.4, set `newPanelPadding = false` under `[feature_toggles]` in `grafana.ini` to keep dashboard font sizes sensible.

## Next steps

- [Tutorial: Preparing databases for monitoring](preparing_databases.md) — install helper functions for OS-level metrics
- [Tutorial: Upgrading](upgrading.md) — keep pgwatch up to date
- [Concept: Long-term installations](../concept/long_term_installations.md) — operating pgwatch in production over months and years
