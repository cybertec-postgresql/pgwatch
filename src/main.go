package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/psutil"
	"github.com/cybertec-postgresql/pgwatch3/sinks"
	"github.com/cybertec-postgresql/pgwatch3/sources"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type ControlMessage struct {
	Action string // START, STOP, PAUSE
	Config map[string]float64
}

type MetricFetchMessage struct {
	DBUniqueName        string
	DBUniqueNameOrig    string
	MetricName          string
	Source              sources.Kind
	Interval            time.Duration
	CreatedOn           time.Time
	StmtTimeoutOverride int64
}

type ChangeDetectionResults struct { // for passing around DDL/index/config change detection results
	Created int
	Altered int
	Dropped int
}

type DBVersionMapEntry struct {
	LastCheckedOn    time.Time
	IsInRecovery     bool
	Version          uint
	VersionStr       string
	RealDbname       string
	SystemIdentifier string
	IsSuperuser      bool // if true and no helpers are installed, use superuser SQL version of metric if available
	Extensions       map[string]uint
	ExecEnv          string
	ApproxDBSizeB    int64
}

type ExistingPartitionInfo struct {
	StartTime time.Time
	EndTime   time.Time
}

const (
	epochColumnName                 string        = "epoch_ns" // this column (epoch in nanoseconds) is expected in every metric query
	tagPrefix                       string        = "tag_"
	metricDefinitionRefreshInterval time.Duration = time.Minute * 2 // min time before checking for new/changed metric definitions
	persistQueueMaxSize                           = 10000           // storage queue max elements. when reaching the limit, older metrics will be dropped.
)

const (
	gathererStatusStart     = "START"
	gathererStatusStop      = "STOP"
	metricdbIdent           = "metricDb"
	configdbIdent           = "configDb"
	contextPrometheusScrape = "prometheus-scrape"

	monitoredDbsDatastoreSyncIntervalSeconds = 600              // write actively monitored DBs listing to metrics store after so many seconds
	monitoredDbsDatastoreSyncMetricName      = "configured_dbs" // FYI - for Postgres datastore there's also the admin.all_unique_dbnames table with all recent DB unique names with some metric data
	recoPrefix                               = "reco_"          // special handling for metrics with such prefix, data stored in RECO_METRIC_NAME
	recoMetricName                           = "recommendations"
	specialMetricChangeEvents                = "change_events"
	specialMetricServerLogEventCounts        = "server_log_event_counts"
	specialMetricPgbouncer                   = "^pgbouncer_(stats|pools)$"
	specialMetricPgpoolStats                 = "pgpool_stats"
	specialMetricInstanceUp                  = "instance_up"
	specialMetricDbSize                      = "db_size"     // can be transparently switched to db_size_approx on instances with very slow FS access (Azure Single Server)
	specialMetricTableStats                  = "table_stats" // can be transparently switched to table_stats_approx on instances with very slow FS (Azure Single Server)
	metricCPULoad                            = "cpu_load"
	metricPsutilCPU                          = "psutil_cpu"
	metricPsutilDisk                         = "psutil_disk"
	metricPsutilDiskIoTotal                  = "psutil_disk_io_total"
	metricPsutilMem                          = "psutil_mem"

	dbSizeCachingInterval = 30 * time.Minute
	dbMetricJoinStr       = "¤¤¤" // just some unlikely string for a DB name to avoid using maps of maps for DB+metric data
	execEnvUnknown        = "UNKNOWN"
	execEnvAzureSingle    = "AZURE_SINGLE"
	execEnvAzureFlexible  = "AZURE_FLEXIBLE"
	execEnvGoogle         = "GOOGLE"
)

var specialMetrics = map[string]bool{recoMetricName: true, specialMetricChangeEvents: true, specialMetricServerLogEventCounts: true}
var directlyFetchableOSMetrics = map[string]bool{metricPsutilCPU: true, metricPsutilDisk: true, metricPsutilDiskIoTotal: true, metricPsutilMem: true, metricCPULoad: true}
var metricDefinitionMap metrics.MetricVersionDefs
var metricDefMapLock = sync.RWMutex{}
var hostMetricIntervalMap = make(map[string]float64) // [db1_metric] = 30
var dbPgVersionMap = make(map[string]DBVersionMapEntry)
var dbPgVersionMapLock = sync.RWMutex{}
var dbGetPgVersionMapLock = make(map[string]*sync.RWMutex) // synchronize initial PG version detection to 1 instance for each defined host
var monitoredDbCache map[string]sources.MonitoredDatabase
var monitoredDbCacheLock sync.RWMutex

var monitoredDbConnCacheLock = sync.RWMutex{}
var lastSQLFetchError sync.Map

var fileBasedMetrics = false

// / internal statistics calculation
var lastSuccessfulDatastoreWriteTimeEpoch int64
var datastoreWriteFailuresCounter uint64
var datastoreWriteSuccessCounter uint64
var totalMetricFetchFailuresCounter uint64
var datastoreTotalWriteTimeMicroseconds uint64
var totalMetricsFetchedCounter uint64
var totalMetricsReusedFromCacheCounter uint64
var totalMetricsDroppedCounter uint64
var totalDatasetsFetchedCounter uint64
var metricPointsPerMinuteLast5MinAvg int64 = -1 // -1 means the summarization ticker has not yet run
var gathererStartTime = time.Now()

var PGDummyMetricTables = make(map[string]time.Time)
var PGDummyMetricTablesLock = sync.RWMutex{}
var failedInitialConnectHosts = make(map[string]bool) // hosts that couldn't be connected to even once

var lastMonitoredDBsUpdate time.Time
var instanceMetricCache = make(map[string](metrics.Measurements)) // [dbUnique+metric]lastly_fetched_data
var instanceMetricCacheLock = sync.RWMutex{}
var instanceMetricCacheTimestamp = make(map[string]time.Time) // [dbUnique+metric]last_fetch_time
var instanceMetricCacheTimestampLock = sync.RWMutex{}
var MinExtensionInfoAvailable uint = 901
var regexIsAlpha = regexp.MustCompile("^[a-zA-Z]+$")
var rBouncerAndPgpoolVerMatch = regexp.MustCompile(`\d+\.+\d+`) // extract $major.minor from "4.1.2 (karasukiboshi)" or "PgBouncer 1.12.0"
var regexIsPgbouncerMetrics = regexp.MustCompile(specialMetricPgbouncer)
var unreachableDBsLock sync.RWMutex
var unreachableDB = make(map[string]time.Time)
var pgBouncerNumericCountersStartVersion uint // pgBouncer changed internal counters data type in v1.12

var lastDBSizeMB = make(map[string]int64)
var lastDBSizeFetchTime = make(map[string]time.Time) // cached for DB_SIZE_CACHING_INTERVAL
var lastDBSizeCheckLock sync.RWMutex

var prevLoopMonitoredDBs []sources.MonitoredDatabase // to be able to detect DBs removed from config
var undersizedDBs = make(map[string]bool)            // DBs below the --min-db-size-mb limit, if set
var undersizedDBsLock = sync.RWMutex{}
var recoveryIgnoredDBs = make(map[string]bool) // DBs in recovery state and OnlyIfMaster specified in config
var recoveryIgnoredDBsLock = sync.RWMutex{}

var logger log.LoggerHookerIface

// VersionToInt parses a given version and returns an integer  or
// an error if unable to parse the version. Only parses valid semantic versions.
// Performs checking that can find errors within the version.
// Examples: v1.2 -> 0102, v9.6.3 -> 0906, v11 -> 1100
func VersionToInt(v string) uint {
	if len(v) == 0 {
		return 0
	}
	var major int
	var minor int

	matches := regexp.MustCompile(`(\d+).?(\d*)`).FindStringSubmatch(v)
	if len(matches) == 0 {
		return 0
	}
	if len(matches) > 1 {
		major, _ = strconv.Atoi(matches[1])
		if len(matches) > 2 {
			minor, _ = strconv.Atoi(matches[2])
		}
	}
	return uint(major*100 + minor)
}

func RestoreSQLConnPoolLimitsForPreviouslyDormantDB(dbUnique string) {
	if !opts.UseConnPooling {
		return
	}
	monitoredDbConnCacheLock.Lock()
	defer monitoredDbConnCacheLock.Unlock()

	conn, ok := monitoredDbConnCache[dbUnique]
	if !ok || conn == nil {
		logger.Error("DB conn to re-instate pool limits not found, should not happen")
		return
	}

	logger.Debugf("[%s] Re-instating SQL connection pool max connections ...", dbUnique)

	// conn.SetMaxIdleConns(opts.MaxParallelConnectionsPerDb)
	// conn.SetMaxOpenConns(opts.MaxParallelConnectionsPerDb)

}

func InitPGVersionInfoFetchingLockIfNil(md sources.MonitoredDatabase) {
	dbPgVersionMapLock.Lock()
	if _, ok := dbGetPgVersionMapLock[md.DBUniqueName]; !ok {
		dbGetPgVersionMapLock[md.DBUniqueName] = &sync.RWMutex{}
	}
	dbPgVersionMapLock.Unlock()
}

func GetMonitoredDatabaseByUniqueName(name string) (sources.MonitoredDatabase, error) {
	monitoredDbCacheLock.RLock()
	defer monitoredDbCacheLock.RUnlock()
	_, exists := monitoredDbCache[name]
	if !exists {
		return sources.MonitoredDatabase{}, errors.New("DBUnique not found")
	}
	return monitoredDbCache[name], nil
}

func UpdateMonitoredDBCache(data []sources.MonitoredDatabase) {
	monitoredDbCacheNew := make(map[string]sources.MonitoredDatabase)

	for _, row := range data {
		monitoredDbCacheNew[row.DBUniqueName] = row
	}

	monitoredDbCacheLock.Lock()
	monitoredDbCache = monitoredDbCacheNew
	monitoredDbCacheLock.Unlock()
}

type UIntSlice []uint

func (x UIntSlice) Len() int           { return len(x) }
func (x UIntSlice) Less(i, j int) bool { return x[i] < x[j] }
func (x UIntSlice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// assumes upwards compatibility for versions
func GetMetricVersionProperties(metric string, vme DBVersionMapEntry, metricDefMap map[string]map[uint]metrics.MetricProperties) (metrics.MetricProperties, error) {
	var keys UIntSlice
	var mdm metrics.MetricVersionDefs

	if metricDefMap != nil {
		mdm = metricDefMap
	} else {
		metricDefMapLock.RLock()
		mdm = deepCopyMetricDefinitionMap(metricDefinitionMap) // copy of global cache
		metricDefMapLock.RUnlock()
	}

	_, ok := mdm[metric]
	if !ok || len(mdm[metric]) == 0 {
		logger.Debug("metric", metric, "not found")
		return metrics.MetricProperties{}, errors.New("metric SQL not found")
	}

	for k := range mdm[metric] {
		keys = append(keys, k)
	}

	sort.Sort(keys)

	var bestVer uint
	var minVer uint
	var found bool
	for _, ver := range keys {
		if vme.Version >= ver {
			bestVer = ver
			found = true
		}
		if minVer == 0 || ver < minVer {
			minVer = ver
		}
	}

	if !found {
		if vme.Version < minVer { // metric not yet available for given PG ver
			return metrics.MetricProperties{}, fmt.Errorf("no suitable SQL found for metric \"%s\", server version \"%s\" too old. min defined SQL ver: %d", metric, vme.VersionStr, minVer)
		}
		return metrics.MetricProperties{}, fmt.Errorf("no suitable SQL found for metric \"%s\", version \"%s\"", metric, vme.VersionStr)
	}

	ret := mdm[metric][bestVer]

	// check if SQL def. override defined for some specific extension version and replace the metric SQL-s if so
	if ret.MetricAttrs.ExtensionVersionOverrides != nil && len(ret.MetricAttrs.ExtensionVersionOverrides) > 0 {
		if vme.Extensions != nil && len(vme.Extensions) > 0 {
			logger.Debugf("[%s] extension version based override request found: %+v", metric, ret.MetricAttrs.ExtensionVersionOverrides)
			for _, extOverride := range ret.MetricAttrs.ExtensionVersionOverrides {
				var matching = true
				for _, extVer := range extOverride.ExpectedExtensionVersions { // "natural" sorting of metric definition assumed
					installedExtVer, ok := vme.Extensions[extVer.ExtName]
					if !ok || installedExtVer < VersionToInt(extVer.ExtMinVersion) {
						matching = false
					}
				}
				if matching { // all defined extensions / versions (if many) need to match
					_, ok := mdm[extOverride.TargetMetric]
					if !ok {
						logger.Warningf("extension based override metric not found for metric %s. substitute metric name: %s", metric, extOverride.TargetMetric)
						continue
					}
					mvp, err := GetMetricVersionProperties(extOverride.TargetMetric, vme, mdm)
					if err != nil {
						logger.Warningf("undefined extension based override for metric %s, substitute metric name: %s, version: %s not found", metric, extOverride.TargetMetric, bestVer)
						continue
					}
					logger.Debugf("overriding metric %s based on the extension_version_based_overrides metric attribute with %s:%s", metric, extOverride.TargetMetric, bestVer)
					if mvp.SQL != "" {
						ret.SQL = mvp.SQL
					}
					if mvp.SQLSU != "" {
						ret.SQLSU = mvp.SQLSU
					}
				}
			}
		}
	}
	return ret, nil
}

func GetAllRecoMetricsForVersion(vme DBVersionMapEntry) map[string]metrics.MetricProperties {
	mvpMap := make(map[string]metrics.MetricProperties)
	metricDefMapLock.RLock()
	defer metricDefMapLock.RUnlock()
	for m := range metricDefinitionMap {
		if strings.HasPrefix(m, recoPrefix) {
			mvp, err := GetMetricVersionProperties(m, vme, metricDefinitionMap)
			if err != nil {
				logger.Warningf("Could not get SQL definition for metric \"%s\", PG %s", m, vme.VersionStr)
			} else if !mvp.MetricAttrs.IsPrivate {
				mvpMap[m] = mvp
			}
		}
	}
	return mvpMap
}

func GetRecommendations(dbUnique string, vme DBVersionMapEntry) (metrics.Measurements, error) {
	retData := make(metrics.Measurements, 0)
	startTimeEpochNs := time.Now().UnixNano()

	recoMetrics := GetAllRecoMetricsForVersion(vme)
	logger.Debugf("Processing %d recommendation metrics for \"%s\"", len(recoMetrics), dbUnique)

	for m, mvp := range recoMetrics {
		data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
		if err != nil {
			if strings.Contains(err.Error(), "does not exist") { // some more exotic extensions missing is expected, don't pollute the error log
				logger.Infof("[%s:%s] Could not execute recommendations SQL: %v", dbUnique, m, err)
			} else {
				logger.Errorf("[%s:%s] Could not execute recommendations SQL: %v", dbUnique, m, err)
			}
			continue
		}
		for _, d := range data {
			d[epochColumnName] = startTimeEpochNs
			d["major_ver"] = vme.Version / 10
			retData = append(retData, d)
		}
	}
	if len(retData) == 0 { // insert a dummy entry minimally so that Grafana can show at least a dropdown
		dummy := make(metrics.Measurement)
		dummy["tag_reco_topic"] = "dummy"
		dummy["tag_object_name"] = "-"
		dummy["recommendation"] = "no recommendations"
		dummy[epochColumnName] = startTimeEpochNs
		dummy["major_ver"] = vme.Version / 10
		retData = append(retData, dummy)
	}
	return retData, nil
}

func FilterPgbouncerData(data metrics.Measurements, databaseToKeep string, vme DBVersionMapEntry) metrics.Measurements {
	filteredData := make(metrics.Measurements, 0)

	for _, dr := range data {
		//log.Debugf("bouncer dr: %+v", dr)
		if _, ok := dr["database"]; !ok {
			logger.Warning("Expected 'database' key not found from pgbouncer_stats, not storing data")
			continue
		}
		if (len(databaseToKeep) > 0 && dr["database"] != databaseToKeep) || dr["database"] == "pgbouncer" { // always ignore the internal 'pgbouncer' DB
			logger.Debugf("Skipping bouncer stats for pool entry %v as not the specified DBName of %s", dr["database"], databaseToKeep)
			continue // and all others also if a DB / pool name was specified in config
		}

		dr["tag_database"] = dr["database"] // support multiple databases / pools via tags if DbName left empty
		delete(dr, "database")              // remove the original pool name

		if vme.Version >= pgBouncerNumericCountersStartVersion { // v1.12 counters are of type numeric instead of int64
			for k, v := range dr {
				if k == "tag_database" {
					continue
				}
				decimalCounter, err := decimal.NewFromString(string(v.([]uint8)))
				if err != nil {
					logger.Errorf("Could not parse \"%+v\" to Decimal: %s", string(v.([]uint8)), err)
					return filteredData
				}
				dr[k] = decimalCounter.IntPart() // technically could cause overflow...but highly unlikely for 2^63
			}
		}
		filteredData = append(filteredData, dr)
	}

	return filteredData
}

func FetchMetrics(ctx context.Context, msg MetricFetchMessage, hostState map[string]map[string]string, storageCh chan<- []metrics.MeasurementMessage, context string) ([]metrics.MeasurementMessage, error) {
	var vme DBVersionMapEntry
	var dbpgVersion uint
	var err, firstErr error
	var sql string
	var retryWithSuperuserSQL = true
	var data, cachedData metrics.Measurements
	var md sources.MonitoredDatabase
	var fromCache, isCacheable bool

	vme, err = DBGetPGVersion(ctx, msg.DBUniqueName, msg.Source, false)
	if err != nil {
		logger.Error("failed to fetch pg version for ", msg.DBUniqueName, msg.MetricName, err)
		return nil, err
	}
	if msg.MetricName == specialMetricDbSize || msg.MetricName == specialMetricTableStats {
		if vme.ExecEnv == execEnvAzureSingle && vme.ApproxDBSizeB > 1e12 { // 1TB
			subsMetricName := msg.MetricName + "_approx"
			mvpApprox, err := GetMetricVersionProperties(subsMetricName, vme, nil)
			if err == nil && mvpApprox.MetricAttrs.MetricStorageName == msg.MetricName {
				logger.Infof("[%s:%s] Transparently swapping metric to %s due to hard-coded rules...", msg.DBUniqueName, msg.MetricName, subsMetricName)
				msg.MetricName = subsMetricName
			}
		}
	}
	dbpgVersion = vme.Version

	if msg.Source == sources.SourcePgBouncer {
		dbpgVersion = 0 // version is 0.0 for all pgbouncer sql per convention
	}

	mvp, err := GetMetricVersionProperties(msg.MetricName, vme, nil)
	if err != nil && msg.MetricName != recoMetricName {
		epoch, ok := lastSQLFetchError.Load(msg.MetricName + dbMetricJoinStr + fmt.Sprintf("%v", dbpgVersion))
		if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
			logger.Infof("Failed to get SQL for metric '%s', version '%s': %v", msg.MetricName, vme.VersionStr, err)
			lastSQLFetchError.Store(msg.MetricName+dbMetricJoinStr+fmt.Sprintf("%v", dbpgVersion), time.Now().Unix())
		}
		if strings.Contains(err.Error(), "too old") {
			return nil, nil
		}
		return nil, err
	}

	isCacheable = IsCacheableMetric(msg, mvp)
	if isCacheable && opts.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(opts.InstanceLevelCacheMaxSeconds) {
		cachedData = GetFromInstanceCacheIfNotOlderThanSeconds(msg, opts.InstanceLevelCacheMaxSeconds)
		if len(cachedData) > 0 {
			fromCache = true
			goto send_to_storageChannel
		}
	}

retry_with_superuser_sql: // if 1st fetch with normal SQL fails, try with SU SQL if it's defined

	sql = mvp.SQL

	if opts.Metric.NoHelperFunctions && mvp.CallsHelperFunctions && mvp.SQLSU != "" {
		logger.Debugf("[%s:%s] Using SU SQL instead of normal one due to --no-helper-functions input", msg.DBUniqueName, msg.MetricName)
		sql = mvp.SQLSU
		retryWithSuperuserSQL = false
	}

	if (vme.IsSuperuser || (retryWithSuperuserSQL && firstErr != nil)) && mvp.SQLSU != "" {
		sql = mvp.SQLSU
		retryWithSuperuserSQL = false
	}
	if sql == "" && !(msg.MetricName == specialMetricChangeEvents || msg.MetricName == recoMetricName) {
		// let's ignore dummy SQL-s
		logger.Debugf("[%s:%s] Ignoring fetch message - got an empty/dummy SQL string", msg.DBUniqueName, msg.MetricName)
		return nil, nil
	}

	if (mvp.MasterOnly && vme.IsInRecovery) || (mvp.StandbyOnly && !vme.IsInRecovery) {
		logger.Debugf("[%s:%s] Skipping fetching of  as server not in wanted state (IsInRecovery=%v)", msg.DBUniqueName, msg.MetricName, vme.IsInRecovery)
		return nil, nil
	}

	if msg.MetricName == specialMetricChangeEvents && context != contextPrometheusScrape { // special handling, multiple queries + stateful
		CheckForPGObjectChangesAndStore(msg.DBUniqueName, vme, storageCh, hostState) // TODO no hostState for Prometheus currently
	} else if msg.MetricName == recoMetricName && context != contextPrometheusScrape {
		if data, err = GetRecommendations(msg.DBUniqueName, vme); err != nil {
			return nil, err
		}
	} else if msg.Source == sources.SourcePgPool {
		if data, err = FetchMetricsPgpool(msg, vme, mvp); err != nil {
			return nil, err
		}
	} else {
		data, err = DBExecReadByDbUniqueName(mainContext, msg.DBUniqueName, sql)

		if err != nil {
			// let's soften errors to "info" from functions that expect the server to be a primary to reduce noise
			if strings.Contains(err.Error(), "recovery is in progress") {
				dbPgVersionMapLock.RLock()
				ver := dbPgVersionMap[msg.DBUniqueName]
				dbPgVersionMapLock.RUnlock()
				if ver.IsInRecovery {
					logger.Debugf("[%s:%s] failed to fetch metrics: %s", msg.DBUniqueName, msg.MetricName, err)
					return nil, err
				}
			}

			if msg.MetricName == specialMetricInstanceUp {
				logger.WithError(err).Debugf("[%s:%s] failed to fetch metrics. marking instance as not up", msg.DBUniqueName, msg.MetricName)
				data = make(metrics.Measurements, 1)
				data[0] = metrics.Measurement{"epoch_ns": time.Now().UnixNano(), "is_up": 0} // should be updated if the "instance_up" metric definition is changed
				goto send_to_storageChannel
			}

			if strings.Contains(err.Error(), "connection refused") {
				SetDBUnreachableState(msg.DBUniqueName)
			}

			if retryWithSuperuserSQL && mvp.SQLSU != "" {
				firstErr = err
				logger.Infof("[%s:%s] Normal fetch failed, re-trying to fetch with SU SQL", msg.DBUniqueName, msg.MetricName)
				goto retry_with_superuser_sql
			}
			if firstErr != nil {
				logger.WithField("database", msg.DBUniqueName).WithField("metric", msg.MetricName).Error(err)
				return nil, firstErr // returning the initial error
			}
			logger.Infof("[%s:%s] failed to fetch metrics: %s", msg.DBUniqueName, msg.MetricName, err)

			return nil, err
		}
		md, err = GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
		if err != nil {
			logger.Errorf("[%s:%s] could not get monitored DB details", msg.DBUniqueName, err)
			return nil, err
		}

		logger.WithFields(map[string]any{"database": msg.DBUniqueName, "metric": msg.MetricName, "rows": len(data)}).Info("measurements fetched")
		if regexIsPgbouncerMetrics.MatchString(msg.MetricName) { // clean unwanted pgbouncer pool stats here as not possible in SQL
			data = FilterPgbouncerData(data, md.GetDatabaseName(), vme)
		}

		ClearDBUnreachableStateIfAny(msg.DBUniqueName)

	}

	if isCacheable && opts.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(opts.InstanceLevelCacheMaxSeconds) {
		PutToInstanceCache(msg, data)
	}

send_to_storageChannel:

	if (opts.Metric.RealDbnameField > "" || opts.Metric.SystemIdentifierField > "") && msg.Source == sources.SourcePostgres {
		dbPgVersionMapLock.RLock()
		ver := dbPgVersionMap[msg.DBUniqueName]
		dbPgVersionMapLock.RUnlock()
		data = AddDbnameSysinfoIfNotExistsToQueryResultData(msg, data, ver)
	}

	if mvp.MetricAttrs.MetricStorageName != "" {
		logger.Debugf("[%s] rerouting metric %s data to %s based on metric attributes", msg.DBUniqueName, msg.MetricName, mvp.MetricAttrs.MetricStorageName)
		msg.MetricName = mvp.MetricAttrs.MetricStorageName
	}
	if fromCache {
		md, err = GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
		if err != nil {
			logger.Errorf("[%s:%s] could not get monitored DB details", msg.DBUniqueName, err)
			return nil, err
		}
		logger.Infof("[%s:%s] loaded %d rows from the instance cache", msg.DBUniqueName, msg.MetricName, len(cachedData))
		atomic.AddUint64(&totalMetricsReusedFromCacheCounter, uint64(len(cachedData)))
		return []metrics.MeasurementMessage{{DBName: msg.DBUniqueName, MetricName: msg.MetricName, Data: cachedData, CustomTags: md.CustomTags,
			MetricDef: mvp, RealDbname: vme.RealDbname, SystemIdentifier: vme.SystemIdentifier}}, nil
	}
	atomic.AddUint64(&totalMetricsFetchedCounter, uint64(len(data)))
	return []metrics.MeasurementMessage{{DBName: msg.DBUniqueName, MetricName: msg.MetricName, Data: data, CustomTags: md.CustomTags,
		MetricDef: mvp, RealDbname: vme.RealDbname, SystemIdentifier: vme.SystemIdentifier}}, nil

}

func SetDBUnreachableState(dbUnique string) {
	unreachableDBsLock.Lock()
	unreachableDB[dbUnique] = time.Now()
	unreachableDBsLock.Unlock()
}

func ClearDBUnreachableStateIfAny(dbUnique string) {
	unreachableDBsLock.Lock()
	delete(unreachableDB, dbUnique)
	unreachableDBsLock.Unlock()
}

func GetFromInstanceCacheIfNotOlderThanSeconds(msg MetricFetchMessage, maxAgeSeconds int64) metrics.Measurements {
	var clonedData metrics.Measurements
	instanceMetricCacheTimestampLock.RLock()
	instanceMetricTS, ok := instanceMetricCacheTimestamp[msg.DBUniqueNameOrig+msg.MetricName]
	instanceMetricCacheTimestampLock.RUnlock()
	if !ok {
		//log.Debugf("[%s:%s] no instance cache entry", msg.DBUniqueNameOrig, msg.MetricName)
		return nil
	}

	if time.Now().Unix()-instanceMetricTS.Unix() > maxAgeSeconds {
		//log.Debugf("[%s:%s] instance cache entry too old", msg.DBUniqueNameOrig, msg.MetricName)
		return nil
	}

	logger.Debugf("[%s:%s] reading metric data from instance cache of \"%s\"", msg.DBUniqueName, msg.MetricName, msg.DBUniqueNameOrig)
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

func PutToInstanceCache(msg MetricFetchMessage, data metrics.Measurements) {
	if len(data) == 0 {
		return
	}
	dataCopy := deepCopyMetricData(data)
	logger.Debugf("[%s:%s] filling instance cache", msg.DBUniqueNameOrig, msg.MetricName)
	instanceMetricCacheLock.Lock()
	instanceMetricCache[msg.DBUniqueNameOrig+msg.MetricName] = dataCopy
	instanceMetricCacheLock.Unlock()

	instanceMetricCacheTimestampLock.Lock()
	instanceMetricCacheTimestamp[msg.DBUniqueNameOrig+msg.MetricName] = time.Now()
	instanceMetricCacheTimestampLock.Unlock()
}

func IsCacheableMetric(msg MetricFetchMessage, mvp metrics.MetricProperties) bool {
	if !(msg.Source == sources.SourcePostgresContinuous || msg.Source == sources.SourcePatroniContinuous) {
		return false
	}
	return mvp.MetricAttrs.IsInstanceLevel
}

func AddDbnameSysinfoIfNotExistsToQueryResultData(msg MetricFetchMessage, data metrics.Measurements, ver DBVersionMapEntry) metrics.Measurements {
	enrichedData := make(metrics.Measurements, 0)

	logger.Debugf("Enriching all rows of [%s:%s] with sysinfo (%s) / real dbname (%s) if set. ", msg.DBUniqueName, msg.MetricName, ver.SystemIdentifier, ver.RealDbname)
	for _, dr := range data {
		if opts.Metric.RealDbnameField > "" && ver.RealDbname > "" {
			old, ok := dr[opts.Metric.RealDbnameField]
			if !ok || old == "" {
				dr[opts.Metric.RealDbnameField] = ver.RealDbname
			}
		}
		if opts.Metric.SystemIdentifierField > "" && ver.SystemIdentifier > "" {
			old, ok := dr[opts.Metric.SystemIdentifierField]
			if !ok || old == "" {
				dr[opts.Metric.SystemIdentifierField] = ver.SystemIdentifier
			}
		}
		enrichedData = append(enrichedData, dr)
	}
	return enrichedData
}

func StoreMetrics(metrics []metrics.MeasurementMessage, storageCh chan<- []metrics.MeasurementMessage) (int, error) {

	if len(metrics) > 0 {
		atomic.AddUint64(&totalDatasetsFetchedCounter, 1)
		storageCh <- metrics
		return len(metrics), nil
	}

	return 0, nil
}

func deepCopyMetricData(data metrics.Measurements) metrics.Measurements {
	newData := make(metrics.Measurements, len(data))

	for i, dr := range data {
		newRow := make(map[string]any)
		for k, v := range dr {
			newRow[k] = v
		}
		newData[i] = newRow
	}

	return newData
}

func deepCopyMetricDefinitionMap(mdm map[string]map[uint]metrics.MetricProperties) map[string]map[uint]metrics.MetricProperties {
	newMdm := make(map[string]map[uint]metrics.MetricProperties)

	for metric, verMap := range mdm {
		newMdm[metric] = make(map[uint]metrics.MetricProperties)
		for ver, mvp := range verMap {
			newMdm[metric][ver] = mvp
		}
	}
	return newMdm
}

// ControlMessage notifies of shutdown + interval change
func MetricGathererLoop(ctx context.Context, dbUniqueName, dbUniqueNameOrig string, srcType sources.Kind, metricName string, configMap map[string]float64, controlCh <-chan ControlMessage, storeCh chan<- []metrics.MeasurementMessage) {
	config := configMap
	interval := config[metricName]
	hostState := make(map[string]map[string]string)
	var lastUptimeS int64 = -1 // used for "server restarted" event detection
	var lastErrorNotificationTime time.Time
	var vme DBVersionMapEntry
	var mvp metrics.MetricProperties
	var err error
	failedFetches := 0
	// metricNameForStorage := metricName
	lastDBVersionFetchTime := time.Unix(0, 0) // check DB ver. ev. 5 min
	var stmtTimeoutOverride int64

	l := logger.WithField("database", dbUniqueName).WithField("metric", metricName)
	if metricName == specialMetricServerLogEventCounts {
		logparseLoop(dbUniqueName, metricName, configMap, controlCh, storeCh) // no return
		return
	}

	for {
		if lastDBVersionFetchTime.Add(time.Minute * time.Duration(5)).Before(time.Now()) {
			vme, err = DBGetPGVersion(ctx, dbUniqueName, srcType, false) // in case of errors just ignore metric "disabled" time ranges
			if err != nil {
				lastDBVersionFetchTime = time.Now()
			}

			mvp, err = GetMetricVersionProperties(metricName, vme, nil)
			if err == nil && mvp.MetricAttrs.StatementTimeoutSeconds > 0 {
				stmtTimeoutOverride = mvp.MetricAttrs.StatementTimeoutSeconds
			} else {
				stmtTimeoutOverride = 0
			}
		}

		if IsMetricCurrentlyDisabledForHost(metricName, vme, dbUniqueName) {
			l.Debugf("Ignoring fetch as metric disabled for current time range")
			continue
		}
		var metricStoreMessages []metrics.MeasurementMessage
		var err error
		mfm := MetricFetchMessage{
			DBUniqueName:        dbUniqueName,
			DBUniqueNameOrig:    dbUniqueNameOrig,
			MetricName:          metricName,
			Source:              srcType,
			Interval:            time.Second * time.Duration(interval),
			StmtTimeoutOverride: stmtTimeoutOverride,
		}

		// 1st try local overrides for some metrics if operating in push mode
		if opts.DirectOSStats && IsDirectlyFetchableMetric(metricName) {
			metricStoreMessages, err = FetchStatsDirectlyFromOS(mfm, vme, mvp)
			if err != nil {
				l.WithError(err).Errorf("Could not reader metric directly from OS")
			}
		}
		t1 := time.Now()
		if metricStoreMessages == nil {
			metricStoreMessages, err = FetchMetrics(ctx, mfm, hostState, storeCh, "")
		}
		t2 := time.Now()

		if t2.Sub(t1) > (time.Second * time.Duration(interval)) {
			l.Warningf("Total fetching time of %vs bigger than %vs interval", t2.Sub(t1).Truncate(time.Millisecond*100).Seconds(), interval)
		}

		if err != nil {
			failedFetches++
			// complain only 1x per 10min per host/metric...
			if lastErrorNotificationTime.IsZero() || lastErrorNotificationTime.Add(time.Second*time.Duration(600)).Before(time.Now()) {
				l.WithError(err).Error("failed to fetch metric data")
				if failedFetches > 1 {
					l.Errorf("Total failed fetches: %d", failedFetches)
				}
				lastErrorNotificationTime = time.Now()
			}
		} else if metricStoreMessages != nil {
			if len(metricStoreMessages[0].Data) > 0 {

				// pick up "server restarted" events here to avoid doing extra selects from CheckForPGObjectChangesAndStore code
				if metricName == "db_stats" {
					postmasterUptimeS, ok := (metricStoreMessages[0].Data)[0]["postmaster_uptime_s"]
					if ok {
						if lastUptimeS != -1 {
							if postmasterUptimeS.(int64) < lastUptimeS { // restart (or possibly also failover when host is routed) happened
								message := "Detected server restart (or failover) of \"" + dbUniqueName + "\""
								l.Warning(message)
								detectedChangesSummary := make(metrics.Measurements, 0)
								entry := metrics.Measurement{"details": message, "epoch_ns": (metricStoreMessages[0].Data)[0]["epoch_ns"]}
								detectedChangesSummary = append(detectedChangesSummary, entry)
								metricStoreMessages = append(metricStoreMessages,
									metrics.MeasurementMessage{
										DBName:     dbUniqueName,
										SourceType: string(srcType),
										MetricName: "object_changes",
										Data:       detectedChangesSummary,
										CustomTags: metricStoreMessages[0].CustomTags,
									})
							}
						}
						lastUptimeS = postmasterUptimeS.(int64)
					}
				}

				_, _ = StoreMetrics(metricStoreMessages, storeCh)
			}
		}

		select {
		case <-ctx.Done():
			return
		case msg := <-controlCh:
			l.Debug("got control msg", msg)
			if msg.Action == gathererStatusStart {
				config = msg.Config
				interval = config[metricName]
				l.Debug("started MetricGathererLoop with interval:", interval)
			} else if msg.Action == gathererStatusStop {
				l.Debug("exiting MetricGathererLoop with interval:", interval)
				return
			}
		case <-time.After(time.Second * time.Duration(interval)):
			l.Debugf("MetricGathererLoop slept for %s", time.Second*time.Duration(interval))
		}

	}
}

func FetchStatsDirectlyFromOS(msg MetricFetchMessage, vme DBVersionMapEntry, mvp metrics.MetricProperties) ([]metrics.MeasurementMessage, error) {
	var data []map[string]any
	var err error

	if msg.MetricName == metricCPULoad { // could function pointers work here?
		data, err = psutil.GetLoadAvgLocal()
	} else if msg.MetricName == metricPsutilCPU {
		data, err = psutil.GetGoPsutilCPU(msg.Interval)
	} else if msg.MetricName == metricPsutilDisk {
		data, err = GetGoPsutilDiskPG(msg.DBUniqueName)
	} else if msg.MetricName == metricPsutilDiskIoTotal {
		data, err = psutil.GetGoPsutilDiskTotals()
	} else if msg.MetricName == metricPsutilMem {
		data, err = psutil.GetGoPsutilMem()
	}
	if err != nil {
		return nil, err
	}

	msm := DatarowsToMetricstoreMessage(data, msg, vme, mvp)
	return []metrics.MeasurementMessage{msm}, nil
}

// data + custom tags + counters
func DatarowsToMetricstoreMessage(data metrics.Measurements, msg MetricFetchMessage, vme DBVersionMapEntry, mvp metrics.MetricProperties) metrics.MeasurementMessage {
	md, err := GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		logger.Errorf("Could not resolve DBUniqueName %s, cannot set custom attributes for gathered data: %v", msg.DBUniqueName, err)
	}

	atomic.AddUint64(&totalMetricsFetchedCounter, uint64(len(data)))

	return metrics.MeasurementMessage{
		DBName:           msg.DBUniqueName,
		SourceType:       string(msg.Source),
		MetricName:       msg.MetricName,
		CustomTags:       md.CustomTags,
		Data:             data,
		MetricDef:        mvp,
		RealDbname:       vme.RealDbname,
		SystemIdentifier: vme.SystemIdentifier,
	}
}

func IsDirectlyFetchableMetric(metric string) bool {
	_, ok := directlyFetchableOSMetrics[metric]
	return ok
}

func IsMetricCurrentlyDisabledForHost(metricName string, vme DBVersionMapEntry, dbUniqueName string) bool {
	_, isSpecialMetric := specialMetrics[metricName]

	mvp, err := GetMetricVersionProperties(metricName, vme, nil)
	if err != nil {
		if isSpecialMetric || strings.Contains(err.Error(), "too old") {
			return false
		}
		logger.Warningf("[%s][%s] Ignoring any possible time based gathering restrictions, could not get metric details", dbUniqueName, metricName)
		return false
	}

	md, err := GetMonitoredDatabaseByUniqueName(dbUniqueName) // TODO caching?
	if err != nil {
		logger.Warningf("[%s][%s] Ignoring any possible time based gathering restrictions, could not get DB details", dbUniqueName, metricName)
		return false
	}

	if md.HostConfig.PerMetricDisabledTimes == nil && mvp.MetricAttrs.DisabledDays == "" && len(mvp.MetricAttrs.DisableTimes) == 0 {
		//log.Debugf("[%s][%s] No time based gathering restrictions defined", dbUniqueName, metricName)
		return false
	}

	metricHasOverrides := false
	if md.HostConfig.PerMetricDisabledTimes != nil {
		for _, hcdt := range md.HostConfig.PerMetricDisabledTimes {
			if slices.Index(hcdt.Metrics, metricName) > -1 && (hcdt.DisabledDays != "" || len(hcdt.DisabledTimes) > 0) {
				metricHasOverrides = true
				break
			}
		}
		if !metricHasOverrides && mvp.MetricAttrs.DisabledDays == "" && len(mvp.MetricAttrs.DisableTimes) == 0 {
			//log.Debugf("[%s][%s] No time based gathering restrictions defined", dbUniqueName, metricName)
			return false
		}
	}

	return IsInDisabledTimeDayRange(time.Now(), mvp.MetricAttrs.DisabledDays, mvp.MetricAttrs.DisableTimes, md.HostConfig.PerMetricDisabledTimes, metricName, dbUniqueName)
}

// days: 0 = Sun, ranges allowed
func IsInDaySpan(locTime time.Time, days, _, _ string) bool {
	//log.Debug("IsInDaySpan", locTime, days, metric, dbUnique)
	if days == "" {
		return false
	}
	curDayInt := int(locTime.Weekday())
	daysMap := DaysStringToIntMap(days)
	//log.Debugf("curDayInt %v, daysMap %+v", curDayInt, daysMap)
	_, ok := daysMap[curDayInt]
	return ok
}

func DaysStringToIntMap(days string) map[int]bool { // TODO validate with some regex when reading in configs, have dbname info then
	ret := make(map[int]bool)
	for _, s := range strings.Split(days, ",") {
		if strings.Contains(s, "-") {
			dayRange := strings.Split(s, "-")
			if len(dayRange) != 2 {
				logger.Warningf("Ignoring invalid day range specification: %s. Check config", s)
				continue
			}
			startDay, err := strconv.Atoi(dayRange[0])
			endDay, err2 := strconv.Atoi(dayRange[1])
			if err != nil || err2 != nil {
				logger.Warningf("Ignoring invalid day range specification: %s. Check config", s)
				continue
			}
			for i := startDay; i <= endDay && i >= 0 && i <= 7; i++ {
				ret[i] = true
			}

		} else {
			day, err := strconv.Atoi(s)
			if err != nil {
				logger.Warningf("Ignoring invalid day range specification: %s. Check config", days)
				continue
			}
			ret[day] = true
		}
	}
	if _, ok := ret[7]; ok { // Cron allows either 0 or 7 for Sunday
		ret[0] = true
	}
	return ret
}

func IsInTimeSpan(checkTime time.Time, timeRange, metric, dbUnique string) bool {
	layout := "15:04"
	var t1, t2 time.Time
	var err error

	timeRange = strings.TrimSpace(timeRange)
	if len(timeRange) < 11 {
		logger.Warningf("[%s][%s] invalid time range: %s. Check config", dbUnique, metric, timeRange)
		return false
	}
	s1 := timeRange[0:5]
	s2 := timeRange[6:11]
	tz := strings.TrimSpace(timeRange[11:])

	if len(tz) > 1 { // time zone specified
		if regexIsAlpha.MatchString(tz) {
			layout = "15:04 MST"
		} else {
			layout = "15:04 -0700"
		}
		t1, err = time.Parse(layout, s1+" "+tz)
		if err == nil {
			t2, err = time.Parse(layout, s2+" "+tz)
		}
	} else { // no time zone
		t1, err = time.Parse(layout, s1)
		if err == nil {
			t2, err = time.Parse(layout, s2)
		}
	}

	if err != nil {
		logger.Warningf("[%s][%s] Ignoring invalid disabled time range: %s. Check config. Erorr: %v", dbUnique, metric, timeRange, err)
		return false
	}

	check, err := time.Parse("15:04 -0700", strconv.Itoa(checkTime.Hour())+":"+strconv.Itoa(checkTime.Minute())+" "+t1.Format("-0700")) // UTC by default
	if err != nil {
		logger.Warningf("[%s][%s] Ignoring invalid disabled time range: %s. Check config. Error: %v", dbUnique, metric, timeRange, err)
		return false
	}

	if t1.After(t2) {
		t2 = t2.AddDate(0, 0, 1)
	}

	return check.Before(t2) && check.After(t1)
}

func IsInDisabledTimeDayRange(localTime time.Time, metricAttrsDisabledDays string, metricAttrsDisabledTimes []string, hostConfigPerMetricDisabledTimes []sources.HostConfigPerMetricDisabledTimes, metric, dbUnique string) bool {
	hostConfigMetricMatch := false
	for _, hcdi := range hostConfigPerMetricDisabledTimes { // host config takes precedence when both specified
		dayMatchFound := false
		timeMatchFound := false
		if slices.Index(hcdi.Metrics, metric) > -1 {
			hostConfigMetricMatch = true
			if !dayMatchFound && hcdi.DisabledDays != "" && IsInDaySpan(localTime, hcdi.DisabledDays, metric, dbUnique) {
				dayMatchFound = true
			}
			for _, dt := range hcdi.DisabledTimes {
				if IsInTimeSpan(localTime, dt, metric, dbUnique) {
					timeMatchFound = true
					break
				}
			}
		}
		if hostConfigMetricMatch && (timeMatchFound || len(hcdi.DisabledTimes) == 0) && (dayMatchFound || hcdi.DisabledDays == "") {
			//log.Debugf("[%s][%s] Host config ignored time/day match, skipping fetch", dbUnique, metric)
			return true
		}
	}

	if !hostConfigMetricMatch && (metricAttrsDisabledDays != "" || len(metricAttrsDisabledTimes) > 0) {
		dayMatchFound := IsInDaySpan(localTime, metricAttrsDisabledDays, metric, dbUnique)
		timeMatchFound := false
		for _, timeRange := range metricAttrsDisabledTimes {
			if IsInTimeSpan(localTime, timeRange, metric, dbUnique) {
				timeMatchFound = true
				break
			}
		}
		if (timeMatchFound || len(metricAttrsDisabledTimes) == 0) && (dayMatchFound || metricAttrsDisabledDays == "") {
			//log.Debugf("[%s][%s] metrics.MetricAttrs ignored time/day match, skipping fetch", dbUnique, metric)
			return true
		}
	}

	return false
}

func UpdateMetricDefinitions(newMetrics map[string]map[uint]metrics.MetricProperties, _ map[string]string) {
	metricDefMapLock.Lock()
	metricDefinitionMap = newMetrics

	metricDefMapLock.Unlock()
	logger.Debug("metrics definitions refreshed - nr. found:", len(newMetrics))
}

func ExpandEnvVarsForConfigEntryIfStartsWithDollar(md sources.MonitoredDatabase) (sources.MonitoredDatabase, int) {
	var changed int

	if strings.HasPrefix(md.Encryption, "$") {
		md.Encryption = os.ExpandEnv(md.Encryption)
		changed++
	}
	if strings.HasPrefix(string(md.Kind), "$") {
		md.Kind = sources.Kind(os.ExpandEnv(string(md.Kind)))
		changed++
	}
	if strings.HasPrefix(md.DBUniqueName, "$") {
		md.DBUniqueName = os.ExpandEnv(md.DBUniqueName)
		changed++
	}
	if strings.HasPrefix(md.IncludePattern, "$") {
		md.IncludePattern = os.ExpandEnv(md.IncludePattern)
		changed++
	}
	if strings.HasPrefix(md.ExcludePattern, "$") {
		md.ExcludePattern = os.ExpandEnv(md.ExcludePattern)
		changed++
	}
	if strings.HasPrefix(md.PresetMetrics, "$") {
		md.PresetMetrics = os.ExpandEnv(md.PresetMetrics)
		changed++
	}
	if strings.HasPrefix(md.PresetMetricsStandby, "$") {
		md.PresetMetricsStandby = os.ExpandEnv(md.PresetMetricsStandby)
		changed++
	}

	return md, changed
}

func getMonitoredDatabasesSnapshot() map[string]sources.MonitoredDatabase {
	mdSnap := make(map[string]sources.MonitoredDatabase)

	if monitoredDbCache != nil {
		monitoredDbCacheLock.RLock()
		defer monitoredDbCacheLock.RUnlock()

		for _, row := range monitoredDbCache {
			mdSnap[row.DBUniqueName] = row
		}
	}

	return mdSnap
}

func StatsServerHandler(w http.ResponseWriter, _ *http.Request) {
	jsonResponseTemplate := `
{
	"secondsFromLastSuccessfulDatastoreWrite": %d,
	"totalMetricsFetchedCounter": %d,
	"totalMetricsReusedFromCacheCounter": %d,
	"totalDatasetsFetchedCounter": %d,
	"metricPointsPerMinuteLast5MinAvg": %v,
	"metricsDropped": %d,
	"totalMetricFetchFailuresCounter": %d,
	"datastoreWriteFailuresCounter": %d,
	"datastoreSuccessfulWritesCounter": %d,
	"datastoreAvgSuccessfulWriteTimeMillis": %.1f,
	"databasesMonitored": %d,
	"databasesConfigured": %d,
	"unreachableDBs": %d,
	"gathererUptimeSeconds": %d
}
`
	now := time.Now()
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
	gathererUptimeSeconds := uint64(now.Sub(gathererStartTime).Seconds())
	var metricPointsPerMinute int64
	metricPointsPerMinute = atomic.LoadInt64(&metricPointsPerMinuteLast5MinAvg)
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
	_, _ = io.WriteString(w, fmt.Sprintf(jsonResponseTemplate, time.Now().Unix()-secondsFromLastSuccessfulDatastoreWrite, totalMetrics, cacheMetrics, totalDatasets, metricPointsPerMinute, metricsDropped, metricFetchFailuresCounter, datastoreFailures, datastoreSuccess, datastoreAvgSuccessfulWriteTimeMillis, databasesMonitored, databasesConfigured, unreachableDBs, gathererUptimeSeconds))
}

// Calculates 1min avg metric fetching statistics for last 5min for StatsServerHandler to display
func StatsSummarizer(ctx context.Context) {
	var prevMetricsCounterValue uint64
	var currentMetricsCounterValue uint64
	ticker := time.NewTicker(time.Minute * 5)
	lastSummarization := gathererStartTime
	for {
		select {
		case now := <-ticker.C:
			currentMetricsCounterValue = atomic.LoadUint64(&totalMetricsFetchedCounter)
			atomic.StoreInt64(&metricPointsPerMinuteLast5MinAvg, int64(math.Round(float64(currentMetricsCounterValue-prevMetricsCounterValue)*60/now.Sub(lastSummarization).Seconds())))
			prevMetricsCounterValue = currentMetricsCounterValue
			lastSummarization = now
		case <-ctx.Done():
			return
		}
	}
}

func SyncMonitoredDBsToDatastore(ctx context.Context, monitoredDbs []sources.MonitoredDatabase, persistenceChannel chan []metrics.MeasurementMessage) {
	if len(monitoredDbs) > 0 {
		msms := make([]metrics.MeasurementMessage, len(monitoredDbs))
		now := time.Now()

		for _, mdb := range monitoredDbs {
			db := metrics.Measurement{
				"tag_group":                   mdb.Group,
				"master_only":                 mdb.OnlyIfMaster,
				"epoch_ns":                    now.UnixNano(),
				"continuous_discovery_prefix": mdb.DBUniqueNameOrig,
			}
			for k, v := range mdb.CustomTags {
				db[tagPrefix+k] = v
			}
			msms = append(msms, metrics.MeasurementMessage{
				DBName:     mdb.DBUniqueName,
				MetricName: monitoredDbsDatastoreSyncMetricName,
				Data:       metrics.Measurements{db},
			})
		}
		select {
		case persistenceChannel <- msms:
			//continue
		case <-ctx.Done():
			return
		}
	}
}

func shouldDbBeMonitoredBasedOnCurrentState(md sources.MonitoredDatabase) bool {
	return !IsDBDormant(md.DBUniqueName)
}

func ControlChannelsMapToList(controlChannels map[string]chan ControlMessage) []string {
	controlChannelList := make([]string, len(controlChannels))
	i := 0
	for key := range controlChannels {
		controlChannelList[i] = key
		i++
	}
	return controlChannelList
}

func CloseResourcesForRemovedMonitoredDBs(metricsWriter *sinks.MultiWriter, currentDBs, prevLoopDBs []sources.MonitoredDatabase, shutDownDueToRoleChange map[string]bool) {
	var curDBsMap = make(map[string]bool)

	for _, curDB := range currentDBs {
		curDBsMap[curDB.DBUniqueName] = true
	}

	for _, prevDB := range prevLoopDBs {
		if _, ok := curDBsMap[prevDB.DBUniqueName]; !ok { // removed from config
			CloseOrLimitSQLConnPoolForMonitoredDBIfAny(prevDB.DBUniqueName)
			_ = metricsWriter.SyncMetrics(prevDB.DBUniqueName, "", "remove")
		}
	}

	// or to be ignored due to current instance state
	for roleChangedDB := range shutDownDueToRoleChange {
		CloseOrLimitSQLConnPoolForMonitoredDBIfAny(roleChangedDB)
		_ = metricsWriter.SyncMetrics(roleChangedDB, "", "remove")
	}
}

func SetUndersizedDBState(dbUnique string, state bool) {
	undersizedDBsLock.Lock()
	undersizedDBs[dbUnique] = state
	undersizedDBsLock.Unlock()
}

func IsDBUndersized(dbUnique string) bool {
	undersizedDBsLock.RLock()
	defer undersizedDBsLock.RUnlock()
	undersized, ok := undersizedDBs[dbUnique]
	if ok {
		return undersized
	}
	return false
}

func SetRecoveryIgnoredDBState(dbUnique string, state bool) {
	recoveryIgnoredDBsLock.Lock()
	recoveryIgnoredDBs[dbUnique] = state
	recoveryIgnoredDBsLock.Unlock()
}

func IsDBIgnoredBasedOnRecoveryState(dbUnique string) bool {
	recoveryIgnoredDBsLock.RLock()
	defer recoveryIgnoredDBsLock.RUnlock()
	recoveryIgnored, ok := undersizedDBs[dbUnique]
	if ok {
		return recoveryIgnored
	}
	return false
}

func IsDBDormant(dbUnique string) bool {
	return IsDBUndersized(dbUnique) || IsDBIgnoredBasedOnRecoveryState(dbUnique)
}

func DoesEmergencyTriggerfileExist() bool {
	// Main idea of the feature is to be able to quickly free monitored DBs / network of any extra "monitoring effect" load.
	// In highly automated K8s / IaC environments such a temporary change might involve pull requests, peer reviews, CI/CD etc
	// which can all take too long vs "exec -it pgwatch3-pod -- touch /tmp/pgwatch3-emergency-pause".
	// After creating the file it can still take up to --servers-refresh-loop-seconds (2min def.) for change to take effect!
	if opts.EmergencyPauseTriggerfile == "" {
		return false
	}
	_, err := os.Stat(opts.EmergencyPauseTriggerfile)
	return err == nil
}

var opts *config.Options

// version output variables
var (
	commit  = "000000"
	version = "master"
	date    = "unknown"
	dbapi   = "00534"
)

func printVersion() {
	fmt.Printf(`pgwatch3:
  Version:      %s
  DB Schema:    %s
  Git Commit:   %s
  Built:        %s
`, version, dbapi, commit, date)
}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func SetupCloseHandler(cancel context.CancelFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Debug("SetupCloseHandler received an interrupt from OS. Closing session...")
		cancel()
		exitCode.Store(ExitCodeUserCancel)
	}()
}

const (
	ExitCodeOK int32 = iota
	ExitCodeConfigError
	ExitCodeWebUIError
	ExitCodeUpgradeError
	ExitCodeUserCancel
	ExitCodeShutdownCommand
	ExitCodeFatalError
)

var exitCode atomic.Int32

var mainContext context.Context

func LoadMetricDefs(ctx context.Context) (err error) {
	var metricDefs metrics.MetricVersionDefs
	var renamingDefs map[string]string
	if fileBasedMetrics {
		metricDefs, renamingDefs, err = metrics.ReadMetricsFromFolder(ctx, opts.Metric.MetricsFolder)
	} else {
		metricDefs, renamingDefs, err = metrics.ReadMetricsFromPostgres(ctx, configDb)
	}
	if err == nil {
		UpdateMetricDefinitions(metricDefs, renamingDefs)
	}
	return
}

func SyncMetricDefs(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(metricDefinitionRefreshInterval):
			// reread metric definitions
		}
		if err := LoadMetricDefs(ctx); err != nil {
			logger.Errorf("Could not refresh metric definitions: %w", err)
		}
	}
}

func main() {
	var (
		err                error
		cancel             context.CancelFunc
		measurementsWriter *sinks.MultiWriter
	)
	exitCode.Store(ExitCodeOK)
	defer func() {
		if err := recover(); err != nil {
			exitCode.Store(ExitCodeFatalError)
			log.GetLogger(mainContext).WithField("callstack", string(debug.Stack())).Error(err)
		}
		os.Exit(int(exitCode.Load()))
	}()

	mainContext, cancel = context.WithCancel(context.Background())
	SetupCloseHandler(cancel)
	defer cancel()

	opts, err = config.NewConfig(os.Stdout)
	if err != nil {
		if opts != nil {
			if opts.VersionOnly() {
				printVersion()
			}
			if opts.Source.AesGcmPasswordToEncrypt > "" { // special flag - encrypt and exit
				fmt.Println(opts.Encrypt())
			}
			return
		}
		fmt.Println("Configuration error: ", err)
		exitCode.Store(ExitCodeConfigError)
		return
	}

	logger = log.Init(opts.Logging)
	mainContext = log.WithLogger(mainContext, logger)

	uifs, _ := fs.Sub(webuifs, "webui/build")
	ui := webserver.Init(opts.WebUI, uifs, uiapi, logger)
	if ui == nil {
		exitCode.Store(ExitCodeWebUIError)
		return
	}

	logger.Debugf("opts: %+v", opts)

	if opts.IsAdHocMode() && opts.AdHocUniqueName == "adhoc" {
		logger.Warning("In ad-hoc mode: using default unique name 'adhoc' for metrics storage. use --adhoc-name to override.")
	}

	// running in config file based mode?
	configKind, err := opts.GetConfigKind()
	switch {
	case err != nil:
		logger.Fatal(err)
	case configKind == config.ConfigFile || configKind == config.ConfigFolder:
		fileBasedMetrics = true
	case configKind == config.ConfigPgURL:
		if configDb, err = db.InitAndTestConfigStoreConnection(mainContext, opts.Source.Config); err != nil {
			logger.WithError(err).Fatal("Could not connect to configuration database")
		}
	}

	if opts.Source.Init {
		return
	}

	pgBouncerNumericCountersStartVersion = VersionToInt("1.12")

	if !opts.Ping {
		go StatsSummarizer(mainContext)
	}

	controlChannels := make(map[string](chan ControlMessage)) // [db1+metric1]=chan
	measurementCh := make(chan []metrics.MeasurementMessage, 10000)

	var monitoredDbs sources.MonitoredDatabases
	var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
	var metricConfig map[string]float64                       // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state

	if err = LoadMetricDefs(mainContext); err != nil {
		logger.Errorf("Could not load metric definitions: %w", err)
		os.Exit(int(ExitCodeConfigError))
	}
	go SyncMetricDefs(mainContext)

	if measurementsWriter, err = sinks.NewMultiWriter(mainContext, opts, metricDefinitionMap); err != nil {
		logger.Fatal(err)
	}
	if !opts.Ping {
		go measurementsWriter.WriteMeasurements(mainContext, measurementCh)
	}

	firstLoop := true
	mainLoopCount := 0

	var srcReader sources.Reader

	if fileBasedMetrics {
		srcReader, err = sources.NewYAMLConfigReader(mainContext, opts)
	} else {
		srcReader, err = sources.NewPostgresConfigReader(mainContext, opts)
	}
	if err != nil {
		logger.Fatal(err)
	}

	for { //main loop
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		var controlChannelNameList []string
		gatherersShutDown := 0

		if monitoredDbs, err = srcReader.GetMonitoredDatabases(); err != nil {
			if firstLoop {
				logger.Fatal("could not fetch active hosts - check config!", err)
			} else {
				logger.Error("could not fetch active hosts, using last valid config data. err:", err)
				time.Sleep(time.Second * time.Duration(opts.Source.Refresh))
				continue
			}
		}

		if DoesEmergencyTriggerfileExist() {
			logger.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", opts.EmergencyPauseTriggerfile)
			monitoredDbs = make([]sources.MonitoredDatabase, 0)
		}

		UpdateMonitoredDBCache(monitoredDbs)

		if lastMonitoredDBsUpdate.IsZero() || lastMonitoredDBsUpdate.Before(time.Now().Add(-1*time.Second*monitoredDbsDatastoreSyncIntervalSeconds)) {
			monitoredDbsCopy := make([]sources.MonitoredDatabase, len(monitoredDbs))
			copy(monitoredDbsCopy, monitoredDbs)
			go SyncMonitoredDBsToDatastore(mainContext, monitoredDbsCopy, measurementCh)
			lastMonitoredDBsUpdate = time.Now()
		}

		logger.
			WithField("databases", len(monitoredDbs)).
			WithField("metrics", len(metricDefinitionMap)).
			Log(func() logrus.Level {
				if firstLoop && len(monitoredDbs)*len(metricDefinitionMap) == 0 {
					return logrus.WarnLevel
				}
				return logrus.InfoLevel
			}(), "host info refreshed")

		firstLoop = false // only used for failing when 1st config reading fails

		for _, host := range monitoredDbs {
			logger.WithField("database", host.DBUniqueName).
				WithField("metric", host.Metrics).
				WithField("tags", host.CustomTags).
				WithField("config", host.HostConfig).Debug()

			dbUnique := host.DBUniqueName
			dbUniqueOrig := host.DBUniqueNameOrig
			srcType := host.Kind
			metricConfig = host.Metrics
			wasInstancePreviouslyDormant := IsDBDormant(dbUnique)

			if host.Encryption == "aes-gcm-256" && len(opts.Source.AesGcmKeyphrase) == 0 && len(opts.Source.AesGcmKeyphraseFile) == 0 {
				// Warn if any encrypted hosts found but no keyphrase given
				logger.Warningf("Encrypted password type found for host \"%s\", but no decryption keyphrase specified. Use --aes-gcm-keyphrase or --aes-gcm-keyphrase-file params", dbUnique)
			}

			err := InitSQLConnPoolForMonitoredDBIfNil(host)
			if err != nil {
				logger.Warningf("Could not init SQL connection pool for %s, retrying on next main loop. Err: %v", dbUnique, err)
				continue
			}

			InitPGVersionInfoFetchingLockIfNil(host)

			_, connectFailedSoFar := failedInitialConnectHosts[dbUnique]

			if connectFailedSoFar { // idea is not to spwan any runners before we've successfully pinged the DB
				var err error
				var ver DBVersionMapEntry

				if connectFailedSoFar {
					logger.Infof("retrying to connect to uninitialized DB \"%s\"...", dbUnique)
				} else {
					logger.Infof("new host \"%s\" found, checking connectivity...", dbUnique)
				}

				ver, err = DBGetPGVersion(mainContext, dbUnique, srcType, true)
				if err != nil {
					logger.Errorf("could not start metric gathering for DB \"%s\" due to connection problem: %s", dbUnique, err)
					if opts.AdHocConnString != "" {
						logger.Errorf("will retry in %ds...", opts.Source.Refresh)
					}
					failedInitialConnectHosts[dbUnique] = true
					continue
				}
				logger.Infof("Connect OK. [%s] is on version %s (in recovery: %v)", dbUnique, ver.VersionStr, ver.IsInRecovery)
				if connectFailedSoFar {
					delete(failedInitialConnectHosts, dbUnique)
				}
				if ver.IsInRecovery && host.OnlyIfMaster {
					logger.Infof("[%s] not added to monitoring due to 'master only' property", dbUnique)
					continue
				}
				metricConfig = host.Metrics
				hostLastKnownStatusInRecovery[dbUnique] = ver.IsInRecovery
				if ver.IsInRecovery && len(host.MetricsStandby) > 0 {
					metricConfig = host.MetricsStandby
				}

				if !opts.Ping && (host.IsSuperuser || opts.IsAdHocMode() && opts.AdHocCreateHelpers) && host.IsPostgresSource() && !ver.IsInRecovery {
					if opts.Metric.NoHelperFunctions {
						logger.Infof("[%s] Skipping rollout out helper functions due to the --no-helper-functions flag ...", dbUnique)
					} else {
						logger.Infof("Trying to create helper functions if missing for \"%s\"...", dbUnique)
						_ = TryCreateMetricsFetchingHelpers(dbUnique)
					}
				}

			}

			if host.IsPostgresSource() {
				var DBSizeMB int64

				if opts.Source.MinDbSizeMB >= 8 { // an empty DB is a bit less than 8MB
					DBSizeMB, _ = DBGetSizeMB(dbUnique) // ignore errors, i.e. only remove from monitoring when we're certain it's under the threshold
					if DBSizeMB != 0 {
						if DBSizeMB < opts.Source.MinDbSizeMB {
							logger.Infof("[%s] DB will be ignored due to the --min-db-size-mb filter. Current (up to %v cached) DB size = %d MB", dbUnique, dbSizeCachingInterval, DBSizeMB)
							hostsToShutDownDueToRoleChange[dbUnique] = true // for the case when DB size was previosly above the threshold
							SetUndersizedDBState(dbUnique, true)
							continue
						}
						SetUndersizedDBState(dbUnique, false)
					}
				}
				ver, err := DBGetPGVersion(mainContext, dbUnique, host.Kind, false)
				if err == nil { // ok to ignore error, re-tried on next loop
					lastKnownStatusInRecovery := hostLastKnownStatusInRecovery[dbUnique]
					if ver.IsInRecovery && host.OnlyIfMaster {
						logger.Infof("[%s] to be removed from monitoring due to 'master only' property and status change", dbUnique)
						hostsToShutDownDueToRoleChange[dbUnique] = true
						SetRecoveryIgnoredDBState(dbUnique, true)
						continue
					} else if lastKnownStatusInRecovery != ver.IsInRecovery {
						if ver.IsInRecovery && len(host.MetricsStandby) > 0 {
							logger.Warningf("Switching metrics collection for \"%s\" to standby config...", dbUnique)
							metricConfig = host.MetricsStandby
							hostLastKnownStatusInRecovery[dbUnique] = true
						} else {
							logger.Warningf("Switching metrics collection for \"%s\" to primary config...", dbUnique)
							metricConfig = host.Metrics
							hostLastKnownStatusInRecovery[dbUnique] = false
							SetRecoveryIgnoredDBState(dbUnique, false)
						}
					}
				}

				if wasInstancePreviouslyDormant && !IsDBDormant(dbUnique) {
					RestoreSQLConnPoolLimitsForPreviouslyDormantDB(dbUnique)
				}

				if mainLoopCount == 0 && opts.TryCreateListedExtsIfMissing != "" && !ver.IsInRecovery {
					extsToCreate := strings.Split(opts.TryCreateListedExtsIfMissing, ",")
					extsCreated := TryCreateMissingExtensions(dbUnique, extsToCreate, ver.Extensions)
					logger.Infof("[%s] %d/%d extensions created based on --try-create-listed-exts-if-missing input %v", dbUnique, len(extsCreated), len(extsToCreate), extsCreated)
				}
			}

			if opts.Ping {
				continue // don't launch metric fetching threads
			}

			for metricName, interval := range metricConfig {
				metric := metricName
				metricDefOk := false

				if strings.HasPrefix(metric, recoPrefix) {
					metric = recoMetricName
				}
				// interval := metricConfig[metric]

				if metric == recoMetricName {
					metricDefOk = true
				} else {
					metricDefMapLock.RLock()
					_, metricDefOk = metricDefinitionMap[metric]
					metricDefMapLock.RUnlock()
				}

				dbMetric := dbUnique + dbMetricJoinStr + metric
				_, chOk := controlChannels[dbMetric]

				if metricDefOk && !chOk { // initialize a new per db/per metric control channel
					if interval > 0 {
						hostMetricIntervalMap[dbMetric] = interval
						logger.WithField("database", dbUnique).WithField("metric", metric).WithField("interval", interval).Info("starting gatherer")
						controlChannels[dbMetric] = make(chan ControlMessage, 1)

						metricNameForStorage := metricName
						if _, isSpecialMetric := specialMetrics[metricName]; !isSpecialMetric {
							vme, err := DBGetPGVersion(mainContext, dbUnique, srcType, false)
							if err != nil {
								logger.Warning("Failed to determine possible re-routing name, Grafana dashboards with re-routed metrics might not show all hosts")
							} else {
								mvp, err := GetMetricVersionProperties(metricName, vme, nil)
								if err != nil && !strings.Contains(err.Error(), "too old") {
									logger.Warning("Failed to determine possible re-routing name, Grafana dashboards with re-routed metrics might not show all hosts")
								} else if mvp.MetricAttrs.MetricStorageName != "" {
									metricNameForStorage = mvp.MetricAttrs.MetricStorageName
								}
							}
						}

						if err := measurementsWriter.SyncMetrics(dbUnique, metricNameForStorage, "add"); err != nil {
							logger.Error(err)
						}

						go MetricGathererLoop(mainContext, dbUnique, dbUniqueOrig, srcType, metric, metricConfig, controlChannels[dbMetric], measurementCh)
					}
				} else if (!metricDefOk && chOk) || interval <= 0 {
					// metric definition files were recently removed or interval set to zero
					logger.Warning("shutting down metric", metric, "for", host.DBUniqueName)
					controlChannels[dbMetric] <- ControlMessage{Action: gathererStatusStop}
					delete(controlChannels, dbMetric)
				} else if !metricDefOk {
					epoch, ok := lastSQLFetchError.Load(metric)
					if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
						logger.Warningf("metric definition \"%s\" not found for \"%s\"", metric, dbUnique)
						lastSQLFetchError.Store(metric, time.Now().Unix())
					}
				} else {
					// check if interval has changed
					if hostMetricIntervalMap[dbMetric] != interval {
						logger.Warning("sending interval update for", dbUnique, metric)
						controlChannels[dbMetric] <- ControlMessage{Action: gathererStatusStart, Config: metricConfig}
						hostMetricIntervalMap[dbMetric] = interval
					}
				}
			}
		}

		if opts.Ping {
			if len(failedInitialConnectHosts) > 0 {
				logger.Errorf("Could not reach %d configured DB host out of %d", len(failedInitialConnectHosts), len(monitoredDbs))
				os.Exit(len(failedInitialConnectHosts))
			}
			logger.Infof("All configured %d DB hosts were reachable", len(monitoredDbs))
			os.Exit(0)
		}

		if mainLoopCount == 0 {
			goto MainLoopSleep
		}

		// loop over existing channels and stop workers if DB or metric removed from config
		// or state change makes it uninteresting
		logger.Debug("checking if any workers need to be shut down...")
		controlChannelNameList = ControlChannelsMapToList(controlChannels)

		for _, dbMetric := range controlChannelNameList {
			var currentMetricConfig map[string]float64
			var dbInfo sources.MonitoredDatabase
			var ok, dbRemovedFromConfig bool
			singleMetricDisabled := false
			splits := strings.Split(dbMetric, dbMetricJoinStr)
			db := splits[0]
			metric := splits[1]
			//log.Debugf("Checking if need to shut down worker for [%s:%s]...", db, metric)

			_, wholeDbShutDownDueToRoleChange := hostsToShutDownDueToRoleChange[db]
			if !wholeDbShutDownDueToRoleChange {
				monitoredDbCacheLock.RLock()
				dbInfo, ok = monitoredDbCache[db]
				monitoredDbCacheLock.RUnlock()
				if !ok { // normal removing of DB from config
					dbRemovedFromConfig = true
					logger.Debugf("DB %s removed from config, shutting down all metric worker processes...", db)
				}
			}

			if !(wholeDbShutDownDueToRoleChange || dbRemovedFromConfig) { // maybe some single metric was disabled
				dbPgVersionMapLock.RLock()
				verInfo, ok := dbPgVersionMap[db]
				dbPgVersionMapLock.RUnlock()
				if !ok {
					logger.Warningf("Could not find PG version info for DB %s, skipping shutdown check of metric worker process for %s", db, metric)
					continue
				}

				if verInfo.IsInRecovery && len(dbInfo.MetricsStandby) > 0 {
					currentMetricConfig = dbInfo.MetricsStandby
				} else {
					currentMetricConfig = dbInfo.Metrics
				}

				interval, isMetricActive := currentMetricConfig[metric]
				if !isMetricActive || interval <= 0 {
					singleMetricDisabled = true
				}
			}

			if mainContext.Err() != nil || wholeDbShutDownDueToRoleChange || dbRemovedFromConfig || singleMetricDisabled {
				logger.Infof("shutting down gatherer for [%s:%s] ...", db, metric)
				controlChannels[dbMetric] <- ControlMessage{Action: gathererStatusStop}
				delete(controlChannels, dbMetric)
				logger.Debugf("control channel for [%s:%s] deleted", db, metric)
				gatherersShutDown++
				ClearDBUnreachableStateIfAny(db)
				if err := measurementsWriter.SyncMetrics(db, metric, "remove"); err != nil {
					logger.Error(err)
				}
			}
		}

		if gatherersShutDown > 0 {
			logger.Warningf("sent STOP message to %d gatherers (it might take some time for them to stop though)", gatherersShutDown)
		}

		// Destroy conn pools and metric writers
		CloseResourcesForRemovedMonitoredDBs(measurementsWriter, monitoredDbs, prevLoopMonitoredDBs, hostsToShutDownDueToRoleChange)

	MainLoopSleep:
		mainLoopCount++
		prevLoopMonitoredDBs = monitoredDbs

		logger.Debugf("main sleeping %ds...", opts.Source.Refresh)
		select {
		case <-time.After(time.Second * time.Duration(opts.Source.Refresh)):
			// pass
		case <-mainContext.Done():
			return
		}
	}
}
