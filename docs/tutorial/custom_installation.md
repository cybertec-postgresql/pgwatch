---
title: Custom installation
---

This chapter describes how to set up pgwatch manually, giving you full control over each component. For a simpler setup, see the [Docker installation](docker_installation.md) guide.

## Overview

pgwatch consists of four main components:

1. **Metrics collector** - The pgwatch daemon that gathers metrics from your databases
2. **Configuration store** - Where you define which databases to monitor and their settings  
    - PostgreSQL database or YAML file
3. **Metrics storage** - Where collected metrics are stored  
    - PostgreSQL, Prometheus, custom gRPC server, or JSON
4. **Visualization** - Grafana dashboards for analyzing metrics

## Requirements

- PostgreSQL v14+ (latest major version recommended)
- Grafana (for visualization)
- A user account on each database you want to monitor

## Installation Methods

Choose how you want to manage your monitoring configurations:

1. **PostgreSQL Database**  
    - Store monitored databases and metrics configs in a PostgreSQL database.  

2. **YAML File**
    - Store monitored databases and metrics configs in a YAML file.
  
!!! info
    You can use pgwatch's built-in web UI or REST API to manage both configuration stores.

## Installation Steps

### 1. Install pgwatch

- **Using apt (Debian/Ubuntu)**
    ```bash
    # Follow instructions from: https://wiki.postgresql.org/wiki/Apt#Quickstart to add the official PostgreSQL apt repository 
    sudo apt update && sudo apt install pgwatch
    ```

- **From GitHub releases**
    ```bash
    # Find the latest release at https://github.com/cybertec-postgresql/pgwatch/releases
    wget https://github.com/cybertec-postgresql/pgwatch/releases/download/v5.1.0/pgwatch_Linux_x86_64.deb
    sudo dpkg -i pgwatch_Linux_x86_64.deb
    ```

- **Build from source**
    ```bash
    # Install Go - https://golang.org/doc/install
    # Install Protoc - https://grpc.io/docs/languages/go/quickstart/

    git clone https://github.com/cybertec-postgresql/pgwatch.git
    cd pgwatch/internal/webui
    yarn install --network-timeout 100000 && yarn build
    cd ../../
    go generate ./api/pb
    go build ./cmd/pgwatch/
    ```

    The executable will be created in the current directory. Copy it to `/usr/bin/pgwatch` for system-wide access.

### 2. Bootstrap the configuration store

#### Using a PostgreSQL database

Create a database to store pgwatch configurations:

```bash
psql -c "create user pgwatch password 'your_password'"
psql -c "create database pgwatch owner pgwatch"
```

pgwatch will automatically create the required config tables on first run. To create them manually:

```bash
pgwatch --sources=postgresql://pgwatch:your_password@localhost:5432/pgwatch config init
```

!!! note
    See [Bootstrapping the Configuration Database](../howto/config_db_bootstrap.md) for detailed instructions.


#### Using a YAML file

Create `/etc/pgwatch/sources.yaml`:

```yaml
- name: my_database
  conn_str: postgresql://pgwatch:your_password@localhost:5432/mydb
  preset_metrics: exhaustive
  is_enabled: true
  group: default

# - name: the_second_monitored_database
# ...
```

**Sources configuration options**:

| Option | Description | Example |
|--------|-------------|---------|
| `name` | Unique name for this source | `mydb` |
| `kind` | Source type: `postgres`, `postgres-continuous-discovery`, `pgbouncer`, `pgpool`, `patroni` | `postgres` |
| `conn_str` | PostgreSQL or etcd connection string | `postgresql://user:pass@host/db` or `etcd://host1:1234,host2:1344/scope/member` |
| `preset_metrics` | Preset to use: `minimal`, `basic`, `exhaustive`, `unprivileged`, etc. | `exhaustive` |
| `custom_metrics` | Custom metrics with intervals (seconds) | `{ backends: 300 }` |
| `include_pattern` | Regex to filter databases (for continuous discovery) | `^mydb_` |
| `exclude_pattern` | Regex to exclude databases (for continuous discovery) | `^test_` |
| `is_enabled` | Enable/disable monitoring | `true` |
| `group` | For distributed pgwatch setups with centralized configs | `default` |
| `custom_tags` | Custom tags added to all metrics | `{ env: production }` |


!!! note
    Allow up to 2 minutes - can be adjusted via `--refresh` - for newly added sources to be picked up by a running pgwatch daemon.

### 3. Bootstrap the metrics storage database

Create a database to store collected metrics:

```bash
psql -c "create database pgwatch_metrics owner pgwatch"
```

!!! note
    See [Bootstrapping the Metrics Measurements Database (Sink)](../howto/metrics_db_bootstrap.md) for detailed instructions.

### 4. Prepare databases for monitoring

On each database you want to monitor, create a dedicated user:

```sql
CREATE USER pgwatch WITH PASSWORD 'your_password';
GRANT pg_monitor TO pgwatch;
```

!!! note
    For additional details, see [Preparing databases for monitoring](preparing_databases.md).

### 5. Start pgwatch

```bash
pgwatch \
    --sources=postgresql://pgwatch:your_password@localhost:5432/pgwatch \
    --sink=postgresql://pgwatch:your_password@localhost:5432/pgwatch_metrics
    # or use --sources=/etc/pgwatch/sources.yaml
```

Wait a few seconds to see the success of initial metrics fetches.

#### Running as a systemd service

Create `/etc/systemd/system/pgwatch.service`:

```ini
[Unit]
Description=pgwatch
After=network-online.target

[Service]
Type=exec
User=pgwatch
ExecStart=/usr/bin/pgwatch --sources=postgresql://pgwatch:your_password@localhost:5432/pgwatch --sink=postgresql://pgwatch:your_password@localhost:5432/pgwatch_metrics
# or ExecStart=/usr/bin/pgwatch --sources=/etc/pgwatch/sources.yaml --sink=postgresql://pgwatch:your_password@localhost:5432/pgwatch_metrics
Restart=on-failure
TimeoutStartSec=0
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl start pgwatch
sudo systemctl enable pgwatch
```

### 6. Configure monitored sources

Add databases to monitor via the pgwatch web UI (port 8080 by default) or direct SQL:

```sql
INSERT INTO pgwatch.source (name, conn_str, preset_metrics, is_enabled)
VALUES ('mydb', 'postgresql://pgwatch:your_password@localhost:5432/mydb', 'exhaustive', true);
```

!!! note
    Allow up to 2 minutes - can be adjusted via `--refresh` - for new sources to appear in metrics collection.

### 7. Install and configure Grafana

1. Refer to the official Grafana documentation for the [installation](https://grafana.com/docs/grafana/latest/setup-grafana/installation/), [configuration](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/), and [data sources](https://grafana.com/docs/grafana/latest/datasources/) setup steps.
2. Import the default postgres and/or prometheus dashboards from the [`grafana/`](https://github.com/cybertec-postgresql/pgwatch/tree/master/grafana) folder into your Grafana instance.

!!! note
    The default built-in dashboards expect `postgres/prometheus` data sources with uids `pgwatch-metrics/pgwatch-prometheus` by default.

## Next Steps

- Explore the [Components](../concept/components.md) chapter to better understand the pgwatch architecture.
