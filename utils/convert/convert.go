package main

import (
	"io/fs"
	"math"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"flag"
	"fmt"

	"gopkg.in/yaml.v3"
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
		IsInstanceLevel           bool                 `yaml:"is_instance_level,omitempty"`
		MetricStorageName         string               `yaml:"metric_storage_name,omitempty"`
		ExtensionVersionOverrides []ExtensionOverrides `yaml:"extension_version_based_overrides,omitempty"`
		IsPrivate                 bool                 `yaml:"is_private,omitempty"`                // used only for extension overrides currently and ignored otherwise
		DisabledDays              string               `yaml:"disabled_days,omitempty"`             // Cron style, 0 = Sunday. Ranges allowed: 0,2-4
		DisableTimes              []string             `yaml:"disabled_times,omitempty"`            // "11:00-13:00"
		StatementTimeoutSeconds   int64                `yaml:"statement_timeout_seconds,omitempty"` // overrides per monitored DB settings
	}

	SQLs map[int]string

	Metric struct {
		SQLs        SQLs
		InitSQL     string   `yaml:"init_sql,omitempty"`
		NodeStatus  string   `yaml:"node_status,omitempty"`
		Gauges      []string `yaml:",omitempty"`
		MetricAttrs `yaml:",inline,omitempty"`
	}

	Metrics map[string]Metric
)

const (
	FileBasedMetricHelpersDir = "00_helpers"
	PresetConfigYAMLFile      = "preset-configs.yaml"
)

// VersionToInt parses a given version and returns an integer  or
// an error if unable to parse the version. Only parses valid semantic versions.
// Performs checking that can find errors within the version.
// Examples: v1.2 -> 01_02_00, v9.6.3 -> 09_06_03, v11 -> 11_00_00
var regVer = regexp.MustCompile(`(\d+).?(\d*).?(\d*)`)

func VersionToInt(version string) (v int) {
	if matches := regVer.FindStringSubmatch(version); len(matches) > 1 {
		for i, match := range matches[1:] {
			v += func() (m int) { m, _ = strconv.Atoi(match); return }() * int(math.Pow10(4-i*2))
		}
	}
	return
}

// expected is following structure: metric_name/pg_ver/metric(_master|standby).sql
func ReadMetricsFromFolder(folder string) (metricsMap Metrics, err error) {
	metricFolders, err := os.ReadDir(folder)
	if err != nil {
		return
	}

	metricsMap = make(map[string]Metric)
	metricNamePattern := `^[a-z0-9_\.]+$`
	regexMetricNameFilter := regexp.MustCompile(metricNamePattern)
	regexIsDigitOrPunctuation := regexp.MustCompile(`^[\d\.]+$`)

	fmt.Printf("Searching for metrics from path %s ...\n", folder)

	for _, metricFolder := range metricFolders {
		if metricFolder.Name() == FileBasedMetricHelpersDir ||
			!metricFolder.IsDir() ||
			!regexMetricNameFilter.MatchString(metricFolder.Name()) {
			continue
		}

		var versionFolders []fs.DirEntry
		versionFolders, err = os.ReadDir(path.Join(folder, metricFolder.Name()))
		if err != nil {
			return
		}

		Metric := Metric{}
		Metric.MetricAttrs, _ = ParseMetricAttrsFromYAML(path.Join(folder, metricFolder.Name(), "metric_attrs.yaml"))
		_ = ParseMetricPrometheusAttrsFromYAML(path.Join(folder, metricFolder.Name(), "column_attrs.yaml"), &Metric)
		Metric.SQLs = make(SQLs)

		var version int
		for _, versionFolder := range versionFolders {
			if strings.HasSuffix(versionFolder.Name(), ".md") || versionFolder.Name() == "column_attrs.yaml" || versionFolder.Name() == "metric_attrs.yaml" {
				continue
			}
			if !regexIsDigitOrPunctuation.MatchString(versionFolder.Name()) {
				fmt.Printf("Invalid metric structure - version folder names should consist of only numerics/dots, found: %s", versionFolder.Name())
				continue
			}
			if version, err = strconv.Atoi(versionFolder.Name()); err != nil {
				version = 11 // the oldest supported
			}

			var metricDefs []fs.DirEntry
			if metricDefs, err = os.ReadDir(path.Join(folder, metricFolder.Name(), versionFolder.Name())); err != nil {
				return
			}

			for _, metricDef := range metricDefs {
				if strings.HasPrefix(metricDef.Name(), "metric") && strings.HasSuffix(metricDef.Name(), ".sql") {
					p := path.Join(folder, metricFolder.Name(), versionFolder.Name(), metricDef.Name())
					metricSQL, err := os.ReadFile(p)
					if err != nil {
						continue
					}
					switch {
					case strings.Contains(metricDef.Name(), "_master"):
						Metric.NodeStatus = "primary"
					case strings.Contains(metricDef.Name(), "_standby"):
						Metric.NodeStatus = "standby"
					}
					Metric.SQLs[version] = strings.TrimRight(strings.TrimSpace(string(metricSQL)), ";")
				}
			}
		}
		metricsMap[metricFolder.Name()] = Metric
	}
	return
}

func ParseMetricPrometheusAttrsFromYAML(path string, m *Metric) (err error) {
	type OldMetricPrometheusAttrs struct {
		PrometheusGaugeColumns    []string `yaml:"prometheus_gauge_columns,omitempty"`
		PrometheusIgnoredColumns  []string `yaml:"prometheus_ignored_columns,omitempty"` // for cases where we don't want some columns to be exposed in Prom mode
		PrometheusAllGaugeColumns bool     `yaml:"prometheus_all_gauge_columns,omitempty"`
	}

	var val []byte
	var oldPromAttrs OldMetricPrometheusAttrs
	if val, err = os.ReadFile(path); err == nil {
		if err = yaml.Unmarshal(val, &oldPromAttrs); err != nil {
			return
		}
		if oldPromAttrs.PrometheusAllGaugeColumns {
			m.Gauges = []string{"*"}
			return
		}
		m.Gauges = slices.Clone(oldPromAttrs.PrometheusGaugeColumns)
	}

	return
}

func ParseMetricAttrsFromYAML(path string) (a MetricAttrs, err error) {
	var val []byte
	if val, err = os.ReadFile(path); err == nil {
		err = yaml.Unmarshal(val, &a)
	}
	return
}

type Presets map[string]Preset

type Preset struct {
	Name        string `yaml:"name,omitempty"`
	Description string
	Metrics     map[string]int
}

// Expects "preset metrics" definition file named preset-config.yaml to be present in provided --metrics folder
func ReadPresetsFromFolder(folder string) (presets Presets, err error) {
	var presetMetrics []byte
	fmt.Printf("Searching for presents from path %s ...\n", folder)
	if presetMetrics, err = os.ReadFile(path.Join(folder, PresetConfigYAMLFile)); err != nil {
		return
	}
	var oldPresets []Preset
	if err = yaml.Unmarshal(presetMetrics, &oldPresets); err != nil {
		return
	}
	presets = make(Presets, 0)
	for _, p := range oldPresets {
		pname := p.Name
		p.Name = ""
		presets[pname] = p
	}
	return
}

func WriteMetricsToFile(metricDefs any, filename string) error {
	yamlData, err := yaml.Marshal(metricDefs)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, yamlData, 0644)
}

func moveHelpersToMetrics(helpers, metrics Metrics) {
new_helper:
	for helperName, h := range helpers {
		for metricName, m := range metrics {
			for _, sql := range m.SQLs {
				if strings.Contains(sql, helperName) {
					for _, v := range h.SQLs {
						m.InitSQL = v
						metrics[metricName] = m
						continue new_helper
					}
				}
			}
		}
	}
}

func main() {
	// Define command-line flags
	src := flag.String("src", "", "pgwatch v2 metric folder, e.g. `./metrics/sql`")
	dst := flag.String("dst", "", "pgwatch v3 output metric file, e.g. `metrics.yaml`")

	// Parse command-line flags
	flag.Parse()

	// Check if src flag is provided
	if *src == "" {
		fmt.Println("Error: src option is required")
		return
	}

	// Check if dst flag is provided
	if *dst == "" {
		fmt.Println("Error: dst option is required")
		return
	}
	helpers, err := ReadMetricsFromFolder(path.Join(*src, FileBasedMetricHelpersDir))
	if err != nil {
		panic(err)
	}
	metrics, err := ReadMetricsFromFolder(*src)
	if err != nil {
		panic(err)
	}
	moveHelpersToMetrics(helpers, metrics)
	presets, err := ReadPresetsFromFolder(*src)
	if err != nil {
		panic(err)
	}
	err = WriteMetricsToFile(struct {
		Metrics Metrics `yaml:"metrics,omitempty"`
		Presets Presets `yaml:"presets,omitempty"`
	}{
		metrics,
		presets,
	}, *dst)
	if err != nil {
		panic(err)
	}
}
