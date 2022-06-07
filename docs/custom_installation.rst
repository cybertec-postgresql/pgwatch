.. _custom_installation:

Custom installation
===================

As described in the :ref:`Components <components>` chapter, there a couple of ways how to set up up pgwatch3. Two most
common ways though are the central *Config DB* based "pull" approach and the *YAML file* based "push" approach, plus
Grafana to visualize the gathered metrics.

Config DB based setup
---------------------

**Overview of installation steps**

#. Install Postgres or use any available existing instance - v9.4+ required for the config DB and v11+ for the metrics DB.
#. Bootstrap the Config DB.
#. Bootstrap the metrics storage DB (PostgreSQL here).
#. Install pgwatch3 - either from pre-built packages or by compiling the Go code.
#. Prepare the "to-be-monitored" databases for monitoring by creating a dedicated login role name as a minimum.
#. Optional step - install the administrative Web UI + Python & library dependencies.
#. Add some databases to the monitoring configuration via the Web UI or directly in the Config DB.
#. Start the pgwatch3 metrics collection agent and monitor the logs for any problems.
#. Install and configure Grafana and import the pgwatch3 sample dashboards to start analyzing the metrics.
#. Make sure that there are auto-start SystemD services for all components in place and optionally set up also backups.

**Detailed steps for the Config DB based "pull" approach with Postgres metrics storage**

Below are sample steps to do a custom install from scratch using Postgres for the pgwatch3 configuration DB, metrics DB and
Grafana config DB.

All examples here assume Ubuntu as OS - but it's basically the same for RedHat family of operations systems also, minus package installation syntax differences.

#. **Install Postgres**

   Follow the standard Postgres install procedure basically. Use the latest major version available, but minimally
   v11+ is recommended for the metrics DB due to recent partitioning speedup improvements and also older versions were missing some
   default JSONB casts so that a few built-in Grafana dashboards need adjusting otherwise.

   To get the latest Postgres versions, official Postgres PGDG repos are to be preferred over default disto repos. Follow
   the instructions from:

   * https://wiki.postgresql.org/wiki/Apt - for Debian / Ubuntu based systems

   * https://www.postgresql.org/download/linux/redhat/ - for CentOS / RedHat based systems

#. **Install pgwatch3** - either from pre-built packages or by compiling the Go code

   * Using pre-built packages

     The pre-built DEB / RPM / Tar packages are available on the `Github releases <https://github.com/cybertec-postgresql/pgwatch3/releases>`_ page.

     ::

       # find out the latest package link and replace below, using v1.8.0 here
       wget https://github.com/cybertec-postgresql/pgwatch3/releases/download/v1.8.0/pgwatch3_v1.8.0-SNAPSHOT-064fdaf_linux_64-bit.deb
       sudo dpkg -i pgwatch3_v1.8.0-SNAPSHOT-064fdaf_linux_64-bit.deb

   * Compiling the Go code yourself

     This method of course is not needed unless dealing with maximum security environments or some slight code changes are required.

     #. Install Go by following the `official instructions <https://golang.org/doc/install>`_

     #. Get the pgwatch3 project's code and compile the gatherer daemon

        ::

          git clone https://github.com/cybertec-postgresql/pgwatch3.git
          cd pgwatch3/pgwatch3
          ./build_gatherer.sh
          # after fetching all the Go library dependencies (can take minutes)
          # an executable named "pgwatch3" should be generated. Additionally it's a good idea
          # to copy it to /usr/bin/pgwatch3-daemon as that's what the default SystemD service expects.

    * Configure a SystemD auto-start service (optional)

      Sample startup scripts can be found at */etc/pgwatch3/startup-scripts/pgwatch3.service* or online
      `here <https://github.com/cybertec-postgresql/pgwatch3/blob/master/pgwatch3/startup-scripts/pgwatch3.service>`__.
      Note that they are OS agnostic and might need some light adjustment of paths, etc - so always test them out.

#. **Boostrap the config DB**

   #. Create a user to "own" the pgwatch3 schema

      Typically called *pgwatch3* but can be anything really, if the schema creation file is adjusted accordingly.

      ::

        psql -c "create user pgwatch3 password 'xyz'"
        psql -c "create database pgwatch3 owner pgwatch3"

   #. Roll out the pgwatch3 config schema

      The schema will most importantly hold connection strings of DB-s to be monitored and the metric definitions.

      ::

        # FYI - one could get the below schema files also directly from Github
        # if re-using some existing remote Postgres instance where pgwatch3 was not installed
        psql -f /etc/pgwatch3/sql/config_store/config_store.sql pgwatch3
        psql -f /etc/pgwatch3/sql/config_store/metric_definitions.sql pgwatch3

.. _metrics_db_bootstrap:

#. **Bootstrap the metrics storage DB**

   #. Create a dedicated database for storing metrics and a user to "own" the metrics schema

      Here again default scripts expect a role named "pgwatch3" but can be anything if to adjust the scripts.

      ::

        psql -c "create database pgwatch3_metrics owner pgwatch3"

   #. Roll out the pgwatch3 metrics storage schema

      This is a place to pause and first think how many databases will be monitored, i.e. how much data generated, and based
      on that one should choose an according metrics storage schema. There are a couple of different options available that
      are described `here <https://github.com/cybertec-postgresql/pgwatch3/tree/master/pgwatch3/sql/metric_store>`__ in detail,
      but the gist of it is that you don't want too complex partitioning schemes if you don't have zounds of data and don't
      need the fastest queries. For a smaller amount of monitored DBs (a couple dozen to a hundred) the default "metric-time"
      is a good choice. For hundreds of databases, aggressive intervals, or long term storage usage of the TimescaleDB extension
      is recommended.

      ::

        cd /etc/pgwatch3/sql/metric_store
        psql -f roll_out_metric_time.psql pgwatch3_metrics

      **NB! Default retention for Postgres storage is 2 weeks!** To change, use the ``--pg-retention-days / PW2_PG_RETENTION_DAYS`` gatherer parameter.

#. **Prepare the "to-be-monitored" databases for metrics collection**

   As a minimum we need a plain unprivileged login user. Better though is to grant the user also the *pg_monitor* system role,
   available on v10+. Superuser privileges should be normally avoided for obvious reasons of course, but for initial testing in safe
   environments it can make the initial preparation (automatic *helper* rollouts) a bit easier still, given superuser privileges
   are later stripped.

   NB! To get most out of your metrics some *SECURITY DEFINER* wrappers functions called "helpers" are recommended on the DB-s under monitoring.
   See the detailed chapter on the "preparation" topic :ref:`here <helper_functions>` for more details.

#. **Install Python 3 and start the Web UI (optional)**

   NB! The Web UI is not strictly required but makes life a lot easier for *Config DB* based setups. Technically it would be fine also to manage connection
   strings of the monitored DB-s directly in the "pgwatch3.monitored_db" table and add/adjust metrics in the "pgwatch3.metric" table,
   and *Preset Configs* in the "pgwatch3.preset_config" table.

   #. Install Python 3 and Web UI requirements

      ::

         # first we need Python 3 and "pip" - the Python package manager
         sudo apt install python3 python3-pip
         cd /etc/pgwatch3/webpy/
         sudo pip3 install -U -r requirements_pg_metrics.txt

   #. Exposing component logs (optional)

      For use cases where exposing the component (Grafana, Postgres, gatherer daemon, Web UI itself) logs over the
      "/logs" endpoint remotely is wanted, then in the custom setup mode some actual code changes are needed to specify
      where logs of all components are situated - see top of the pgwatch3.py file for that. Defaults are set to work with the Docker image.

   #. Start the Web UI

      ::

        # NB! The defaults assume a local Config DB named pgwatch3, DB user pgwatch3
        python3 web.py --datastore=postgres --pg-metric-store-conn-str="dbname=pgwatch3_metrics user=pgwatch3"

      Default port for the Web UI: **8080**. See ``web.py --help`` for all options.

   #. Configure a SystemD auto-start service (optional)

      Sample startup scripts can be found at */etc/pgwatch3/webpy/startup-scripts/pgwatch3-webui.service* or online
      `here <https://github.com/cybertec-postgresql/pgwatch3/blob/master/webpy/startup-scripts/pgwatch3-webui.service>`__.
      Note that they are OS agnostic and always need some light adjustment of paths, etc - so always test them out.


#. **Configure DB-s and metrics / intervals to be monitored**

   * From the Web UI "/dbs" page

   * Via direct inserts into the Config DB *pgwatch3.monitored_db* table

#. **Start the pgwatch3 metrics collection agent**

   #. The gatherer has quite some parameters (use the *\-\-help* flag to show them all), but simplest form would be:

      ::

        # default connections params expect a trusted localhost Config DB setup
        # so mostly the 2nd line is not needed actually
        pgwatch3-daemon \
          --host=localhost --user=pgwatch3 --dbname=pgwatch3 \
          --datastore=postgres --pg-metric-store-conn-str=postgresql://pgwatch3@localhost:5432/pgwatch3_metrics \
          --verbose=info

        # or via SystemD if set up in step #2
        useradd -m -s /bin/bash pgwatch3 # default SystemD templates run under the pgwatch3 user
        sudo systemctl start pgwatch3
        sudo systemctl status pgwatch3

      After initial verification that all works it's usually good idea to set verbosity back to default by removing the
      *verbose* flag.

      Another tip to configure connection strings inside SystemD service files is to use the "systemd-escape" utility to
      escape special characters like spaces etc if using the LibPQ connect string syntax rather than JDBC syntax.

   #. Monitor the console or log output for any problems

      If you see metrics trickling into the "pgwatch3_metrics" database (metric names are mapped to table names and tables
      are auto-created), then congratulations - the deployment is working! When using some more aggressive *preset metrics config*
      then there are usually still some errors though, due to the fact that some more extensions or privileges are missing
      on the monitored database side. See the according chapter :ref:`here <preparing_databases>`.

   NB! When you're compiling your own gatherer then the executable file will be named just *pgwatch3* instead of *pgwatch3-daemon*
   to avoid mixups.

.. _custom_install_grafana:

#. **Install Grafana**

   #. Create a Postgres database to hold Grafana internal config, like dashboards etc

      Theoretically it's not absolutely required to use Postgres for storing Grafana internal settings / dashboards, but
      doing so has 2 advantages - you can easily roll out all pgwatch3 built-in dashboards and one can also do remote backups
      of the Grafana configuration easily.

      ::

        psql -c "create user pgwatch3_grafana password 'xyz'"
        psql -c "create database pgwatch3_grafana owner pgwatch3_grafana"

   #. Follow the instructions from `https://grafana.com/docs/grafana/latest/installation/debian/ <https://grafana.com/docs/grafana/latest/installation/debian/>`_, basically
      something like:

      ::

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

   #. Configure Grafana config to use our pgwatch3_grafana DB

      Place something like below in the "[database]" section of /etc/grafana/grafana.ini

      ::

        [database]
        type = postgres
        host = my-postgres-db:5432
        name = pgwatch3_grafana
        user = pgwatch3_grafana
        password = xyz

      Taking a look at [server], [security] and [auth*] sections is also recommended.

   #. Set up the pgwatch3 metrics database as the default datasource

      We need to tell Grafana where our metrics data is located. Add a datasource via the Grafana UI (Admin -> Data sources)
      or adjust and execute the "pgwatch3/bootstrap/grafana_datasource.sql" script on the *pgwatch3_grafana* DB.

   #. Add pgwatch3 predefined dashboards to Grafana

      This could be done by importing the pgwatch3 dashboard definition JSON-s manually, one by one, from the "grafana_dashboards" folder
      ("Import Dashboard" from the Grafana top menu) or via as small helper script located at */etc/pgwatch3/grafana-dashboards/import_all.sh*.
      The script needs some adjustment for metrics storage type, connect data and file paths.

   #. Optionally install also Grafana plugins

      Currently one pre-configured dashboard (Biggest relations treemap) use an extra plugin - if planning to that dash, then run the following:

      ::

        grafana-cli plugins install savantly-heatmap-panel

   #. Start discovering the preset dashbaords

      If the previous step of launching pgwatch3 daemon succeeded and it was more than some minutes ago, one should already
      see some graphs on dashboards like "DB overview" or "DB overview Unprivileged / Developer mode" for example.

.. _yaml_setup:

YAML based setup
----------------

From v1.4 one can also deploy the pgwatch3 gatherer daemons more easily in a de-centralized way, by specifying monitoring configuration via YAML files. In that case there is no need for a central Postgres "config DB".

**YAML installation steps**

#. Install pgwatch3 - either from pre-built packages or by compiling the Go code.
#. Specify hosts you want to monitor and with which metrics / aggressivness in a YAML file or files,
   following the example config located at */etc/pgwatch3/config/instances.yaml* or online
   `here <https://github.com/cybertec-postgresql/pgwatch3/blob/master/pgwatch3/config/instances.yaml>`__.
   Note that you can also use env. variables inside the YAML templates!
#. Bootstrap the metrics storage DB (not needed it using Prometheus mode).
#. Prepare the "to-be-monitored" databases for monitoring by creating a dedicated login role name as a :ref:`minimum <preparing_databases>`.
#. Run the pgatch2 gatherer specifying the YAML config file (or folder), and also the folder where metric definitions are
   located. Default location: */etc/pgwatch3/metrics*.
#. Install and configure Grafana and import the pgwatch3 sample dashboards to start analyzing the metrics. See above for instructions.
#. Make sure that there are auto-start SystemD services for all components in place and optionally set up also backups.

Relevant gatherer parameters / env. vars: ``--config / PW2_CONFIG`` and ``--metrics-folder / PW2_METRICS_FOLDER``.

For details on individual steps like installing pgwatch3 see the above paragraph.

NB! The Web UI component cannot be used in file based mode.
