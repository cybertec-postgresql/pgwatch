package reaper

import (
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
)

var lastSQLFetchError sync.Map

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
