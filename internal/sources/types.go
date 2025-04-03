package sources

import (
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
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
	return slices.Contains(Kinds, k)
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
		IsEnabled            bool               `yaml:"is_enabled" db:"is_enabled"`
		CustomTags           map[string]string  `yaml:"custom_tags" db:"custom_tags"`
		HostConfig           HostConfigAttrs    `yaml:"host_config" db:"host_config"`
		OnlyIfMaster         bool               `yaml:"only_if_master" db:"only_if_master"`
	}

	Sources []Source
)

func (srcs Sources) Validate() (Sources, error) {
	names := map[string]any{}
	for _, src := range srcs {
		if _, ok := names[src.Name]; ok {
			return nil, fmt.Errorf("duplicate source with name '%s' found", src.Name)
		}
		names[src.Name] = nil
		if src.Kind == "" {
			src.Kind = SourcePostgres
		}
	}
	return srcs, nil
}

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
