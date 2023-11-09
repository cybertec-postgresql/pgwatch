package metrics

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/cybertec-postgresql/pgwatch3/log"
	"gopkg.in/yaml.v2"
)

// Writer is an interface that writes metrics values
type Writer interface {
	SyncMetric(dbUnique, metricName string) error
	Write(msgs []MetricStoreMessage) error
}

type MultiWriter struct {
	writers []Writer
	sync.Mutex
}

func (mw *MultiWriter) AddWriter(w Writer) {
	mw.Lock()
	mw.writers = append(mw.writers, w)
	mw.Unlock()
}

func (mw *MultiWriter) SyncMetrics(dbUnique, metricName string) (err error) {
	for _, w := range mw.writers {
		err = errors.Join(err, w.SyncMetric(dbUnique, metricName))
	}
	return
}

func (mw *MultiWriter) WriteMetrics(ctx context.Context, storageCh <-chan []MetricStoreMessage) {
	var err error
	logger := log.GetLogger(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-storageCh:
			for _, w := range mw.writers {
				err = w.Write(msg)
				if err != nil {
					logger.Error(err)
				}
			}
		}
	}
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
