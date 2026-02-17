package sinks

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PromMetricCache = map[string]map[string]metrics.MeasurementEnvelope // [dbUnique][metric]lastly_fetched_data

// PrometheusWriter is a sink that allows to expose metric measurements to Prometheus scrapper.
// Prometheus collects metrics data from pgwatch by scraping metrics HTTP endpoints.
type PrometheusWriter struct {
	sync.RWMutex
	logger              log.Logger
	ctx                 context.Context
	lastScrapeErrors    prometheus.Gauge
	totalScrapes        prometheus.Counter
	totalScrapeFailures prometheus.Counter
	gauges              map[string]([]string) // map of metric names to their gauge names, used for Prometheus gauge metrics
	Namespace           string
	Cache               PromMetricCache // [dbUnique][metric]lastly_fetched_data
}

const promInstanceUpStateMetric = "instance_up"

// timestamps older than that will be ignored on the Prom scraper side anyway, so better don't emit at all and just log a notice
const promScrapingStalenessHardDropLimit = time.Minute * time.Duration(10)

func (promw *PrometheusWriter) Println(v ...any) {
	promw.logger.Errorln(v...)
}

func NewPrometheusWriter(ctx context.Context, connstr string) (promw *PrometheusWriter, err error) {
	addr, namespace, found := strings.Cut(connstr, "/")
	if !found {
		namespace = "pgwatch"
	}
	l := log.GetLogger(ctx).WithField("sink", "prometheus").WithField("address", addr)
	ctx = log.WithLogger(ctx, l)
	promw = &PrometheusWriter{
		ctx:       ctx,
		logger:    l,
		Namespace: namespace,
		Cache:     make(PromMetricCache),
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
		Addr: addr,
		Handler: promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{
				ErrorLog:      promw,
				ErrorHandling: promhttp.ContinueOnError,
			},
		),
	}

	ln, err := net.Listen("tcp", promServer.Addr)
	if err != nil {
		return nil, err
	}

	go func() { log.GetLogger(ctx).Error(promServer.Serve(ln)) }()

	l.Info(`measurements sink is activated`)
	return
}

func (promw *PrometheusWriter) DefineMetrics(metrics *metrics.Metrics) (err error) {
	promw.Lock()
	defer promw.Unlock()
	promw.gauges = make(map[string]([]string))
	for name, m := range metrics.MetricDefs {
		promw.gauges[name] = m.Gauges
	}
	return nil
}

func (promw *PrometheusWriter) Write(msg metrics.MeasurementEnvelope) error {
	if len(msg.Data) == 0 { // no batching in async prom mode, so using 0 indexing ok
		return nil
	}
	promw.PromAsyncCacheAddMetricData(msg.DBName, msg.MetricName, msg)
	return nil
}

func (promw *PrometheusWriter) PromAsyncCacheAddMetricData(dbUnique, metric string, msgArr metrics.MeasurementEnvelope) { // cache structure: [dbUnique][metric]lastly_fetched_data
	promw.Lock()
	defer promw.Unlock()
	if _, ok := promw.Cache[dbUnique]; !ok {
		promw.Cache[dbUnique] = make(map[string]metrics.MeasurementEnvelope)
	}
	promw.Cache[dbUnique][metric] = msgArr
}

func (promw *PrometheusWriter) PromAsyncCacheInitIfRequired(dbUnique, _ string) { // cache structure: [dbUnique][metric]lastly_fetched_data
	promw.Lock()
	defer promw.Unlock()
	if _, ok := promw.Cache[dbUnique]; !ok {
		promw.Cache[dbUnique] = make(map[string]metrics.MeasurementEnvelope)
	}
}

func (promw *PrometheusWriter) PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, metric string) {
	promw.Lock()
	defer promw.Unlock()

	if metric == "" {
		delete(promw.Cache, dbUnique) // whole host removed from config
	} else {
		delete(promw.Cache[dbUnique], metric)
	}
}

func (promw *PrometheusWriter) SyncMetric(sourceName, metricName string, op SyncOp) error {
	switch op {
	case DeleteOp:
		promw.PurgeMetricsFromPromAsyncCacheIfAny(sourceName, metricName)
	case AddOp:
		promw.PromAsyncCacheInitIfRequired(sourceName, metricName)
	}
	return nil
}

func (promw *PrometheusWriter) Describe(_ chan<- *prometheus.Desc) {
}

func (promw *PrometheusWriter) Collect(ch chan<- prometheus.Metric) {
	var rows int
	var lastScrapeErrors float64
	promw.totalScrapes.Add(1)
	ch <- promw.totalScrapes

	promw.Lock()
	if len(promw.Cache) == 0 {
		promw.Unlock()
		promw.logger.Warning("No dbs configured for monitoring. Check config")
		ch <- promw.totalScrapeFailures
		promw.lastScrapeErrors.Set(0)
		ch <- promw.lastScrapeErrors
		return
	}
	snapshot := promw.Cache
	promw.Cache = make(PromMetricCache)
	promw.Unlock()

	t1 := time.Now()
	for _, metricsMessages := range snapshot {
		for metric, metricMessages := range metricsMessages {
			if metric == "change_events" {
				continue // not supported
			}
			promMetrics := promw.MetricStoreMessageToPromMetrics(metricMessages)
			rows += len(promMetrics)
			for _, pm := range promMetrics { // collect & send later in batch? capMetricChan = 1000 limit in prometheus code
				ch <- pm
			}
		}
	}
	promw.logger.WithField("count", rows).WithField("elapsed", time.Since(t1)).Info("measurements written")
	ch <- promw.totalScrapeFailures
	promw.lastScrapeErrors.Set(lastScrapeErrors)
	ch <- promw.lastScrapeErrors
}

func (promw *PrometheusWriter) MetricStoreMessageToPromMetrics(msg metrics.MeasurementEnvelope) []prometheus.Metric {
	promMetrics := make([]prometheus.Metric, 0)
	var epochTime time.Time
	if len(msg.Data) == 0 {
		return promMetrics
	}

	promw.RLock()
	gauges := promw.gauges[msg.MetricName]
	promw.RUnlock()

	epochTime = time.Unix(0, msg.Data.GetEpoch())

	if epochTime.Before(time.Now().Add(-promScrapingStalenessHardDropLimit)) {
		promw.logger.Warningf("Dropping metric %s:%s cache set due to staleness (>%v)...", msg.DBName, msg.MetricName, promScrapingStalenessHardDropLimit)
		promw.PurgeMetricsFromPromAsyncCacheIfAny(msg.DBName, msg.MetricName)
		return promMetrics
	}

	for _, dr := range msg.Data {
		labels := make(map[string]string)
		fields := make(map[string]float64)
		if msg.CustomTags != nil {
			labels = maps.Clone(msg.CustomTags)
		}
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
				case string:
					labels[k] = t
				case int, int32, int64, float32, float64:
					f, err := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
					if err != nil {
						promw.logger.Debugf("skipping scraping column %s of [%s:%s]: %v", k, msg.DBName, msg.MetricName, err)
					}
					fields[k] = f
				case bool:
					fields[k] = map[bool]float64{true: 1, false: 0}[t]
				default:
					promw.logger.Debugf("skipping scraping column %s of [%s:%s], unsupported datatype: %v", k, msg.DBName, msg.MetricName, t)
					continue
				}
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
				len(gauges) > 0 && (gauges[0] == "*" || slices.Contains(gauges, field)) {
				fieldPromDataType = prometheus.GaugeValue
			}
			var desc *prometheus.Desc
			if promw.Namespace != "" {
				if msg.MetricName == promInstanceUpStateMetric { // handle the special "instance_up" check
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s", promw.Namespace, msg.MetricName),
						msg.MetricName, labelKeys, nil)
				} else {
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s_%s", promw.Namespace, msg.MetricName, field),
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
