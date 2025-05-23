package reaper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

var monitoredDbCache map[string]*sources.SourceConn
var monitoredDbCacheLock sync.RWMutex

var lastSQLFetchError sync.Map

func UpdateMonitoredDBCache(data sources.SourceConns) {
	monitoredDbCacheNew := make(map[string]*sources.SourceConn)
	for _, row := range data {
		monitoredDbCacheNew[row.Name] = row
	}
	monitoredDbCacheLock.Lock()
	monitoredDbCache = monitoredDbCacheNew
	monitoredDbCacheLock.Unlock()
}

func GetMonitoredDatabaseByUniqueName(ctx context.Context, name string) (*sources.SourceConn, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	monitoredDbCacheLock.RLock()
	defer monitoredDbCacheLock.RUnlock()
	md, exists := monitoredDbCache[name]
	if !exists || md == nil || md.Conn == nil {
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

	if time.Now().UnixNano()-instanceMetricEpochNs > age.Nanoseconds() {
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
