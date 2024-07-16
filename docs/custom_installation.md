---
title: Custom installation
---

As described in the [Components](components.md) 
chapter, there a couple of ways how to set up up pgwatch3.
Two most common ways though are the central *Config DB* based "pull"
approach and the *YAML file* based "push" approach, plus Grafana to
visualize the gathered metrics.

## Config DB based setup

### Overview of installation steps

1.  Install Postgres or use any available existing instance - v9.4+
    required for the config DB and v11+ for the metrics DB.
1.  Bootstrap the Config DB.
1.  Bootstrap the metrics storage DB (PostgreSQL here).
1.  Install pgwatch3 - either from pre-built packages or by compiling
    the Go code.
1.  Prepare the "to-be-monitored" databases for monitoring by creating
    a dedicated login role name as a minimum.
1.  Optional step - install the administrative Web UI + Python & library
    dependencies.
1.  Add some databases to the monitoring configuration via the Web UI or
    directly in the Config DB.
1.  Start the pgwatch3 metrics collection agent and monitor the logs for
    any problems.
1.  Install and configure Grafana and import the pgwatch3 sample
    dashboards to start analyzing the metrics.
1. Make sure that there are auto-start SystemD services for all
    components in place and optionally set up also backups.

### Detailed steps for the Config DB based "pull" approach with Postgres metrics storage

Below are sample steps to do a custom install from scratch using
Postgres for the pgwatch3 configuration DB, metrics DB and Grafana
config DB.

All examples here assume Ubuntu as OS - but it's basically the same for
RedHat family of operations systems also, minus package installation
syntax differences.

1.  **Install Postgres**

    Follow the standard Postgres install procedure basically. Use the
    latest major version available, but minimally v11+ is recommended
    for the metrics DB due to recent partitioning speedup improvements
    and also older versions were missing some default JSONB casts so
    that a few built-in Grafana dashboards need adjusting otherwise.

    To get the latest Postgres versions, official Postgres PGDG repos
    are to be preferred over default disto repos. Follow the
    instructions from:

    -   <https://wiki.postgresql.org/wiki/Apt> - for Debian / Ubuntu
        based systems
    -   <https://www.postgresql.org/download/linux/redhat/> - for CentOS
        / RedHat based systems

1.  **Install pgwatch3** - either from pre-built packages or by
    compiling the Go code

    -   Using pre-built packages

        The pre-built DEB / RPM / Tar packages are available on the
        [Github
        releases](https://github.com/cybertec-postgresql/pgwatch3/releases)
        page.

            # find out the latest package link and replace below, using v1.8.0 here
            wget https://github.com/cybertec-postgresql/pgwatch3/releases/download/v1.8.0/pgwatch3_v1.8.0-SNAPSHOT-064fdaf_linux_64-bit.deb
            sudo dpkg -i pgwatch3_v1.8.0-SNAPSHOT-064fdaf_linux_64-bit.deb

    -   Compiling the Go code yourself

        This method of course is not needed unless dealing with maximum
        security environments or some slight code changes are required.

        1.  Install Go by following the [official
            instructions](https://golang.org/doc/install)

        2.  Get the pgwatch3 project's code and compile the gatherer
            daemon

                git clone https://github.com/cybertec-postgresql/pgwatch3.git
                cd pgwatch3/src/webui
                yarn install --network-timeout 100000 && yarn build
                cd ..
                go build

            After fetching all the Go library dependencies (can take minutes)
            an executable named "pgwatch3" should be generated. Additionally it's a good idea
            to copy it to `/usr/bin/pgwatch3`.

    -   Configure a SystemD auto-start service (optional)

        Sample startup scripts can be found at
        */etc/pgwatch3/startup-scripts/pgwatch3.service* or online
        [here](https://github.com/cybertec-postgresql/pgwatch3/blob/master/pgwatch3/startup-scripts/pgwatch3.service).
        Note that they are OS agnostic and might need some light
        adjustment of paths, etc - so always test them out.

1.  **Boostrap the config DB**

    1.  Create a user to "own" the `pgwatch3` schema

        Typically called `pgwatch3` but can be anything really, if the
        schema creation file is adjusted accordingly.

            psql -c "create user pgwatch3 password 'xyz'"
            psql -c "create database pgwatch3 owner pgwatch3"

    2.  Roll out the pgwatch3 config schema

        The schema will most importantly hold connection strings of DB-s
        to be monitored and the metric definitions.

            # FYI - one could get the below schema files also directly from Github
            # if re-using some existing remote Postgres instance where pgwatch3 was not installed
            psql -f /etc/pgwatch3/sql/config_store/config_store.sql pgwatch3
            psql -f /etc/pgwatch3/sql/config_store/metric_definitions.sql pgwatch3

1.  **Bootstrap the measurements storage DB**

    1.  Create a dedicated database for storing metrics and a user to
        "own" the metrics schema

        Here again default scripts expect a role named `pgwatch3` but
        can be anything if to adjust the scripts.

            psql -c "create database pgwatch3_metrics owner pgwatch3"

    2.  Roll out the pgwatch3 metrics storage schema

        This is a place to pause and first think how many databases will
        be monitored, i.e. how much data generated, and based on that
        one should choose an according metrics storage schema. There are
        a couple of different options available that are described
        [here](https://github.com/cybertec-postgresql/pgwatch3/tree/master/pgwatch3/sql/metric_store)
        in detail, but the gist of it is that you don't want too
        complex partitioning schemes if you don't have zounds of data
        and don't need the fastest queries. For a smaller amount of
        monitored DBs (a couple dozen to a hundred) the default
        "metric-time" is a good choice. For hundreds of databases,
        aggressive intervals, or long term storage usage of the
        TimescaleDB extension is recommended.

            cd /etc/pgwatch3/sql/metric_store
            psql -f roll_out_metric_time.psql pgwatch3_metrics

        !!! Note 
            Default retention for Postgres storage is 2 weeks!
            To change, use the `--pg-retention-days / PW3_PG_RETENTION_DAYS`
            gatherer parameter.

1.  **Prepare the "to-be-monitored" databases for metrics collection**

    As a minimum we need a plain unprivileged login user. Better though
    is to grant the user also the `pg_monitor` system role, available on
    v10+. Superuser privileges should be normally avoided for obvious
    reasons of course, but for initial testing in safe environments it
    can make the initial preparation (automatic *helper* rollouts) a bit
    easier still, given superuser privileges are later stripped.

    To get most out of your metrics some `SECURITY DEFINER` wrappers
    functions called "helpers" are recommended on the DB-s under
    monitoring. See the detailed chapter on the "preparation" topic
    [here](preparing_databases.md) for more
    details.

1.  **Configure DB-s and metrics / intervals to be monitored**

    -   From the Web UI "/dbs" page
    -   Via direct inserts into the Config DB `pgwatch3.monitored_db` table

1.  **Start the pgwatch3 metrics collection agent**

    1.  The gatherer has quite some parameters (use the `--help` flag
        to show them all), but simplest form would be:

            pgwatch3-daemon \
              --host=localhost --user=pgwatch3 --dbname=pgwatch3 \
              --datastore=postgres --pg-metric-store-conn-str=postgresql://pgwatch3@localhost:5432/pgwatch3_metrics \
              --verbose=info
        
        Default connections params expect a trusted localhost Config DB setup
        so mostly the 2nd line is not needed actually.

        Or via SystemD if set up in previous steps

            useradd -m -s /bin/bash pgwatch3 # default SystemD templates run under the pgwatch3 user
            sudo systemctl start pgwatch3
            sudo systemctl status pgwatch3

        After initial verification that all works it's usually good
        idea to set verbosity back to default by removing the *verbose*
        flag.

        Another tip to configure connection strings inside SystemD
        service files is to use the "systemd-escape" utility to escape
        special characters like spaces etc if using the LibPQ connect
        string syntax rather than JDBC syntax.

    2.  Monitor the console or log output for any problems

        If you see metrics trickling into the "pgwatch3_metrics"
        database (metric names are mapped to table names and tables are
        auto-created), then congratulations - the deployment is working!
        When using some more aggressive *preset metrics config* then
        there are usually still some errors though, due to the fact that
        some more extensions or privileges are missing on the monitored
        database side. See the according chapter
        [here](preparing_databases.md).

    !!! Info
        When you're compiling your own gatherer then the executable file
        will be named just `pgwatch3` instead of `pgwatch3-daemon` to avoid
        mixups.

1.  **Install Grafana**
    1.  Create a Postgres database to hold Grafana internal config, like
        dashboards etc

        Theoretically it's not absolutely required to use Postgres for
        storing Grafana internal settings / dashboards, but doing so has
        2 advantages - you can easily roll out all pgwatch3 built-in
        dashboards and one can also do remote backups of the Grafana
        configuration easily.

            psql -c "create user pgwatch3_grafana password 'xyz'"
            psql -c "create database pgwatch3_grafana owner pgwatch3_grafana"

    2.  Follow the instructions from
        <https://grafana.com/docs/grafana/latest/installation/debian/>,
        basically something like:

            wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
            echo "deb https://packages.grafana.com/oss/deb stable main" | sudo tee -a /etc/apt/sources.list.d/grafana.list
            sudo apt-get update && sudo apt-get install grafana

            # review / change config settings and security, etc
            sudo vi /etc/grafana/grafana.ini

            # start and enable auto-start on boot
            sudo systemctl daemon-reload
            sudo systemctl start grafana-server
            sudo systemctl status grafana-server

        Default Grafana port: 3000

    3.  Configure Grafana config to use our `pgwatch3_grafana` DB

        Place something like below in the `[database]` section of
        `/etc/grafana/grafana.ini`

            [database]
            type = postgres
            host = my-postgres-db:5432
            name = pgwatch3_grafana
            user = pgwatch3_grafana
            password = xyz

        Taking a look at `[server], [security]` and `[auth*]`
        sections is also recommended.

    4.  Set up the `pgwatch3` metrics database as the default datasource

        We need to tell Grafana where our metrics data is located. Add a
        datasource via the Grafana UI (Admin -\> Data sources) or adjust
        and execute the "pgwatch3/bootstrap/grafana_datasource.sql"
        script on the `pgwatch3_grafana` DB.

    5.  Add pgwatch3 predefined dashboards to Grafana

        This could be done by importing the pgwatch3 dashboard
        definition JSON-s manually, one by one, from the "grafana"
        folder ("Import Dashboard" from the Grafana top menu) or via
        as small helper script located at
        */etc/pgwatch3/grafana-dashboards/import_all.sh*. The script
        needs some adjustment for metrics storage type, connect data and
        file paths.

    6.  Optionally install also Grafana plugins

        Currently one pre-configured dashboard (Biggest relations
        treemap) use an extra plugin - if planning to that dash, then
        run the following:

            grafana-cli plugins install savantly-heatmap-panel

    7.  Start discovering the preset dashbaords

        If the previous step of launching pgwatch3 daemon succeeded and
        it was more than some minutes ago, one should already see some
        graphs on dashboards like "DB overview" or "DB overview
        Unprivileged / Developer mode" for example.


## YAML based setup

From v1.4 one can also deploy the pgwatch3 gatherer daemons more easily
in a de-centralized way, by specifying monitoring configuration via YAML
files. In that case there is no need for a central Postgres "config
DB".

**YAML installation steps**

1.  Install pgwatch3 - either from pre-built packages or by compiling
    the Go code.
2.  Specify hosts you want to monitor and with which metrics /
    aggressivness in a YAML file or files, following the example config
    located at */etc/pgwatch3/config/instances.yaml* or online
    [here](https://github.com/cybertec-postgresql/pgwatch3/blob/master/pgwatch3/config/instances.yaml).
    Note that you can also use env. variables inside the YAML templates!
3.  Bootstrap the metrics storage DB (not needed it using Prometheus
    mode).
4.  Prepare the "to-be-monitored" databases for monitoring by creating
    a dedicated login role name as a
    [minimum](preparing_databases.md).
5.  Run the pgatch2 gatherer specifying the YAML config file (or
    folder), and also the folder where metric definitions are located.
    Default location: */etc/pgwatch3/metrics*.
6.  Install and configure Grafana and import the pgwatch3 sample
    dashboards to start analyzing the metrics. See above for
    instructions.
7.  Make sure that there are auto-start SystemD services for all
    components in place and optionally set up also backups.

Relevant gatherer parameters / env. vars: `--config / PW3_CONFIG` and
`--metrics-folder / PW3_METRICS_FOLDER`.

For details on individual steps like installing pgwatch3 see the above
paragraph.
