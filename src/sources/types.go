package sources

import (
	"context"
	"maps"
	"slices"

	"github.com/cybertec-postgresql/pgwatch3/db"
	pgx "github.com/jackc/pgx/v5"
)

type Kind string

const (
	SourcePostgres           Kind = "postgres"
	SourcePostgresContinuous Kind = "postgres-continuous-discovery"
	SourcePgBouncer          Kind = "pgbouncer"
	SourcePgPool             Kind = "pgpool"
	SourcePatroni            Kind = "patroni"
	SourcePatroniContinuous  Kind = "patroni-continuous-discovery"
	SourcePatroniNamespace   Kind = "patroni-namespace-discovery"
)

var Kinds = []Kind{
	SourcePostgres,
	SourcePostgresContinuous,
	SourcePgBouncer,
	SourcePgPool,
	SourcePatroni,
	SourcePatroniContinuous,
	SourcePatroniNamespace,
}

func (k Kind) IsValid() bool {
	return slices.Contains[[]Kind, Kind](Kinds, k)
}

type (

	// Source represents a configuration how to get databases to monitor. It can be a single database,
	// a group of databases in postgres cluster, a group of databases in HA patroni cluster.
	// pgbouncer and pgpool kinds are purely to indicate that the monitored database connection is made
	// through a connection pooler, which supports its own additional metrics. If one is not interested in
	// those additional metrics, it is ok to specify the connection details as a regular postgres source.
	Source struct {
		Name                 string             `yaml:"name" db:"name"`
		Group                string             `yaml:"group" db:"group"`
		ConnStr              string             `yaml:"conn_str" db:"connstr"`
		Metrics              map[string]float64 `yaml:"custom_metrics" db:"config"`
		MetricsStandby       map[string]float64 `yaml:"custom_metrics_standby" db:"config_standby"`
		Kind                 Kind               `yaml:"kind" db:"dbtype"`
		IncludePattern       string             `yaml:"include_pattern" db:"include_pattern"`
		ExcludePattern       string             `yaml:"exclude_pattern" db:"exclude_pattern"`
		PresetMetrics        string             `yaml:"preset_metrics" db:"preset_config"`
		PresetMetricsStandby string             `yaml:"preset_metrics_standby" db:"preset_config_standby"`
		IsSuperuser          bool               `yaml:"is_superuser" db:"is_superuser"`
		IsEnabled            bool               `yaml:"is_enabled" db:"is_enabled"`
		CustomTags           map[string]string  `yaml:"custom_tags" db:"custom_tags"`
		HostConfig           HostConfigAttrs    `yaml:"host_config" db:"host_config"`
		OnlyIfMaster         bool               `yaml:"only_if_master" db:"only_if_master"`
	}

	Sources []Source
)

func (s *Source) GetDatabaseName() string {
	if с, err := pgx.ParseConfig(s.ConnStr); err == nil {
		return с.Database
	}
	return ""
}

func (s *Source) Clone() *Source {
	c := new(Source)
	*c = *s
	c.Metrics = maps.Clone(s.Metrics)
	c.MetricsStandby = maps.Clone(s.MetricsStandby)
	c.CustomTags = maps.Clone(s.CustomTags)
	return c
}

// MonitoredDatabase represents a single database to monitor. Unlike source, it contains a database connection.
// Continuous discovery sources (postgres-continuous-discovery, patroni-continuous-discovery, patroni-namespace-discovery)
// will produce multiple monitored databases structs based on the discovered databases.
type (
	MonitoredDatabase struct {
		Source
		Conn       db.PgxPoolIface
		ConnConfig *pgx.ConnConfig
	}

	MonitoredDatabases []*MonitoredDatabase
)

// Ping will try to establish a brand new connection to the server and return any error
func (md *MonitoredDatabase) Ping(ctx context.Context) error {
	c, err := pgx.Connect(ctx, md.ConnStr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close(ctx) }()
	return c.Ping(ctx)
}

// Connect will establish a connection to the database if it's not already connected.
// If the connection is already established, it pings the server to ensure it's still alive.
func (md *MonitoredDatabase) Connect(ctx context.Context) (err error) {
	if md.Conn == nil {
		if md.Conn, err = db.New(ctx, md.ConnStr); err != nil {
			return err
		}
	}
	return md.Conn.Ping(ctx)
}

// GetDatabaseName returns the database name from the connection string
func (md *MonitoredDatabase) GetDatabaseName() string {
	var err error
	if md.ConnConfig == nil {
		if md.ConnConfig, err = pgx.ParseConfig(md.ConnStr); err != nil {
			return ""
		}
	}
	return md.ConnConfig.Database
}

// SetDatabaseName sets the database name in the connection config but
// does not update the connection string
func (md *MonitoredDatabase) SetDatabaseName(name string) {
	var err error
	if md.ConnConfig == nil {
		if md.ConnConfig, err = pgx.ParseConfig(md.ConnStr); err != nil {
			return
		}
	}
	md.ConnConfig.Database = name
}

func (md *MonitoredDatabase) IsPostgresSource() bool {
	return md.Kind != SourcePgBouncer && md.Kind != SourcePgPool
}

func (mds MonitoredDatabases) GetMonitoredDatabase(DBUniqueName string) *MonitoredDatabase {
	for _, md := range mds {
		if md.Name == DBUniqueName {
			return md
		}
	}
	return nil
}

// SyncFromReader will update the monitored databases with the latest configuration from the reader.
// Any resolution errors will be returned, e.g. etcd unavailability.
// It's up to the caller to proceed with the databases available or stop the execution due to errors.
func (mds MonitoredDatabases) SyncFromReader(r Reader) (newmds MonitoredDatabases, err error) {
	srcs, err := r.GetSources()
	if err != nil {
		return nil, err
	}
	newmds, err = srcs.ResolveDatabases()
	for _, newMD := range newmds {
		if md := mds.GetMonitoredDatabase(newMD.Name); md != nil {
			newMD.Conn = md.Conn
			newMD.ConnConfig = md.ConnConfig
		}
	}
	return newmds, err
}

type HostConfigAttrs struct {
	DcsType                string   `yaml:"dcs_type"`
	DcsEndpoints           []string `yaml:"dcs_endpoints"`
	Scope                  string
	Namespace              string
	Username               string
	Password               string
	CAFile                 string                             `yaml:"ca_file"`
	CertFile               string                             `yaml:"cert_file"`
	KeyFile                string                             `yaml:"key_file"`
	LogsGlobPath           string                             `yaml:"logs_glob_path"`   // default $data_directory / $log_directory / *.csvlog
	LogsMatchRegex         string                             `yaml:"logs_match_regex"` // default is for CSVLOG format. needs to capture following named groups: log_time, user_name, database_name and error_severity
	PerMetricDisabledTimes []HostConfigPerMetricDisabledTimes `yaml:"per_metric_disabled_intervals"`
}

type HostConfigPerMetricDisabledTimes struct { // metric gathering override per host / metric / time
	Metrics       []string `yaml:"metrics"`
	DisabledTimes []string `yaml:"disabled_times"`
	DisabledDays  string   `yaml:"disabled_days"`
}

type Reader interface {
	GetSources() (Sources, error)
}

type Writer interface {
	WriteSources(Sources) error
	DeleteSource(string) error
	UpdateSource(md Source) error
}

type ReaderWriter interface {
	Reader
	Writer
}
