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
}

// MetricStoreOpts specifies the storage configuration to store metrics data
type MetricOpts struct {
	Group                string `short:"g" long:"group" mapstructure:"group" description:"Group (or groups, comma separated) for filtering which DBs to monitor. By default all are monitored" env:"PW3_GROUP"`
	MetricsFolder        string `short:"m" long:"metrics-folder" mapstructure:"metrics-folder" description:"Folder of metrics definitions" env:"PW3_METRICS_FOLDER"`
	NoHelperFunctions    bool   `long:"no-helper-functions" mapstructure:"no-helper-functions" description:"Ignore metric definitions using helper functions (in form get_smth()) and don't also roll out any helpers automatically" env:"PW3_NO_HELPER_FUNCTIONS"`
	Datastore            string `long:"datastore" mapstructure:"datastore" description:"[postgres|prometheus|graphite|json]" default:"influx" env:"PW3_DATASTORE"`
	PGMetricStoreConnStr string `long:"pg-metric-store-conn-str" mapstructure:"pg-metric-store-conn-str" description:"PG Metric Store" env:"PW3_PG_METRIC_STORE_CONN_STR"`
	PGRetentionDays      int64  `long:"pg-retention-days" mapstructure:"pg-retention-days" description:"If set, metrics older than that will be deleted" default:"14" env:"PW3_PG_RETENTION_DAYS"`
	PrometheusPort       int64  `long:"prometheus-port" mapstructure:"prometheus-port" description:"Prometheus port. Effective with --datastore=prometheus" default:"9187" env:"PW3_PROMETHEUS_PORT"`
	PrometheusListenAddr string `long:"prometheus-listen-addr" mapstructure:"prometheus-listen-addr" description:"Network interface to listen on" default:"0.0.0.0" env:"PW3_PROMETHEUS_LISTEN_ADDR"`
	PrometheusNamespace  string `long:"prometheus-namespace" mapstructure:"prometheus-namespace" description:"Prefix for all non-process (thus Postgres) metrics" default:"pgwatch3" env:"PW3_PROMETHEUS_NAMESPACE"`
	PrometheusAsyncMode  bool   `long:"prometheus-async-mode" mapstructure:"prometheus-async-mode" description:"Gather in background as with other storage and cache last fetch results in memory" env:"PW3_PROMETHEUS_ASYNC_MODE"`
	GraphiteHost         string `long:"graphite-host" mapstructure:"graphite-host" description:"Graphite host" env:"PW3_GRAPHITEHOST"`
	GraphitePort         string `long:"graphite-port" mapstructure:"graphite-port" description:"Graphite port" env:"PW3_GRAPHITEPORT"`
	JsonStorageFile      string `long:"json-storage-file" mapstructure:"json-storage-file" description:"Path to file where metrics will be stored when --datastore=json, one metric set per line" env:"PW3_JSON_STORAGE_FILE"`
}

type CmdOptions struct {
	Connection ConnectionOpts `group:"Connection" mapstructure:"Connection"`
	Metric     MetricOpts     `group:"Metric" mapstructure:"Metric"`
	Verbose    string         `short:"v" long:"verbose" mapstructure:"verbose" description:"Chat level [DEBUG|INFO|WARN]. Default: WARN" env:"PW3_VERBOSE"`
	// Params for running based on local config files, enabled distributed "push model" based metrics gathering. Metrics are sent directly to Influx/Graphite.
	Config                  string `short:"c" long:"config" mapstructure:"config" description:"File or folder of YAML files containing info on which DBs to monitor and where to store metrics" env:"PW3_CONFIG"`
	BatchingDelayMs         int64  `long:"batching-delay-ms" mapstructure:"batching-delay-ms" description:"Max milliseconds to wait for a batched metrics flush. [Default: 250]" default:"250" env:"PW3_BATCHING_MAX_DELAY_MS"`
	AdHocConnString         string `long:"adhoc-conn-str" mapstructure:"adhoc-conn-str" description:"Ad-hoc mode: monitor a single Postgres DB specified by a standard Libpq connection string" env:"PW3_ADHOC_CONN_STR"`
	AdHocDBType             string `long:"adhoc-dbtype" mapstructure:"adhoc-dbtype" description:"Ad-hoc mode: postgres|postgres-continuous-discovery" default:"postgres" env:"PW3_ADHOC_DBTYPE"`
	AdHocConfig             string `long:"adhoc-config" mapstructure:"adhoc-config" description:"Ad-hoc mode: a preset config name or a custom JSON config" env:"PW3_ADHOC_CONFIG"`
	AdHocCreateHelpers      bool   `long:"adhoc-create-helpers" mapstructure:"adhoc-create-helpers" description:"Ad-hoc mode: try to auto-create helpers. Needs superuser to succeed" env:"PW3_ADHOC_CREATE_HELPERS"`
	AdHocUniqueName         string `long:"adhoc-name" mapstructure:"adhoc-name" description:"Ad-hoc mode: Unique 'dbname' for Influx" default:"adhoc" env:"PW3_ADHOC_NAME"`
	InternalStatsPort       int64  `long:"internal-stats-port" mapstructure:"internal-stats-port" description:"Port for inquiring monitoring status in JSON format" default:"8081" env:"PW3_INTERNAL_STATS_PORT"`
	DirectOSStats           bool   `long:"direct-os-stats" mapstructure:"direct-os-stats" description:"Extract OS related psutil statistics not via PL/Python wrappers but directly on host" env:"PW3_DIRECT_OS_STATS"`
	UseConnPooling          bool   `long:"use-conn-pooling" mapstructure:"use-conn-pooling" description:"Enable re-use of metrics fetching connections" env:"PW3_USE_CONN_POOLING"`
	AesGcmKeyphrase         string `long:"aes-gcm-keyphrase" mapstructure:"aes-gcm-keyphrase" description:"Decryption key for AES-GCM-256 passwords" env:"PW3_AES_GCM_KEYPHRASE"`
	AesGcmKeyphraseFile     string `long:"aes-gcm-keyphrase-file" mapstructure:"aes-gcm-keyphrase-file" description:"File with decryption key for AES-GCM-256 passwords" env:"PW3_AES_GCM_KEYPHRASE_FILE"`
	AesGcmPasswordToEncrypt string `long:"aes-gcm-password-to-encrypt" mapstructure:"aes-gcm-password-to-encrypt" description:"A special mode, returns the encrypted plain-text string and quits. Keyphrase(file) must be set. Useful for YAML mode" env:"PW3_AES_GCM_PASSWORD_TO_ENCRYPT"`
	// NB! "Test data" mode needs to be combined with "ad-hoc" mode to get an initial set of metrics from a real source
	TestdataMultiplier           int    `long:"testdata-multiplier" mapstructure:"testdata-multiplier" description:"For how many hosts to generate data" env:"PW3_TESTDATA_MULTIPLIER"`
	TestdataDays                 int    `long:"testdata-days" mapstructure:"testdata-days" description:"For how many days to generate data" env:"PW3_TESTDATA_DAYS"`
	AddRealDbname                bool   `long:"add-real-dbname" mapstructure:"add-real-dbname" description:"Add real DB name to each captured metric" env:"PW3_ADD_REAL_DBNAME"`
	RealDbnameField              string `long:"real-dbname-field" mapstructure:"real-dbname-field" description:"Tag key for real DB name if --add-real-dbname enabled" env:"PW3_REAL_DBNAME_FIELD" default:"real_dbname"`
	AddSystemIdentifier          bool   `long:"add-system-identifier" mapstructure:"add-system-identifier" description:"Add system identifier to each captured metric" env:"PW3_ADD_SYSTEM_IDENTIFIER"`
	SystemIdentifierField        string `long:"system-identifier-field" mapstructure:"system-identifier-field" description:"Tag key for system identifier value if --add-system-identifier" env:"PW3_SYSTEM_IDENTIFIER_FIELD" default:"sys_id"`
	InstanceLevelCacheMaxSeconds int64  `long:"instance-level-cache-max-seconds" mapstructure:"instance-level-cache-max-seconds" description:"Max allowed staleness for instance level metric data shared between DBs of an instance. Affects 'continuous' host types only. Set to 0 to disable" env:"PW3_INSTANCE_LEVEL_CACHE_MAX_SECONDS" default:"30"`
	MinDbSizeMB                  int64  `long:"min-db-size-mb" mapstructure:"min-db-size-mb" description:"Smaller size DBs will be ignored and not monitored until they reach the threshold." env:"PW3_MIN_DB_SIZE_MB" default:"0"`
	MaxParallelConnectionsPerDb  int    `long:"max-parallel-connections-per-db" mapstructure:"max-parallel-connections-per-db" description:"Max parallel metric fetches per DB. Note the multiplication effect on multi-DB instances" env:"PW3_MAX_PARALLEL_CONNECTIONS_PER_DB" default:"2"`
	Version                      bool   `long:"version" mapstructure:"version" description:"Show Git build version and exit" env:"PW3_VERSION"`
	Ping                         bool   `long:"ping" mapstructure:"ping" description:"Try to connect to all configured DB-s, report errors and then exit" env:"PW3_PING"`
	EmergencyPauseTriggerfile    string `long:"emergency-pause-triggerfile" mapstructure:"emergency-pause-triggerfile" description:"When the file exists no metrics will be temporarily fetched / scraped" env:"PW3_EMERGENCY_PAUSE_TRIGGERFILE" default:"/tmp/pgwatch3-emergency-pause"`
	TryCreateListedExtsIfMissing string `long:"try-create-listed-exts-if-missing" mapstructure:"try-create-listed-exts-if-missing" description:"Try creating the listed extensions (comma sep.) on first connect for all monitored DBs when missing. Main usage - pg_stat_statements" env:"PW3_TRY_CREATE_LISTED_EXTS_IF_MISSING" default:""`
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

var nonOptionArgs []string

// Parse will parse command line arguments and initialize pgengine
func Parse(writer io.Writer) (*flags.Parser, error) {
	cmdOpts := new(CmdOptions)
	parser := flags.NewParser(cmdOpts, flags.PrintErrors)
	var err error
	if nonOptionArgs, err = parser.Parse(); err != nil {
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
