package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
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
	"github.com/cybertec-postgresql/pgwatch3/metrics/sinks"
	"github.com/cybertec-postgresql/pgwatch3/psutil"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"

	"github.com/coreos/go-systemd/daemon"
	"golang.org/x/crypto/pbkdf2"
	"gopkg.in/yaml.v2"
)

type MonitoredDatabase struct {
	DBUniqueName         string `yaml:"unique_name"`
	DBUniqueNameOrig     string // to preserve belonging to a specific instance for continuous modes where DBUniqueName will be dynamic
	Group                string
	Encryption           string             `yaml:"encryption"`
	ConnStr              string             `yaml:"conn_str"`
	Metrics              map[string]float64 `yaml:"custom_metrics"`
	MetricsStandby       map[string]float64 `yaml:"custom_metrics_standby"`
	DBType               string
	DBNameIncludePattern string            `yaml:"dbname_include_pattern"`
	DBNameExcludePattern string            `yaml:"dbname_exclude_pattern"`
	PresetMetrics        string            `yaml:"preset_metrics"`
	PresetMetricsStandby string            `yaml:"preset_metrics_standby"`
	IsSuperuser          bool              `yaml:"is_superuser"`
	IsEnabled            bool              `yaml:"is_enabled"`
	CustomTags           map[string]string `yaml:"custom_tags"`
	HostConfig           HostConfigAttrs   `yaml:"host_config"`
	OnlyIfMaster         bool              `yaml:"only_if_master"`
}

func (md MonitoredDatabase) GetDatabaseName() string {
	if conf, err := pgx.ParseConfig(md.ConnStr); err == nil {
		return conf.Database
	}
	return ""
}

type HostConfigAttrs struct {
	DcsType                string   `yaml:"dcs_type"`
	DcsEndpoints           []string `yaml:"dcs_endpoints"`
	Scope                  string
	Namespace              string
	Username               string
	Password               string
	CAFile                 string                             `yaml:"ca_file"`
	CertFile               string                             `yaml:"cert_file"`
	KeyFile                string                             `yaml:"key_file"`
	LogsGlobPath           string                             `yaml:"logs_glob_path"`   // default $data_directory / $log_directory / *.csvlog
	LogsMatchRegex         string                             `yaml:"logs_match_regex"` // default is for CSVLOG format. needs to capture following named groups: log_time, user_name, database_name and error_severity
	PerMetricDisabledTimes []HostConfigPerMetricDisabledTimes `yaml:"per_metric_disabled_intervals"`
}

type HostConfigPerMetricDisabledTimes struct { // metric gathering override per host / metric / time
	Metrics       []string `yaml:"metrics"`
	DisabledTimes []string `yaml:"disabled_times"`
	DisabledDays  string   `yaml:"disabled_days"`
}

type PresetConfig struct {
	Name        string
	Description string
	Metrics     map[string]float64
}

type ControlMessage struct {
	Action string // START, STOP, PAUSE
	Config map[string]float64
}

type MetricFetchMessage struct {
	DBUniqueName        string
	DBUniqueNameOrig    string
	MetricName          string
	DBType              string
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
	epochColumnName             string = "epoch_ns" // this column (epoch in nanoseconds) is expected in every metric query
	tagPrefix                   string = "tag_"
	metricDefinitionRefreshTime int64  = 120   // min time before checking for new/changed metric definitions
	persistQueueMaxSize                = 10000 // storage queue max elements. when reaching the limit, older metrics will be dropped.
)

// actual requirements depend a lot of metric type and nr. of obects in schemas,
// but 100k should be enough for 24h, assuming 5 hosts monitored with "exhaustive" preset config
const (
	presetConfigYAMLFile = "preset-configs.yaml"

	gathererStatusStart     = "START"
	gathererStatusStop      = "STOP"
	metricdbIdent           = "metricDb"
	configdbIdent           = "configDb"
	contextPrometheusScrape = "prometheus-scrape"
	dcsTypeEtcd             = "etcd"
	dcsTypeZookeeper        = "zookeeper"
	dcsTypeConsul           = "consul"

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

var dbTypeMap = map[string]bool{config.DbTypePg: true, config.DbTypePgCont: true, config.DbTypeBouncer: true, config.DbTypePatroni: true, config.DbTypePatroniCont: true, config.DbTypePgPOOL: true, config.DbTypePatroniNamespaceDiscovery: true}
var dbTypes = []string{config.DbTypePg, config.DbTypePgCont, config.DbTypeBouncer, config.DbTypePatroni, config.DbTypePatroniCont, config.DbTypePatroniNamespaceDiscovery} // used for informational purposes
var specialMetrics = map[string]bool{recoMetricName: true, specialMetricChangeEvents: true, specialMetricServerLogEventCounts: true}
var directlyFetchableOSMetrics = map[string]bool{metricPsutilCPU: true, metricPsutilDisk: true, metricPsutilDiskIoTotal: true, metricPsutilMem: true, metricCPULoad: true}
var metricDefinitionMap map[string]map[uint]metrics.MetricProperties
var metricDefMapLock = sync.RWMutex{}
var hostMetricIntervalMap = make(map[string]float64) // [db1_metric] = 30
var dbPgVersionMap = make(map[string]DBVersionMapEntry)
var dbPgVersionMapLock = sync.RWMutex{}
var dbGetPgVersionMapLock = make(map[string]*sync.RWMutex) // synchronize initial PG version detection to 1 instance for each defined host
var monitoredDbCache map[string]MonitoredDatabase
var monitoredDbCacheLock sync.RWMutex

var monitoredDbConnCacheLock = sync.RWMutex{}
var lastSQLFetchError sync.Map

var fileBasedMetrics = false
var presetMetricDefMap map[string]map[string]float64 // read from metrics folder in "file mode"
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
var instanceMetricCache = make(map[string](metrics.MetricData)) // [dbUnique+metric]lastly_fetched_data
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
var mainLoopInitialized int32 // 0/1

var prevLoopMonitoredDBs []MonitoredDatabase // to be able to detect DBs removed from config
var undersizedDBs = make(map[string]bool)    // DBs below the --min-db-size-mb limit, if set
var undersizedDBsLock = sync.RWMutex{}
var recoveryIgnoredDBs = make(map[string]bool) // DBs in recovery state and OnlyIfMaster specified in config
var recoveryIgnoredDBsLock = sync.RWMutex{}

var MetricSchema db.MetricSchemaType

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

func InitPGVersionInfoFetchingLockIfNil(md MonitoredDatabase) {
	dbPgVersionMapLock.Lock()
	if _, ok := dbGetPgVersionMapLock[md.DBUniqueName]; !ok {
		dbGetPgVersionMapLock[md.DBUniqueName] = &sync.RWMutex{}
	}
	dbPgVersionMapLock.Unlock()
}

func GetMonitoredDatabasesFromConfigDB() ([]MonitoredDatabase, error) {
	monitoredDBs := make([]MonitoredDatabase, 0)
	activeHostData, err := GetAllActiveHostsFromConfigDB()
	groups := strings.Split(opts.Metric.Group, ",")
	skippedEntries := 0

	if err != nil {
		logger.Errorf("Failed to read monitoring config from DB: %s", err)
		return monitoredDBs, err
	}

	for _, row := range activeHostData {

		if len(opts.Metric.Group) > 0 { // filter out rows with non-matching groups
			matched := false
			for _, g := range groups {
				if row["md_group"].(string) == g {
					matched = true
					break
				}
			}
			if !matched {
				skippedEntries++
				continue
			}
		}
		if skippedEntries > 0 {
			logger.Infof("Filtered out %d config entries based on --groups input", skippedEntries)
		}

		metricConfig, err := jsonTextToMap(row["md_config"].(string))
		if err != nil {
			logger.Warningf("Cannot parse metrics JSON config for \"%s\": %v", row["md_name"].(string), err)
			continue
		}
		metricConfigStandby := make(map[string]float64)
		if configStandby, ok := row["md_config_standby"]; ok {
			metricConfigStandby, err = jsonTextToMap(configStandby.(string))
			if err != nil {
				logger.Warningf("Cannot parse standby metrics JSON config for \"%s\". Ignoring standby config: %v", row["md_name"].(string), err)
			}
		}
		customTags, err := jsonTextToStringMap(row["md_custom_tags"].(string))
		if err != nil {
			logger.Warningf("Cannot parse custom tags JSON for \"%s\". Ignoring custom tags. Error: %v", row["md_name"].(string), err)
			customTags = nil
		}
		hostConfigAttrs := HostConfigAttrs{}
		err = yaml.Unmarshal([]byte(row["md_host_config"].(string)), &hostConfigAttrs)
		if err != nil {
			logger.Warningf("Cannot parse host config JSON for \"%s\". Ignoring host config. Error: %v", row["md_name"].(string), err)
		}

		md := MonitoredDatabase{
			DBUniqueName:         row["md_name"].(string),
			DBUniqueNameOrig:     row["md_name"].(string),
			IsSuperuser:          row["md_is_superuser"].(bool),
			ConnStr:              row["md_connstr"].(string),
			Encryption:           row["md_encryption"].(string),
			Metrics:              metricConfig,
			MetricsStandby:       metricConfigStandby,
			DBType:               row["md_dbtype"].(string),
			DBNameIncludePattern: row["md_include_pattern"].(string),
			DBNameExcludePattern: row["md_exclude_pattern"].(string),
			Group:                row["md_group"].(string),
			HostConfig:           hostConfigAttrs,
			OnlyIfMaster:         row["md_only_if_master"].(bool),
			CustomTags:           customTags}

		if _, ok := dbTypeMap[md.DBType]; !ok {
			logger.Warningf("Ignoring host \"%s\" - unknown dbtype: %s. Expected one of: %+v", md.DBUniqueName, md.DBType, dbTypes)
			continue
		}

		if md.Encryption == "aes-gcm-256" && opts.AesGcmKeyphrase != "" {
			md.ConnStr = decrypt(md.DBUniqueName, opts.AesGcmKeyphrase, md.ConnStr)
		}

		if md.DBType == config.DbTypePgCont {
			resolved, err := ResolveDatabasesFromConfigEntry(md)
			if err != nil {
				logger.Errorf("Failed to resolve DBs for \"%s\": %s", md.DBUniqueName, err)
				if md.Encryption == "aes-gcm-256" && opts.AesGcmKeyphrase == "" {
					logger.Errorf("No decryption key set. Use the --aes-gcm-keyphrase or --aes-gcm-keyphrase params to set")
				}
				continue
			}
			tempArr := make([]string, 0)
			for _, rdb := range resolved {
				monitoredDBs = append(monitoredDBs, rdb)
				tempArr = append(tempArr, rdb.ConnStr)
			}
			logger.Debugf("Resolved %d DBs with prefix \"%s\": [%s]", len(resolved), md.DBUniqueName, strings.Join(tempArr, ", "))
		} else if md.DBType == config.DbTypePatroni || md.DBType == config.DbTypePatroniCont || md.DBType == config.DbTypePatroniNamespaceDiscovery {
			resolved, err := ResolveDatabasesFromPatroni(md)
			if err != nil {
				logger.Errorf("Failed to resolve DBs for \"%s\": %s", md.DBUniqueName, err)
				continue
			}
			tempArr := make([]string, 0)
			for _, rdb := range resolved {
				monitoredDBs = append(monitoredDBs, rdb)
				tempArr = append(tempArr, rdb.ConnStr)
			}
			logger.Debugf("Resolved %d DBs with prefix \"%s\": [%s]", len(resolved), md.DBUniqueName, strings.Join(tempArr, ", "))
		} else {
			monitoredDBs = append(monitoredDBs, md)
		}
	}
	return monitoredDBs, err
}

func GetMonitoredDatabaseByUniqueName(name string) (MonitoredDatabase, error) {
	monitoredDbCacheLock.RLock()
	defer monitoredDbCacheLock.RUnlock()
	_, exists := monitoredDbCache[name]
	if !exists {
		return MonitoredDatabase{}, errors.New("DBUnique not found")
	}
	return monitoredDbCache[name], nil
}

func UpdateMonitoredDBCache(data []MonitoredDatabase) {
	monitoredDbCacheNew := make(map[string]MonitoredDatabase)

	for _, row := range data {
		monitoredDbCacheNew[row.DBUniqueName] = row
	}

	monitoredDbCacheLock.Lock()
	monitoredDbCache = monitoredDbCacheNew
	monitoredDbCacheLock.Unlock()
}

func MetricsBatcher(ctx context.Context, batchingMaxDelayMillis int64, bufferedStorageCh <-chan []metrics.MetricStoreMessage, storageCh chan<- []metrics.MetricStoreMessage) {
	if batchingMaxDelayMillis <= 0 {
		logger.Fatalf("Check --batching-delay-ms, zero/negative batching delay:", batchingMaxDelayMillis)
	}
	var datapointCounter int
	var maxBatchSize = 1000                        // flush on maxBatchSize metric points or batchingMaxDelayMillis passed
	batch := make([]metrics.MetricStoreMessage, 0) // no size limit here as limited in persister already

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Millisecond * time.Duration(batchingMaxDelayMillis)):
			if len(batch) > 0 {
				flushed := make([]metrics.MetricStoreMessage, len(batch))
				copy(flushed, batch)
				logger.WithField("datasets", len(batch)).Debug("Flushing metrics due to batching timeout")
				storageCh <- flushed
				batch = make([]metrics.MetricStoreMessage, 0)
				datapointCounter = 0
			}
		case msg := <-bufferedStorageCh:
			for _, m := range msg { // in reality msg are sent by fetchers one by one though
				batch = append(batch, m)
				datapointCounter += len(m.Data)
				if datapointCounter > maxBatchSize { // flush. also set some last_sent_timestamp so that ticker would pass a round?
					flushed := make([]metrics.MetricStoreMessage, len(batch))
					copy(flushed, batch)
					logger.WithField("datasets", len(batch)).Debugf("Flushing metrics due to maxBatchSize limit of %d datapoints", maxBatchSize)
					storageCh <- flushed
					batch = make([]metrics.MetricStoreMessage, 0)
					datapointCounter = 0
				}
			}
		}
	}
}

type UIntSlice []uint

func (x UIntSlice) Len() int           { return len(x) }
func (x UIntSlice) Less(i, j int) bool { return x[i] < x[j] }
func (x UIntSlice) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

// assumes upwards compatibility for versions
func GetMetricVersionProperties(metric string, vme DBVersionMapEntry, metricDefMap map[string]map[uint]metrics.MetricProperties) (metrics.MetricProperties, error) {
	var keys UIntSlice
	var mdm map[string]map[uint]metrics.MetricProperties

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

func GetRecommendations(dbUnique string, vme DBVersionMapEntry) (metrics.MetricData, error) {
	retData := make(metrics.MetricData, 0)
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
		dummy := make(metrics.MetricEntry)
		dummy["tag_reco_topic"] = "dummy"
		dummy["tag_object_name"] = "-"
		dummy["recommendation"] = "no recommendations"
		dummy[epochColumnName] = startTimeEpochNs
		dummy["major_ver"] = vme.Version / 10
		retData = append(retData, dummy)
	}
	return retData, nil
}

func FilterPgbouncerData(data metrics.MetricData, databaseToKeep string, vme DBVersionMapEntry) metrics.MetricData {
	filteredData := make(metrics.MetricData, 0)

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

func FetchMetrics(ctx context.Context, msg MetricFetchMessage, hostState map[string]map[string]string, storageCh chan<- []metrics.MetricStoreMessage, context string) ([]metrics.MetricStoreMessage, error) {
	var vme DBVersionMapEntry
	var dbpgVersion uint
	var err, firstErr error
	var sql string
	var retryWithSuperuserSQL = true
	var data, cachedData metrics.MetricData
	var md MonitoredDatabase
	var fromCache, isCacheable bool

	vme, err = DBGetPGVersion(ctx, msg.DBUniqueName, msg.DBType, false)
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

	if msg.DBType == config.DbTypeBouncer {
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
	} else if msg.DBType == config.DbTypePgPOOL {
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
				data = make(metrics.MetricData, 1)
				data[0] = metrics.MetricEntry{"epoch_ns": time.Now().UnixNano(), "is_up": 0} // should be updated if the "instance_up" metric definition is changed
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

	if (opts.Metric.RealDbnameField > "" || opts.Metric.SystemIdentifierField > "") && msg.DBType == config.DbTypePg {
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
		return []metrics.MetricStoreMessage{{DBName: msg.DBUniqueName, MetricName: msg.MetricName, Data: cachedData, CustomTags: md.CustomTags,
			MetricDefinitionDetails: mvp, RealDbname: vme.RealDbname, SystemIdentifier: vme.SystemIdentifier}}, nil
	}
	atomic.AddUint64(&totalMetricsFetchedCounter, uint64(len(data)))
	return []metrics.MetricStoreMessage{{DBName: msg.DBUniqueName, MetricName: msg.MetricName, Data: data, CustomTags: md.CustomTags,
		MetricDefinitionDetails: mvp, RealDbname: vme.RealDbname, SystemIdentifier: vme.SystemIdentifier}}, nil

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

func GetFromInstanceCacheIfNotOlderThanSeconds(msg MetricFetchMessage, maxAgeSeconds int64) metrics.MetricData {
	var clonedData metrics.MetricData
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

func PutToInstanceCache(msg MetricFetchMessage, data metrics.MetricData) {
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
	if !(msg.DBType == config.DbTypePgCont || msg.DBType == config.DbTypePatroniCont) {
		return false
	}
	return mvp.MetricAttrs.IsInstanceLevel
}

func AddDbnameSysinfoIfNotExistsToQueryResultData(msg MetricFetchMessage, data metrics.MetricData, ver DBVersionMapEntry) metrics.MetricData {
	enrichedData := make(metrics.MetricData, 0)

	logger.Debugf("Enriching all rows of [%s:%s] with sysinfo (%s) / real dbname (%s) if set. ", msg.DBUniqueName, msg.MetricName, ver.SystemIdentifier, ver.RealDbname)
	for _, dr := range data {
		if opts.Metric.RealDbnameField > "" && ver.RealDbname > "" {
			old, ok := dr[tagPrefix+opts.Metric.RealDbnameField]
			if !ok || old == "" {
				dr[tagPrefix+opts.Metric.RealDbnameField] = ver.RealDbname
			}
		}
		if opts.Metric.SystemIdentifierField > "" && ver.SystemIdentifier > "" {
			old, ok := dr[tagPrefix+opts.Metric.SystemIdentifierField]
			if !ok || old == "" {
				dr[tagPrefix+opts.Metric.SystemIdentifierField] = ver.SystemIdentifier
			}
		}
		enrichedData = append(enrichedData, dr)
	}
	return enrichedData
}

func StoreMetrics(metrics []metrics.MetricStoreMessage, storageCh chan<- []metrics.MetricStoreMessage) (int, error) {

	if len(metrics) > 0 {
		atomic.AddUint64(&totalDatasetsFetchedCounter, 1)
		storageCh <- metrics
		return len(metrics), nil
	}

	return 0, nil
}

func deepCopyMetricData(data metrics.MetricData) metrics.MetricData {
	newData := make(metrics.MetricData, len(data))

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
func MetricGathererLoop(ctx context.Context, dbUniqueName, dbUniqueNameOrig, dbType, metricName string, configMap map[string]float64, controlCh <-chan ControlMessage, storeCh chan<- []metrics.MetricStoreMessage) {
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
			vme, err = DBGetPGVersion(ctx, dbUniqueName, dbType, false) // in case of errors just ignore metric "disabled" time ranges
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
		var metricStoreMessages []metrics.MetricStoreMessage
		var err error
		mfm := MetricFetchMessage{
			DBUniqueName:        dbUniqueName,
			DBUniqueNameOrig:    dbUniqueNameOrig,
			MetricName:          metricName,
			DBType:              dbType,
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
								detectedChangesSummary := make(metrics.MetricData, 0)
								entry := metrics.MetricEntry{"details": message, "epoch_ns": (metricStoreMessages[0].Data)[0]["epoch_ns"]}
								detectedChangesSummary = append(detectedChangesSummary, entry)
								metricStoreMessages = append(metricStoreMessages,
									metrics.MetricStoreMessage{DBName: dbUniqueName, DBType: dbType,
										MetricName: "object_changes", Data: detectedChangesSummary, CustomTags: metricStoreMessages[0].CustomTags})
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

func FetchStatsDirectlyFromOS(msg MetricFetchMessage, vme DBVersionMapEntry, mvp metrics.MetricProperties) ([]metrics.MetricStoreMessage, error) {
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
	return []metrics.MetricStoreMessage{msm}, nil
}

// data + custom tags + counters
func DatarowsToMetricstoreMessage(data metrics.MetricData, msg MetricFetchMessage, vme DBVersionMapEntry, mvp metrics.MetricProperties) metrics.MetricStoreMessage {
	md, err := GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		logger.Errorf("Could not resolve DBUniqueName %s, cannot set custom attributes for gathered data: %v", msg.DBUniqueName, err)
	}

	atomic.AddUint64(&totalMetricsFetchedCounter, uint64(len(data)))

	return metrics.MetricStoreMessage{
		DBName:                  msg.DBUniqueName,
		DBType:                  msg.DBType,
		MetricName:              msg.MetricName,
		CustomTags:              md.CustomTags,
		Data:                    data,
		MetricDefinitionDetails: mvp,
		RealDbname:              vme.RealDbname,
		SystemIdentifier:        vme.SystemIdentifier,
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

func IsInDisabledTimeDayRange(localTime time.Time, metricAttrsDisabledDays string, metricAttrsDisabledTimes []string, hostConfigPerMetricDisabledTimes []HostConfigPerMetricDisabledTimes, metric, dbUnique string) bool {
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

func jsonTextToMap(jsonText string) (map[string]float64, error) {
	retmap := make(map[string]float64)
	if jsonText == "" {
		return retmap, nil
	}
	var hostConfig map[string]any
	if err := json.Unmarshal([]byte(jsonText), &hostConfig); err != nil {
		return nil, err
	}
	for k, v := range hostConfig {
		retmap[k] = v.(float64)
	}
	return retmap, nil
}

func jsonTextToStringMap(jsonText string) (map[string]string, error) {
	retmap := make(map[string]string)
	if jsonText == "" {
		return retmap, nil
	}
	var iMap map[string]any
	if err := json.Unmarshal([]byte(jsonText), &iMap); err != nil {
		return nil, err
	}
	for k, v := range iMap {
		retmap[k] = fmt.Sprintf("%v", v)
	}
	return retmap, nil
}

// Expects "preset metrics" definition file named preset-config.yaml to be present in provided --metrics folder
func ReadPresetMetricsConfigFromFolder(folder string, _ bool) (map[string]map[string]float64, error) {
	pmm := make(map[string]map[string]float64)

	logger.Infof("Reading preset metric config from path %s ...", path.Join(folder, presetConfigYAMLFile))
	presetMetrics, err := os.ReadFile(path.Join(folder, presetConfigYAMLFile))
	if err != nil {
		logger.Errorf("Failed to read preset metric config definition at: %s", folder)
		return pmm, err
	}
	pcs := make([]PresetConfig, 0)
	err = yaml.Unmarshal(presetMetrics, &pcs)
	if err != nil {
		logger.Errorf("Unmarshaling error reading preset metric config: %v", err)
		return pmm, err
	}
	for _, pc := range pcs {
		pmm[pc.Name] = pc.Metrics
	}
	logger.Infof("%d preset metric definitions found", len(pcs))
	return pmm, err
}

func ExpandEnvVarsForConfigEntryIfStartsWithDollar(md MonitoredDatabase) (MonitoredDatabase, int) {
	var changed int

	if strings.HasPrefix(md.Encryption, "$") {
		md.Encryption = os.ExpandEnv(md.Encryption)
		changed++
	}
	if strings.HasPrefix(md.DBType, "$") {
		md.DBType = os.ExpandEnv(md.DBType)
		changed++
	}
	if strings.HasPrefix(md.DBUniqueName, "$") {
		md.DBUniqueName = os.ExpandEnv(md.DBUniqueName)
		changed++
	}
	if strings.HasPrefix(md.DBNameIncludePattern, "$") {
		md.DBNameIncludePattern = os.ExpandEnv(md.DBNameIncludePattern)
		changed++
	}
	if strings.HasPrefix(md.DBNameExcludePattern, "$") {
		md.DBNameExcludePattern = os.ExpandEnv(md.DBNameExcludePattern)
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

func ConfigFileToMonitoredDatabases(configFilePath string) ([]MonitoredDatabase, error) {
	hostList := make([]MonitoredDatabase, 0)

	logger.Debugf("Converting monitoring YAML config to MonitoredDatabase from path %s ...", configFilePath)
	yamlFile, err := os.ReadFile(configFilePath)
	if err != nil {
		logger.Errorf("Error reading file %s: %s", configFilePath, err)
		return hostList, err
	}
	// TODO check mod timestamp or hash, from a global "caching map"
	c := make([]MonitoredDatabase, 0) // there can be multiple configs in a single file
	yamlFile = []byte(string(yamlFile))
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		logger.Errorf("Unmarshaling error: %v", err)
		return hostList, err
	}
	for _, v := range c {
		if v.DBType == "" {
			v.DBType = config.DbTypePg
		}
		if v.IsEnabled {
			logger.Debugf("Found active monitoring config entry: %#v", v)
			if v.Group == "" {
				v.Group = "default"
			}
			vExp, changed := ExpandEnvVarsForConfigEntryIfStartsWithDollar(v)
			if changed > 0 {
				logger.Debugf("[%s] %d config attributes expanded from ENV", vExp.DBUniqueName, changed)
			}
			hostList = append(hostList, vExp)
		}
	}
	if len(hostList) == 0 {
		logger.Warningf("Could not find any valid monitoring configs from file: %s", configFilePath)
	}
	return hostList, nil
}

// reads through the YAML files containing descriptions on which hosts to monitor
func ReadMonitoringConfigFromFileOrFolder(fileOrFolder string) ([]MonitoredDatabase, error) {
	hostList := make([]MonitoredDatabase, 0)

	fi, err := os.Stat(fileOrFolder)
	if err != nil {
		logger.Errorf("Could not Stat() path: %s", fileOrFolder)
		return hostList, err
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		logger.Infof("Reading monitoring config from path %s ...", fileOrFolder)

		err := filepath.Walk(fileOrFolder, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err // abort on first failure
			}
			if info.Mode().IsRegular() && (strings.HasSuffix(strings.ToLower(info.Name()), ".yaml") || strings.HasSuffix(strings.ToLower(info.Name()), ".yml")) {
				logger.Debug("Found YAML config file:", info.Name())
				mdbs, err := ConfigFileToMonitoredDatabases(path)
				if err == nil {
					hostList = append(hostList, mdbs...)
				}
			}
			return nil
		})
		if err != nil {
			logger.Errorf("Could not successfully Walk() path %s: %s", fileOrFolder, err)
			return hostList, err
		}
	case mode.IsRegular():
		hostList, err = ConfigFileToMonitoredDatabases(fileOrFolder)
	}

	return hostList, err
}

// Resolves regexes if exact DBs were not specified exact
func GetMonitoredDatabasesFromMonitoringConfig(mc []MonitoredDatabase) []MonitoredDatabase {
	md := make([]MonitoredDatabase, 0)
	if len(mc) == 0 {
		return md
	}
	for _, e := range mc {
		//log.Debugf("Processing config item: %#v", e)
		if e.Metrics == nil && len(e.PresetMetrics) > 0 {
			mdef, ok := presetMetricDefMap[e.PresetMetrics]
			if !ok {
				logger.Errorf("Failed to resolve preset config \"%s\" for \"%s\"", e.PresetMetrics, e.DBUniqueName)
				continue
			}
			e.Metrics = mdef
		}
		if _, ok := dbTypeMap[e.DBType]; !ok {
			logger.Warningf("Ignoring host \"%s\" - unknown dbtype: %s. Expected one of: %+v", e.DBUniqueName, e.DBType, dbTypes)
			continue
		}
		if e.IsEnabled && e.Encryption == "aes-gcm-256" && opts.AesGcmKeyphrase != "" {
			e.ConnStr = decrypt(e.DBUniqueName, opts.AesGcmKeyphrase, e.ConnStr)
		}
		if e.DBType == config.DbTypePatroni && e.GetDatabaseName() == "" {
			logger.Warningf("Ignoring host \"%s\" as \"dbname\" attribute not specified but required by dbtype=patroni", e.DBUniqueName)
			continue
		}
		if e.DBType == config.DbTypePg && e.GetDatabaseName() == "" {
			logger.Warningf("Ignoring host \"%s\" as \"dbname\" attribute not specified but required by dbtype=postgres", e.DBUniqueName)
			continue
		}
		if len(e.GetDatabaseName()) == 0 || e.DBType == config.DbTypePgCont || e.DBType == config.DbTypePatroni || e.DBType == config.DbTypePatroniCont || e.DBType == config.DbTypePatroniNamespaceDiscovery {
			if e.DBType == config.DbTypePgCont {
				logger.Debugf("Adding \"%s\" (host=%s, port=%s) to continuous monitoring ...", e.DBUniqueName, e.ConnStr)
			}
			var foundDbs []MonitoredDatabase
			var err error

			if e.DBType == config.DbTypePatroni || e.DBType == config.DbTypePatroniCont || e.DBType == config.DbTypePatroniNamespaceDiscovery {
				foundDbs, err = ResolveDatabasesFromPatroni(e)
			} else {
				foundDbs, err = ResolveDatabasesFromConfigEntry(e)
			}
			if err != nil {
				logger.Errorf("Failed to resolve DBs for \"%s\": %s", e.DBUniqueName, err)
				continue
			}
			tempArr := make([]string, 0)
			for _, r := range foundDbs {
				md = append(md, r)
				tempArr = append(tempArr, r.GetDatabaseName())
			}
			logger.Debugf("Resolved %d DBs with prefix \"%s\": [%s]", len(foundDbs), e.DBUniqueName, strings.Join(tempArr, ", "))
		} else {
			md = append(md, e)
		}
	}
	return md
}

func getMonitoredDatabasesSnapshot() map[string]MonitoredDatabase {
	mdSnap := make(map[string]MonitoredDatabase)

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

func FilterMonitoredDatabasesByGroup(monitoredDBs []MonitoredDatabase, group string) ([]MonitoredDatabase, int) {
	ret := make([]MonitoredDatabase, 0)
	groups := strings.Split(group, ",")
	for _, md := range monitoredDBs {
		// matched := false
		for _, g := range groups {
			if md.Group == g {
				ret = append(ret, md)
				break
			}
		}
	}
	return ret, len(monitoredDBs) - len(ret)
}

func encrypt(passphrase, plaintext string) string { // called when --password-to-encrypt set
	key, salt := deriveKey(passphrase, nil)
	iv := make([]byte, 12)
	_, _ = rand.Read(iv)
	b, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(b)
	data := aesgcm.Seal(nil, iv, []byte(plaintext), nil)
	return hex.EncodeToString(salt) + "-" + hex.EncodeToString(iv) + "-" + hex.EncodeToString(data)
}

func deriveKey(passphrase string, salt []byte) ([]byte, []byte) {
	if salt == nil {
		salt = make([]byte, 8)
		_, _ = rand.Read(salt)
	}
	return pbkdf2.Key([]byte(passphrase), salt, 1000, 32, sha256.New), salt
}

func decrypt(dbUnique, passphrase, ciphertext string) string {
	arr := strings.Split(ciphertext, "-")
	if len(arr) != 3 {
		logger.Warningf("Aes-gcm-256 encrypted password for \"%s\" should consist of 3 parts - using 'as is'", dbUnique)
		return ciphertext
	}
	salt, _ := hex.DecodeString(arr[0])
	iv, _ := hex.DecodeString(arr[1])
	data, _ := hex.DecodeString(arr[2])
	key, _ := deriveKey(passphrase, salt)
	b, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(b)
	data, _ = aesgcm.Open(nil, iv, data, nil)
	//log.Debug("decoded", string(data))
	return string(data)
}

func SyncMonitoredDBsToDatastore(ctx context.Context, monitoredDbs []MonitoredDatabase, persistenceChannel chan []metrics.MetricStoreMessage) {
	if len(monitoredDbs) > 0 {
		msms := make([]metrics.MetricStoreMessage, len(monitoredDbs))
		now := time.Now()

		for _, mdb := range monitoredDbs {
			db := metrics.MetricEntry{
				"tag_group":                   mdb.Group,
				"master_only":                 mdb.OnlyIfMaster,
				"epoch_ns":                    now.UnixNano(),
				"continuous_discovery_prefix": mdb.DBUniqueNameOrig,
			}
			for k, v := range mdb.CustomTags {
				db["tag_"+k] = v
			}
			msms = append(msms, metrics.MetricStoreMessage{
				DBName:     mdb.DBUniqueName,
				MetricName: monitoredDbsDatastoreSyncMetricName,
				Data:       metrics.MetricData{db},
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

func shouldDbBeMonitoredBasedOnCurrentState(md MonitoredDatabase) bool {
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

func CloseResourcesForRemovedMonitoredDBs(metricsWriter *sinks.MultiWriter, currentDBs, prevLoopDBs []MonitoredDatabase, shutDownDueToRoleChange map[string]bool) {
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

var opts *config.CmdOptions

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

func main() {
	var (
		err           error
		cancel        context.CancelFunc
		metricsWriter *sinks.MultiWriter
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
		if opts != nil && opts.VersionOnly() {
			printVersion()
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

	if opts.AesGcmPasswordToEncrypt > "" { // special flag - encrypt and exit
		fmt.Println(encrypt(opts.AesGcmKeyphrase, opts.AesGcmPasswordToEncrypt))
		return
	}

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
		if configDb, err = db.InitAndTestConfigStoreConnection(mainContext, opts.Connection.Config); err != nil {
			logger.WithError(err).Fatal("Could not connect to configuration database")
		}
	}

	if opts.Connection.Init {
		return
	}

	pgBouncerNumericCountersStartVersion = VersionToInt("1.12")

	if !opts.Ping {
		go StatsSummarizer(mainContext)
	}

	controlChannels := make(map[string](chan ControlMessage)) // [db1+metric1]=chan
	persistCh := make(chan []metrics.MetricStoreMessage, 10000)
	var bufferedPersistCh chan []metrics.MetricStoreMessage

	if !opts.Ping {

		if opts.BatchingDelayMs > 0 {
			bufferedPersistCh = make(chan []metrics.MetricStoreMessage, 10000) // "staging area" for metric storage batching, when enabled
			logger.Info("starting MetricsBatcher...")
			go MetricsBatcher(mainContext, opts.BatchingDelayMs, bufferedPersistCh, persistCh)
		}

		if metricsWriter, err = sinks.NewMultiWriter(mainContext, opts); err != nil {
			logger.Fatal(err)
		}
		go metricsWriter.WriteMetrics(mainContext, persistCh)

		_, _ = daemon.SdNotify(false, "READY=1") // Notify systemd, does nothing outside of systemd
	}

	firstLoop := true
	mainLoopCount := 0
	var monitoredDbs []MonitoredDatabase
	var lastMetricsRefreshTime int64
	var metricDefs map[string]map[uint]metrics.MetricProperties
	var renamingDefs map[string]string
	var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
	var metricConfig map[string]float64                       // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state

	for { //main loop
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		var controlChannelNameList []string
		gatherersShutDown := 0

		if time.Now().Unix()-lastMetricsRefreshTime > metricDefinitionRefreshTime {
			if fileBasedMetrics {
				metricDefs, renamingDefs, err = metrics.ReadMetricsFromFolder(mainContext, opts.Metric.MetricsFolder)
			} else {
				metricDefs, renamingDefs, err = db.ReadMetricsFromPostgres(mainContext, configDb)
			}
			if err == nil {
				UpdateMetricDefinitions(metricDefs, renamingDefs)
				lastMetricsRefreshTime = time.Now().Unix()
			} else {
				if firstLoop {
					logger.Fatal(err)
				}
				logger.Errorf("Could not refresh metric definitions: %w", err)
			}
		}

		if fileBasedMetrics {
			pmc, err := ReadPresetMetricsConfigFromFolder(opts.Metric.MetricsFolder, false)
			if err != nil {
				if firstLoop {
					logger.Fatalf("Could not read preset metric config from \"%s\": %s", path.Join(opts.Metric.MetricsFolder, presetConfigYAMLFile), err)
				} else {
					logger.Errorf("Could not read preset metric config from \"%s\": %s", path.Join(opts.Metric.MetricsFolder, presetConfigYAMLFile), err)
				}
			} else {
				presetMetricDefMap = pmc
				logger.Debugf("Loaded preset metric config: %#v", pmc)
			}

			if opts.IsAdHocMode() {
				adhocconfig, ok := pmc[opts.AdHocConfig]
				if !ok {
					logger.Warningf("Could not find a preset metric config named \"%s\", assuming JSON config...", opts.AdHocConfig)
					adhocconfig, err = jsonTextToMap(opts.AdHocConfig)
					if err != nil {
						logger.Fatalf("Could not parse --adhoc-config(%s): %v", opts.AdHocConfig, err)
					}
				}
				md := MonitoredDatabase{DBUniqueName: opts.AdHocUniqueName, DBType: opts.AdHocDBType, Metrics: adhocconfig, ConnStr: opts.AdHocConnString}
				if opts.AdHocDBType == config.DbTypePg {
					monitoredDbs = []MonitoredDatabase{md}
				} else {
					resolved, err := ResolveDatabasesFromConfigEntry(md)
					if err != nil {
						if firstLoop {
							logger.Fatalf("Failed to resolve DBs for ConnStr \"%s\": %s", opts.AdHocConnString, err)
						} else { // keep previously found list
							logger.Errorf("Failed to resolve DBs for ConnStr \"%s\": %s", opts.AdHocConnString, err)
						}
					} else {
						monitoredDbs = resolved
					}
				}
			} else {
				mc, err := ReadMonitoringConfigFromFileOrFolder(opts.Connection.Config)
				if err == nil {
					logger.Debugf("Found %d monitoring config entries", len(mc))
					if len(opts.Metric.Group) > 0 {
						var removedCount int
						mc, removedCount = FilterMonitoredDatabasesByGroup(mc, opts.Metric.Group)
						logger.Infof("Filtered out %d config entries based on --groups=%s", removedCount, opts.Metric.Group)
					}
					monitoredDbs = GetMonitoredDatabasesFromMonitoringConfig(mc)
					logger.Debugf("Found %d databases to monitor from %d config items...", len(monitoredDbs), len(mc))
				} else {
					if firstLoop {
						logger.Fatalf("Could not read/parse monitoring config from path: %s. err: %v", opts.Connection.Config, err)
					} else {
						logger.Errorf("Could not read/parse monitoring config from path: %s. using last valid config data. err: %v", opts.Connection.Config, err)
					}
					time.Sleep(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds))
					continue
				}
			}
		} else {
			monitoredDbs, err = GetMonitoredDatabasesFromConfigDB()
			if err != nil {
				if firstLoop {
					logger.Fatal("could not fetch active hosts - check config!", err)
				} else {
					logger.Error("could not fetch active hosts, using last valid config data. err:", err)
					time.Sleep(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds))
					continue
				}
			}
		}

		if DoesEmergencyTriggerfileExist() {
			logger.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", opts.EmergencyPauseTriggerfile)
			monitoredDbs = make([]MonitoredDatabase, 0)
		}

		UpdateMonitoredDBCache(monitoredDbs)

		if lastMonitoredDBsUpdate.IsZero() || lastMonitoredDBsUpdate.Before(time.Now().Add(-1*time.Second*monitoredDbsDatastoreSyncIntervalSeconds)) {
			monitoredDbsCopy := make([]MonitoredDatabase, len(monitoredDbs))
			copy(monitoredDbsCopy, monitoredDbs)
			if opts.BatchingDelayMs > 0 {
				go SyncMonitoredDBsToDatastore(mainContext, monitoredDbsCopy, bufferedPersistCh)
			} else {
				go SyncMonitoredDBsToDatastore(mainContext, monitoredDbsCopy, persistCh)
			}
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
			dbType := host.DBType
			metricConfig = host.Metrics
			wasInstancePreviouslyDormant := IsDBDormant(dbUnique)

			if host.Encryption == "aes-gcm-256" && len(opts.AesGcmKeyphrase) == 0 && len(opts.AesGcmKeyphraseFile) == 0 {
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

				ver, err = DBGetPGVersion(mainContext, dbUnique, dbType, true)
				if err != nil {
					logger.Errorf("could not start metric gathering for DB \"%s\" due to connection problem: %s", dbUnique, err)
					if opts.AdHocConnString != "" {
						logger.Errorf("will retry in %ds...", opts.Connection.ServersRefreshLoopSeconds)
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

				if !opts.Ping && (host.IsSuperuser || opts.IsAdHocMode() && opts.AdHocCreateHelpers) && IsPostgresDBType(dbType) && !ver.IsInRecovery {
					if opts.Metric.NoHelperFunctions {
						logger.Infof("[%s] Skipping rollout out helper functions due to the --no-helper-functions flag ...", dbUnique)
					} else {
						logger.Infof("Trying to create helper functions if missing for \"%s\"...", dbUnique)
						_ = TryCreateMetricsFetchingHelpers(dbUnique)
					}
				}

			}

			if IsPostgresDBType(host.DBType) {
				var DBSizeMB int64

				if opts.MinDbSizeMB >= 8 { // an empty DB is a bit less than 8MB
					DBSizeMB, _ = DBGetSizeMB(dbUnique) // ignore errors, i.e. only remove from monitoring when we're certain it's under the threshold
					if DBSizeMB != 0 {
						if DBSizeMB < opts.MinDbSizeMB {
							logger.Infof("[%s] DB will be ignored due to the --min-db-size-mb filter. Current (up to %v cached) DB size = %d MB", dbUnique, dbSizeCachingInterval, DBSizeMB)
							hostsToShutDownDueToRoleChange[dbUnique] = true // for the case when DB size was previosly above the threshold
							SetUndersizedDBState(dbUnique, true)
							continue
						}
						SetUndersizedDBState(dbUnique, false)
					}
				}
				ver, err := DBGetPGVersion(mainContext, dbUnique, host.DBType, false)
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
							vme, err := DBGetPGVersion(mainContext, dbUnique, dbType, false)
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

						if err := metricsWriter.SyncMetrics(dbUnique, metricNameForStorage, "add"); err != nil {
							logger.Error(err)
						}

						if opts.BatchingDelayMs > 0 {
							go MetricGathererLoop(mainContext, dbUnique, dbUniqueOrig, dbType, metric, metricConfig, controlChannels[dbMetric], bufferedPersistCh)
						} else {
							go MetricGathererLoop(mainContext, dbUnique, dbUniqueOrig, dbType, metric, metricConfig, controlChannels[dbMetric], persistCh)
						}
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

		atomic.StoreInt32(&mainLoopInitialized, 1) // to hold off scraping until metric fetching runners have been initialized

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
			var dbInfo MonitoredDatabase
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
				if err := metricsWriter.SyncMetrics(db, metric, "remove"); err != nil {
					logger.Error(err)
				}
			}
		}

		if gatherersShutDown > 0 {
			logger.Warningf("sent STOP message to %d gatherers (it might take some time for them to stop though)", gatherersShutDown)
		}

		// Destroy conn pools and metric writers
		CloseResourcesForRemovedMonitoredDBs(metricsWriter, monitoredDbs, prevLoopMonitoredDBs, hostsToShutDownDueToRoleChange)

	MainLoopSleep:
		mainLoopCount++
		prevLoopMonitoredDBs = monitoredDbs

		logger.Debugf("main sleeping %ds...", opts.Connection.ServersRefreshLoopSeconds)
		select {
		case <-time.After(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds)):
			// pass
		case <-mainContext.Done():
			return
		}
	}
}
