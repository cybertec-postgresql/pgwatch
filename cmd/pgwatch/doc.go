// pgwatch is a command line tool that collects metrics from PostgreSQL databases and forwards to external sinks.
//
// Usage:
//
//	pgwatch [OPTIONS] [config | metric | source]
//
// Sources:
//
//	-s, --sources=                           Postgres URI, file or folder of YAML
//	                                         files containing info on which DBs
//	                                         to monitor [$PW_SOURCES]
//	    --refresh=                           How frequently to resync sources and
//	                                         metrics (default: 120) [$PW_REFRESH]
//	-g, --group=                             Groups for filtering which databases
//	                                         to monitor. By default all are
//	                                         monitored [$PW_GROUP]
//	    --min-db-size-mb=                    Smaller size DBs will be ignored and
//	                                         not monitored until they reach the
//	                                         threshold. (default: 0)
//	                                         [$PW_MIN_DB_SIZE_MB]
//	    --max-parallel-connections-per-db=   Max parallel metric fetches per DB.
//	                                         Note the multiplication effect on
//	                                         multi-DB instances (default: 4)
//	                                         [$PW_MAX_PARALLEL_CONNECTIONS_PER_DB]
//	    --try-create-listed-exts-if-missing= Try creating the listed extensions
//	                                         (comma sep.) on first connect for
//	                                         all monitored DBs when missing. Main
//	                                         usage - pg_stat_statements
//	                                         [$PW_TRY_CREATE_LISTED_EXTS_IF_MISSING]
//
// Metrics:
//
//	-m, --metrics=                           File or folder of YAML files with
//	                                         metrics definitions [$PW_METRICS]
//	    --create-helpers                     Create helper database objects from
//	                                         metric definitions
//	                                         [$PW_CREATE_HELPERS]
//	    --direct-os-stats                    Extract OS related psutil statistics
//	                                         not via PL/Python wrappers but
//	                                         directly on host
//	                                         [$PW_DIRECT_OS_STATS]
//	    --instance-level-cache-max-seconds=  Max allowed staleness for instance
//	                                         level metric data shared between DBs
//	                                         of an instance. Affects 'continuous'
//	                                         host types only. Set to 0 to disable
//	                                         (default: 30)
//	                                         [$PW_INSTANCE_LEVEL_CACHE_MAX_SECONDS]
//	    --emergency-pause-triggerfile=       When the file exists no metrics will
//	                                         be temporarily fetched / scraped
//	                                         (default:
//	                                         /tmp/pgwatch-emergency-pause)
//	                                         [$PW_EMERGENCY_PAUSE_TRIGGERFILE]
//
// Sinks:
//
//	--sink=                              URI where metrics will be stored,
//	                                     can be used multiple times [$PW_SINK]
//	--batching-delay=                    Sink-specific batching flush delay;
//										 may be ignored by some sinks
//	                                     (default: 950ms)
//	                                     [$PW_BATCHING_DELAY]
//	--retention=                         If set, metrics older than that will
//	                                     be deleted (default: 14)
//	                                     [$PW_RETENTION]
//	--real-dbname-field=                 Tag key for real database name
//	                                     (default: real_dbname)
//	                                     [$PW_REAL_DBNAME_FIELD]
//	--system-identifier-field=           Tag key for system identifier value
//	                                     (default: sys_id)
//	                                     [$PW_SYSTEM_IDENTIFIER_FIELD]
//
// Logging:
//
//	-v, --log-level=[debug|info|error]       Verbosity level for stdout and log
//	                                         file (default: info)
//	    --log-file=                          File name to store logs
//	    --log-file-format=[json|text]        Format of file logs (default: json)
//	    --log-file-rotate                    Rotate log files
//	    --log-file-size=                     Maximum size in MB of the log file
//	                                         before it gets rotated (default: 100)
//	    --log-file-age=                      Number of days to retain old log
//	                                         files, 0 means forever (default: 0)
//	    --log-file-number=                   Maximum number of old log files to
//	                                         retain, 0 to retain all (default: 0)
//
// WebUI:
//
//	--web-disable=[all|ui]               Disable REST API and/or web UI
//	                                     [$PW_WEBDISABLE]
//	--web-addr=                          TCP address in the form 'host:port'
//	                                     to listen on (default: :8080)
//	                                     [$PW_WEBADDR]
//	--web-user=                          Admin login [$PW_WEBUSER]
//	--web-password=                      Admin password [$PW_WEBPASSWORD]
//
// Help Options:
//
//	-h, --help                               Show this help message
//
// Available commands:
//
//	config         Manage configurations
//	   init            Initialize configuration
//	   upgrade         Upgrade configuration schema
//	metric         Manage metrics
//	   print-init      Get and print init SQL for a given metric or preset
//	   print-sql       Get and print SQL for a given metric
//	source         Manage sources
//	   ping            Try to connect to configured sources, report errors if any and exit
package main
