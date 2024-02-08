package sources

import (
	"slices"

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
	DBUniqueName         string             `yaml:"unique_name" db:"md_name"`
	DBUniqueNameOrig     string             // to preserve belonging to a specific instance for continuous modes where DBUniqueName will be dynamic
	Group                string             `db:"md_group" db:"md_group"`
	Encryption           string             `yaml:"encryption" db:"md_encryption"`
	ConnStr              string             `yaml:"conn_str" db:"md_connstr"`
	Metrics              map[string]float64 `yaml:"custom_metrics" db:"md_config"`
	MetricsStandby       map[string]float64 `yaml:"custom_metrics_standby" db:"md_config_standby"`
	Kind                 Kind               `yaml:"kind" db:"md_dbtype"`
	IncludePattern       string             `yaml:"include_pattern" db:"md_include_pattern"`
	ExcludePattern       string             `yaml:"exclude_pattern" db:"md_exclude_pattern"`
	PresetMetrics        string             `yaml:"preset_metrics"`
	PresetMetricsStandby string             `yaml:"preset_metrics_standby"`
	IsSuperuser          bool               `yaml:"is_superuser" db:"md_is_superuser"`
	IsEnabled            bool               `yaml:"is_enabled"`
	CustomTags           map[string]string  `yaml:"custom_tags" db:"md_custom_tags"`
	HostConfig           HostConfigAttrs    `yaml:"host_config" db:"md_host_config"`
	OnlyIfMaster         bool               `yaml:"only_if_master" db:"md_only_if_master"`
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
		return ResolveDatabasesFromConfigEntry(md)
	}
	return nil, nil
}

type MonitoredDatabases []MonitoredDatabase

// Expand() updates list of monitored objects from continuous monitoring sources, e.g. patroni
func (mds MonitoredDatabases) Expand() (MonitoredDatabases, error) {
	resolvedDbs := make(MonitoredDatabases, len(mds))
	for _, md := range mds {
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
