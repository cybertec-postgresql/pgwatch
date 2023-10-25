package main

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Exporter struct {
	lastScrapeErrors                  prometheus.Gauge
	totalScrapes, totalScrapeFailures prometheus.Counter
}

const promInstanceUpStateMetric = "instance_up"

// timestamps older than that will be ignored on the Prom scraper side anyways, so better don't emit at all and just log a notice
const promScrapingStalenessHardDropLimit = time.Minute * time.Duration(10)

func NewExporter() *Exporter {
	return &Exporter{
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
}

// Not really needed for scraping to work
func (e *Exporter) Describe(_ chan<- *prometheus.Desc) {
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	var lastScrapeErrors float64

	e.totalScrapes.Add(1)
	ch <- e.totalScrapes

	isInitialized := atomic.LoadInt32(&mainLoopInitialized)
	if isInitialized == 0 {
		logger.Warning("Main loop not yet initialized, not scraping DBs")
		return
	}
	monitoredDatabases := getMonitoredDatabasesSnapshot()
	if len(monitoredDatabases) == 0 {
		logger.Warning("No dbs configured for monitoring. Check config")
		ch <- e.totalScrapeFailures
		e.lastScrapeErrors.Set(0)
		ch <- e.lastScrapeErrors
		return
	}
	for name, md := range monitoredDatabases {
		setInstanceUpDownState(ch, md) // makes easier to differentiate between PG instance / machine  failures
		// https://prometheus.io/docs/instrumenting/writing_exporters/#failed-scrapes
		if !shouldDbBeMonitoredBasedOnCurrentState(md) {
			logger.Infof("[%s] Not scraping DB due to user set constraints like DB size or standby state", md.DBUniqueName)
			continue
		}
		fetchedFromCacheCounts := make(map[string]int)

		for metric, interval := range md.Metrics {
			if metric == specialMetricChangeEvents {
				logger.Infof("[%s] Skipping change_events metric as host state is not supported for Prometheus currently", md.DBUniqueName)
				continue
			}
			if metric == promInstanceUpStateMetric {
				continue // always included in Prometheus case
			}
			if interval > 0 {
				var metricStoreMessages []metrics.MetricStoreMessage
				var err error
				var ok bool

				if opts.Metric.PrometheusAsyncMode {
					promAsyncMetricCacheLock.RLock()
					metricStoreMessages, ok = promAsyncMetricCache[md.DBUniqueName][metric]
					promAsyncMetricCacheLock.RUnlock()
					if !ok {
						// could be re-mapped metric name
						metricNameRemapLock.RLock()
						mappedName, isMapped := metricNameRemaps[metric]
						metricNameRemapLock.RUnlock()
						if isMapped {
							promAsyncMetricCacheLock.RLock()
							metricStoreMessages, ok = promAsyncMetricCache[md.DBUniqueName][mappedName]
							promAsyncMetricCacheLock.RUnlock()
						}
					}
					if ok {
						logger.Debugf("[%s:%s] fetched %d rows from the prom cache ...", md.DBUniqueName, metric, len(metricStoreMessages[0].Data))
						fetchedFromCacheCounts[metric] = len(metricStoreMessages[0].Data)
					} else {
						logger.Debugf("[%s:%s] could not find data from the prom cache. maybe gathering interval not yet reached or zero rows returned, ignoring", md.DBUniqueName, metric)
						fetchedFromCacheCounts[metric] = 0
					}
				} else {
					logger.Debugf("scraping [%s:%s]...", md.DBUniqueName, metric)
					metricStoreMessages, err = FetchMetrics(mainContext,
						MetricFetchMessage{DBUniqueName: name, DBUniqueNameOrig: md.DBUniqueNameOrig, MetricName: metric, DBType: md.DBType, Interval: time.Second * time.Duration(interval)},
						nil,
						nil,
						contextPrometheusScrape)
					if err != nil {
						logger.Errorf("failed to scrape [%s:%s]: %v", name, metric, err)
						e.totalScrapeFailures.Add(1)
						lastScrapeErrors++
						continue
					}
				}
				if len(metricStoreMessages) > 0 {
					promMetrics := MetricStoreMessageToPromMetrics(metricStoreMessages[0])
					for _, pm := range promMetrics { // collect & send later in batch? capMetricChan = 1000 limit in prometheus code
						ch <- pm
					}
				}
			}
		}
		if opts.Metric.PrometheusAsyncMode {
			logger.Infof("[%s] rowcounts fetched from the prom cache on scrape request: %+v", md.DBUniqueName, fetchedFromCacheCounts)
		}
	}

	ch <- e.totalScrapeFailures
	e.lastScrapeErrors.Set(lastScrapeErrors)
	ch <- e.lastScrapeErrors

	atomic.StoreInt64(&lastSuccessfulDatastoreWriteTimeEpoch, time.Now().Unix())
}

func setInstanceUpDownState(ch chan<- prometheus.Metric, md MonitoredDatabase) {
	logger.Debugf("checking availability of configured DB [%s:%s]...", md.DBUniqueName, promInstanceUpStateMetric)
	vme, err := DBGetPGVersion(mainContext, md.DBUniqueName, md.DBType, !opts.Metric.PrometheusAsyncMode) // in async mode 2min cache can mask smaller downtimes!
	data := make(metrics.MetricEntry)
	if err != nil {
		data[promInstanceUpStateMetric] = 0
		logger.Errorf("[%s:%s] could not determine instance version, reporting as 'down': %v", md.DBUniqueName, promInstanceUpStateMetric, err)
	} else {
		data[promInstanceUpStateMetric] = 1
	}
	data[epochColumnName] = time.Now().UnixNano()

	pm := MetricStoreMessageToPromMetrics(metrics.MetricStoreMessage{
		DBUniqueName:     md.DBUniqueName,
		DBType:           md.DBType,
		MetricName:       promInstanceUpStateMetric,
		CustomTags:       md.CustomTags,
		Data:             metrics.MetricData{data},
		RealDbname:       vme.RealDbname,
		SystemIdentifier: vme.SystemIdentifier,
	})

	if len(pm) > 0 {
		ch <- pm[0]
	} else {
		logger.Errorf("Could not formulate an instance state report - should not happen")
	}
}

func getMonitoredDatabasesSnapshot() map[string]MonitoredDatabase {
	mdSnap := make(map[string]MonitoredDatabase)

	if monitoredDbCache != nil {
		monitoredDbCacheLock.RLock()
		defer monitoredDbCacheLock.RUnlock()

		for _, row := range monitoredDbCache {
			mdSnap[row.DBUniqueName] = row
		}
	}

	return mdSnap
}

func MetricStoreMessageToPromMetrics(msg metrics.MetricStoreMessage) []prometheus.Metric {
	promMetrics := make([]prometheus.Metric, 0)

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

		if opts.Metric.PrometheusAsyncMode && epochTime.Before(epochNow.Add(-1*promScrapingStalenessHardDropLimit)) {
			logger.Warningf("[%s][%s] Dropping metric set due to staleness (>%v) ...", msg.DBUniqueName, msg.MetricName, promScrapingStalenessHardDropLimit)
			PurgeMetricsFromPromAsyncCacheIfAny(msg.DBUniqueName, msg.MetricName)
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

		if opts.AddRealDbname && opts.RealDbnameField != "" && msg.RealDbname != "" {
			labels[opts.RealDbnameField] = msg.RealDbname
		}
		if opts.AddSystemIdentifier && opts.SystemIdentifierField != "" && msg.SystemIdentifier != "" {
			labels[opts.SystemIdentifierField] = msg.SystemIdentifier
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

			if msg.MetricDefinitionDetails.ColumnAttrs.PrometheusAllGaugeColumns {
				fieldPromDataType = prometheus.GaugeValue
			} else {
				for _, gaugeColumns := range msg.MetricDefinitionDetails.ColumnAttrs.PrometheusGaugeColumns {
					if gaugeColumns == field {
						fieldPromDataType = prometheus.GaugeValue
						break
					}
				}
			}
			if msg.MetricName == promInstanceUpStateMetric {
				fieldPromDataType = prometheus.GaugeValue
			}

			for _, ignoredColumns := range msg.MetricDefinitionDetails.ColumnAttrs.PrometheusIgnoredColumns {
				if ignoredColumns == field {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			var desc *prometheus.Desc
			if opts.Metric.PrometheusNamespace != "" {
				if msg.MetricName == promInstanceUpStateMetric { // handle the special "instance_up" check
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s", opts.Metric.PrometheusNamespace, msg.MetricName),
						msg.MetricName, labelKeys, nil)
				} else {
					desc = prometheus.NewDesc(fmt.Sprintf("%s_%s_%s", opts.Metric.PrometheusNamespace, msg.MetricName, field),
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

func StartPrometheusExporter(ctx context.Context) {
	promExporter := NewExporter()
	prometheus.MustRegister(promExporter)
	promServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", opts.Metric.PrometheusListenAddr, opts.Metric.PrometheusPort),
		Handler: promhttp.Handler(),
	}
	logger.Infof("starting Prometheus exporter on %s:%d ...", opts.Metric.PrometheusListenAddr, opts.Metric.PrometheusPort)
	go logger.Error(promServer.ListenAndServe())
	<-ctx.Done()
	_ = promServer.Close()
}
