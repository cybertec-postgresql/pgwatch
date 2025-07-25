package sources

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"
)

var ErrSourceNotFound = errors.New("source not found")
var ErrSourceExists = errors.New("source already exists")

type Kind string

const (
	SourcePostgres           Kind = "postgres"
	SourcePostgresContinuous Kind = "postgres-continuous-discovery"
	SourcePgBouncer          Kind = "pgbouncer"
	SourcePgPool             Kind = "pgpool"
	SourcePatroni            Kind = "patroni"
)

var Kinds = []Kind{
	SourcePostgres,
	SourcePostgresContinuous,
	SourcePgBouncer,
	SourcePgPool,
	SourcePatroni,
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
		OnlyIfMaster         bool               `yaml:"only_if_master" db:"only_if_master"`
	}

	Sources []Source
)

func (s Source) IsDefaultGroup() bool {
	return s.Group == "" || s.Group == "default"
}

func (srcs Sources) Validate() (Sources, error) {
	names := map[string]any{}
	for _, src := range srcs {
		if _, ok := names[src.Name]; ok {
			return nil, fmt.Errorf("duplicate source with name '%s' found", src.Name)
		}
		names[src.Name] = nil
		switch src.Kind {
		case "":
			src.Kind = SourcePostgres
		case "patroni-continuous-discovery", "patroni-namespace-discovery":
			// deprecated, use SourcePatroni
			src.Kind = SourcePatroni
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

func (s Source) Equal(s2 Source) bool {
	var eq bool
	if s.PresetMetrics != "" || s2.PresetMetrics != "" {
		eq = (s.PresetMetrics == s2.PresetMetrics)
	} else {
		eq = reflect.DeepEqual(s.Metrics, s2.Metrics)
	}

	if s.PresetMetricsStandby != "" || s2.PresetMetricsStandby != "" {
		eq = eq && (s.PresetMetricsStandby == s2.PresetMetricsStandby)
	} else {
		eq = eq && reflect.DeepEqual(s.MetricsStandby, s2.MetricsStandby)
	}

	return eq &&
		s.Name == s2.Name &&
		s.Group == s2.Group &&
		s.ConnStr == s2.ConnStr &&
		s.Kind == s2.Kind &&
		s.IsEnabled == s2.IsEnabled &&
		s.IncludePattern == s2.IncludePattern &&
		s.ExcludePattern == s2.ExcludePattern &&
		s.OnlyIfMaster == s2.OnlyIfMaster &&
		reflect.DeepEqual(s.CustomTags, s2.CustomTags)
}

func (s *Source) Clone() *Source {
	c := new(Source)
	*c = *s
	c.Metrics = maps.Clone(s.Metrics)
	c.MetricsStandby = maps.Clone(s.MetricsStandby)
	c.CustomTags = maps.Clone(s.CustomTags)
	return c
}

type Reader interface {
	GetSources() (Sources, error)
}

type Writer interface {
	WriteSources(Sources) error
	DeleteSource(string) error
	UpdateSource(md Source) error
	CreateSource(md Source) error
}

type ReaderWriter interface {
	Reader
	Writer
}
