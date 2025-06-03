package sinks

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusWriter is a sink that allows to expose metric measurements to Prometheus scrapper.
// Prometheus collects metrics data from pgwatch by scraping metrics HTTP endpoints.
type PrometheusWriter struct {
	ctx                               context.Context
	lastScrapeErrors                  prometheus.Gauge
	totalScrapes, totalScrapeFailures prometheus.Counter
	PrometheusNamespace               string
}

const promInstanceUpStateMetric = "instance_up"

// timestamps older than that will be ignored on the Prom scraper side anyway, so better don't emit at all and just log a notice
const promScrapingStalenessHardDropLimit = time.Minute * time.Duration(10)

func NewPrometheusWriter(ctx context.Context, connstr string) (promw *PrometheusWriter, err error) {
	addr, namespace, found := strings.Cut(connstr, "/")
	if !found {
		namespace = "pgwatch"
	}
	l := log.GetLogger(ctx).WithField("sink", "prometheus").WithField("address", addr)
	ctx = log.WithLogger(ctx, l)
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

	ln, err := net.Listen("tcp", promServer.Addr)
	if err != nil {
		return nil, err
	}

	go func() { log.GetLogger(ctx).Error(promServer.Serve(ln)) }()

	l.Info(`measurements sink is activated`)
	return
}

func (promw *PrometheusWriter) Write(msg metrics.MeasurementEnvelope) error {
	if len(msg.Data) == 0 { // no batching in async prom mode, so using 0 indexing ok
		return nil
	}
	promw.PromAsyncCacheAddMetricData(msg.DBName, msg.MetricName, msg)
	return nil
}

// Async Prom cache
var promAsyncMetricCache = make(map[string]map[string]metrics.MeasurementEnvelope) // [dbUnique][metric]lastly_fetched_data
var promAsyncMetricCacheLock = sync.RWMutex{}

func (promw *PrometheusWriter) PromAsyncCacheAddMetricData(dbUnique, metric string, msgArr metrics.MeasurementEnvelope) { // cache structure: [dbUnique][metric]lastly_fetched_data
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
		metricMap := make(map[string]metrics.MeasurementEnvelope)
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

func (promw *PrometheusWriter) SyncMetric(dbUnique, metricName string, op OpType) error {
	switch op {
	case DeleteOp:
		promw.PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, metricName)
	case AddOp:
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
			promMetrics := promw.MetricStoreMessageToPromMetrics(metricMessages)
			for _, pm := range promMetrics { // collect & send later in batch? capMetricChan = 1000 limit in prometheus code
				ch <- pm
			}
		}

	}

	ch <- promw.totalScrapeFailures
	promw.lastScrapeErrors.Set(lastScrapeErrors)
	ch <- promw.lastScrapeErrors
}

func (promw *PrometheusWriter) setInstanceUpDownState(ch chan<- prometheus.Metric, dbName string) {
	logger := log.GetLogger(promw.ctx)
	data := metrics.NewMeasurement(time.Now().UnixNano())
	data[promInstanceUpStateMetric] = 1

	pm := promw.MetricStoreMessageToPromMetrics(metrics.MeasurementEnvelope{
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
		logger.Error("Could not formulate an instance state report - should not happen")
	}
}

func (promw *PrometheusWriter) MetricStoreMessageToPromMetrics(msg metrics.MeasurementEnvelope) []prometheus.Metric {
	promMetrics := make([]prometheus.Metric, 0)
	logger := log.GetLogger(promw.ctx)
	var epochTime time.Time

	if len(msg.Data) == 0 {
		return promMetrics
	}

	epochTime = time.Unix(0, msg.Data.GetEpoch())

	if epochTime.Before(time.Now().Add(-promScrapingStalenessHardDropLimit)) {
		logger.Warningf("[%s][%s] Dropping metric set due to staleness (>%v) ...", msg.DBName, msg.MetricName, promScrapingStalenessHardDropLimit)
		promw.PurgeMetricsFromPromAsyncCacheIfAny(msg.DBName, msg.MetricName)
		return promMetrics
	}

	for _, dr := range msg.Data {
		labels := make(map[string]string)
		fields := make(map[string]float64)
		labels["dbname"] = msg.DBName

		for k, v := range dr {
			if v == nil || v == "" || k == metrics.EpochColumnName {
				continue // not storing NULLs. epoch checked/assigned once
			}

			if strings.HasPrefix(k, "tag_") {
				tag := k[4:]
				labels[tag] = fmt.Sprintf("%v", v)
			} else {
				switch t := v.(type) {
				case int, int32, int64, float32, float64:
					f, err := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
					if err != nil {
						logger.Debugf("skipping scraping column %s of [%s:%s]: %v", k, msg.DBName, msg.MetricName, err)
					}
					fields[k] = f
				case bool:
					fields[k] = map[bool]float64{true: 1, false: 0}[t]
				default:
					logger.Debugf("skipping scraping column %s of [%s:%s], unsupported datatype: %v", k, msg.DBName, msg.MetricName, t)
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
