package config

import (
	"io"
	"os"

	flags "github.com/jessevdk/go-flags"
)

// ConnectionOpts specifies the database connection options
type ConnectionOpts struct {
	Host                      string `long:"host" mapstructure:"host" description:"PG config DB host" default:"localhost" env:"PW3_PGHOST"`
	Port                      string `short:"p" long:"port" mapstructure:"port" description:"PG config DB port" default:"5432" env:"PW3_PGPORT"`
	Dbname                    string `short:"d" long:"dbname" mapstructure:"dbname" description:"PG config DB dbname" default:"pgwatch3" env:"PW3_PGDATABASE"`
	User                      string `short:"u" long:"user" mapstructure:"user" description:"PG config DB user" default:"pgwatch3" env:"PW3_PGUSER"`
	Password                  string `long:"password" mapstructure:"password" description:"PG config DB password" env:"PW3_PGPASSWORD"`
	PgRequireSSL              bool   `long:"pg-require-ssl" mapstructure:"pg-require-ssl" description:"PG config DB SSL connection only" env:"PW3_PGSSL"`
	ServersRefreshLoopSeconds int    `long:"servers-refresh-loop-seconds" mapstructure:"servers-refresh-loop-seconds" description:"Sleep time for the main loop" env:"PW3_SERVERS_REFRESH_LOOP_SECONDS" default:"120"`
	Init                      bool   `long:"init" description:"Initialize database schema to the latest version and exit. Can be used with --upgrade"`
}

// MetricStoreOpts specifies the storage configuration to store metrics data
type MetricOpts struct {
	Group                string `short:"g" long:"group" mapstructure:"group" description:"Group (or groups, comma separated) for filtering which DBs to monitor. By default all are monitored" env:"PW3_GROUP"`
	MetricsFolder        string `short:"m" long:"metrics-folder" mapstructure:"metrics-folder" description:"Folder of metrics definitions" env:"PW3_METRICS_FOLDER"`
	NoHelperFunctions    bool   `long:"no-helper-functions" mapstructure:"no-helper-functions" description:"Ignore metric definitions using helper functions (in form get_smth()) and don't also roll out any helpers automatically" env:"PW3_NO_HELPER_FUNCTIONS"`
	Datastore            string `long:"datastore" mapstructure:"datastore" choice:"postgres" choice:"prometheus" choice:"json" default:"postgres" env:"PW3_DATASTORE"`
	PGMetricStoreConnStr string `long:"pg-metric-store-conn-str" mapstructure:"pg-metric-store-conn-str" description:"PG Metric Store" env:"PW3_PG_METRIC_STORE_CONN_STR"`
	PGRetentionDays      int64  `long:"pg-retention-days" mapstructure:"pg-retention-days" description:"If set, metrics older than that will be deleted" default:"14" env:"PW3_PG_RETENTION_DAYS"`
	PrometheusPort       int64  `long:"prometheus-port" mapstructure:"prometheus-port" description:"Prometheus port. Effective with --datastore=prometheus" default:"9187" env:"PW3_PROMETHEUS_PORT"`
	PrometheusListenAddr string `long:"prometheus-listen-addr" mapstructure:"prometheus-listen-addr" description:"Network interface to listen on" default:"0.0.0.0" env:"PW3_PROMETHEUS_LISTEN_ADDR"`
	PrometheusNamespace  string `long:"prometheus-namespace" mapstructure:"prometheus-namespace" description:"Prefix for all non-process (thus Postgres) metrics" default:"pgwatch3" env:"PW3_PROMETHEUS_NAMESPACE"`
	PrometheusAsyncMode  bool   `long:"prometheus-async-mode" mapstructure:"prometheus-async-mode" description:"Gather in background as with other storage and cache last fetch results in memory" env:"PW3_PROMETHEUS_ASYNC_MODE"`
	JSONStorageFile      string `long:"json-storage-file" mapstructure:"json-storage-file" description:"Path to file where metrics will be stored when --datastore=json, one metric set per line" env:"PW3_JSON_STORAGE_FILE"`
}

// LoggingOpts specifies the logging configuration
type LoggingOpts struct {
	// LogDBLevel    string `long:"log-database-level" mapstructure:"log-database-level" description:"Verbosity level for database storing" choice:"debug" choice:"info" choice:"error" choice:"none" default:"info"`
	LogLevel      string `short:"v" long:"log-level" mapstructure:"log-level" description:"Verbosity level for stdout and log file" choice:"debug" choice:"info" choice:"error" default:"info"`
	LogFile       string `long:"log-file" mapstructure:"log-file" description:"File name to store logs"`
	LogFileFormat string `long:"log-file-format" mapstructure:"log-file-format" description:"Format of file logs" choice:"json" choice:"text" default:"json"`
	LogFileRotate bool   `long:"log-file-rotate" mapstructure:"log-file-rotate" description:"Rotate log files"`
	LogFileSize   int    `long:"log-file-size" mapstructure:"log-file-size" description:"Maximum size in MB of the log file before it gets rotated" default:"100"`
	LogFileAge    int    `long:"log-file-age" mapstructure:"log-file-age" description:"Number of days to retain old log files, 0 means forever" default:"0"`
	LogFileNumber int    `long:"log-file-number" mapstructure:"log-file-number" description:"Maximum number of old log files to retain, 0 to retain all" default:"0"`
}

// StartOpts specifies the application startup options
type StartOpts struct {
	// File    string `short:"f" long:"file" description:"SQL script file to execute during startup"`
	// Upgrade bool   `long:"upgrade" description:"Upgrade database to the latest version"`
	// Debug   bool   `long:"debug" description:"Run in debug mode. Only asynchronous chains will be executed"`
}

// WebUIOpts specifies the internal web UI server options
type WebUIOpts struct {
	WebAddr     string `long:"web-addr" mapstructure:"web-addr" description:"TCP address in the form 'host:port' to listen on" default:":8080" env:"PW3_WEBADDR"`
	WebUser     string `long:"web-user" mapstructure:"web-user" description:"Admin login" env:"PW3_WEBUSER"`
	WebPassword string `long:"web-password" mapstructure:"web-password" description:"Admin password" env:"PW3_WEBPASSWORD"`
}

type CmdOptions struct {
	Connection                   ConnectionOpts `group:"Connection" mapstructure:"Connection"`
	Metric                       MetricOpts     `group:"Metric" mapstructure:"Metric"`
	Logging                      LoggingOpts    `group:"Logging" mapstructure:"Logging"`
	WebUI                        WebUIOpts      `group:"WebUI" mapstructure:"WebUI"`
	Start                        StartOpts      `group:"Start" mapstructure:"Start"`
	Config                       string         `short:"c" long:"config" mapstructure:"config" description:"File or folder of YAML files containing info on which DBs to monitor and where to store metrics" env:"PW3_CONFIG"`
	BatchingDelayMs              int64          `long:"batching-delay-ms" mapstructure:"batching-delay-ms" description:"Max milliseconds to wait for a batched metrics flush. [Default: 250]" default:"250" env:"PW3_BATCHING_MAX_DELAY_MS"`
	AdHocConnString              string         `long:"adhoc-conn-str" mapstructure:"adhoc-conn-str" description:"Ad-hoc mode: monitor a single Postgres DB specified by a standard Libpq connection string" env:"PW3_ADHOC_CONN_STR"`
	AdHocDBType                  string         `long:"adhoc-dbtype" mapstructure:"adhoc-dbtype" description:"Ad-hoc mode: postgres|postgres-continuous-discovery" default:"postgres" env:"PW3_ADHOC_DBTYPE"`
	AdHocConfig                  string         `long:"adhoc-config" mapstructure:"adhoc-config" description:"Ad-hoc mode: a preset config name or a custom JSON config" env:"PW3_ADHOC_CONFIG"`
	AdHocCreateHelpers           bool           `long:"adhoc-create-helpers" mapstructure:"adhoc-create-helpers" description:"Ad-hoc mode: try to auto-create helpers. Needs superuser to succeed" env:"PW3_ADHOC_CREATE_HELPERS"`
	AdHocUniqueName              string         `long:"adhoc-name" mapstructure:"adhoc-name" description:"Ad-hoc mode: Unique 'dbname' for Influx" default:"adhoc" env:"PW3_ADHOC_NAME"`
	DirectOSStats                bool           `long:"direct-os-stats" mapstructure:"direct-os-stats" description:"Extract OS related psutil statistics not via PL/Python wrappers but directly on host" env:"PW3_DIRECT_OS_STATS"`
	UseConnPooling               bool           `long:"use-conn-pooling" mapstructure:"use-conn-pooling" description:"Enable re-use of metrics fetching connections" env:"PW3_USE_CONN_POOLING"`
	AesGcmKeyphrase              string         `long:"aes-gcm-keyphrase" mapstructure:"aes-gcm-keyphrase" description:"Decryption key for AES-GCM-256 passwords" env:"PW3_AES_GCM_KEYPHRASE"`
	AesGcmKeyphraseFile          string         `long:"aes-gcm-keyphrase-file" mapstructure:"aes-gcm-keyphrase-file" description:"File with decryption key for AES-GCM-256 passwords" env:"PW3_AES_GCM_KEYPHRASE_FILE"`
	AesGcmPasswordToEncrypt      string         `long:"aes-gcm-password-to-encrypt" mapstructure:"aes-gcm-password-to-encrypt" description:"A special mode, returns the encrypted plain-text string and quits. Keyphrase(file) must be set. Useful for YAML mode" env:"PW3_AES_GCM_PASSWORD_TO_ENCRYPT"`
	AddRealDbname                bool           `long:"add-real-dbname" mapstructure:"add-real-dbname" description:"Add real DB name to each captured metric" env:"PW3_ADD_REAL_DBNAME"`
	RealDbnameField              string         `long:"real-dbname-field" mapstructure:"real-dbname-field" description:"Tag key for real DB name if --add-real-dbname enabled" env:"PW3_REAL_DBNAME_FIELD" default:"real_dbname"`
	AddSystemIdentifier          bool           `long:"add-system-identifier" mapstructure:"add-system-identifier" description:"Add system identifier to each captured metric" env:"PW3_ADD_SYSTEM_IDENTIFIER"`
	SystemIdentifierField        string         `long:"system-identifier-field" mapstructure:"system-identifier-field" description:"Tag key for system identifier value if --add-system-identifier" env:"PW3_SYSTEM_IDENTIFIER_FIELD" default:"sys_id"`
	InstanceLevelCacheMaxSeconds int64          `long:"instance-level-cache-max-seconds" mapstructure:"instance-level-cache-max-seconds" description:"Max allowed staleness for instance level metric data shared between DBs of an instance. Affects 'continuous' host types only. Set to 0 to disable" env:"PW3_INSTANCE_LEVEL_CACHE_MAX_SECONDS" default:"30"`
	MinDbSizeMB                  int64          `long:"min-db-size-mb" mapstructure:"min-db-size-mb" description:"Smaller size DBs will be ignored and not monitored until they reach the threshold." env:"PW3_MIN_DB_SIZE_MB" default:"0"`
	MaxParallelConnectionsPerDb  int            `long:"max-parallel-connections-per-db" mapstructure:"max-parallel-connections-per-db" description:"Max parallel metric fetches per DB. Note the multiplication effect on multi-DB instances" env:"PW3_MAX_PARALLEL_CONNECTIONS_PER_DB" default:"2"`
	Version                      bool           `long:"version" mapstructure:"version" description:"Show Git build version and exit" env:"PW3_VERSION"`
	Ping                         bool           `long:"ping" mapstructure:"ping" description:"Try to connect to all configured DB-s, report errors and then exit" env:"PW3_PING"`
	EmergencyPauseTriggerfile    string         `long:"emergency-pause-triggerfile" mapstructure:"emergency-pause-triggerfile" description:"When the file exists no metrics will be temporarily fetched / scraped" env:"PW3_EMERGENCY_PAUSE_TRIGGERFILE" default:"/tmp/pgwatch3-emergency-pause"`
	TryCreateListedExtsIfMissing string         `long:"try-create-listed-exts-if-missing" mapstructure:"try-create-listed-exts-if-missing" description:"Try creating the listed extensions (comma sep.) on first connect for all monitored DBs when missing. Main usage - pg_stat_statements" env:"PW3_TRY_CREATE_LISTED_EXTS_IF_MISSING" default:""`
}

// Verbose returns true if the debug log is enabled
func (c CmdOptions) Verbose() bool {
	return c.Logging.LogLevel == "debug"
}

func (c CmdOptions) IsAdHocMode() bool {
	return len(c.AdHocConnString)+len(c.AdHocConfig) > 0
}

// VersionOnly returns true if the `--version` is the only argument
func (c CmdOptions) VersionOnly() bool {
	return len(os.Args) == 2 && c.Version
}

// NewCmdOptions returns a new instance of CmdOptions with default values
func NewCmdOptions(args ...string) *CmdOptions {
	cmdOpts := new(CmdOptions)
	_, _ = flags.NewParser(cmdOpts, flags.PrintErrors).ParseArgs(args)
	return cmdOpts
}

// var nonOptionArgs []string

// Parse will parse command line arguments and initialize pgengine
func Parse(writer io.Writer) (*flags.Parser, error) {
	cmdOpts := new(CmdOptions)
	parser := flags.NewParser(cmdOpts, flags.PrintErrors)
	var err error
	if _, err = parser.Parse(); err != nil {
		if !flags.WroteHelp(err) {
			parser.WriteHelp(writer)
			return nil, err
		}
	}
	// if cmdOpts.Config != "" {
	// 	if _, err := os.Stat(cmdOpts.Config); os.IsNotExist(err) {
	// 		return nil, err
	// 	}
	// }
	//non-option arguments
	// if len(nonOptionArgs) > 0 && cmdOpts.Connection.PgURL == "" {
	// 	cmdOpts.Connection.PgURL = nonOptionArgs[0]
	// }
	return parser, nil
}
