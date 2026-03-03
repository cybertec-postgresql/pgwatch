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

// PrometheusWriter is a Prometheus exporter that implements the prometheus.Collector
// interface using the "unchecked collector" pattern (empty Describe method).
//
// Design decisions based on Prometheus exporter guidelines
// (https://prometheus.io/docs/instrumenting/writing_exporters/#collectors):
//
//   - Metrics are collected periodically by pgwatch background goroutines and
//     cached in-memory. On scrape, the collector reads a snapshot of the cache
//     and emits fresh MustNewConstMetric values. The cache is NOT consumed on
//     scrape — parallel or back-to-back scrapes see the same data until the
//     next Write() updates arrive.
//
//   - This is an "unchecked collector": Describe() sends no descriptors, which
//     tells the Prometheus registry to skip consistency checks. This is necessary
//     because the set of metrics is dynamic (driven by monitored databases and
//     their query results). Safety is ensured by deduplicating metric identities
//     within each Collect() call.
//
//   - Label keys are always sorted lexicographically before building descriptors
//     and label value slices. This guarantees deterministic descriptor identity
//     regardless of Go map iteration order.
//
//   - Self-instrumentation metrics (scrape counts, error counts) are registered
//     separately with the Prometheus default registry and are NOT emitted through
//     the custom Collect() method, avoiding mixing of checked and unchecked
//     metric lifecycles.
type PrometheusWriter struct {
	sync.RWMutex
	logger    log.Logger
	ctx       context.Context
	gauges    map[string]([]string) // map of metric names to their gauge column names
	Namespace string
	Cache     PromMetricCache // [dbUnique][metric]lastly_fetched_data

	// Self-instrumentation metrics. Emitted through the custom Collect() method
	// rather than registered separately. These have fixed descriptors and are
	// guaranteed unique within each scrape, so they are safe within an unchecked
	// collector.
	lastScrapeErrors    prometheus.Gauge
	totalScrapes        prometheus.Counter
	totalScrapeFailures prometheus.Counter
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

	lastScrapeErrors := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "exporter_last_scrape_errors",
		Help:      "Last scrape error count for all monitored hosts / metrics",
	})
	totalScrapes := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_total_scrapes",
		Help:      "Total scrape attempts.",
	})
	totalScrapeFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_total_scrape_failures",
		Help:      "Number of errors while executing metric queries",
	})

	promw = &PrometheusWriter{
		ctx:                 ctx,
		logger:              l,
		Namespace:           namespace,
		Cache:               make(PromMetricCache),
		lastScrapeErrors:    lastScrapeErrors,
		totalScrapes:        totalScrapes,
		totalScrapeFailures: totalScrapeFailures,
	}

	// Register the custom collector (unchecked — empty Describe).
	// Self-instrumentation metrics (totalScrapes, totalScrapeFailures,
	// lastScrapeErrors) are emitted through Collect() alongside dynamic metrics.
	// They have fixed descriptors and are guaranteed unique per scrape.
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

// Describe is intentionally empty — this makes PrometheusWriter an "unchecked
// collector" per the prometheus.Collector contract.
//
// An unchecked collector is required when the set of metrics is not known at
// registration time (pgwatch's metrics are driven by monitored databases and
// their query results, which change at runtime).
//
// Safety guarantee: Collect() ensures no duplicate (metric name + label values)
// combinations are emitted within a single scrape via an explicit dedup set.
//
// See: https://pkg.go.dev/github.com/prometheus/client_golang/prometheus#hdr-Custom_Collectors_and_constant_Metrics
func (promw *PrometheusWriter) Describe(_ chan<- *prometheus.Desc) {
}

// Collect implements prometheus.Collector. It reads a snapshot of the metric
// cache and emits const metrics. The cache is NOT consumed — parallel scrapes
// see the same data until background Write() calls update it.
func (promw *PrometheusWriter) Collect(ch chan<- prometheus.Metric) {
	promw.totalScrapes.Add(1)
	ch <- promw.totalScrapes

	promw.RLock()
	if len(promw.Cache) == 0 {
		promw.RUnlock()
		promw.logger.Warning("No dbs configured for monitoring. Check config")
		ch <- promw.totalScrapeFailures
		promw.lastScrapeErrors.Set(0)
		ch <- promw.lastScrapeErrors
		return
	}
	snapshot := promw.snapshotCache()
	promw.RUnlock()

	var rows int
	var lastScrapeErrors float64
	// Deduplication set: prevents emitting the same metric identity twice within
	// a single scrape. Key is "metric_fqdn\xfflabel_val1\xfflabel_val2\xff...".
	// This is the safety contract required for an unchecked collector.
	seen := make(map[string]struct{})

	t1 := time.Now()
	for _, metricsMessages := range snapshot {
		for metric, metricMessages := range metricsMessages {
			if metric == "change_events" {
				continue // not supported
			}
			promMetrics, errors := promw.metricStoreMessageToPromMetrics(metricMessages, seen)
			lastScrapeErrors += float64(errors)
			rows += len(promMetrics)
			for _, pm := range promMetrics {
				ch <- pm
			}
		}
	}
	promw.logger.WithField("count", rows).WithField("elapsed", time.Since(t1)).Info("measurements written")
	ch <- promw.totalScrapeFailures
	promw.lastScrapeErrors.Set(lastScrapeErrors)
	ch <- promw.lastScrapeErrors
}

// snapshotCache creates a shallow copy of the cache map hierarchy.
// Must be called under at least promw.RLock().
// The MeasurementEnvelope values are not deep-copied because writers always
// replace entire envelopes (never mutate them in place).
func (promw *PrometheusWriter) snapshotCache() PromMetricCache {
	snapshot := make(PromMetricCache, len(promw.Cache))
	for db, metricMap := range promw.Cache {
		snapshot[db] = maps.Clone(metricMap)
	}
	return snapshot
}

// metricStoreMessageToPromMetrics converts a MeasurementEnvelope into a slice
// of Prometheus const metrics with deterministic label ordering and dedup.
//
// Parameters:
//   - msg: the measurement envelope containing metric data rows
//   - seen: deduplication set shared across the entire Collect() call;
//     entries are "fqdn\xffval1\xffval2..." strings
//
// Returns the metrics and the count of errors encountered.
func (promw *PrometheusWriter) metricStoreMessageToPromMetrics(msg metrics.MeasurementEnvelope, seen map[string]struct{}) ([]prometheus.Metric, int) {
	promMetrics := make([]prometheus.Metric, 0)
	var errorCount int
	if len(msg.Data) == 0 {
		return promMetrics, 0
	}

	promw.RLock()
	gauges := promw.gauges[msg.MetricName]
	promw.RUnlock()

	epochTime := time.Unix(0, msg.Data.GetEpoch())

	if epochTime.Before(time.Now().Add(-promScrapingStalenessHardDropLimit)) {
		promw.logger.Warningf("Dropping metric %s:%s cache set due to staleness (>%v)...", msg.DBName, msg.MetricName, promScrapingStalenessHardDropLimit)
		promw.PurgeMetricsFromPromAsyncCacheIfAny(msg.DBName, msg.MetricName)
		return promMetrics, 0
	}

	for _, dr := range msg.Data {
		labels := make(map[string]string)
		fields := make(map[string]float64)
		if msg.CustomTags != nil {
			labels = maps.Clone(msg.CustomTags)
		}
		labels["dbname"] = msg.DBName

		for k, v := range dr {
			if k == metrics.EpochColumnName {
				continue // not storing NULLs. epoch checked/assigned once
			}

			if strings.HasPrefix(k, "tag_") {
				tag := k[4:]
				if v == nil {
					labels[tag] = ""
				} else {
					labels[tag] = fmt.Sprintf("%v", v)
				}
			} else {
				if v == nil || v == "" {
					continue
				}
				switch t := v.(type) {
				case string:
					// Only tag_ prefixed columns become labels (handled above).
					// Plain string columns (e.g. data_dir, version_str) are not
					// numeric values and must not be promoted to labels — doing so
					// creates high-cardinality label sets and metric identity
					// explosion, which is the root cause of duplicate emission errors.
					promw.logger.Debugf("skipping non-tag string column %s of [%s:%s]", k, msg.DBName, msg.MetricName)
					continue
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

		// Sort label keys for deterministic descriptor identity.
		// Go maps iterate in random order; without sorting, the same label set
		// could produce different prometheus.Desc objects across rows or scrapes,
		// leading to "duplicate metric" errors.
		labelKeys := slices.Sorted(maps.Keys(labels))
		labelValues := make([]string, len(labelKeys))
		for i, k := range labelKeys {
			labelValues[i] = labels[k]
		}

		for field, value := range fields {
			fieldPromDataType := prometheus.CounterValue
			if msg.MetricName == promInstanceUpStateMetric ||
				len(gauges) > 0 && (gauges[0] == "*" || slices.Contains(gauges, field)) {
				fieldPromDataType = prometheus.GaugeValue
			}

			// Build the fully-qualified metric name.
			var fqdn string
			if promw.Namespace != "" {
				if msg.MetricName == promInstanceUpStateMetric {
					fqdn = fmt.Sprintf("%s_%s", promw.Namespace, msg.MetricName)
				} else {
					fqdn = fmt.Sprintf("%s_%s_%s", promw.Namespace, msg.MetricName, field)
				}
			} else {
				if msg.MetricName == promInstanceUpStateMetric {
					fqdn = field
				} else {
					fqdn = fmt.Sprintf("%s_%s", msg.MetricName, field)
				}
			}

			// Deduplicate: skip if this exact (fqdn + label values) was already
			// emitted in this scrape. Required for unchecked collector safety.
			identity := fqdn + "\xff" + strings.Join(labelValues, "\xff")
			if _, dup := seen[identity]; dup {
				continue
			}
			seen[identity] = struct{}{}

			desc := prometheus.NewDesc(fqdn, msg.MetricName, labelKeys, nil)

			// Use NewConstMetric (not MustNewConstMetric) to avoid panics on
			// label mismatch. With dynamic, data-driven label sets a mismatch
			// is possible (e.g. when a column appears/disappears between rows).
			m, err := prometheus.NewConstMetric(desc, fieldPromDataType, value, labelValues...)
			if err != nil {
				promw.logger.Debugf("skipping metric %s for [%s:%s]: %v", fqdn, msg.DBName, msg.MetricName, err)
				errorCount++
				continue
			}
			promMetrics = append(promMetrics, prometheus.NewMetricWithTimestamp(epochTime, m))
		}
	}
	return promMetrics, errorCount
}
