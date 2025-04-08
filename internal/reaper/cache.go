package reaper

import (
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
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
		return nil, fmt.Errorf("database %s not found in cache", name)
	}
	return md, nil
}

var instanceMetricCache = make(map[string](metrics.Measurements)) // [dbUnique+metric]lastly_fetched_data
var instanceMetricCacheLock = sync.RWMutex{}
var instanceMetricCacheTimestamp = make(map[string]time.Time) // [dbUnique+metric]last_fetch_time
var instanceMetricCacheTimestampLock = sync.RWMutex{}

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

func IsCacheableMetric(msg MetricFetchConfig, mvp metrics.Metric) bool {
	switch msg.Source {
	case sources.SourcePostgresContinuous, sources.SourcePatroniContinuous:
		return false
	default:
		return mvp.IsInstanceLevel
	}
}

func PutToInstanceCache(msg MetricFetchConfig, data metrics.Measurements) {
	if len(data) == 0 {
		return
	}
	dataCopy := deepCopyMetricData(data)
	instanceMetricCacheLock.Lock()
	instanceMetricCache[msg.DBUniqueNameOrig+msg.MetricName] = dataCopy
	instanceMetricCacheLock.Unlock()

	instanceMetricCacheTimestampLock.Lock()
	instanceMetricCacheTimestamp[msg.DBUniqueNameOrig+msg.MetricName] = time.Now()
	instanceMetricCacheTimestampLock.Unlock()
}

func deepCopyMetricData(data metrics.Measurements) metrics.Measurements {
	newData := make(metrics.Measurements, len(data))
	for i, dr := range data {
		newData[i] = maps.Clone(dr)
	}
	return newData
}
