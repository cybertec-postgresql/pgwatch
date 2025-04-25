package metrics

import (
	"maps"
	"time"

	"github.com/jackc/pgx/v5"
)

type (
	ExtensionInfo struct {
		ExtName       string `yaml:"ext_name"`
		ExtMinVersion string `yaml:"ext_min_version"`
	}

	ExtensionOverrides struct {
		TargetMetric              string          `yaml:"target_metric"`
		ExpectedExtensionVersions []ExtensionInfo `yaml:"expected_extension_versions"`
	}

	MetricAttrs struct {
		ExtensionVersionOverrides []ExtensionOverrides `yaml:"extension_version_based_overrides,omitempty"`
		IsPrivate                 bool                 `yaml:"is_private,omitempty"`                // used only for extension overrides currently and ignored otherwise
		DisabledDays              string               `yaml:"disabled_days,omitempty"`             // Cron style, 0 = Sunday. Ranges allowed: 0,2-4
		DisableTimes              []string             `yaml:"disabled_times,omitempty"`            // "11:00-13:00"
		StatementTimeoutSeconds   int64                `yaml:"statement_timeout_seconds,omitempty"` // overrides per monitored DB settings
	}

	SQLs map[int]string

	Metric struct {
		SQLs            SQLs
		InitSQL         string   `yaml:"init_sql,omitempty"`
		NodeStatus      string   `yaml:"node_status,omitempty"`
		Gauges          []string `yaml:",omitempty"`
		IsInstanceLevel bool     `yaml:"is_instance_level,omitempty"`
		StorageName     string   `yaml:"storage_name,omitempty"`
		Description     string   `yaml:"description,omitempty"`
	}

	MetricDefs map[string]Metric
)

func (m Metric) PrimaryOnly() bool {
	return m.NodeStatus == "primary"
}

func (m Metric) StandbyOnly() bool {
	return m.NodeStatus == "standby"
}

func (m Metric) GetSQL(version int) string {
	// Check if there's an exact match for i
	if val, ok := m.SQLs[version]; ok {
		return val
	}

	// Find the closest value less than version
	var closestVersion int
	for v := range m.SQLs {
		if v < version && (closestVersion == 0 || v > closestVersion) {
			closestVersion = v
		}
	}
	return m.SQLs[closestVersion]
}

type PresetDefs map[string]Preset

type Preset struct {
	Description string
	Metrics     map[string]float64
}

const (
	EpochColumnName string = "epoch_ns" // this column (epoch in nanoseconds) is expected in every metric query
	TagPrefix       string = "tag_"
)

type Measurement map[string]any

// RowToMap returns a map scanned from row.
func RowToMeasurement(row pgx.CollectableRow) (map[string]any, error) {
	value := NewMeasurement(time.Now().UnixNano())
	err := row.Scan((*Measurement)(&value))
	return value, err
}

func (rs *Measurement) ScanRow(rows pgx.Rows) error {
	values, err := rows.Values()
	if err != nil {
		return err
	}
	// *rs = make(Measurement, len(values))
	for i := range values {
		(*rs)[string(rows.FieldDescriptions()[i].Name)] = values[i]
	}
	return nil
}

func NewMeasurement(epoch int64) Measurement {
	m := make(Measurement)
	m[EpochColumnName] = epoch
	return m
}

func (m Measurement) GetEpoch() int64 {
	if v, ok := m[EpochColumnName]; ok {
		if epoch, ok := v.(int64); ok {
			return epoch
		}
	}
	return time.Now().UnixNano()
}

type Measurements []map[string]any

func (m Measurements) GetEpoch() int64 {
	if len(m) == 0 {
		return time.Now().UnixNano()
	}
	return Measurement(m[0]).GetEpoch()
}

func (m Measurements) IsEpochSet() bool {
	if len(m) == 0 {
		return false
	}
	_, ok := m[0][EpochColumnName]
	return ok
}

func (m Measurements) DeepCopy() Measurements {
	newData := make(Measurements, len(m))
	for i, dr := range m {
		newData[i] = maps.Clone(dr)
	}
	return newData
}

// Touch updates the last modified time of the metric definitions
func (m Measurements) Touch() {
	ns := time.Now().UnixNano()
	for _, measurement := range m {
		measurement[EpochColumnName] = ns
	}
}

type MeasurementEnvelope struct {
	DBName           string
	SourceType       string
	MetricName       string
	CustomTags       map[string]string
	Data             Measurements
	MetricDef        Metric
	RealDbname       string
	SystemIdentifier string
}

type Metrics struct {
	MetricDefs MetricDefs `yaml:"metrics"`
	PresetDefs PresetDefs `yaml:"presets"`
}
type Reader interface {
	GetMetrics() (*Metrics, error)
}

type Writer interface {
	WriteMetrics(metricDefs *Metrics) error
	DeleteMetric(metricName string) error
	DeletePreset(presetName string) error
	UpdateMetric(metricName string, metric Metric) error
	UpdatePreset(presetName string, preset Preset) error
}

type Migrator interface {
	Migrate() error
	NeedsMigration() (bool, error)
}

type ReaderWriter interface {
	Reader
	Writer
}
