package sinks

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PrometheusWriter struct {
	ctx                               context.Context
	asyncMode                         bool
	lastScrapeErrors                  prometheus.Gauge
	totalScrapes, totalScrapeFailures prometheus.Counter
	AddRealDbname                     bool
	RealDbnameField                   string
	AddSystemIdentifier               bool
	SystemIdentifierField             string
	PrometheusNamespace               string
}

const promInstanceUpStateMetric = "instance_up"

// timestamps older than that will be ignored on the Prom scraper side anyways, so better don't emit at all and just log a notice
const promScrapingStalenessHardDropLimit = time.Minute * time.Duration(10)

func NewPrometheusWriter(ctx context.Context, opts *config.CmdOptions) (promw *PrometheusWriter, err error) {
	promw = &PrometheusWriter{
		ctx:                   ctx,
		AddRealDbname:         opts.AddRealDbname,
		RealDbnameField:       opts.RealDbnameField,
		AddSystemIdentifier:   opts.AddSystemIdentifier,
		SystemIdentifierField: opts.SystemIdentifierField,
		asyncMode:             opts.Metric.PrometheusAsyncMode,
		PrometheusNamespace:   opts.Metric.PrometheusNamespace,
		lastScrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: opts.Metric.PrometheusNamespace,
			Name:      "exporter_last_scrape_errors",
			Help:      "Last scrape error count for all monitored hosts / metrics",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: opts.Metric.PrometheusNamespace,
			Name:      "exporter_total_scrapes",
			Help:      "Total scrape attempts.",
		}),
		totalScrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: opts.Metric.PrometheusNamespace,
			Name:      "exporter_total_scrape_failures",
			Help:      "Number of errors while executing metric queries",
		}),
	}

	if err = prometheus.Register(promw); err != nil {
		return
	}
	promServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", opts.Metric.PrometheusListenAddr, opts.Metric.PrometheusPort),
		Handler: promhttp.Handler(),
	}
	go func() {
		log.GetLogger(ctx).Error(promServer.ListenAndServe())
	}()
	return
}

func (promw *PrometheusWriter) Write(msgs []metrics.MetricStoreMessage) error {
	if len(msgs) == 0 || len(msgs[0].Data) == 0 { // no batching in async prom mode, so using 0 indexing ok
		return nil
	}
	msg := msgs[0]
	promw.PromAsyncCacheAddMetricData(msg.DBUniqueName, msg.MetricName, msgs)
	return nil
}

// Async Prom cache
var promAsyncMetricCache = make(map[string]map[string][]metrics.MetricStoreMessage) // [dbUnique][metric]lastly_fetched_data
var promAsyncMetricCacheLock = sync.RWMutex{}

func (promw *PrometheusWriter) PromAsyncCacheAddMetricData(dbUnique, metric string, msgArr []metrics.MetricStoreMessage) { // cache structure: [dbUnique][metric]lastly_fetched_data
	promAsyncMetricCacheLock.Lock()
	defer promAsyncMetricCacheLock.Unlock()
	if _, ok := promAsyncMetricCache[dbUnique]; ok {
		promAsyncMetricCache[dbUnique][metric] = msgArr
	}
}

func (promw *PrometheusWriter) PromAsyncCacheInitIfRequired(dbUnique, _ string) { // cache structure: [dbUnique][metric]lastly_fetched_data
	if promw.asyncMode {
		promAsyncMetricCacheLock.Lock()
		defer promAsyncMetricCacheLock.Unlock()
		if _, ok := promAsyncMetricCache[dbUnique]; !ok {
			metricMap := make(map[string][]metrics.MetricStoreMessage)
			promAsyncMetricCache[dbUnique] = metricMap
		}
	}
}

func (promw *PrometheusWriter) PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, metric string) {
	if promw.asyncMode {
		promAsyncMetricCacheLock.Lock()
		defer promAsyncMetricCacheLock.Unlock()

		if metric == "" {
			delete(promAsyncMetricCache, dbUnique) // whole host removed from config
		} else {
			delete(promAsyncMetricCache[dbUnique], metric)
		}
	}
}

func (promw *PrometheusWriter) SyncMetric(dbUnique, metricName, op string) error {
	if !promw.asyncMode {
		return nil
	}
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
	// vme, err := db.DBGetPGVersion(promw.ctx, dbName, md.DBType, !promw.asyncMode) // in async mode 2min cache can mask smaller downtimes!
	data := make(metrics.MetricEntry)
	// if err != nil {
	// 	data[promInstanceUpStateMetric] = 0
	// 	logger.Errorf("[%s:%s] could not determine instance version, reporting as 'down': %v", md.DBUniqueName, promInstanceUpStateMetric, err)
	// } else {
	// 	data[promInstanceUpStateMetric] = 1
	// }
	data[promInstanceUpStateMetric] = 1
	data[epochColumnName] = time.Now().UnixNano()

	pm := promw.MetricStoreMessageToPromMetrics(metrics.MetricStoreMessage{
		DBUniqueName:     dbName,
		DBType:           "postgres", //md.DBType,
		MetricName:       promInstanceUpStateMetric,
		CustomTags:       nil, //md.CustomTags,
		Data:             metrics.MetricData{data},
		RealDbname:       dbName, //vme.RealDbname,
		SystemIdentifier: dbName, //vme.SystemIdentifier,
	})

	if len(pm) > 0 {
		ch <- pm[0]
	} else {
		logger.Errorf("Could not formulate an instance state report - should not happen")
	}
}

func (promw *PrometheusWriter) MetricStoreMessageToPromMetrics(msg metrics.MetricStoreMessage) []prometheus.Metric {
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

		if promw.asyncMode && epochTime.Before(epochNow.Add(-1*promScrapingStalenessHardDropLimit)) {
			logger.Warningf("[%s][%s] Dropping metric set due to staleness (>%v) ...", msg.DBUniqueName, msg.MetricName, promScrapingStalenessHardDropLimit)
			promw.PurgeMetricsFromPromAsyncCacheIfAny(msg.DBUniqueName, msg.MetricName)
			return promMetrics
		}
	}

	for _, dr := range msg.Data {
		labels := make(map[string]string)
		fields := make(map[string]float64)
		labels["dbname"] = msg.DBUniqueName

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
						logger.Debugf("Skipping scraping column %s of [%s:%s]: %v", k, msg.DBUniqueName, msg.MetricName, err)
					}
					fields[k] = f
				} else if dataType == "bool" {
					if v.(bool) {
						fields[k] = 1
					} else {
						fields[k] = 0
					}
				} else {
					logger.Debugf("Skipping scraping column %s of [%s:%s], unsupported datatype: %s", k, msg.DBUniqueName, msg.MetricName, dataType)
					continue
				}
			}
		}
		if msg.CustomTags != nil {
			for k, v := range msg.CustomTags {
				labels[k] = fmt.Sprintf("%v", v)
			}
		}

		if promw.AddRealDbname && promw.RealDbnameField != "" && msg.RealDbname != "" {
			labels[promw.RealDbnameField] = msg.RealDbname
		}
		if promw.AddSystemIdentifier && promw.SystemIdentifierField != "" && msg.SystemIdentifier != "" {
			labels[promw.SystemIdentifierField] = msg.SystemIdentifier
		}

		labelKeys := make([]string, 0)
		labelValues := make([]string, 0)
		for k, v := range labels {
			labelKeys = append(labelKeys, k)
			labelValues = append(labelValues, v)
		}

		for field, value := range fields {
			skip := false
			fieldPromDataType := prometheus.CounterValue

			if msg.MetricDefinitionDetails.PrometheusAttrs.PrometheusAllGaugeColumns {
				fieldPromDataType = prometheus.GaugeValue
			} else {
				for _, gaugeColumns := range msg.MetricDefinitionDetails.PrometheusAttrs.PrometheusGaugeColumns {
					if gaugeColumns == field {
						fieldPromDataType = prometheus.GaugeValue
						break
					}
				}
			}
			if msg.MetricName == promInstanceUpStateMetric {
				fieldPromDataType = prometheus.GaugeValue
			}

			for _, ignoredColumns := range msg.MetricDefinitionDetails.PrometheusAttrs.PrometheusIgnoredColumns {
				if ignoredColumns == field {
					skip = true
					break
				}
			}
			if skip {
				continue
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
