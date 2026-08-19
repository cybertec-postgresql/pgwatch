---
title: Upgrading
---

This tutorial walks you through the upgrade paths for a pgwatch deployment. The pgwatch daemon itself rarely needs urgent upgrades, but the metrics, dashboards, and bundled components (PostgreSQL, Grafana) move quickly — staying current is worth doing for security alone.

If you launched pgwatch with Docker, follow [Step 1](#step-1-update-the-docker-image). If you installed pgwatch natively, skip ahead to [Step 2](#step-2-upgrade-a-native-installation).

## Step 1 — Update the Docker image

### With named volumes (recommended)

This is the easiest case — see [Tutorial: Installing using Docker — More future-proof setup](docker_installation.md#more-future-proof-setup) for how to set this up.

1. `docker compose pull` (or `docker pull cybertecpostgresql/pgwatch-demo:latest`).
2. `docker compose up -d` (or stop and re-run the existing `docker run` line).
3. Watch the logs for the upgrade banner. If you see `config database schema is outdated, please run migrations`, follow [Apply pgwatch schema migrations](#apply-pgwatch-schema-migrations) below.

### Without volumes

This path is only viable when you do not care about preserving historical metric data. If you do, set up volumes first or back up the bundled Postgres database before upgrading.

1. Stop the old container: `docker stop pgwatch-demo`.
2. Back up the bundled Postgres config DB to a host folder so you can re-import it:

    ```bash
    mkdir -p ~/pgwatch_backups
    docker run --rm --volumes-from pgwatch-demo \
        -v ~/pgwatch_backups:/backup busybox \
        cp -a /var/lib/postgresql /backup/
    ```

3. Pull the new image and start a fresh container with the same env / ports / volumes.
4. Re-import the config from the backup if needed.

## Step 2 — Upgrade a native installation

Native installations have looser coupling between components. The pgwatch daemon, the configuration store, the metrics sink, and Grafana can usually be upgraded independently.

1. Upgrade the gatherer binary: `apt upgrade pgwatch` (Debian/Ubuntu) or `dnf upgrade pgwatch` (RPM distros).
2. Restart the daemon:

    ```bash
    sudo systemctl restart pgwatch
    ```

3. Watch the logs for the upgrade banner. If you see `config database schema is outdated`, follow [Apply pgwatch schema migrations](#apply-pgwatch-schema-migrations) below.

4. Upgrade Grafana via your package manager. Grafana has a built-in schema migrator — updating the binaries and restarting is enough.

5. For PostgreSQL upgrades, follow the official [release-notes guidance](https://www.postgresql.org/docs/current/release.html). Major version upgrades need a planned maintenance window.

## Apply pgwatch schema migrations

Whenever the pgwatch binary version is newer than the schema in the Config DB, pgwatch refuses to start until migrations are applied. The fix is one command:

```bash
pgwatch \
  --sources=postgresql://pgwatch:secret@localhost:5432/pgwatch \
  --sink=postgresql://pgwatch:secret@localhost:5432/pgwatch_metrics \
  config upgrade
```

The command automatically detects which databases (sources, metrics, sinks) need migration and applies every pending migration in order. You only need to pass the connection strings you actually use.

## Update metric definitions

Built-in metric SQL ships with each release.

- **YAML mode** — refresh the metrics file the daemon was started with (`--metrics`); the new SQL comes in with the package upgrade.
- **Config-DB mode** — back up any custom metrics, then re-run `config init` against the metrics database. The new binary's built-in definitions overwrite `pgwatch.metric`.

    ```bash
    # 1. Back up custom metrics (YAML export of everything currently in the DB)
    pgwatch --metrics=postgresql://pgwatch:secret@localhost:5432/pgwatch metric list > my_metrics.yaml

    # 2. Re-initialise the metrics database with the built-in definitions
    pgwatch --metrics=postgresql://pgwatch:secret@localhost:5432/pgwatch config init
    ```

    !!! warning
        `config init --metrics=...` rewrites the `pgwatch.metric` table. **Save your custom metrics first** (step 1, or via the [REST API](../reference/rest.md)); otherwise they will be overwritten. Re-apply them after the init.

## Update Grafana dashboards

There is no automatic migration for the built-in dashboards — pgwatch leaves user-modified dashboards alone to avoid clobbering customisation. To pick up new panels:

1. Note any customisations you have made to the built-in dashboards.
2. Rename or delete the existing dashboards.
3. Import the latest JSON from [`grafana/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana) in the repository.
4. Re-apply your customisations.

For longer-term dashboard management strategy, see [Concept: Long-term installations → Dashboard maintenance](../concept/long_term_installations.md#dashboard-maintenance).
