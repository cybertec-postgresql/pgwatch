---
title: Custom installation
---

As described in the [Components](../concept/components.md) chapter,
there is a couple of ways to set up pgwatch.
The two most common ones are using a central **configuration database**
or using a **YAML file**, plus Grafana to visualize the gathered metrics.

### Overview of installation steps

1. Install Postgres or use an existing instance - v11+ is required but the latest major version is recommended.
2. Bootstrap the configuration database or Edit the YAML file[s].
3. Bootstrap the metric [measurements storage](https://pgwat.ch/devel/concept/components.html#measurements-storage) aka sink (PostgreSQL here).
4. Install pgwatch
5. [Prepare](preparing_databases.md) the "to-be-monitored" databases for monitoring by creating
    a dedicated login role name as a minimum.
6. Add some databases to the monitoring configuration via the Web UI, REST API or
    directly in the configuration database.
7. Start the pgwatch metrics collection agent and monitor the logs for
    any problems.
8. Install and configure Grafana and import the pgwatch sample
    dashboards to start analyzing the metrics.
9. Make sure that there are auto-start services for all
    components in place and optionally set up also backups.

### Detailed installation steps

Below are the sample steps for a custom installation from scratch using Postgres
for the pgwatch configuration database, measurements database, and Grafana configuration database.

All examples here assume Ubuntu as OS, but it's basically
the same for RedHat family of operating systems also,
minus package installation syntax differences.

#### 1. **Install Postgres**

Follow the standard Postgres install procedure. Use the
latest major version available, but minimally v11+ is required.

To get the latest Postgres versions, official Postgres PGDG repos
are preferred over default distro repos. Follow the
instructions from:

- <https://www.postgresql.org/download/linux/debian/> - for Debian / Ubuntu
    based systems
- <https://www.postgresql.org/download/linux/redhat/> - for CentOS
    / RedHat based systems
- <https://www.postgresql.org/download/windows/> - for Windows

#### 2. **Install pgwatch**

- From the official [PostgreSQL Apt Repository](https://wiki.postgresql.org/wiki/Apt#Quickstart):

    ```bash
    sudo apt update && sudo apt install pgwatch
    ```

- Using pre-built packages from the [GitHub releases](https://github.com/cybertec-postgresql/pgwatch/releases) page:

    ```bash
    # find out the latest package link and replace below, using v4.0 here
    wget https://github.com/cybertec-postgresql/pgwatch/releases/download/v4.0.0/pgwatch_Linux_x86_64.deb
    sudo dpkg -i pgwatch_Linux_x86_64.deb
    ```

- Compiling the Go code yourself

    This method of course is not needed unless dealing with maximum
    security environments or some slight code changes are required.

    1. Install Go by following the [official instructions](https://golang.org/doc/install)

    2. Get the pgwatch project's code and compile the gatherer daemon

        ```bash
        # Clone the Repo
        git clone https://github.com/cybertec-postgresql/pgwatch.git

        # Build the webui
        cd pgwatch/internal/webui
        yarn install --network-timeout 100000 && yarn build

        # Install protoc and generate the protobuf files
        sudo apt update && sudo apt install -y protobuf-compiler protoc-gen-go protoc-gen-go-grpc
        go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
        cd ../../ && protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/pb/pgwatch.proto

        # Compile pgwatch
        go build ./cmd/pgwatch/
        ```

        After fetching all the Go library dependencies (can take minutes)
        an executable named "pgwatch" should be generated. Additionally,
        it's a good idea to copy it to `/usr/bin/pgwatch`.

- Configure a SystemD auto-start service (optional). Here is the sample:

    ```ini
    [Unit]
    Description=pgwatch
    After=network-online.target

    # After=<postgresql@17-main.service>

    [Service]
    User=pgwatch
    Type=exec
    ExecStart=/usr/bin/pgwatch --sources=postgresql://pgwatch:xyz@localhost:5432/pgwatch --sink=postgresql://pgwatch:xyz@localhost:5432/pgwatch_metrics
    Restart=on-failure
    TimeoutStartSec=0
    RestartSec=5s
    TimeoutStopSec=60s

    [Install]
    WantedBy=multi-user.target
    ```

#### 3. **Bootstrap the configuration database or Edit the YAML file[s]**

##### a. Using a configuration database

!!! note
    The detailed steps are described in the
    [Bootstrapping the Configuration Database](../howto/config_db_bootstrap.md) chapter

- Create a user to "own" the `pgwatch` database

    Typically called `pgwatch` but can be anything really, if the
    schema creation file is adjusted accordingly.

    ```terminal
    psql -c "create user pgwatch password 'xyz'"
    psql -c "create database pgwatch owner pgwatch"
    ```

- Roll out the pgwatch config schema (optional)

    pgwatch will automatically create the necessary tables and indexes in the database when it starts. But in case
    you want to create the schema as a separate step, you can use the `config init` command-line command:

    ```terminal
    pgwatch --sources=postgresql://pgwatch:xyz@localhost/pgwatch config init
    ```

##### b. Using a YAML Configuration file

The content of the file is an array of sources definitions, like this:

```yaml
- name: test1       # An arbitrary unique name for the monitored source
kind: postgres    # One of the:
                    # - postgres
                    # - postgres-continuous-discovery
                    # - pgbouncer
                    # - pgpool
                    # - patroni
                    # - patroni-continuous-discovery
                    # - patroni-namespace-discover
                    # Defaults to postgres if not specified
conn_str: postgresql://pgwatch:xyz@somehost/mydb
preset_metrics: exhaustive # from list of presets defined in "metrics.yaml" or in the config DB
custom_metrics:    # map of metrics and intervals, if both preset_metrics and custom_metrics are specified, custom wins
        backends: 300
        archiver: 120
preset_metrics_standby: # optional preset configuration for standby state, same as preset_metrics
custom_metrics_standby: # optional custom metrics for standby state, same as custom_metrics
include_pattern: # regex to filter databases to actually monitor for the "continuous" modes
exclude_pattern:
is_enabled: true
group: default # just for logical grouping of DB hosts or for "sharding", i.e. splitting the workload between many gatherer daemons
custom_tags:      # option to add arbitrary tags for every stored data row,
        aws_instance_id: i-0af01c0123456789a       # for example to fetch data from some other source onto a same Grafana graph
...
```

#### 4. **Bootstrap the measurements storage database (sink)**

!!! note
    The detailed steps are described in the
    [Bootstrapping the Metrics Measurements Database (Sink)](../howto/metrics_db_bootstrap.md) chapter

Create a dedicated database for storing metrics and a user to
"own" the measurements schema. Here again default scripts expect a
role named `pgwatch` but can be anything if to adjust the scripts:

```terminal
psql -c "create database pgwatch_metrics owner pgwatch"
```

#### 5. **Prepare the "to-be-monitored" databases for metrics collection**

As a minimum we need a plain unprivileged login user. Better though
is to grant the user also the `pg_monitor` system role, available on
v10+. Superuser privileges **should be normally avoided** for obvious
reasons of course, but for initial testing in safe environments it
can make the initial preparation (automatic *helper* rollouts) a bit
easier still, given superuser privileges are later stripped.

To get most out of your metrics some `SECURITY DEFINER` wrappers
functions called "helpers" are recommended on the DB-s under
monitoring. See the detailed chapter on the ["preparation" topic](preparing_databases.md)
for more details.

#### 6. **Start the pgwatch metrics collection agent**

The gatherer has quite some parameters (use the `--help` flag
to show them all), but simplest form would be:

```terminal
pgwatch \
    --sources=postgresql://pgwatch:xyz@localhost:5432/pgwatch \
    --sink=postgresql://pgwatch:xyz@localhost:5432/pgwatch_metrics \
    --log-level=debug
```

Or via SystemD if set up in previous steps

```terminal
useradd -m -s /bin/bash pgwatch # default SystemD templates run under the pgwatch user
sudo systemctl start pgwatch
sudo systemctl status pgwatch
```

After initial verification that all works, it's usually good
idea to set verbosity back to default by removing the *--log-level=debug*
flag.

#### 7. **Configure sources and metrics with intervals to be monitored**

- from the Web UI "Sources" page
- via direct inserts into the Config DB `pgwatch.source` table

#### 8. **Monitor the console or log output for any problems**

Wait for a few minutes or restart the gatherer daemon to reread
the monitored sources and metrics configuration. You can control
the refresh timeout via the `--refresh` parameter,
default is 120 seconds.

If you see metrics trickling into the "pgwatch_metrics"
database (metric names are mapped to table names and tables are
auto-created), then congratulations - the deployment is working!
When using some more aggressive *preset metrics config* then
there are usually still some errors though, due to the fact that
some more extensions or privileges are missing on the monitored
database side. See the [according chapter](preparing_databases.md).

#### 9. **Install Grafana**

1. Create a Postgres database to hold Grafana internal config, like
    dashboards etc.

    Theoretically it's not absolutely required to use Postgres for
    storing Grafana internal settings, but doing so has
    2 advantages - you can easily roll out all pgwatch built-in
    dashboards and one can also do remote backups of the Grafana
    configuration easily.

    ```terminal
    psql -c "create user pgwatch_grafana password 'xyz'"
    psql -c "create database pgwatch_grafana owner pgwatch_grafana"
    ```

2. Follow the instructions from [Grafana documentation](https://grafana.com/docs/grafana/latest/installation/),
basically something like:

    ```terminal
    wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
    echo "deb https://packages.grafana.com/oss/deb stable main" | sudo tee -a /etc/apt/sources.list.d/grafana.list
    sudo apt-get update && sudo apt-get install grafana

    # review / change config settings and security, etc
    sudo vi /etc/grafana/grafana.ini

    # start and enable auto-start on boot
    sudo systemctl daemon-reload
    sudo systemctl start grafana-server
    sudo systemctl status grafana-server
    ```

    Default Grafana port: 3000

3. Configure Grafana config to use our `pgwatch_grafana` DB

    Place something like this below in the `[database]` section of
    `/etc/grafana/grafana.ini`

    ```ini
    [database]
    type = postgres
    host = my-postgres-db:5432
    name = pgwatch_grafana
    user = pgwatch_grafana
    password = xyz
    ```

    Taking a look at `[server], [security]` and `[auth*]`
    sections is also recommended.

4. Set up the `pgwatch` metrics database as the default datasource

    We need to tell Grafana where our metrics data is located. Add a
    datasource via the Grafana UI (Admin -\> Data sources) or use
    `pgwatch/grafana/postgres_datasource.yml` for
    [provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/).

5. Add pgwatch predefined dashboards to Grafana

    This could be done by importing the pgwatch dashboard
    definition JSONs manually, one by one, from the `pgwatch/grafana`
    folder ("Import Dashboard" from the Grafana top menu) or by
    [dashboard provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/).

6. Optionally install also Grafana plugins

    Currently, one pre-configured dashboard (Biggest relations
    treemap) use an extra plugin - if planning to use that dash, then
    run the following:

    ```terminal
    grafana-cli plugins install savantly-heatmap-panel
    ```

7. Start discovering the preset dashbaords

    If the previous step of launching pgwatch succeeded, and
    it was more than some minutes ago, one should already see some
    graphs on dashboards like "DB overview" or "DB overview
    Unprivileged / Developer mode" for example.