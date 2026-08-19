---
title: Configure pgwatch with YAML files
---

This how-to shows how to run pgwatch with **YAML files** instead of the PostgreSQL configuration database. Use it when you prefer to keep configuration in version control, manage it with Ansible or another config-management tool, or simply avoid standing up an extra database.

For the PostgreSQL-based alternative, see [Tutorial: Custom installation — single happy path](../tutorial/custom_installation.md). For background on the two modes, see [Concept: Installation options](../concept/installation_options.md).

## Prerequisites

- A running PostgreSQL instance to host the metrics sink (any version 14+)
- An OS user that owns the YAML files and the systemd unit

## Steps

1. Create the configuration directory and write a `sources.yaml`:

    ```bash
    sudo mkdir -p /etc/pgwatch
    sudo chown pgwatch:pgwatch /etc/pgwatch
    sudo -u pgwatch tee /etc/pgwatch/sources.yaml > /dev/null <<'YAML'
    - name: my_database
      kind: postgres
      conn_str: postgresql://pgwatch:secret@db-host:5432/mydb
      preset_metrics: exhaustive
      is_enabled: true
      group: default

    # - name: the_second_monitored_database
    #   kind: postgres
    #   conn_str: postgresql://...
    #   ...
    YAML
    ```

2. Create the sink database:

    ```bash
    sudo -u postgres psql -c "create database pgwatch_metrics owner pgwatch"
    ```

    pgwatch creates the schema in this database automatically on first start. To do it explicitly:

    ```bash
    pgwatch --sink=postgresql://pgwatch:secret@db-host:5432/pgwatch_metrics config init
    ```

3. Start pgwatch, pointing it at the YAML file:

    ```bash
    pgwatch \
      --sources=/etc/pgwatch/sources.yaml \
      --sink=postgresql://pgwatch:secret@db-host:5432/pgwatch_metrics
    ```

4. Wait up to two minutes for the refresh loop to pick up the new sources (tune via `--refresh`).

## Managing many sources

A folder of YAML files works just as well as a single file. Drop one file per monitored cluster into `/etc/pgwatch/sources.d/` and pass the directory to `--sources`:

```bash
pgwatch --sources=/etc/pgwatch/sources.d/ --sink=...
```

Environment variables can be referenced from inside YAML values, which keeps secrets out of source control:

```yaml
- name: prod-app
  kind: postgres
  conn_str: $PROD_APP_CONN_STR
```

## Verifying the configuration

List the sources pgwatch has loaded:

```bash
curl -H "Token: $TOKEN" http://localhost:8080/source | jq '.[].Name'
```

You should see every entry from `sources.yaml` (or every YAML file in the sources directory).

## Caveats

- The Web UI runs in **read-only** mode for sources/metrics/presets when pgwatch is configured from YAML — see [Reference: REST API — note on YAML mode](../reference/rest.md#api-patterns).
- The metric definitions file is separate from the sources file. Pass it with `--metrics=/path/to/metrics.yaml` (or a directory). The defaults ship with the package.

## See also

- [Concept: Installation options](../concept/installation_options.md) — Config-DB vs YAML trade-offs
- [Reference: CLI & environment variables](../reference/cli_env.md) — every `--sources`, `--metrics`, and `--sink` flag
