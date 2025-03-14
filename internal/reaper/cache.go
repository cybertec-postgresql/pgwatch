package reaper

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/sirupsen/logrus"
)

var monitoredDbCache map[string]*sources.SourceConn
var monitoredDbCacheLock sync.RWMutex
var MonitoredDatabasesSettings = make(map[string]MonitoredDatabaseSettings)
var MonitoredDatabasesSettingsLock = sync.RWMutex{}
var MonitoredDatabasesSettingsGetLock = make(map[string]*sync.RWMutex) // synchronize initial PG version detection to 1 instance for each defined host

var lastDBSizeMB = make(map[string]int64)
var lastDBSizeFetchTime = make(map[string]time.Time) // cached for DB_SIZE_CACHING_INTERVAL
var lastDBSizeCheckLock sync.RWMutex

var prevLoopMonitoredDBs sources.SourceConns // to be able to detect DBs removed from config

var lastSQLFetchError sync.Map

func InitPGVersionInfoFetchingLockIfNil(md *sources.SourceConn) {
	MonitoredDatabasesSettingsLock.Lock()
	if _, ok := MonitoredDatabasesSettingsGetLock[md.Name]; !ok {
		MonitoredDatabasesSettingsGetLock[md.Name] = &sync.RWMutex{}
	}
	MonitoredDatabasesSettingsLock.Unlock()
}

func UpdateMonitoredDBCache(data sources.SourceConns) {
	monitoredDbCacheNew := make(map[string]*sources.SourceConn)
	for _, row := range data {
		monitoredDbCacheNew[row.Name] = row
	}
	monitoredDbCacheLock.Lock()
	monitoredDbCache = monitoredDbCacheNew
	monitoredDbCacheLock.Unlock()
}

func GetMonitoredDatabaseByUniqueName(name string) (*sources.SourceConn, error) {
	monitoredDbCacheLock.RLock()
	defer monitoredDbCacheLock.RUnlock()
	md, exists := monitoredDbCache[name]
	if !exists || md == nil {
		return nil, fmt.Errorf("Database %s not found in cache", name)
	}
	return md, nil
}

// assumes upwards compatibility for versions
func GetMetricVersionProperties(metric string, _ MonitoredDatabaseSettings, metricDefMap *metrics.Metrics) (metrics.Metric, error) {
	mdm := new(metrics.Metrics)
	if metricDefMap != nil {
		mdm = metricDefMap
	} else {
		metricDefMapLock.RLock()
		mdm.MetricDefs = maps.Clone(metricDefinitionMap.MetricDefs) // copy of global cache
		metricDefMapLock.RUnlock()
	}

	return mdm.MetricDefs[metric], nil
}

// LoadMetrics loads metric definitions from the reader
func (r *Reaper) LoadMetrics() (err error) {
	var metricDefs *metrics.Metrics
	if metricDefs, err = r.MetricsReaderWriter.GetMetrics(); err != nil {
		return
	}
	metricDefMapLock.Lock()
	metricDefinitionMap.MetricDefs = maps.Clone(metricDefs.MetricDefs)
	metricDefinitionMap.PresetDefs = maps.Clone(metricDefs.PresetDefs)
	metricDefMapLock.Unlock()
	r.logger.
		WithField("metrics", len(metricDefinitionMap.MetricDefs)).
		WithField("presets", len(metricDefinitionMap.PresetDefs)).
		Log(func() logrus.Level {
			if len(metricDefinitionMap.PresetDefs)*len(metricDefinitionMap.MetricDefs) == 0 {
				return logrus.WarnLevel
			}
			return logrus.InfoLevel
		}(), "metrics and presets refreshed")
	return
}

// LoadSources loads sources from the reader
func (r *Reaper) LoadSources() (err error) {
	if DoesEmergencyTriggerfileExist(r.Metrics.EmergencyPauseTriggerfile) {
		r.logger.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", r.Metrics.EmergencyPauseTriggerfile)
		monitoredSources = make([]*sources.SourceConn, 0)
		return nil
	}
	if monitoredSources, err = monitoredSources.SyncFromReader(r.SourcesReaderWriter); err != nil {
		return err
	}
	r.logger.WithField("sources", len(monitoredSources)).Info("sources refreshed")
	return nil
}

// WriteMeasurements() writes the metrics to the sinks
func (r *Reaper) WriteMeasurements(ctx context.Context) {
	var err error
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-r.measurementCh:
			if err = r.SinksWriter.Write(msg); err != nil {
				r.logger.Error(err)
			}
		}
	}
}

func GetFromInstanceCacheIfNotOlderThanSeconds(msg MetricFetchConfig, maxAgeSeconds int64) metrics.Measurements {
	var clonedData metrics.Measurements
	instanceMetricCacheTimestampLock.RLock()
	instanceMetricTS, ok := instanceMetricCacheTimestamp[msg.DBUniqueNameOrig+msg.MetricName]
	instanceMetricCacheTimestampLock.RUnlock()
	if !ok || time.Now().Unix()-instanceMetricTS.Unix() > maxAgeSeconds {
		return nil
	}

	instanceMetricCacheLock.RLock()
	instanceMetricData, ok := instanceMetricCache[msg.DBUniqueNameOrig+msg.MetricName]
	if !ok {
		instanceMetricCacheLock.RUnlock()
		return nil
	}
	clonedData = deepCopyMetricData(instanceMetricData)
	instanceMetricCacheLock.RUnlock()

	return clonedData
}
