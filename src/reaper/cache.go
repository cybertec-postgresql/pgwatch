package reaper

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/log"
	"github.com/cybertec-postgresql/pgwatch/metrics"
	"github.com/cybertec-postgresql/pgwatch/sources"
)

var monitoredDbCache map[string]*sources.MonitoredDatabase
var monitoredDbCacheLock sync.RWMutex
var MonitoredDatabasesSettings = make(map[string]MonitoredDatabaseSettings)
var MonitoredDatabasesSettingsLock = sync.RWMutex{}
var MonitoredDatabasesSettingsGetLock = make(map[string]*sync.RWMutex) // synchronize initial PG version detection to 1 instance for each defined host

var unreachableDBsLock sync.RWMutex
var unreachableDB = make(map[string]time.Time)

var lastDBSizeMB = make(map[string]int64)
var lastDBSizeFetchTime = make(map[string]time.Time) // cached for DB_SIZE_CACHING_INTERVAL
var lastDBSizeCheckLock sync.RWMutex

var prevLoopMonitoredDBs sources.MonitoredDatabases // to be able to detect DBs removed from config
var undersizedDBs = make(map[string]bool)           // DBs below the --min-db-size-mb limit, if set
var undersizedDBsLock = sync.RWMutex{}
var recoveryIgnoredDBs = make(map[string]bool) // DBs in recovery state and OnlyIfMaster specified in config
var recoveryIgnoredDBsLock = sync.RWMutex{}

var hostMetricIntervalMap = make(map[string]float64) // [db1_metric] = 30

var lastSQLFetchError sync.Map

func InitPGVersionInfoFetchingLockIfNil(md *sources.MonitoredDatabase) {
	MonitoredDatabasesSettingsLock.Lock()
	if _, ok := MonitoredDatabasesSettingsGetLock[md.Name]; !ok {
		MonitoredDatabasesSettingsGetLock[md.Name] = &sync.RWMutex{}
	}
	MonitoredDatabasesSettingsLock.Unlock()
}

func UpdateMonitoredDBCache(data sources.MonitoredDatabases) {
	monitoredDbCacheNew := make(map[string]*sources.MonitoredDatabase)
	for _, row := range data {
		monitoredDbCacheNew[row.Name] = row
	}
	monitoredDbCacheLock.Lock()
	monitoredDbCache = monitoredDbCacheNew
	monitoredDbCacheLock.Unlock()
}

func GetMonitoredDatabaseByUniqueName(name string) (*sources.MonitoredDatabase, error) {
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

// LoadMetricDefs loads metric definitions from the reader
func LoadMetricDefs(r metrics.Reader) (err error) {
	var metricDefs *metrics.Metrics
	if metricDefs, err = r.GetMetrics(); err != nil {
		return
	}
	metricDefMapLock.Lock()
	metricDefinitionMap.MetricDefs = maps.Clone(metricDefs.MetricDefs)
	metricDefinitionMap.PresetDefs = maps.Clone(metricDefs.PresetDefs)
	metricDefMapLock.Unlock()
	return
}

const metricDefinitionRefreshInterval time.Duration = time.Minute * 2 // min time before checking for new/changed metric definitions

// SyncMetricDefs refreshes metric definitions at regular intervals
func SyncMetricDefs(ctx context.Context, r metrics.Reader) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(metricDefinitionRefreshInterval):
			if err := LoadMetricDefs(r); err != nil {
				log.GetLogger(ctx).Errorf("Could not refresh metric definitions: %w", err)
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
