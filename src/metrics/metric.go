package metrics

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/cybertec-postgresql/pgwatch3/log"
)

type MetricColumnAttrs struct {
	PrometheusGaugeColumns    []string `yaml:"prometheus_gauge_columns"`
	PrometheusIgnoredColumns  []string `yaml:"prometheus_ignored_columns"` // for cases where we don't want some columns to be exposed in Prom mode
	PrometheusAllGaugeColumns bool     `yaml:"prometheus_all_gauge_columns"`
}

type ExtensionOverrides struct {
	TargetMetric              string          `yaml:"target_metric"`
	ExpectedExtensionVersions []ExtensionInfo `yaml:"expected_extension_versions"`
}

type ExtensionInfo struct {
	ExtName       string `yaml:"ext_name"`
	ExtMinVersion string `yaml:"ext_min_version"`
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

type MetricVersionProperties struct {
	SQL                  string
	SQLSU                string
	MasterOnly           bool
	StandbyOnly          bool
	ColumnAttrs          MetricColumnAttrs // Prometheus Metric Type (Counter is default) and ignore list
	MetricAttrs          MetricAttrs
	CallsHelperFunctions bool
}

type MetricEntry map[string]any
type MetricData []map[string]any

type MetricStoreMessage struct {
	DBUniqueName            string
	DBType                  string
	MetricName              string
	CustomTags              map[string]string
	Data                    MetricData
	MetricDefinitionDetails MetricVersionProperties
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

// expected is following structure: metric_name/pg_ver/metric(_master|standby).sql
func ReadMetricsFromFolder(logger log.LoggerIface, folder string) (
	metricsMap map[string]map[uint]MetricVersionProperties,
	metricNameRemapsNew map[string]string,
	err error) {

	metricNamePattern := `^[a-z0-9_\.]+$`
	regexMetricNameFilter := regexp.MustCompile(metricNamePattern)
	regexIsDigitOrPunctuation := regexp.MustCompile(`^[\d\.]+$`)

	logger.Infof("Searching for metrics from path %s ...", folder)
	metricFolders, err := os.ReadDir(folder)
	if err != nil {
		return
	}

	for _, f := range metricFolders {
		if f.IsDir() {
			if f.Name() == FileBasedMetricHelpersDir {
				continue // helpers are pulled in when needed
			}
			if !regexMetricNameFilter.MatchString(f.Name()) {
				logger.Warningf("Ignoring metric '%s' as name not fitting pattern: %s", f.Name(), metricNamePattern)
				continue
			}
			//log.Debugf("Processing metric: %s", f.Name())
			var pgVers []fs.DirEntry
			pgVers, err = os.ReadDir(path.Join(folder, f.Name()))
			if err != nil {
				return
			}

			var MetricAttrs MetricAttrs
			if _, err = os.Stat(path.Join(folder, f.Name(), "metric_attrs.yaml")); err == nil {
				MetricAttrs, err = ParseMetricAttrsFromYAML(path.Join(folder, f.Name(), "metric_attrs.yaml"))
				if MetricAttrs.MetricStorageName != "" {
					metricNameRemapsNew[f.Name()] = MetricAttrs.MetricStorageName
				}
			}

			var metricColumnAttrs MetricColumnAttrs
			if _, err = os.Stat(path.Join(folder, f.Name(), "column_attrs.yaml")); err == nil {
				if metricColumnAttrs, err = ParseMetricColumnAttrsFromYAML(path.Join(folder, f.Name(), "column_attrs.yaml")); err != nil {
					return
				}
			}

			for _, pgVer := range pgVers {
				if strings.HasSuffix(pgVer.Name(), ".md") || pgVer.Name() == "column_attrs.yaml" || pgVer.Name() == "metric_attrs.yaml" {
					continue
				}
				if !regexIsDigitOrPunctuation.MatchString(pgVer.Name()) {
					logger.Warningf("Invalid metric structure - version folder names should consist of only numerics/dots, found: %s", pgVer.Name())
					continue
				}
				var dir int
				dir, err = strconv.Atoi(pgVer.Name())
				if err != nil {
					return
				}
				dirName := uint(dir)
				var metricDefs []fs.DirEntry
				if metricDefs, err = os.ReadDir(path.Join(folder, f.Name(), pgVer.Name())); err != nil {
					return
				}

				foundMetricDefFiles := make(map[string]bool) // to warn on accidental duplicates
				for _, md := range metricDefs {
					if strings.HasPrefix(md.Name(), "metric") && strings.HasSuffix(md.Name(), ".sql") {
						p := path.Join(folder, f.Name(), pgVer.Name(), md.Name())
						metricSQL, err := os.ReadFile(p)
						if err != nil {
							logger.Errorf("Failed to read metric definition at: %s", p)
							continue
						}
						_, exists := foundMetricDefFiles[md.Name()]
						if exists {
							logger.Warningf("Multiple definitions found for metric [%s:%s], using the last one (%s)...", f.Name(), pgVer.Name(), md.Name())
						}
						foundMetricDefFiles[md.Name()] = true

						//log.Debugf("Metric definition for \"%s\" ver %s: %s", f.Name(), pgVer.Name(), metric_sql)
						mvpVer, ok := metricsMap[f.Name()]
						var mvp MetricVersionProperties
						if !ok {
							metricsMap[f.Name()] = make(map[uint]MetricVersionProperties)
						}
						mvp, ok = mvpVer[dirName]
						if !ok {
							mvp = MetricVersionProperties{SQL: string(metricSQL[:]), ColumnAttrs: metricColumnAttrs, MetricAttrs: MetricAttrs}
						}
						mvp.CallsHelperFunctions = DoesMetricDefinitionCallHelperFunctions(mvp.SQL)
						if strings.Contains(md.Name(), "_master") {
							mvp.MasterOnly = true
						}
						if strings.Contains(md.Name(), "_standby") {
							mvp.StandbyOnly = true
						}
						if strings.Contains(md.Name(), "_su") {
							mvp.SQLSU = string(metricSQL[:])
						}
						metricsMap[f.Name()][dirName] = mvp
					}
				}
			}
		}
	}
	return
}

var regexSQLHelperFunctionCalled = regexp.MustCompile(`(?si)^\s*(select|with).*\s+get_\w+\(\)[\s,$]+`) // SQL helpers expected to follow get_smth() naming

func DoesMetricDefinitionCallHelperFunctions(sqlDefinition string) bool {
	return regexSQLHelperFunctionCalled.MatchString(sqlDefinition)
}

func ParseMetricColumnAttrsFromYAML(path string) (c MetricColumnAttrs, err error) {
	var val []byte
	if val, err = os.ReadFile(path); err == nil {
		err = yaml.Unmarshal(val, &c)
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
