package config

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	flags "github.com/jessevdk/go-flags"
)

type Kind int

const (
	ConfigPgURL Kind = iota
	ConfigFile
	ConfigFolder
	ConfigError
)

// SourceOpts specifies the sources retrieval options
type SourceOpts struct {
	Config                       string   `short:"c" long:"config" mapstructure:"config" description:"Postgres URI, file or folder of YAML files containing info on which DBs to monitor and where to store metrics" env:"PW3_CONFIG"`
	Refresh                      int      `long:"refresh" mapstructure:"refresh" description:"How frequently to resync sources and metrics" env:"PW3_REFRESH" default:"120"`
	Init                         bool     `long:"init" description:"Initialize configuration database schema to the latest version and exit. Can be used with --upgrade"`
	Groups                       []string `short:"g" long:"group" mapstructure:"group" description:"Groups for filtering which databases to monitor. By default all are monitored" env:"PW3_GROUP"`
	MinDbSizeMB                  int64    `long:"min-db-size-mb" mapstructure:"min-db-size-mb" description:"Smaller size DBs will be ignored and not monitored until they reach the threshold." env:"PW3_MIN_DB_SIZE_MB" default:"0"`
	MaxParallelConnectionsPerDb  int      `long:"max-parallel-connections-per-db" mapstructure:"max-parallel-connections-per-db" description:"Max parallel metric fetches per DB. Note the multiplication effect on multi-DB instances" env:"PW3_MAX_PARALLEL_CONNECTIONS_PER_DB" default:"2"`
	TryCreateListedExtsIfMissing string   `long:"try-create-listed-exts-if-missing" mapstructure:"try-create-listed-exts-if-missing" description:"Try creating the listed extensions (comma sep.) on first connect for all monitored DBs when missing. Main usage - pg_stat_statements" env:"PW3_TRY_CREATE_LISTED_EXTS_IF_MISSING" default:""`
}

// MetricOpts specifies metric definitions
type MetricOpts struct {
	Metrics                      string `short:"m" long:"metrics" mapstructure:"metrics" description:"File or folder of YAML files with metrics definitions" env:"PW3_METRICS"`
	NoHelperFunctions            bool   `long:"no-helper-functions" mapstructure:"no-helper-functions" description:"Ignore metric definitions using helper functions (in form get_smth()) and don't also roll out any helpers automatically" env:"PW3_NO_HELPER_FUNCTIONS"`
	DirectOSStats                bool   `long:"direct-os-stats" mapstructure:"direct-os-stats" description:"Extract OS related psutil statistics not via PL/Python wrappers but directly on host" env:"PW3_DIRECT_OS_STATS"`
	InstanceLevelCacheMaxSeconds int64  `long:"instance-level-cache-max-seconds" mapstructure:"instance-level-cache-max-seconds" description:"Max allowed staleness for instance level metric data shared between DBs of an instance. Affects 'continuous' host types only. Set to 0 to disable" env:"PW3_INSTANCE_LEVEL_CACHE_MAX_SECONDS" default:"30"`
	EmergencyPauseTriggerfile    string `long:"emergency-pause-triggerfile" mapstructure:"emergency-pause-triggerfile" description:"When the file exists no metrics will be temporarily fetched / scraped" env:"PW3_EMERGENCY_PAUSE_TRIGGERFILE" default:"/tmp/pgwatch3-emergency-pause"`
}

// MeasurementOpts specifies the storage configuration to store metrics measurements
type MeasurementOpts struct {
	Sinks                 []string      `long:"sink" mapstructure:"sink" description:"URI where metrics will be stored" env:"PW3_SINK"`
	BatchingDelay         time.Duration `long:"batching-delay" mapstructure:"batching-delay" description:"Max milliseconds to wait for a batched metrics flush. [Default: 250ms]" default:"250ms" env:"PW3_BATCHING_MAX_DELAY"`
	Retention             int           `long:"retention" mapstructure:"retention" description:"If set, metrics older than that will be deleted" default:"14" env:"PW3_RETENTION"`
	RealDbnameField       string        `long:"real-dbname-field" mapstructure:"real-dbname-field" description:"Tag key for real DB name if --add-real-dbname enabled" env:"PW3_REAL_DBNAME_FIELD" default:"real_dbname"`
	SystemIdentifierField string        `long:"system-identifier-field" mapstructure:"system-identifier-field" description:"Tag key for system identifier value if --add-system-identifier" env:"PW3_SYSTEM_IDENTIFIER_FIELD" default:"sys_id"`
}

// LoggingOpts specifies the logging configuration
type LoggingOpts struct {
	LogLevel      string `short:"v" long:"log-level" mapstructure:"log-level" description:"Verbosity level for stdout and log file" choice:"debug" choice:"info" choice:"error" default:"info"`
	LogFile       string `long:"log-file" mapstructure:"log-file" description:"File name to store logs"`
	LogFileFormat string `long:"log-file-format" mapstructure:"log-file-format" description:"Format of file logs" choice:"json" choice:"text" default:"json"`
	LogFileRotate bool   `long:"log-file-rotate" mapstructure:"log-file-rotate" description:"Rotate log files"`
	LogFileSize   int    `long:"log-file-size" mapstructure:"log-file-size" description:"Maximum size in MB of the log file before it gets rotated" default:"100"`
	LogFileAge    int    `long:"log-file-age" mapstructure:"log-file-age" description:"Number of days to retain old log files, 0 means forever" default:"0"`
	LogFileNumber int    `long:"log-file-number" mapstructure:"log-file-number" description:"Maximum number of old log files to retain, 0 to retain all" default:"0"`
}

// WebUIOpts specifies the internal web UI server options
type WebUIOpts struct {
	WebAddr     string `long:"web-addr" mapstructure:"web-addr" description:"TCP address in the form 'host:port' to listen on" default:":8080" env:"PW3_WEBADDR"`
	WebUser     string `long:"web-user" mapstructure:"web-user" description:"Admin login" env:"PW3_WEBUSER"`
	WebPassword string `long:"web-password" mapstructure:"web-password" description:"Admin password" env:"PW3_WEBPASSWORD"`
}

type Options struct {
	Sources      SourceOpts      `group:"Sources"`
	Metrics      MetricOpts      `group:"Metrics"`
	Measurements MeasurementOpts `group:"Measurements"`
	Logging      LoggingOpts     `group:"Logging"`
	WebUI        WebUIOpts       `group:"WebUI"`
	Version      bool            `long:"version" mapstructure:"version" description:"Show Git build version and exit" env:"PW3_VERSION"`
	Ping         bool            `long:"ping" mapstructure:"ping" description:"Try to connect to all configured DB-s, report errors and then exit" env:"PW3_PING"`
}

// New returns a new instance of CmdOptions
func New(writer io.Writer) (*Options, error) {
	cmdOpts := new(Options)
	parser := flags.NewParser(cmdOpts, flags.HelpFlag)
	var err error
	if _, err = parser.Parse(); err != nil {
		if !flags.WroteHelp(err) {
			parser.WriteHelp(writer)
			return cmdOpts, err
		}
	}
	return cmdOpts, validateConfig(cmdOpts)
}

// Verbose returns true if the debug log is enabled
func (c Options) Verbose() bool {
	return c.Logging.LogLevel == "debug"
}

// VersionOnly returns true if the `--version` is the only argument
func (c Options) VersionOnly() bool {
	return len(os.Args) == 2 && c.Version
}

func (c Options) GetConfigKind() (_ Kind, err error) {
	if _, err := pgx.ParseConfig(c.Sources.Config); err == nil {
		return Kind(ConfigPgURL), nil
	}
	var fi os.FileInfo
	if fi, err = os.Stat(c.Sources.Config); err == nil {
		if fi.IsDir() {
			return Kind(ConfigFolder), nil
		}
		return Kind(ConfigFile), nil
	}
	return Kind(ConfigError), err
}

func validateConfig(c *Options) error {
	if c.Sources.Config == "" && !c.VersionOnly() {
		return errors.New("--config was not specified")
	}
	if c.Sources.Refresh <= 1 {
		return errors.New("--servers-refresh-loop-seconds must be greater than 1")
	}
	if c.Sources.MaxParallelConnectionsPerDb < 1 {
		return errors.New("--max-parallel-connections-per-db must be >= 1")
	}

	// validate that input is boolean is set
	if c.Measurements.BatchingDelay <= 0 || c.Measurements.BatchingDelay > time.Hour {
		return errors.New("--batching-delay-ms must be between 0 and 3600000")
	}

	return nil
}
