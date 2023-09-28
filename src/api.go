package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/db"
	"golang.org/x/exp/slices"
)

type uiapihandler struct{}

var uiapi uiapihandler

func (uiapi uiapihandler) TryConnectToDB(params []byte) (err error) {
	return db.TryDatabaseConnection(context.TODO(), string(params))
}

// AddPreset adds the preset to the list of available presets
func (uiapi uiapihandler) AddPreset(params []byte) error {
	sql := `INSERT INTO pgwatch3.preset_config(pc_name, pc_description, pc_config) VALUES ($1, $2, $3)`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		config, _ := json.Marshal(m["pc_config"])
		_, err = configDb.Exec(context.TODO(), sql, m["pc_name"], m["pc_description"], config)
	}
	return err
}

// UpdatePreset updates the stored preset
func (uiapi uiapihandler) UpdatePreset(id string, params []byte) error {
	fields, values, err := paramsToFieldValues("pgwatch3.preset_config", params)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(`UPDATE pgwatch3.preset_config SET %s WHERE pc_name = $1`,
		strings.Join(fields, ","))
	values = append([]any{id}, values...)
	_, err = configDb.Exec(context.TODO(), sql, values...)
	return err
}

// GetPresets ret	urns the list of available presets
func (uiapi uiapihandler) GetPresets() (res string, err error) {
	sql := `select coalesce(jsonb_agg(to_jsonb(p)), '[]') from pgwatch3.preset_config p`
	err = configDb.QueryRow(context.TODO(), sql).Scan(&res)
	return
}

// DeletePreset removes the preset from the configuration
func (uiapi uiapihandler) DeletePreset(name string) error {
	_, err := configDb.Exec(context.TODO(), "DELETE FROM pgwatch3.preset_config WHERE pc_name = $1", name)
	return err
}

// GetMetrics returns the list of metrics
func (uiapi uiapihandler) GetMetrics() (res string, err error) {
	sql := `select coalesce(jsonb_agg(to_jsonb(m)), '[]') from metric m`
	err = configDb.QueryRow(context.TODO(), sql).Scan(&res)
	return
}

// DeleteMetric removes the metric from the configuration
func (uiapi uiapihandler) DeleteMetric(id int) error {
	_, err := configDb.Exec(context.TODO(), "DELETE FROM pgwatch3.metric WHERE m_id = $1", id)
	return err
}

// AddMetric adds the metric to the list of stored metrics
func (uiapi uiapihandler) AddMetric(params []byte) error {
	sql := `INSERT INTO pgwatch3.metric(
m_name, m_pg_version_from, m_sql, m_comment, m_is_active, m_is_helper, 
m_master_only, m_standby_only, m_column_attrs, m_sql_su)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		_, err = configDb.Exec(context.TODO(), sql, m["m_name"], m["m_pg_version_from"],
			m["m_sql"], m["m_comment"], m["m_is_active"],
			m["m_is_helper"], m["m_master_only"], m["m_standby_only"],
			m["m_column_attrs"], m["m_sql_su"])
	}
	return err
}

// GetDatabases returns the list of monitored databases
func (uiapi uiapihandler) GetDatabases() (res string, err error) {
	sql := `select coalesce(jsonb_agg(to_jsonb(db)), '[]') from monitored_db db`
	err = configDb.QueryRow(context.TODO(), sql).Scan(&res)
	return
}

// DeleteDatabase removes the database from the list of monitored databases
func (uiapi uiapihandler) DeleteDatabase(database string) error {
	_, err := configDb.Exec(context.TODO(), "DELETE FROM pgwatch3.monitored_db WHERE md_unique_name = $1", database)
	return err
}

// AddDatabase adds the database to the list of monitored databases
func (uiapi uiapihandler) AddDatabase(params []byte) error {
	sql := `INSERT INTO pgwatch3.monitored_db(
md_unique_name, md_preset_config_name, md_config, md_hostname, 
md_port, md_dbname, md_user, md_password, md_is_superuser, md_is_enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	var m map[string]any
	err := json.Unmarshal(params, &m)
	if err == nil {
		_, err = configDb.Exec(context.TODO(), sql, m["md_unique_name"], m["md_preset_config_name"],
			m["md_config"], m["md_hostname"], m["md_port"],
			m["md_dbname"], m["md_user"], m["md_password"],
			m["md_is_superuser"], m["md_is_enabled"])
	}
	return err
}

// UpdateMetric updates the stored metric information
func (uiapi uiapihandler) UpdateMetric(id int, params []byte) error {
	fields, values, err := paramsToFieldValues("pgwatch3.metric", params)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(`UPDATE pgwatch3.metric SET %s WHERE m_id = $1`, strings.Join(fields, ","))
	values = append([]any{id}, values...)
	_, err = configDb.Exec(context.TODO(), sql, values...)
	return err
}

// UpdateDatabase updates the monitored database information
func (uiapi uiapihandler) UpdateDatabase(database string, params []byte) error {
	fields, values, err := paramsToFieldValues("pgwatch3.monitored_db", params)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(`UPDATE pgwatch3.monitored_db SET %s WHERE md_unique_name = $1`, strings.Join(fields, ","))
	values = append([]any{database}, values...)
	_, err = configDb.Exec(context.TODO(), sql, values...)
	return err
}

// paramsToFieldValues tranforms JSON body into arrays to be used in UPDATE statement
func paramsToFieldValues(table string, params []byte) (fields []string, values []any, err error) {
	var (
		paramsMap map[string]any
		cols      []string
	)
	if err = json.Unmarshal(params, &paramsMap); err != nil {
		return
	}
	if cols, err = db.GetTableColumns(context.TODO(), configDb, table); err != nil {
		return
	}
	i := 2 // start with the second parameter number, first is reserved for WHERE key value
	for k, v := range paramsMap {
		if slices.Index(cols, k) == -1 {
			continue
		}
		fields = append(fields, fmt.Sprintf("%s = $%d", quoteIdent(k), i))
		if reflect.ValueOf(v).Kind() == reflect.Map { // transform into json key-value
			v, _ = json.Marshal(v)
		}
		values = append(values, v)
		i++
	}
	return
}

func quoteIdent(s string) string {
	return `"` + strings.Replace(s, `"`, `""`, -1) + `"`
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
	datastoreAvgSuccessfulWriteTimeMillis := float64(datastoreTotalTimeMicros) / float64(datastoreSuccess) / 1000.0
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
