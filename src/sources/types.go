package sources

import (
	"context"
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
	SourcePatroni,
	SourcePatroniContinuous,
	SourcePatroniNamespace,
}

func (k Kind) IsValid() bool {
	return slices.Contains[[]Kind, Kind](Kinds, k)
}

type MonitoredDatabase struct {
	DBUniqueName         string             `yaml:"unique_name" db:"name"`
	DBUniqueNameOrig     string             // to preserve belonging to a specific instance for continuous modes where DBUniqueName will be dynamic
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

func (md MonitoredDatabase) GetDatabaseName() string {
	if conf, err := pgx.ParseConfig(md.ConnStr); err == nil {
		return conf.Database
	}
	return ""
}

func (md MonitoredDatabase) IsPostgresSource() bool {
	return md.Kind != SourcePgBouncer && md.Kind != SourcePgPool
}

// ExpandDatabases() return a slice of found databases for continuous monitoring sources, e.g. patroni
func (md MonitoredDatabase) ExpandDatabases() (MonitoredDatabases, error) {
	switch md.Kind {
	case SourcePatroni, SourcePatroniContinuous, SourcePatroniNamespace:
		return ResolveDatabasesFromPatroni(md)
	case SourcePostgresContinuous:
		return md.ResolveDatabasesFromPostgres()
	}
	return nil, nil
}

// "resolving" reads all the DB names from the given host/port, additionally matching/not matching specified regex patterns
func (md MonitoredDatabase) ResolveDatabasesFromPostgres() (resolvedDbs MonitoredDatabases, err error) {
	var (
		c      db.PgxPoolIface
		dbname string
		rows   pgx.Rows
	)
	c, err = db.New(context.TODO(), md.ConnStr)
	if err != nil {
		return
	}
	defer c.Close()

	sql := `select /* pgwatch3_generated */
		quote_ident(datname)::text as datname_escaped
		from pg_database
		where not datistemplate
		and datallowconn
		and has_database_privilege (datname, 'CONNECT')
		and case when length(trim($1)) > 0 then datname ~ $1 else true end
		and case when length(trim($2)) > 0 then not datname ~ $2 else true end`

	if rows, err = c.Query(context.TODO(), sql, md.IncludePattern, md.ExcludePattern); err != nil {
		return nil, err
	}
	for rows.Next() {
		if err = rows.Scan(&dbname); err != nil {
			return nil, err
		}
		rdb := md
		rdb.DBUniqueName += "_" + dbname
		resolvedDbs = append(resolvedDbs, rdb)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return
}

type MonitoredDatabases []MonitoredDatabase

// Expand() updates list of monitored objects from continuous monitoring sources, e.g. patroni
func (mds MonitoredDatabases) Expand() (MonitoredDatabases, error) {
	resolvedDbs := make(MonitoredDatabases, 0, len(mds))
	for _, md := range mds {
		if !md.IsEnabled {
			continue
		}
		dbs, err := md.ExpandDatabases()
		if err != nil {
			return nil, err
		}
		if len(dbs) == 0 {
			resolvedDbs = append(resolvedDbs, md)
			continue
		}
		resolvedDbs = append(resolvedDbs, dbs...)
	}
	return resolvedDbs, nil
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
	GetMonitoredDatabases() (MonitoredDatabases, error)
}

type Writer interface {
	WriteMonitoredDatabases(MonitoredDatabases) error
	DeleteDatabase(string) error
	UpdateDatabase(md MonitoredDatabase) error
}

type ReaderWriter interface {
	Reader
	Writer
}
