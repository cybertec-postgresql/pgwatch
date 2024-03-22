package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/sources"
)

type uiapihandler struct{}

var uiapi uiapihandler

func (uiapi uiapihandler) TryConnectToDB(params []byte) (err error) {
	return db.Ping(context.TODO(), string(params))
}

// UpdatePreset updates the stored preset
func (uiapi uiapihandler) UpdatePreset(name string, params []byte) error {
	var p metrics.Preset
	err := json.Unmarshal(params, &p)
	if err != nil {
		return err
	}
	return metricsReaderWriter.UpdatePreset(name, p)
}

// GetPresets ret	urns the list of available presets
func (uiapi uiapihandler) GetPresets() (res string, err error) {
	var mr *metrics.Metrics
	if mr, err = metricsReaderWriter.GetMetrics(); err != nil {
		return
	}
	b, _ := json.Marshal(mr.PresetDefs)
	res = string(b)
	return
}

// DeletePreset removes the preset from the configuration
func (uiapi uiapihandler) DeletePreset(name string) error {
	return metricsReaderWriter.DeletePreset(name)
}

// GetMetrics returns the list of metrics
func (uiapi uiapihandler) GetMetrics() (res string, err error) {
	var mr *metrics.Metrics
	if mr, err = metricsReaderWriter.GetMetrics(); err != nil {
		return
	}
	b, _ := json.Marshal(mr.MetricDefs)
	res = string(b)
	return
}

// UpdateMetric updates the stored metric information
func (uiapi uiapihandler) UpdateMetric(name string, params []byte) error {
	var m metrics.Metric
	err := json.Unmarshal(params, &m)
	if err != nil {
		return err
	}
	return metricsReaderWriter.UpdateMetric(name, m)
}

// DeleteMetric removes the metric from the configuration
func (uiapi uiapihandler) DeleteMetric(name string) error {
	return metricsReaderWriter.DeleteMetric(name)
}

// GetDatabases returns the list of monitored databases
func (uiapi uiapihandler) GetDatabases() (res string, err error) {
	var dbs sources.MonitoredDatabases
	if dbs, err = sourcesReader.GetMonitoredDatabases(); err != nil {
		return
	}
	b, _ := json.Marshal(dbs)
	res = string(b)
	return
}

// DeleteDatabase removes the database from the list of monitored databases
func (uiapi uiapihandler) DeleteDatabase(database string) error {
	return sourcesReader.DeleteDatabase(database)
}

// UpdateDatabase updates the monitored database information
func (uiapi uiapihandler) UpdateDatabase(database string, params []byte) error {
	var md sources.MonitoredDatabase
	err := json.Unmarshal(params, &md)
	if err != nil {
		return err
	}
	return sourcesReader.UpdateDatabase(database, md)
}

// GetStats
func (uiapi uiapihandler) GetStats() string {
	jsonResponseTemplate := `{
		"main": {
			"version": "%s",
			"dbSchema": "%s",
			"commit": "%s",
			"built": "%s"
		},
		"metrics": {
			"totalMetricsFetchedCounter": %d,
			"totalMetricsReusedFromCacheCounter": %d,
			"metricPointsPerMinuteLast5MinAvg": %v,
			"metricsDropped": %d,
			"totalMetricFetchFailuresCounter": %d
		},
		"datastore": {
			"secondsFromLastSuccessfulDatastoreWrite": %d,
			"datastoreWriteFailuresCounter": %d,
			"datastoreSuccessfulWritesCounter": %d,
			"datastoreAvgSuccessfulWriteTimeMillis": %.1f
		},
		"general": {
			"totalDatasetsFetchedCounter": %d,
			"databasesMonitored": %d,
			"databasesConfigured": %d,
			"unreachableDBs": %d,
			"gathererUptimeSeconds": %d
		}
	}`

	secondsFromLastSuccessfulDatastoreWrite := atomic.LoadInt64(&lastSuccessfulDatastoreWriteTimeEpoch)
	totalMetrics := atomic.LoadUint64(&totalMetricsFetchedCounter)
	cacheMetrics := atomic.LoadUint64(&totalMetricsReusedFromCacheCounter)
	totalDatasets := atomic.LoadUint64(&totalDatasetsFetchedCounter)
	metricsDropped := atomic.LoadUint64(&totalMetricsDroppedCounter)
	metricFetchFailuresCounter := atomic.LoadUint64(&totalMetricFetchFailuresCounter)
	datastoreFailures := atomic.LoadUint64(&datastoreWriteFailuresCounter)
	datastoreSuccess := atomic.LoadUint64(&datastoreWriteSuccessCounter)
	datastoreTotalTimeMicros := atomic.LoadUint64(&datastoreTotalWriteTimeMicroseconds) // successful writes only
	var datastoreAvgSuccessfulWriteTimeMillis float64
	if datastoreSuccess != 0 {
		datastoreAvgSuccessfulWriteTimeMillis = float64(datastoreTotalTimeMicros) / float64(datastoreSuccess) / 1000.0
	} else {
		datastoreAvgSuccessfulWriteTimeMillis = 0
	}
	gathererUptimeSeconds := uint64(time.Since(gathererStartTime).Seconds())
	metricPointsPerMinute := atomic.LoadInt64(&metricPointsPerMinuteLast5MinAvg)
	if metricPointsPerMinute == -1 { // calculate avg. on the fly if 1st summarization hasn't happened yet
		metricPointsPerMinute = int64((totalMetrics * 60) / gathererUptimeSeconds)
	}
	monitoredDbs := getMonitoredDatabasesSnapshot()
	databasesConfigured := len(monitoredDbs) // including replicas
	databasesMonitored := 0
	for _, md := range monitoredDbs {
		if shouldDbBeMonitoredBasedOnCurrentState(md) {
			databasesMonitored++
		}
	}
	unreachableDBsLock.RLock()
	unreachableDBs := len(unreachableDB)
	unreachableDBsLock.RUnlock()
	return fmt.Sprintf(jsonResponseTemplate, version, dbapi, commit, date,
		totalMetrics, cacheMetrics, metricPointsPerMinute, metricsDropped,
		metricFetchFailuresCounter, time.Now().Unix()-secondsFromLastSuccessfulDatastoreWrite,
		datastoreFailures, datastoreSuccess, datastoreAvgSuccessfulWriteTimeMillis,
		totalDatasets, databasesMonitored, databasesConfigured, unreachableDBs,
		gathererUptimeSeconds)
}
