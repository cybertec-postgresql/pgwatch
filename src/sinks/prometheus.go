package sinks

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/log"
	"github.com/cybertec-postgresql/pgwatch/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusWriter is a sink that allows to expose metric measurements to Prometheus scrapper.
// Prometheus collects metrics data from pgwatch3 by scraping metrics HTTP endpoints.
type PrometheusWriter struct {
	ctx                               context.Context
	lastScrapeErrors                  prometheus.Gauge
	totalScrapes, totalScrapeFailures prometheus.Counter
	PrometheusNamespace               string
}

const promInstanceUpStateMetric = "instance_up"

// timestamps older than that will be ignored on the Prom scraper side anyways, so better don't emit at all and just log a notice
const promScrapingStalenessHardDropLimit = time.Minute * time.Duration(10)

func NewPrometheusWriter(ctx context.Context, connstr string) (promw *PrometheusWriter, err error) {
	addr, namespace, found := strings.Cut(connstr, "/")
	if !found {
		namespace = "pgwatch3"
	}
	promw = &PrometheusWriter{
		ctx:                 ctx,
		PrometheusNamespace: namespace,
		lastScrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "exporter_last_scrape_errors",
			Help:      "Last scrape error count for all monitored hosts / metrics",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_total_scrapes",
			Help:      "Total scrape attempts.",
		}),
		totalScrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_total_scrape_failures",
			Help:      "Number of errors while executing metric queries",
		}),
	}

	if err = prometheus.Register(promw); err != nil {
		return
	}
	promServer := &http.Server{
		Addr:    addr,
		Handler: promhttp.Handler(),
	}
	go func() {
		log.GetLogger(ctx).Error(promServer.ListenAndServe())
	}()
	return
}

func (promw *PrometheusWriter) Write(msgs []metrics.MeasurementMessage) error {
	if len(msgs) == 0 || len(msgs[0].Data) == 0 { // no batching in async prom mode, so using 0 indexing ok
		return nil
	}
	msg := msgs[0]
	promw.PromAsyncCacheAddMetricData(msg.DBName, msg.MetricName, msgs)
	return nil
}

// Async Prom cache
var promAsyncMetricCache = make(map[string]map[string][]metrics.MeasurementMessage) // [dbUnique][metric]lastly_fetched_data
var promAsyncMetricCacheLock = sync.RWMutex{}

func (promw *PrometheusWriter) PromAsyncCacheAddMetricData(dbUnique, metric string, msgArr []metrics.MeasurementMessage) { // cache structure: [dbUnique][metric]lastly_fetched_data
	promAsyncMetricCacheLock.Lock()
	defer promAsyncMetricCacheLock.Unlock()
	if _, ok := promAsyncMetricCache[dbUnique]; ok {
		promAsyncMetricCache[dbUnique][metric] = msgArr
	}
}

func (promw *PrometheusWriter) PromAsyncCacheInitIfRequired(dbUnique, _ string) { // cache structure: [dbUnique][metric]lastly_fetched_data
	promAsyncMetricCacheLock.Lock()
	defer promAsyncMetricCacheLock.Unlock()
	if _, ok := promAsyncMetricCache[dbUnique]; !ok {
		metricMap := make(map[string][]metrics.MeasurementMessage)
		promAsyncMetricCache[dbUnique] = metricMap
	}
}

func (promw *PrometheusWriter) PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, metric string) {
	promAsyncMetricCacheLock.Lock()
	defer promAsyncMetricCacheLock.Unlock()

	if metric == "" {
		delete(promAsyncMetricCache, dbUnique) // whole host removed from config
	} else {
		delete(promAsyncMetricCache[dbUnique], metric)
	}
}

func (promw *PrometheusWriter) SyncMetric(dbUnique, metricName, op string) error {
	switch op {
	case "remove":
		promw.PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, metricName)
	case "add":
		promw.PromAsyncCacheInitIfRequired(dbUnique, metricName)
	}
	return nil
}

func (promw *PrometheusWriter) Describe(_ chan<- *prometheus.Desc) {
}

func (promw *PrometheusWriter) Collect(ch chan<- prometheus.Metric) {
	var lastScrapeErrors float64
	logger := log.GetLogger(promw.ctx)
	promw.totalScrapes.Add(1)
	ch <- promw.totalScrapes

	if len(promAsyncMetricCache) == 0 {
		logger.Warning("No dbs configured for monitoring. Check config")
		ch <- promw.totalScrapeFailures
		promw.lastScrapeErrors.Set(0)
		ch <- promw.lastScrapeErrors
		return
	}

	for dbname, metricsMessages := range promAsyncMetricCache {
		promw.setInstanceUpDownState(ch, dbname)
		for metric, metricMessages := range metricsMessages {
			if metric == "change_events" {
				continue // not supported
			}
			if len(metricMessages) > 0 {
				promMetrics := promw.MetricStoreMessageToPromMetrics(metricMessages[0])
				for _, pm := range promMetrics { // collect & send later in batch? capMetricChan = 1000 limit in prometheus code
					ch <- pm
				}
			}
		}

	}

	ch <- promw.totalScrapeFailures
	promw.lastScrapeErrors.Set(lastScrapeErrors)
	ch <- promw.lastScrapeErrors

	// atomic.StoreInt64(&lastSuccessfulDatastoreWriteTimeEpoch, time.Now().Unix())
}

func (promw *PrometheusWriter) setInstanceUpDownState(ch chan<- prometheus.Metric, dbName string) {
	logger := log.GetLogger(promw.ctx)
	data := make(metrics.Measurement)
	data[promInstanceUpStateMetric] = 1
	data[epochColumnName] = time.Now().UnixNano()

	pm := promw.MetricStoreMessageToPromMetrics(metrics.MeasurementMessage{
		DBName:           dbName,
		SourceType:       "postgres",
		MetricName:       promInstanceUpStateMetric,
		CustomTags:       nil, //md.CustomTags,
		Data:             metrics.Measurements{data},
		RealDbname:       dbName, //vme.RealDbname,
		SystemIdentifier: dbName, //vme.SystemIdentifier,
	})

	if len(pm) > 0 {
		ch <- pm[0]
	} else {
		logger.Errorf("Could not formulate an instance state report - should not happen")
	}
}

func (promw *PrometheusWriter) MetricStoreMessageToPromMetrics(msg metrics.MeasurementMessage) []prometheus.Metric {
	promMetrics := make([]prometheus.Metric, 0)
	logger := log.GetLogger(promw.ctx)
	var epochTime time.Time
	var epochNs int64
	var epochNow = time.Now()

	if len(msg.Data) == 0 {
		return promMetrics
	}

	epochNs, ok := (msg.Data[0][epochColumnName]).(int64)
	if !ok {
		if msg.MetricName != "pgbouncer_stats" {
			logger.Warning("No timestamp_ns found, (gatherer) server time will be used. measurement:", msg.MetricName)
		}
		epochTime = time.Now()
	} else {
		epochTime = time.Unix(0, epochNs)

		if epochTime.Before(epochNow.Add(-1 * promScrapingStalenessHardDropLimit)) {
			logger.Warningf("[%s][%s] Dropping metric set due to staleness (>%v) ...", msg.DBName, msg.MetricName, promScrapingStalenessHardDropLimit)
			promw.PurgeMetricsFromPromAsyncCacheIfAny(msg.DBName, msg.MetricName)
			return promMetrics
		}
	}

	for _, dr := range msg.Data {
		labels := make(map[string]string)
		fields := make(map[string]float64)
		labels["dbname"] = msg.DBName

		for k, v := range dr {
			if v == nil || v == "" || k == epochColumnName {
				continue // not storing NULLs. epoch checked/assigned once
			}

			if strings.HasPrefix(k, "tag_") {
				tag := k[4:]
				labels[tag] = fmt.Sprintf("%v", v)
			} else {
				dataType := reflect.TypeOf(v).String()
				if dataType == "float64" || dataType == "float32" || dataType == "int64" || dataType == "int32" || dataType == "int" {
					f, err := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
					if err != nil {
						logger.Debugf("Skipping scraping column %s of [%s:%s]: %v", k, msg.DBName, msg.MetricName, err)
					}
					fields[k] = f
				} else if dataType == "bool" {
					if v.(bool) {
						fields[k] = 1
					} else {
						fields[k] = 0
					}
				} else {
					logger.Debugf("Skipping scraping column %s of [%s:%s], unsupported datatype: %s", k, msg.DBName, msg.MetricName, dataType)
					continue
				}
			}
		}
		if msg.CustomTags != nil {
			for k, v := range msg.CustomTags {
				labels[k] = fmt.Sprintf("%v", v)
			}
		}

		labelKeys := make([]string, 0)
		labelValues := make([]string, 0)
		for k, v := range labels {
			labelKeys = append(labelKeys, k)
			labelValues = append(labelValues, v)
		}

		for field, value := range fields {
			fieldPromDataType := prometheus.CounterValue
			if msg.MetricName == promInstanceUpStateMetric ||
				len(msg.MetricDef.Gauges) > 0 &&
					(msg.MetricDef.Gauges[0] == "*" || slices.Contains(msg.MetricDef.Gauges, field)) {
				fieldPromDataType = prometheus.GaugeValue
			}
			var desc *prometheus.Desc
			if promw.PrometheusNamespace != "" {
				if msg.MetricName == promInstanceUpStateMetric { // handle the special "instance_up" check
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s", promw.PrometheusNamespace, msg.MetricName),
						msg.MetricName, labelKeys, nil)
				} else {
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s_%s", promw.PrometheusNamespace, msg.MetricName, field),
						msg.MetricName, labelKeys, nil)
				}
			} else {
				if msg.MetricName == promInstanceUpStateMetric { // handle the special "instance_up" check
					desc = prometheus.NewDesc(field, msg.MetricName, labelKeys, nil)
				} else {
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s", msg.MetricName, field), msg.MetricName, labelKeys, nil)
				}
			}
			m := prometheus.MustNewConstMetric(desc, fieldPromDataType, value, labelValues...)
			promMetrics = append(promMetrics, prometheus.NewMetricWithTimestamp(epochTime, m))
		}
	}
	return promMetrics
}
