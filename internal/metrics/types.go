package metrics

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

	Metrics struct {
		MetricDefs MetricDefs `yaml:"metrics"`
		PresetDefs PresetDefs `yaml:"presets"`
	}
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

type Measurement map[string]any
type Measurements []map[string]any

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
