package metrics

import (
	"fmt"
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

func (m *Measurement) ScanRow(rows pgx.Rows) error {
	values, err := rows.Values()
	if err != nil {
		return err
	}
	// *rs = make(Measurement, len(values))
	for i := range values {
		(*m)[string(rows.FieldDescriptions()[i].Name)] = values[i]
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
	DBName     string
	MetricName string
	CustomTags map[string]string
	Data       Measurements
}

type Metrics struct {
	MetricDefs MetricDefs `yaml:"metrics,omitempty"`
	PresetDefs PresetDefs `yaml:"presets,omitempty"`
}

// FilterByNames returns a new Metrics struct containing only the specified metrics and/or presets.
// When a preset is requested, it includes both the preset definition and all its metrics.
// If names is empty, returns a full copy of all metrics and presets.
// Returns an error if any name is not found.
func (m *Metrics) FilterByNames(names []string) (*Metrics, error) {
	result := &Metrics{
		MetricDefs: make(MetricDefs),
		PresetDefs: make(PresetDefs),
	}

	// If no names provided, return full copy
	if len(names) == 0 {
		maps.Copy(result.MetricDefs, m.MetricDefs)
		maps.Copy(result.PresetDefs, m.PresetDefs)
		return result, nil
	}

	for _, name := range names {
		if preset, ok := m.PresetDefs[name]; ok {
			result.PresetDefs[name] = preset
			// Include all metrics from the preset
			for metricName := range preset.Metrics {
				if metric, exists := m.MetricDefs[metricName]; exists {
					result.MetricDefs[metricName] = metric
				}
			}
		} else if metric, ok := m.MetricDefs[name]; ok {
			result.MetricDefs[name] = metric
		} else {
			return nil, fmt.Errorf("metric or preset '%s' not found", name)
		}
	}

	return result, nil
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
	CreateMetric(metricName string, metric Metric) error
	CreatePreset(presetName string, preset Preset) error
}

type ReaderWriter interface {
	Reader
	Writer
}
