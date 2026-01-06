package reaper

import (
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/sirupsen/logrus"
)

const (
	monitoredDbsDatastoreSyncIntervalSeconds = 600              // write actively monitored DBs listing to metrics store after so many seconds
	monitoredDbsDatastoreSyncMetricName      = "configured_dbs" // FYI - for Postgres datastore there's also the admin.all_unique_dbnames table with all recent DB unique names with some metric data
	dbMetricJoinStr                          = "¤¤¤"            // just some unlikely string for a DB name to avoid using maps of maps for DB+metric data

)

type ConcurrentMetricDefs struct {
	*metrics.Metrics
	sync.RWMutex
}

func NewConcurrentMetricDefs() *ConcurrentMetricDefs {
	return &ConcurrentMetricDefs{
		Metrics: &metrics.Metrics{
			MetricDefs: make(metrics.MetricDefs),
			PresetDefs: make(metrics.PresetDefs),
		},
	}
}

func (cmd *ConcurrentMetricDefs) GetMetricDef(name string) (m metrics.Metric, ok bool) {
	cmd.RLock()
	defer cmd.RUnlock()
	m, ok = cmd.MetricDefs[name]
	return
}

func (cmd *ConcurrentMetricDefs) GetPresetDef(name string) (m metrics.Preset, ok bool) {
	cmd.RLock()
	defer cmd.RUnlock()
	m, ok = cmd.PresetDefs[name]
	return
}

func (cmd *ConcurrentMetricDefs) GetPresetMetrics(name string) (m map[string]float64) {
	cmd.RLock()
	defer cmd.RUnlock()
	return cmd.PresetDefs[name].Metrics
}

func (cmd *ConcurrentMetricDefs) Assign(newDefs *metrics.Metrics) {
	cmd.Lock()
	defer cmd.Unlock()
	cmd.MetricDefs = maps.Clone(newDefs.MetricDefs)
	cmd.PresetDefs = maps.Clone(newDefs.PresetDefs)
}

type ChangeDetectionResults struct { // for passing around DDL/index/config change detection results
	Target  string
	Created int
	Altered int
	Dropped int
}

func (cdr *ChangeDetectionResults) Total() int {
	return cdr.Created + cdr.Altered + cdr.Dropped
}

func (cdr *ChangeDetectionResults) String() string {
	return fmt.Sprintf("%s: %d/%d/%d", cdr.Target, cdr.Created, cdr.Altered, cdr.Dropped)
}

type ExistingPartitionInfo struct {
	StartTime time.Time
	EndTime   time.Time
}

// LoadMetrics loads metric definitions from the reader
func (r *Reaper) LoadMetrics() (err error) {
	var newDefs *metrics.Metrics
	if newDefs, err = r.MetricsReaderWriter.GetMetrics(); err != nil {
		return
	}
	metricDefs.Assign(newDefs)
	if definer, ok := r.SinksWriter.(sinks.MetricsDefiner); ok {
		err := definer.DefineMetrics(newDefs)
		if err != nil {
			r.logger.Error(err)
		}
	}
	r.logger.
		WithField("metrics", len(newDefs.MetricDefs)).
		WithField("presets", len(newDefs.PresetDefs)).
		Log(func() logrus.Level {
			if len(newDefs.PresetDefs)*len(newDefs.MetricDefs) == 0 {
				return logrus.WarnLevel
			}
			return logrus.InfoLevel
		}(), "metrics and presets refreshed")
	// update the monitored sources with real metric definitions from presets
	for _, md := range r.monitoredSources {
		if md.PresetMetrics > "" {
			md.Metrics = metricDefs.GetPresetMetrics(md.PresetMetrics)
		}
		if md.PresetMetricsStandby > "" {
			md.MetricsStandby = metricDefs.GetPresetMetrics(md.PresetMetricsStandby)
		}
	}
	return
}
