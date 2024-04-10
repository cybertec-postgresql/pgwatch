package reaper

import (
	"context"
	"regexp"
	"strings"
	"time"

	"errors"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

const (
	recoPrefix                        = "reco_" // special handling for metrics with such prefix, data stored in RECO_METRIC_NAME
	recoMetricName                    = "recommendations"
	specialMetricChangeEvents         = "change_events"
	specialMetricServerLogEventCounts = "server_log_event_counts"
	specialMetricPgbouncer            = "^pgbouncer_(stats|pools)$"
	specialMetricPgpoolStats          = "pgpool_stats"
	specialMetricInstanceUp           = "instance_up"
	specialMetricDbSize               = "db_size"     // can be transparently switched to db_size_approx on instances with very slow FS access (Azure Single Server)
	specialMetricTableStats           = "table_stats" // can be transparently switched to table_stats_approx on instances with very slow FS (Azure Single Server)

)

var specialMetrics = map[string]bool{recoMetricName: true, specialMetricChangeEvents: true, specialMetricServerLogEventCounts: true}
var regexIsPgbouncerMetrics = regexp.MustCompile(specialMetricPgbouncer)

func GetAllRecoMetricsForVersion(vme DBVersionMapEntry) (map[string]metrics.Metric, error) {
	mvpMap := make(map[string]metrics.Metric)
	metricDefMapLock.RLock()
	defer metricDefMapLock.RUnlock()
	for m := range metricDefinitionMap.MetricDefs {
		if strings.HasPrefix(m, recoPrefix) {
			mvp, err := GetMetricVersionProperties(m, vme, &metricDefinitionMap)
			if err != nil {
				return nil, err
			}
			mvpMap[m] = mvp
		}
	}
	return mvpMap, nil
}

func GetRecommendations(ctx context.Context, dbUnique string, vme DBVersionMapEntry) (metrics.Measurements, error) {
	retData := make(metrics.Measurements, 0)
	startTimeEpochNs := time.Now().UnixNano()

	recoMetrics, err := GetAllRecoMetricsForVersion(vme)
	if err != nil {
		return nil, err
	}
	for _, mvp := range recoMetrics {
		data, e := DBExecReadByDbUniqueName(ctx, dbUnique, mvp.GetSQL(vme.Version))
		if err != nil {
			err = errors.Join(err, e)
			continue
		}
		for _, d := range data {
			d[epochColumnName] = startTimeEpochNs
			d["major_ver"] = vme.Version / 10
			retData = append(retData, d)
		}
	}
	if len(retData) == 0 { // insert a dummy entry minimally so that Grafana can show at least a dropdown
		dummy := make(metrics.Measurement)
		dummy["tag_reco_topic"] = "dummy"
		dummy["tag_object_name"] = "-"
		dummy["recommendation"] = "no recommendations"
		dummy[epochColumnName] = startTimeEpochNs
		dummy["major_ver"] = vme.Version / 10
		retData = append(retData, dummy)
	}
	return retData, err
}
