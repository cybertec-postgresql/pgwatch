# Available env. variables by components

Some variables influence multiple components. Command line parameters override env. variables (when doing custom deployments).

## Docker image specific

- **PW3_TESTDB** When set, the config DB itself will be added to monitoring as "test". Default: -
- **PW3_PG_SCHEMA_TYPE** Enables to choose different metric storage models for the "pgwatch3" image - [metric-time|metric-dbname-time]. Default: metric-time

## Gatherer daemon

- **PW3_GROUP** Logical grouping/sharding key to monitor a subset of configured hosts. Default: -
- **PW3_PG_METRIC_STORE_CONN_STR** Postgres metric store connection string. Default: -
- **PW3_JSON_STORAGE_FILE** File to store metric values. Default: -
- **PW3_PG_RETENTION_DAYS** Effective when PW3_DATASTORE=postgres. Default: 14
- **PW3_CONFIG** Connection string (`postgresql://user:pwd@host/db`), file or folder of YAML (.yaml/.yml) files containing info on which DBs to monitor and where to store metrics
- **PW3_METRICS_FOLDER** File mode. Folder of metrics definitions
- **PW3_BATCHING_MAX_DELAY_MS** Max milliseconds to wait for a batched metrics flush. Default: 250
- **PW3_ADHOC_CONN_STR** Ad-hoc mode. Monitor a single Postgres DB / instance specified by a standard Libpq connection string
- **PW3_ADHOC_CONFIG** Ad-hoc mode. A preset config name or a custom JSON config
- **PW3_ADHOC_CREATE_HELPERS** Ad-hoc mode. Try to auto-create helpers. Needs superuser to succeed. Default: false
- **PW3_ADHOC_NAME** Ad-hoc mode. Unique 'dbname' for data store. Default: adhoc
- **PW3_ADHOC_DBTYPE** Ad-hoc mode: postgres|postgres-continuous-discovery. Default: postgres
- **PW3_INTERNAL_STATS_PORT** Port for inquiring monitoring status in JSON format. Default: 8081
- **PW3_CONN_POOLING** Enable re-use of metrics fetching connections. "off" means reconnect every time. Default: off
- **PW3_AES_GCM_KEYPHRASE** Keyphrase for encryption/decpyption of connect string passwords.
- **PW3_AES_GCM_KEYPHRASE_FILE** File containing a keyphrase for encryption/decpyption of connect string passwords.
- **PW3_AES_GCM_PASSWORD_TO_ENCRYPT** A special mode, returns the encrypted plain-text string and quits. Keyphrase(file) must be set
- **PW3_PROMETHEUS_PORT** Prometheus port. Effective with --datastore=prometheus. Default: 9187
- **PW3_PROMETHEUS_LISTEN_ADDR** Network interface to listen on. Default: "0.0.0.0"
- **PW3_PROMETHEUS_NAMESPACE** Prefix for all non-process (thus Postgres) metrics. Default: "pgwatch3"
- **PW3_PROMETHEUS_ASYNC_MODE** Gather in background as with other storages and cache last fetch results for each metric in memory. Default: false
- **PW3_ADD_SYSTEM_IDENTIFIER** Add system identifier to each captured metric (PG10+). Default: false
- **PW3_SYSTEM_IDENTIFIER_FIELD** Control name of the "system identifier" field. Default: sys_id
- **PW3_SERVERS_REFRESH_LOOP_SECONDS** Sleep time for the main loop. Default: 120
- **PW3_VERSION** Show Git build version and exit.
- **PW3_PING** Try to connect to all configured DB-s, report errors and then exit.
- **PW3_INSTANCE_LEVEL_CACHE_MAX_SECONDS** Max allowed staleness for instance level metric data shared between DBs of an instance. Affects 'continuous' host types only. Set to 0 to disable. Default: 30
- **PW3_DIRECT_OS_STATS** Extract OS related psutil statistics not via PL/Python wrappers but directly on host, i.e. assumes "push" setup. Default: off.
- **PW3_MIN_DB_SIZE_MB** Smaller size DBs will be ignored and not monitored until they reach the threshold. Default: 0 (no size-based limiting).
- **PW3_MAX_PARALLEL_CONNECTIONS_PER_DB** Max parallel metric fetches per DB. Note the multiplication effect on multi-DB instances. Default: 2
- **PW3_EMERGENCY_PAUSE_TRIGGERFILE** When the file exists no metrics will be temporarily fetched / scraped. Default: /tmp/pgwatch3-emergency-pause
- **PW3_NO_HELPER_FUNCTIONS** Ignore metric definitions using helper functions (in form get_smth()) and don't also roll out any helpers automatically. Default: false
- **PW3_TRY_CREATE_LISTED_EXTS_IF_MISSING** Try creating the listed extensions (comma sep.) on first connect for all monitored DBs when missing. Main usage - pg_stat_statements. Default: ""

## Grafana

- **PW3_GRAFANANOANONYMOUS** Can be set to require login even for viewing dashboards. Default: -
- **PW3_GRAFANAUSER** Administrative user. Default: admin
- **PW3_GRAFANAPASSWORD** Administrative user password. Default: pgwatch3admin
- **PW3_GRAFANASSL** Use SSL. Default: -
- **PW3_GRAFANA_BASEURL** For linking to Grafana "Query details" dashboard from "Stat_stmt. overview". Default: http://0.0.0.0:3000
