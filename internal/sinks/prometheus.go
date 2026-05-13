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
//   - Metrics are collected periodically by reaper and cached in-memory.
//     On scrape, the collector reads a snapshot of the cache
//     and emits fresh NewConstMetric values. The cache is NOT consumed on
//     scrape, so parallel or back-to-back scrapes see the same data until the
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
type PrometheusWriter struct {
	sync.RWMutex
	logger    log.Logger
	ctx       context.Context
	gauges    map[string]([]string) // map of metric names to their gauge column names
	Namespace string
	Cache     PromMetricCache // [dbUnique][metric]lastly_fetched_data

	// Self-instrumentation metrics
	lastScrapeErrors    prometheus.Gauge
	totalScrapes        prometheus.Counter
	totalScrapeFailures prometheus.Counter
}

const promInstanceUpStateMetric = "instance_up"

// timestamps older than that will be ignored on the Prom scraper side anyway, so better don't emit at all and just log a notice
const promCacheTTL = time.Minute * time.Duration(10)

func NewPrometheusWriter(ctx context.Context, connstr string) (promw *PrometheusWriter, err error) {
	addr, namespace, found := strings.Cut(connstr, "/")
	if !found || namespace == "" {
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

// Println implements promhttp.Logger
func (promw *PrometheusWriter) Println(v ...any) {
	promw.logger.Errorln(v...)
}

// DefineMetrics is called by reaper on startup and whenever metric definitions change
func (promw *PrometheusWriter) DefineMetrics(metrics *metrics.Metrics) (err error) {
	promw.Lock()
	defer promw.Unlock()
	promw.gauges = make(map[string]([]string))
	for name, m := range metrics.MetricDefs {
		promw.gauges[name] = m.Gauges
	}
	return nil
}

// Write is called by reaper whenever new measurement data arrives
func (promw *PrometheusWriter) Write(msg metrics.MeasurementEnvelope) error {
	if len(msg.Data) == 0 {
		return nil
	}
	promw.AddCacheEntry(msg.DBName, msg.MetricName, msg)
	return nil
}

// SyncMetric is called by reaper when a metric or monitored source is removed or added,
// allowing the writer to purge or initialize cache entries as needed
func (promw *PrometheusWriter) SyncMetric(sourceName, metricName string, op SyncOp) error {
	switch op {
	case DeleteOp:
		promw.PurgeCacheEntry(sourceName, metricName)
	case AddOp:
		promw.InitCacheEntry(sourceName)
	}
	return nil
}

var notSupportedMetrics = map[string]struct{}{
	"change_events":     {}, // fully consist of text columns
	"pgbouncer_stats":   {}, // this and below metrics column names cannot be renamed with tag_ prefix
	"pgbouncer_clients": {},
	"pgpool_processes":  {},
	"pgpool_stats":      {},
}


func (promw *PrometheusWriter) AddCacheEntry(dbUnique, metric string, msgArr metrics.MeasurementEnvelope) { // cache structure: [dbUnique][metric]lastly_fetched_data
	if _, ok := notSupportedMetrics[metric]; ok && msgArr.SourceKind != "prometheus" {
		return // not supported for DB-sourced metrics
	}
	promw.Lock()
	defer promw.Unlock()
	if _, ok := promw.Cache[dbUnique]; !ok {
		promw.Cache[dbUnique] = make(map[string]metrics.MeasurementEnvelope)
	}
	promw.Cache[dbUnique][metric] = msgArr
}

func (promw *PrometheusWriter) InitCacheEntry(dbUnique string) {
	promw.Lock()
	defer promw.Unlock()
	if _, ok := promw.Cache[dbUnique]; !ok {
		promw.Cache[dbUnique] = make(map[string]metrics.MeasurementEnvelope)
	}
}

func (promw *PrometheusWriter) PurgeCacheEntry(dbUnique, metric string) {
	promw.Lock()
	defer promw.Unlock()
	if metric == "" {
		delete(promw.Cache, dbUnique) // whole host removed from config
		return
	}
	delete(promw.Cache[dbUnique], metric)
}

// Describe is intentionally empty to make PrometheusWriter an "unchecked
// collector" per the prometheus.Collector contract
func (promw *PrometheusWriter) Describe(_ chan<- *prometheus.Desc) {
}

// Collect implements prometheus.Collector. It reads a snapshot of the metric
// cache and emits const metrics. Parallel scrapes see the same data until
// background Write() calls update it
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

	t1 := time.Now()
	for _, metricsMessages := range snapshot {
		for _, envelope := range metricsMessages {
			written, errors := promw.WritePromMetrics(envelope, ch)
			lastScrapeErrors += float64(errors)
			rows += written
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

// WritePromMetrics converts a MeasurementEnvelope into Prometheus const metrics
// and sends them directly to ch. Returns the count of metrics written and errors encountered.
//
// For prom-sourced envelopes (SourceKind == "prometheus") the original family
// name is used as the fully-qualified metric name with no pgwatch namespace
// prefix, and each measurement's own epoch_ns is used as the timestamp.
// For all other sources the existing namespace+family+field naming and a
// single envelope-level timestamp are applied.
func (promw *PrometheusWriter) WritePromMetrics(msg metrics.MeasurementEnvelope, ch chan<- prometheus.Metric) (written int, errorCount int) {
	if len(msg.Data) == 0 {
		return
	}

	isPromSource := msg.SourceKind == "prometheus"

	promw.RLock()
	gauges := promw.gauges[msg.MetricName]
	promw.RUnlock()

	// baseEpochTime is used for staleness check and as the per-row timestamp
	// for non-prom sources (all rows in one envelope share the same instant).
	baseEpochTime := time.Unix(0, msg.Data.GetEpoch())
	if baseEpochTime.Before(time.Now().Add(-promCacheTTL)) {
		promw.logger.Debugf("Dropping metric %s:%s cache set due to staleness (>%v)...", msg.DBName, msg.MetricName, promCacheTTL)
		promw.PurgeCacheEntry(msg.DBName, msg.MetricName)
		return
	}

	seen := make(map[string]any)

	for _, measurement := range msg.Data {
		labels := make(map[string]string)
		fields := make(map[string]float64)
		if msg.CustomTags != nil {
			labels = maps.Clone(msg.CustomTags)
		}
		labels["dbname"] = msg.DBName

		// Prom-sourced measurements each carry their own timestamp.
		rowEpochTime := baseEpochTime
		for k, v := range measurement {
			if k == metrics.EpochColumnName {
				if isPromSource {
					if ns, ok := v.(int64); ok && ns != 0 {
						rowEpochTime = time.Unix(0, ns)
					}
				}
				continue
			}
			if v == nil || v == "" {
				continue
			}

			if tag, found := strings.CutPrefix(k, metrics.TagPrefix); found {
				labels[tag] = fmt.Sprintf("%v", v)
			} else {
				switch t := v.(type) {
				case int, int32, int64, float32, float64:
					fields[k], _ = strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
				case bool:
					fields[k] = map[bool]float64{true: 1, false: 0}[t]
				default:
					promw.logger.Debugf("skipping scraping column %s of [%s:%s], unsupported datatype: %v", k, msg.DBName, msg.MetricName, t)
					continue
				}
			}
		}

		// Sort label keys for deterministic descriptor identity.
		labelKeys := slices.Sorted(maps.Keys(labels))
		labelValues := make([]string, len(labelKeys))
		for i, k := range labelKeys {
			labelValues[i] = labels[k]
		}

		for field, value := range fields {
			var fqName string
			var fieldPromDataType prometheus.ValueType

			if isPromSource {
				// Prom→prom proxy path: ScrapeAll stores exactly one value
				// column per family, named after the family itself. Skip any
				// other numeric column that may appear unexpectedly.
				if field != msg.MetricName {
					continue
				}
				fqName = field
				fieldPromDataType = prometheus.UntypedValue
			} else {
				fieldPromDataType = prometheus.CounterValue
				if msg.MetricName == promInstanceUpStateMetric ||
					len(gauges) > 0 && (gauges[0] == "*" || slices.Contains(gauges, field)) {
					fieldPromDataType = prometheus.GaugeValue
				}
				if msg.MetricName == promInstanceUpStateMetric {
					fqName = fmt.Sprintf("%s_%s", promw.Namespace, msg.MetricName)
				} else {
					fqName = fmt.Sprintf("%s_%s_%s", promw.Namespace, msg.MetricName, field)
				}
			}

			// skip if this exact identity was already emitted in this scrape
			identity := fqName + "_" + strings.Join(labelValues, "_")
			if _, dup := seen[identity]; dup {
				promw.logger.
					WithField("metric", msg.MetricName).
					Warning("duplicate metric identity dropped, prefix differentiating string columns with tag_")
				errorCount++
				continue
			}
			seen[identity] = struct{}{}

			desc := prometheus.NewDesc(fqName, msg.MetricName, labelKeys, nil)
			m, err := prometheus.NewConstMetric(desc, fieldPromDataType, value, labelValues...)
			if err != nil {
				promw.logger.Warningf("skipping metric %s of [%s:%s]: %v", fqName, msg.DBName, msg.MetricName, err)
				errorCount++
				continue
			}
			ch <- prometheus.NewMetricWithTimestamp(rowEpochTime, m)
			written++
		}
	}
	return
}
