package metrics

import (
	"context"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v2"
)

func versionToInt(v string) uint {
	if len(v) == 0 {
		return 0
	}
	var major int
	var minor int

	matches := regexp.MustCompile(`(\d+).?(\d*)`).FindStringSubmatch(v)
	if len(matches) == 0 {
		return 0
	}
	if len(matches) > 1 {
		major, _ = strconv.Atoi(matches[1])
		if len(matches) > 2 {
			minor, _ = strconv.Atoi(matches[2])
		}
	}
	return uint(major*100 + minor)
}

func ReadMetricsFromPostgres(ctx context.Context, conn db.PgxIface) (metricDefMapNew MetricVersionDefs, metricNameRemapsNew map[string]string, err error) {

	sql := `select /* pgwatch3_generated */ m_name, m_pg_version_from::text, m_sql, m_master_only, m_standby_only,
			  coalesce(m_column_attrs::text, '') as m_column_attrs, coalesce(m_column_attrs::text, '') as m_column_attrs,
			  coalesce(ma_metric_attrs::text, '') as ma_metric_attrs, m_sql_su
			from
              pgwatch3.metric
              left join
              pgwatch3.metric_attribute on (ma_metric_name = m_name)
			where
              m_is_active
		    order by
		      1, 2`
	logger := log.GetLogger(ctx)
	logger.Info("updating metrics definitons from ConfigDB...")

	var (
		data []map[string]any
		rows pgx.Rows
	)
	if rows, err = conn.Query(ctx, sql); err == nil {
		data, err = pgx.CollectRows(rows, pgx.RowToMap)
	}
	if err != nil {
		return
	}
	if len(data) == 0 {
		logger.Warning("no active metric definitions found from config DB")
		return
	}
	metricDefMapNew = make(MetricVersionDefs)
	metricNameRemapsNew = make(map[string]string)
	logger.WithField("metrics", len(data)).Debug("Active metrics found in config database (pgwatch3.metric)")
	for _, row := range data {
		_, ok := metricDefMapNew[row["m_name"].(string)]
		if !ok {
			metricDefMapNew[row["m_name"].(string)] = make(map[uint]MetricProperties)
		}
		d := versionToInt(row["m_pg_version_from"].(string))
		ca := MetricPrometheusAttrs{}
		if row["m_column_attrs"].(string) != "" {
			_ = yaml.Unmarshal([]byte(row["m_column_attrs"].(string)), &ca)
		}
		ma := MetricAttrs{}
		if row["ma_metric_attrs"].(string) != "" {
			_ = yaml.Unmarshal([]byte(row["ma_metric_attrs"].(string)), &ma)
			if ma.MetricStorageName != "" {
				metricNameRemapsNew[row["m_name"].(string)] = ma.MetricStorageName
			}
		}
		metricDefMapNew[row["m_name"].(string)][d] = MetricProperties{
			SQL:                  row["m_sql"].(string),
			SQLSU:                row["m_sql_su"].(string),
			MasterOnly:           row["m_master_only"].(bool),
			StandbyOnly:          row["m_standby_only"].(bool),
			PrometheusAttrs:      ca,
			MetricAttrs:          ma,
			CallsHelperFunctions: DoesMetricDefinitionCallHelperFunctions(row["m_sql"].(string)),
		}
	}
	return
}

// expected is following structure: metric_name/pg_ver/metric(_master|standby).sql
func ReadMetricsFromFolder(ctx context.Context, folder string) (
	metricsMap map[string]map[uint]MetricProperties,
	metricNameRemapsNew map[string]string,
	err error) {

	metricNamePattern := `^[a-z0-9_\.]+$`
	regexMetricNameFilter := regexp.MustCompile(metricNamePattern)
	regexIsDigitOrPunctuation := regexp.MustCompile(`^[\d\.]+$`)

	logger := log.GetLogger(ctx)
	logger.Infof("Searching for metrics from path %s ...", folder)
	metricFolders, err := os.ReadDir(folder)
	if err != nil {
		return
	}
	metricsMap = make(map[string]map[uint]MetricProperties)
	metricNameRemapsNew = make(map[string]string)

	for _, f := range metricFolders {
		if err = ctx.Err(); err != nil {
			return
		}
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
				if err != nil && MetricAttrs.MetricStorageName != "" {
					metricNameRemapsNew[f.Name()] = MetricAttrs.MetricStorageName
				}
			}

			var metricPrometheusAttrs MetricPrometheusAttrs
			if _, err = os.Stat(path.Join(folder, f.Name(), "column_attrs.yaml")); err == nil {
				if metricPrometheusAttrs, err = ParseMetricPrometheusAttrsFromYAML(path.Join(folder, f.Name(), "column_attrs.yaml")); err != nil {
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
						var mvp MetricProperties
						if !ok {
							metricsMap[f.Name()] = make(map[uint]MetricProperties)
						}
						mvp, ok = mvpVer[dirName]
						if !ok {
							mvp = MetricProperties{SQL: string(metricSQL[:]), PrometheusAttrs: metricPrometheusAttrs, MetricAttrs: MetricAttrs}
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

func ParseMetricPrometheusAttrsFromYAML(path string) (c MetricPrometheusAttrs, err error) {
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

// Expects "preset metrics" definition file named preset-config.yaml to be present in provided --metrics folder
func ReadPresetMetricsConfigFromFolder(folder string) (pmm map[string]map[string]float64, err error) {
	var (
		pcs           []PresetConfig
		presetMetrics []byte
	)
	if presetMetrics, err = os.ReadFile(path.Join(folder, PresetConfigYAMLFile)); err != nil {
		return
	}
	if err = yaml.Unmarshal(presetMetrics, &pcs); err != nil {
		return pmm, err
	}
	pmm = make(map[string]map[string]float64, len(pcs))
	for _, pc := range pcs {
		pmm[pc.Name] = pc.Metrics
	}
	return pmm, err
}
