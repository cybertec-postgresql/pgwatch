package metrics

import (
	"time"
)

type MetricPrometheusAttrs struct {
	PrometheusGaugeColumns    []string `yaml:"prometheus_gauge_columns"`
	PrometheusIgnoredColumns  []string `yaml:"prometheus_ignored_columns"` // for cases where we don't want some columns to be exposed in Prom mode
	PrometheusAllGaugeColumns bool     `yaml:"prometheus_all_gauge_columns"`
}

type ExtensionInfo struct {
	ExtName       string `yaml:"ext_name"`
	ExtMinVersion string `yaml:"ext_min_version"`
}

type ExtensionOverrides struct {
	TargetMetric              string          `yaml:"target_metric"`
	ExpectedExtensionVersions []ExtensionInfo `yaml:"expected_extension_versions"`
}

type MetricAttrs struct {
	IsInstanceLevel           bool                 `yaml:"is_instance_level"`
	MetricStorageName         string               `yaml:"metric_storage_name"`
	ExtensionVersionOverrides []ExtensionOverrides `yaml:"extension_version_based_overrides"`
	IsPrivate                 bool                 `yaml:"is_private"`                // used only for extension overrides currently and ignored otherwise
	DisabledDays              string               `yaml:"disabled_days"`             // Cron style, 0 = Sunday. Ranges allowed: 0,2-4
	DisableTimes              []string             `yaml:"disabled_times"`            // "11:00-13:00"
	StatementTimeoutSeconds   int64                `yaml:"statement_timeout_seconds"` // overrides per monitored DB settings
}

type MetricProperties struct {
	SQL                  string
	SQLSU                string
	MasterOnly           bool
	StandbyOnly          bool
	PrometheusAttrs      MetricPrometheusAttrs // Prometheus Metric Type (Counter is default) and ignore list
	MetricAttrs          MetricAttrs
	CallsHelperFunctions bool
}

type MetricEntry map[string]any
type MetricData []map[string]any

type MetricStoreMessage struct {
	DBName                  string
	DBType                  string
	MetricName              string
	CustomTags              map[string]string
	Data                    MetricData
	MetricDefinitionDetails MetricProperties
	RealDbname              string
	SystemIdentifier        string
}

type MetricStoreMessagePostgres struct {
	Time    time.Time
	DBName  string
	Metric  string
	Data    map[string]any
	TagData map[string]any
}

const (
	FileBasedMetricHelpersDir = "00_helpers"
)
