package reaper

import (
	"fmt"
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

type InstanceMetricCache struct {
	cache map[string](metrics.Measurements) // [dbUnique+metric]lastly_fetched_data
	sync.RWMutex
}

func NewInstanceMetricCache() *InstanceMetricCache {
	return &InstanceMetricCache{
		cache: make(map[string](metrics.Measurements)),
	}
}

func (imc *InstanceMetricCache) Get(key string, age time.Duration) metrics.Measurements {
	if key == "" {
		return nil
	}
	imc.RLock()
	defer imc.RUnlock()
	instanceMetricEpochNs := (imc.cache[key]).GetEpoch()

	if time.Now().UnixNano()-instanceMetricEpochNs < age.Nanoseconds() {
		return nil
	}
	instanceMetricData, ok := imc.cache[key]
	if !ok {
		return nil
	}
	return instanceMetricData.DeepCopy()
}

func (imc *InstanceMetricCache) Put(key string, data metrics.Measurements) {
	if len(data) == 0 || key == "" {
		return
	}
	imc.Lock()
	defer imc.Unlock()
	m := data.DeepCopy()
	if !m.IsEpochSet() {
		m.Touch()
	}
	imc.cache[key] = m
}
