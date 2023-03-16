package main

import (
	"container/list"
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
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/coreos/go-systemd/daemon"
	"github.com/marpaia/graphite-golang"
	"github.com/op/go-logging"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/pbkdf2"
	"gopkg.in/yaml.v2"
)

type MonitoredDatabase struct {
	DBUniqueName         string `yaml:"unique_name"`
	DBUniqueNameOrig     string // to preserve belonging to a specific instance for continuous modes where DBUniqueName will be dynamic
	Group                string
	Host                 string
	Port                 string
	DBName               string
	User                 string
	Password             string
	PasswordType         string `yaml:"password_type"`
	LibPQConnStr         string `yaml:"libpq_conn_str"`
	SslMode              string
	SslRootCAPath        string             `yaml:"sslrootcert"`
	SslClientCertPath    string             `yaml:"sslcert"`
	SslClientKeyPath     string             `yaml:"sslkey"`
	Metrics              map[string]float64 `yaml:"custom_metrics"`
	MetricsStandby       map[string]float64 `yaml:"custom_metrics_standby"`
	StmtTimeout          int64              `yaml:"stmt_timeout"`
	DBType               string
	DBNameIncludePattern string            `yaml:"dbname_include_pattern"`
	DBNameExcludePattern string            `yaml:"dbname_exclude_pattern"`
	PresetMetrics        string            `yaml:"preset_metrics"`
	PresetMetricsStandby string            `yaml:"preset_metrics_standby"`
	IsSuperuser          bool              `yaml:"is_superuser"`
	IsEnabled            bool              `yaml:"is_enabled"`
	CustomTags           map[string]string `yaml:"custom_tags"` // ignored on graphite
	HostConfig           HostConfigAttrs   `yaml:"host_config"`
	OnlyIfMaster         bool              `yaml:"only_if_master"`
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

type PatroniClusterMember struct {
	Scope   string
	Name    string
	ConnURL string `yaml:"conn_url"`
	Role    string
}

type PresetConfig struct {
	Name        string
	Description string
	Metrics     map[string]float64
}

type MetricColumnAttrs struct {
	PrometheusGaugeColumns    []string `yaml:"prometheus_gauge_columns"`
	PrometheusIgnoredColumns  []string `yaml:"prometheus_ignored_columns"` // for cases where we don't want some columns to be exposed in Prom mode
	PrometheusAllGaugeColumns bool     `yaml:"prometheus_all_gauge_columns"`
}

type MetricAttrs struct {
	IsInstanceLevel           bool                 `yaml:"is_instance_level"`
	MetricStorageName         string               `yaml:"metric_storage_name"`
	ExtensionVersionOverrides []ExtensionOverrides `yaml:"extension_version_based_overrides"`
	IsPrivate                 bool                 `yaml:"is_private"`                // used only for extension overrides currently and ignored otherwise
	DisabledDays              string               `yaml:"disabled_days"`             // Cron style, 0 = Sunday. Ranges allowed: 0,2-4
	DisableTimes              []string             `yaml:"disabled_times"`            // "11:00-13:00"
	StatementTimeoutSeconds   int64                `yaml:"statement_timeout_seconds"` // overrides per monitored DB settings
}

type MetricVersionProperties struct {
	Sql                  string
	SqlSU                string
	MasterOnly           bool
	StandbyOnly          bool
	ColumnAttrs          MetricColumnAttrs // Prometheus Metric Type (Counter is default) and ignore list
	MetricAttrs          MetricAttrs
	CallsHelperFunctions bool
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

type MetricStoreMessage struct {
	DBUniqueName            string
	DBType                  string
	MetricName              string
	CustomTags              map[string]string
	Data                    [](map[string]interface{})
	MetricDefinitionDetails MetricVersionProperties
	RealDbname              string
	SystemIdentifier        string
}

type MetricStoreMessagePostgres struct {
	Time    time.Time
	DBName  string
	Metric  string
	Data    map[string]interface{}
	TagData map[string]interface{}
}

type ChangeDetectionResults struct { // for passing around DDL/index/config change detection results
	Created int
	Altered int
	Dropped int
}

type DBVersionMapEntry struct {
	LastCheckedOn    time.Time
	IsInRecovery     bool
	Version          decimal.Decimal
	VersionStr       string
	RealDbname       string
	SystemIdentifier string
	IsSuperuser      bool // if true and no helpers are installed, use superuser SQL version of metric if available
	Extensions       map[string]decimal.Decimal
	ExecEnv          string
	ApproxDBSizeB    int64
}

type ExistingPartitionInfo struct {
	StartTime time.Time
	EndTime   time.Time
}

type ExtensionOverrides struct {
	TargetMetric              string          `yaml:"target_metric"`
	ExpectedExtensionVersions []ExtensionInfo `yaml:"expected_extension_versions"`
}

type ExtensionInfo struct {
	ExtName       string          `yaml:"ext_name"`
	ExtMinVersion decimal.Decimal `yaml:"ext_min_version"`
}

const EPOCH_COLUMN_NAME string = "epoch_ns" // this column (epoch in nanoseconds) is expected in every metric query
const TAG_PREFIX string = "tag_"
const METRIC_DEFINITION_REFRESH_TIME int64 = 120 // min time before checking for new/changed metric definitions
const GRAPHITE_METRICS_PREFIX string = "pgwatch3"
const PERSIST_QUEUE_MAX_SIZE = 10000 // storage queue max elements. when reaching the limit, older metrics will be dropped.
// actual requirements depend a lot of metric type and nr. of obects in schemas,
// but 100k should be enough for 24h, assuming 5 hosts monitored with "exhaustive" preset config. this would also require ~2 GB RAM per one Influx host
const DATASTORE_GRAPHITE = "graphite"
const DATASTORE_JSON = "json"
const DATASTORE_POSTGRES = "postgres"
const DATASTORE_PROMETHEUS = "prometheus"
const PRESET_CONFIG_YAML_FILE = "preset-configs.yaml"
const FILE_BASED_METRIC_HELPERS_DIR = "00_helpers"
const PG_CONN_RECYCLE_SECONDS = 1800 // applies for monitored nodes
const APPLICATION_NAME = "pgwatch3"  // will be set on all opened PG connections for informative purposes
const GATHERER_STATUS_START = "START"
const GATHERER_STATUS_STOP = "STOP"
const METRICDB_IDENT = "metricDb"
const CONFIGDB_IDENT = "configDb"
const CONTEXT_PROMETHEUS_SCRAPE = "prometheus-scrape"
const DCS_TYPE_ETCD = "etcd"
const DCS_TYPE_ZOOKEEPER = "zookeeper"
const DCS_TYPE_CONSUL = "consul"

const MONITORED_DBS_DATASTORE_SYNC_INTERVAL_SECONDS = 600         // write actively monitored DBs listing to metrics store after so many seconds
const MONITORED_DBS_DATASTORE_SYNC_METRIC_NAME = "configured_dbs" // FYI - for Postgres datastore there's also the admin.all_unique_dbnames table with all recent DB unique names with some metric data
const RECO_PREFIX = "reco_"                                       // special handling for metrics with such prefix, data stored in RECO_METRIC_NAME
const RECO_METRIC_NAME = "recommendations"
const SPECIAL_METRIC_CHANGE_EVENTS = "change_events"
const SPECIAL_METRIC_SERVER_LOG_EVENT_COUNTS = "server_log_event_counts"
const SPECIAL_METRIC_PGBOUNCER = "^pgbouncer_(stats|pools)$"
const SPECIAL_METRIC_PGPOOL_STATS = "pgpool_stats"
const SPECIAL_METRIC_INSTANCE_UP = "instance_up"
const SPECIAL_METRIC_DB_SIZE = "db_size"         // can be transparently switched to db_size_approx on instances with very slow FS access (Azure Single Server)
const SPECIAL_METRIC_TABLE_STATS = "table_stats" // can be transparently switched to table_stats_approx on instances with very slow FS (Azure Single Server)
const METRIC_CPU_LOAD = "cpu_load"
const METRIC_PSUTIL_CPU = "psutil_cpu"
const METRIC_PSUTIL_DISK = "psutil_disk"
const METRIC_PSUTIL_DISK_IO_TOTAL = "psutil_disk_io_total"
const METRIC_PSUTIL_MEM = "psutil_mem"
const DEFAULT_METRICS_DEFINITION_PATH_PKG = "/etc/pgwatch3/metrics" // prebuilt packages / Docker default location
const DEFAULT_METRICS_DEFINITION_PATH_DOCKER = "/pgwatch3/metrics"  // prebuilt packages / Docker default location
const DB_SIZE_CACHING_INTERVAL = 30 * time.Minute
const DB_METRIC_JOIN_STR = "¤¤¤" // just some unlikely string for a DB name to avoid using maps of maps for DB+metric data
const EXEC_ENV_UNKNOWN = "UNKNOWN"
const EXEC_ENV_AZURE_SINGLE = "AZURE_SINGLE"
const EXEC_ENV_AZURE_FLEXIBLE = "AZURE_FLEXIBLE"
const EXEC_ENV_GOOGLE = "GOOGLE"

var dbTypeMap = map[string]bool{config.DBTYPE_PG: true, config.DBTYPE_PG_CONT: true, config.DBTYPE_BOUNCER: true, config.DBTYPE_PATRONI: true, config.DBTYPE_PATRONI_CONT: true, config.DBTYPE_PGPOOL: true, config.DBTYPE_PATRONI_NAMESPACE_DISCOVERY: true}
var dbTypes = []string{config.DBTYPE_PG, config.DBTYPE_PG_CONT, config.DBTYPE_BOUNCER, config.DBTYPE_PATRONI, config.DBTYPE_PATRONI_CONT, config.DBTYPE_PATRONI_NAMESPACE_DISCOVERY} // used for informational purposes
var specialMetrics = map[string]bool{RECO_METRIC_NAME: true, SPECIAL_METRIC_CHANGE_EVENTS: true, SPECIAL_METRIC_SERVER_LOG_EVENT_COUNTS: true}
var directlyFetchableOSMetrics = map[string]bool{METRIC_PSUTIL_CPU: true, METRIC_PSUTIL_DISK: true, METRIC_PSUTIL_DISK_IO_TOTAL: true, METRIC_PSUTIL_MEM: true, METRIC_CPU_LOAD: true}
var graphiteConnection *graphite.Graphite
var graphite_host string
var graphite_port int
var log = logging.MustGetLogger("main")
var metric_def_map map[string]map[decimal.Decimal]MetricVersionProperties
var metric_def_map_lock = sync.RWMutex{}
var host_metric_interval_map = make(map[string]float64) // [db1_metric] = 30
var db_pg_version_map = make(map[string]DBVersionMapEntry)
var db_pg_version_map_lock = sync.RWMutex{}
var db_get_pg_version_map_lock = make(map[string]*sync.RWMutex) // synchronize initial PG version detection to 1 instance for each defined host
var monitored_db_cache map[string]MonitoredDatabase
var monitored_db_cache_lock sync.RWMutex

var monitored_db_conn_cache_lock = sync.RWMutex{}
var last_sql_fetch_error sync.Map
var influx_host_count = 1
var influxConnectStrings [2]string // Max. 2 Influx metrics stores currently supported
var influxSkipSSLCertVerify, influxSkipSSLCertVerify2 bool

// secondary Influx meant for HA or Grafana load balancing for 100+ instances with lots of alerts
var fileBasedMetrics = false
var preset_metric_def_map map[string]map[string]float64 // read from metrics folder in "file mode"
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
var partitionMapMetric = make(map[string]ExistingPartitionInfo)                  // metric = min/max bounds
var partitionMapMetricDbname = make(map[string]map[string]ExistingPartitionInfo) // metric[dbname = min/max bounds]
var testDataGenerationModeWG sync.WaitGroup
var PGDummyMetricTables = make(map[string]time.Time)
var PGDummyMetricTablesLock = sync.RWMutex{}
var PGSchemaType string
var failedInitialConnectHosts = make(map[string]bool) // hosts that couldn't be connected to even once
var forceRecreatePGMetricPartitions = false           // to signal override PG metrics storage cache
var lastMonitoredDBsUpdate time.Time
var instanceMetricCache = make(map[string]([]map[string]interface{})) // [dbUnique+metric]lastly_fetched_data
var instanceMetricCacheLock = sync.RWMutex{}
var instanceMetricCacheTimestamp = make(map[string]time.Time) // [dbUnique+metric]last_fetch_time
var instanceMetricCacheTimestampLock = sync.RWMutex{}
var MinExtensionInfoAvailable, _ = decimal.NewFromString("9.1")
var regexIsAlpha = regexp.MustCompile("^[a-zA-Z]+$")
var rBouncerAndPgpoolVerMatch = regexp.MustCompile(`\d+\.+\d+`) // extract $major.minor from "4.1.2 (karasukiboshi)" or "PgBouncer 1.12.0"
var regexIsPgbouncerMetrics = regexp.MustCompile(SPECIAL_METRIC_PGBOUNCER)
var unreachableDBsLock sync.RWMutex
var unreachableDB = make(map[string]time.Time)
var pgBouncerNumericCountersStartVersion decimal.Decimal // pgBouncer changed internal counters data type in v1.12
// "cache" of last CPU utilization stats for GetGoPsutilCPU to get more exact results and not having to sleep
var prevCPULoadTimeStatsLock sync.RWMutex
var prevCPULoadTimeStats cpu.TimesStat
var prevCPULoadTimestamp time.Time

// Async Prom cache
var promAsyncMetricCache = make(map[string]map[string][]MetricStoreMessage) // [dbUnique][metric]lastly_fetched_data
var promAsyncMetricCacheLock = sync.RWMutex{}
var lastDBSizeMB = make(map[string]int64)
var lastDBSizeFetchTime = make(map[string]time.Time) // cached for DB_SIZE_CACHING_INTERVAL
var lastDBSizeCheckLock sync.RWMutex
var mainLoopInitialized int32 // 0/1

var prevLoopMonitoredDBs []MonitoredDatabase // to be able to detect DBs removed from config
var undersizedDBs = make(map[string]bool)    // DBs below the --min-db-size-mb limit, if set
var undersizedDBsLock = sync.RWMutex{}
var recoveryIgnoredDBs = make(map[string]bool) // DBs in recovery state and OnlyIfMaster specified in config
var recoveryIgnoredDBsLock = sync.RWMutex{}
var regexSQLHelperFunctionCalled = regexp.MustCompile(`(?si)^\s*(select|with).*\s+get_\w+\(\)[\s,$]+`) // SQL helpers expected to follow get_smth() naming
var metricNameRemaps = make(map[string]string)
var metricNameRemapLock = sync.RWMutex{}

func RestoreSqlConnPoolLimitsForPreviouslyDormantDB(dbUnique string) {
	if !opts.UseConnPooling {
		return
	}
	monitored_db_conn_cache_lock.Lock()
	defer monitored_db_conn_cache_lock.Unlock()

	conn, ok := monitoredDbConnCache[dbUnique]
	if !ok || conn == nil {
		log.Error("DB conn to re-instate pool limits not found, should not happen")
		return
	}

	log.Debugf("[%s] Re-instating SQL connection pool max connections ...", dbUnique)

	conn.SetMaxIdleConns(opts.MaxParallelConnectionsPerDb)
	conn.SetMaxOpenConns(opts.MaxParallelConnectionsPerDb)

}

func InitPGVersionInfoFetchingLockIfNil(md MonitoredDatabase) {
	db_pg_version_map_lock.Lock()
	if _, ok := db_get_pg_version_map_lock[md.DBUniqueName]; !ok {
		db_get_pg_version_map_lock[md.DBUniqueName] = &sync.RWMutex{}
	}
	db_pg_version_map_lock.Unlock()
}

func GetMonitoredDatabasesFromConfigDB() ([]MonitoredDatabase, error) {
	monitoredDBs := make([]MonitoredDatabase, 0)
	activeHostData, err := GetAllActiveHostsFromConfigDB()
	groups := strings.Split(opts.Metric.Group, ",")
	skippedEntries := 0

	if err != nil {
		log.Errorf("Failed to read monitoring config from DB: %s", err)
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
			log.Infof("Filtered out %d config entries based on --groups input", skippedEntries)
		}

		metricConfig, err := jsonTextToMap(row["md_config"].(string))
		if err != nil {
			log.Warningf("Cannot parse metrics JSON config for \"%s\": %v", row["md_unique_name"].(string), err)
			continue
		}
		metricConfigStandby := make(map[string]float64)
		if configStandby, ok := row["md_config_standby"]; ok {
			metricConfigStandby, err = jsonTextToMap(configStandby.(string))
			if err != nil {
				log.Warningf("Cannot parse standby metrics JSON config for \"%s\". Ignoring standby config: %v", row["md_unique_name"].(string), err)
			}
		}
		customTags, err := jsonTextToStringMap(row["md_custom_tags"].(string))
		if err != nil {
			log.Warningf("Cannot parse custom tags JSON for \"%s\". Ignoring custom tags. Error: %v", row["md_unique_name"].(string), err)
			customTags = nil
		}
		hostConfigAttrs := HostConfigAttrs{}
		err = yaml.Unmarshal([]byte(row["md_host_config"].(string)), &hostConfigAttrs)
		if err != nil {
			log.Warningf("Cannot parse host config JSON for \"%s\". Ignoring host config. Error: %v", row["md_unique_name"].(string), err)
		}

		md := MonitoredDatabase{
			DBUniqueName:         row["md_unique_name"].(string),
			DBUniqueNameOrig:     row["md_unique_name"].(string),
			Host:                 row["md_hostname"].(string),
			Port:                 row["md_port"].(string),
			DBName:               row["md_dbname"].(string),
			User:                 row["md_user"].(string),
			IsSuperuser:          row["md_is_superuser"].(bool),
			Password:             row["md_password"].(string),
			PasswordType:         row["md_password_type"].(string),
			SslMode:              row["md_sslmode"].(string),
			SslRootCAPath:        row["md_root_ca_path"].(string),
			SslClientCertPath:    row["md_client_cert_path"].(string),
			SslClientKeyPath:     row["md_client_key_path"].(string),
			StmtTimeout:          row["md_statement_timeout_seconds"].(int64),
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
			log.Warningf("Ignoring host \"%s\" - unknown dbtype: %s. Expected one of: %+v", md.DBUniqueName, md.DBType, dbTypes)
			continue
		}

		if md.PasswordType == "aes-gcm-256" && opts.AesGcmKeyphrase != "" {
			md.Password = decrypt(md.DBUniqueName, opts.AesGcmKeyphrase, md.Password)
		}

		if md.DBType == config.DBTYPE_PG_CONT {
			resolved, err := ResolveDatabasesFromConfigEntry(md)
			if err != nil {
				log.Errorf("Failed to resolve DBs for \"%s\": %s", md.DBUniqueName, err)
				if md.PasswordType == "aes-gcm-256" && opts.AesGcmKeyphrase == "" {
					log.Errorf("No decryption key set. Use the --aes-gcm-keyphrase or --aes-gcm-keyphrase params to set")
				}
				continue
			}
			temp_arr := make([]string, 0)
			for _, rdb := range resolved {
				monitoredDBs = append(monitoredDBs, rdb)
				temp_arr = append(temp_arr, rdb.DBName)
			}
			log.Debugf("Resolved %d DBs with prefix \"%s\": [%s]", len(resolved), md.DBUniqueName, strings.Join(temp_arr, ", "))
		} else if md.DBType == config.DBTYPE_PATRONI || md.DBType == config.DBTYPE_PATRONI_CONT || md.DBType == config.DBTYPE_PATRONI_NAMESPACE_DISCOVERY {
			resolved, err := ResolveDatabasesFromPatroni(md)
			if err != nil {
				log.Errorf("Failed to resolve DBs for \"%s\": %s", md.DBUniqueName, err)
				continue
			}
			temp_arr := make([]string, 0)
			for _, rdb := range resolved {
				monitoredDBs = append(monitoredDBs, rdb)
				temp_arr = append(temp_arr, rdb.DBName)
			}
			log.Debugf("Resolved %d DBs with prefix \"%s\": [%s]", len(resolved), md.DBUniqueName, strings.Join(temp_arr, ", "))
		} else {
			monitoredDBs = append(monitoredDBs, md)
		}

		monitoredDBs = append(monitoredDBs)
	}
	return monitoredDBs, err
}

func InitGraphiteConnection(host string, port int) {
	var err error
	log.Debug("Connecting to Graphite...")
	graphiteConnection, err = graphite.NewGraphite(host, port)
	if err != nil {
		log.Fatal("could not connect to Graphite:", err)
	}
	log.Debug("OK")
}

func SendToGraphite(dbname, measurement string, data [](map[string]interface{})) error {
	if len(data) == 0 {
		log.Warning("No data passed to SendToGraphite call")
		return nil
	}
	log.Debugf("Writing %d rows to Graphite", len(data))

	metric_base_prefix := GRAPHITE_METRICS_PREFIX + "." + measurement + "." + dbname + "."
	metrics := make([]graphite.Metric, 0, len(data)*len(data[0]))

	for _, dr := range data {
		var epoch_s int64

		// we loop over columns the first time just to find the timestamp
		for k, v := range dr {
			if v == nil || v == "" {
				continue // not storing NULLs
			} else if k == EPOCH_COLUMN_NAME {
				epoch_s = v.(int64) / 1e9
				break
			}
		}

		if epoch_s == 0 {
			log.Warning("No timestamp_ns found, server time will be used. measurement:", measurement)
			epoch_s = time.Now().Unix()
		}

		for k, v := range dr {
			if v == nil || v == "" {
				continue // not storing NULLs
			}
			if k == EPOCH_COLUMN_NAME {
				continue
			} else {
				var metric graphite.Metric

				if strings.HasPrefix(k, TAG_PREFIX) { // ignore tags for Graphite
					metric.Name = metric_base_prefix + k[4:]
				} else {
					metric.Name = metric_base_prefix + k
				}
				switch t := v.(type) {
				case int:
					metric.Value = fmt.Sprintf("%d", v)
				case int32:
					metric.Value = fmt.Sprintf("%d", v)
				case int64:
					metric.Value = fmt.Sprintf("%d", v)
				case float64:
					metric.Value = fmt.Sprintf("%f", v)
				default:
					log.Infof("Invalid (non-numeric) column type ignored: metric %s, column: %v, return type: %T", measurement, k, t)
					continue
				}
				metric.Timestamp = epoch_s
				metrics = append(metrics, metric)
			}
		}
	} // dr

	log.Debug("Sending", len(metrics), "metric points to Graphite...")
	t1 := time.Now()
	err := graphiteConnection.SendMetrics(metrics)
	t_diff := time.Since(t1)
	if err != nil {
		atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
		log.Error("could not send metric to Graphite:", err)
	} else {
		atomic.StoreInt64(&lastSuccessfulDatastoreWriteTimeEpoch, t1.Unix())
		atomic.AddUint64(&datastoreTotalWriteTimeMicroseconds, uint64(t_diff.Microseconds()))
		atomic.AddUint64(&datastoreWriteSuccessCounter, 1)
		log.Debug("Sent in ", t_diff.Microseconds(), "us")
	}

	return err
}

func GetMonitoredDatabaseByUniqueName(name string) (MonitoredDatabase, error) {
	monitored_db_cache_lock.RLock()
	defer monitored_db_cache_lock.RUnlock()
	_, exists := monitored_db_cache[name]
	if !exists {
		return MonitoredDatabase{}, errors.New("DBUnique not found")
	}
	return monitored_db_cache[name], nil
}

func UpdateMonitoredDBCache(data []MonitoredDatabase) {
	monitored_db_cache_new := make(map[string]MonitoredDatabase)

	for _, row := range data {
		monitored_db_cache_new[row.DBUniqueName] = row
	}

	monitored_db_cache_lock.Lock()
	monitored_db_cache = monitored_db_cache_new
	monitored_db_cache_lock.Unlock()
}

func ProcessRetryQueue(data_source, conn_str, conn_ident string, retry_queue *list.List, limit int) error {
	var err error
	iterations_done := 0

	for retry_queue.Len() > 0 { // send over the whole re-try queue at once if connection works
		log.Debug("Processing retry_queue", conn_ident, ". Items in retry_queue: ", retry_queue.Len())
		msg := retry_queue.Back().Value.([]MetricStoreMessage)

		if data_source == DATASTORE_POSTGRES {
			err = SendToPostgres(msg)
		} else if data_source == DATASTORE_GRAPHITE {
			for _, m := range msg {
				err = SendToGraphite(m.DBUniqueName, m.MetricName, m.Data) // TODO add baching
				if err != nil {
					log.Info("Reconnect to graphite")
					InitGraphiteConnection(graphite_host, graphite_port)
				}
			}
		} else {
			log.Fatal("Invalid datastore:", data_source)
		}
		if err != nil {
			return err // still gone, retry later
		}
		retry_queue.Remove(retry_queue.Back())
		iterations_done++
		if limit > 0 && limit == iterations_done {
			return nil
		}
	}

	return nil
}

func MetricsBatcher(batchingMaxDelayMillis int64, buffered_storage_ch <-chan []MetricStoreMessage, storage_ch chan<- []MetricStoreMessage) {
	if batchingMaxDelayMillis <= 0 {
		log.Fatalf("Check --batching-delay-ms, zero/negative batching delay:", batchingMaxDelayMillis)
	}
	var datapointCounter int
	var maxBatchSize = 1000                // flush on maxBatchSize metric points or batchingMaxDelayMillis passed
	batch := make([]MetricStoreMessage, 0) // no size limit here as limited in persister already
	ticker := time.NewTicker(time.Millisecond * time.Duration(batchingMaxDelayMillis))

	for {
		select {
		case <-ticker.C:
			if len(batch) > 0 {
				flushed := make([]MetricStoreMessage, len(batch))
				copy(flushed, batch)
				log.Debugf("Flushing %d metric datasets due to batching timeout", len(batch))
				storage_ch <- flushed
				batch = make([]MetricStoreMessage, 0)
				datapointCounter = 0
			}
		case msg := <-buffered_storage_ch:
			for _, m := range msg { // in reality msg are sent by fetchers one by one though
				batch = append(batch, m)
				datapointCounter += len(m.Data)
				if datapointCounter > maxBatchSize { // flush. also set some last_sent_timestamp so that ticker would pass a round?
					flushed := make([]MetricStoreMessage, len(batch))
					copy(flushed, batch)
					log.Debugf("Flushing %d metric datasets due to maxBatchSize limit of %d datapoints", len(batch), maxBatchSize)
					storage_ch <- flushed
					batch = make([]MetricStoreMessage, 0)
					datapointCounter = 0
				}
			}
		}
	}
}

func WriteMetricsToJsonFile(msgArr []MetricStoreMessage, jsonPath string) error {
	if len(msgArr) == 0 {
		return nil
	}

	jsonOutFile, err := os.OpenFile(jsonPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
		return err
	}
	defer jsonOutFile.Close()

	log.Infof("Writing %d metric sets to JSON file at \"%s\"...", len(msgArr), jsonPath)
	enc := json.NewEncoder(jsonOutFile)
	for _, msg := range msgArr {
		dataRow := map[string]interface{}{"metric": msg.MetricName, "data": msg.Data, "dbname": msg.DBUniqueName, "custom_tags": msg.CustomTags}
		if opts.AddRealDbname && msg.RealDbname != "" {
			dataRow[opts.RealDbnameField] = msg.RealDbname
		}
		if opts.AddSystemIdentifier && msg.SystemIdentifier != "" {
			dataRow[opts.SystemIdentifierField] = msg.SystemIdentifier
		}
		err = enc.Encode(dataRow)
		if err != nil {
			atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
			return err
		}
	}
	return nil
}

func MetricsPersister(data_store string, storage_ch <-chan []MetricStoreMessage) {
	var last_try = make([]time.Time, influx_host_count)          // if Influx errors out, don't retry before 10s
	var last_drop_warning = make([]time.Time, influx_host_count) // log metric points drops every 10s to not overflow logs in case Influx is down for longer
	var retry_queues = make([]*list.List, influx_host_count)     // separate queues for all Influx hosts
	var in_error = make([]bool, influx_host_count)
	var err error

	for i := 0; i < influx_host_count; i++ {
		retry_queues[i] = list.New()
	}

	for {
		select {
		case msg_arr := <-storage_ch:

			log.Notice(`
	Metric Storage Messages:
	`, msg_arr)

			for i, retry_queue := range retry_queues {

				retry_queue_length := retry_queue.Len()

				if retry_queue_length > 0 {
					if retry_queue_length == PERSIST_QUEUE_MAX_SIZE {
						dropped_msgs := retry_queue.Remove(retry_queue.Back())
						datasets_dropped := len(dropped_msgs.([]MetricStoreMessage))
						datapoints_dropped := 0
						for _, msg := range dropped_msgs.([]MetricStoreMessage) {
							datapoints_dropped += len(msg.Data)
						}
						atomic.AddUint64(&totalMetricsDroppedCounter, uint64(datapoints_dropped))
						if last_drop_warning[i].IsZero() || last_drop_warning[i].Before(time.Now().Add(time.Second*-10)) {
							log.Warningf("Dropped %d oldest data sets with %d data points from queue %d as PERSIST_QUEUE_MAX_SIZE = %d exceeded",
								datasets_dropped, datapoints_dropped, i, PERSIST_QUEUE_MAX_SIZE)
							last_drop_warning[i] = time.Now()
						}
					}
					retry_queue.PushFront(msg_arr)
				} else {
					if data_store == DATASTORE_PROMETHEUS && opts.Metric.PrometheusAsyncMode {
						if len(msg_arr) == 0 || len(msg_arr[0].Data) == 0 { // no batching in async prom mode, so using 0 indexing ok
							continue
						}
						msg := msg_arr[0]
						PromAsyncCacheAddMetricData(msg.DBUniqueName, msg.MetricName, msg_arr)
						log.Infof("[%s:%s] Added %d rows to Prom cache", msg.DBUniqueName, msg.MetricName, len(msg.Data))
					} else if data_store == DATASTORE_POSTGRES {
						err = SendToPostgres(msg_arr)
						if err != nil && strings.Contains(err.Error(), "does not exist") {
							// in case data was cleaned by user externally
							log.Warning("re-initializing metric partition cache due to possible external data cleanup...")
							partitionMapMetric = make(map[string]ExistingPartitionInfo)
							partitionMapMetricDbname = make(map[string]map[string]ExistingPartitionInfo)
						}
					} else if data_store == DATASTORE_GRAPHITE {
						for _, m := range msg_arr {
							err = SendToGraphite(m.DBUniqueName, m.MetricName, m.Data) // TODO does Graphite library support batching?
							if err != nil {
								atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
							}
						}
					} else if data_store == DATASTORE_JSON {
						err = WriteMetricsToJsonFile(msg_arr, opts.Metric.JSONStorageFile)
					} else {
						log.Fatal("Invalid datastore:", data_store)
					}
					last_try[i] = time.Now()

					if err != nil {
						log.Errorf("Failed to write into datastore %d: %s", i, err)
						in_error[i] = true
						retry_queue.PushFront(msg_arr)
					}
				}
			}
		default:
			for i, retry_queue := range retry_queues {
				if retry_queue.Len() > 0 && (!in_error[i] || last_try[i].Before(time.Now().Add(time.Second*-10))) {
					err := ProcessRetryQueue(data_store, influxConnectStrings[i], strconv.Itoa(i), retry_queue, 100)
					if err != nil {
						log.Error("Error processing retry queue", i, ":", err)
						in_error[i] = true
					} else {
						in_error[i] = false
					}
					last_try[i] = time.Now()
				} else {
					time.Sleep(time.Millisecond * 100) // nothing in queue nor in channel
				}
			}
		}
	}
}

// Need to define a sort interface as Go doesn't have support for Numeric/Decimal
type Decimal []decimal.Decimal

func (a Decimal) Len() int           { return len(a) }
func (a Decimal) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Decimal) Less(i, j int) bool { return a[i].LessThan(a[j]) }

// assumes upwards compatibility for versions
func GetMetricVersionProperties(metric string, vme DBVersionMapEntry, metricDefMap map[string]map[decimal.Decimal]MetricVersionProperties) (MetricVersionProperties, error) {
	var keys []decimal.Decimal
	var mdm map[string]map[decimal.Decimal]MetricVersionProperties

	if metricDefMap != nil {
		mdm = metricDefMap
	} else {
		metric_def_map_lock.RLock()
		mdm = deepCopyMetricDefinitionMap(metric_def_map) // copy of global cache
		metric_def_map_lock.RUnlock()
	}

	_, ok := mdm[metric]
	if !ok || len(mdm[metric]) == 0 {
		log.Debug("metric", metric, "not found")
		return MetricVersionProperties{}, errors.New("metric SQL not found")
	}

	for k := range mdm[metric] {
		keys = append(keys, k)
	}

	sort.Sort(Decimal(keys))

	var best_ver decimal.Decimal
	var min_ver decimal.Decimal
	var found bool
	for _, ver := range keys {
		if vme.Version.GreaterThanOrEqual(ver) {
			best_ver = ver
			found = true
		}
		if min_ver.IsZero() || ver.LessThan(min_ver) {
			min_ver = ver
		}
	}

	if !found {
		if vme.Version.LessThan(min_ver) { // metric not yet available for given PG ver
			return MetricVersionProperties{}, fmt.Errorf("no suitable SQL found for metric \"%s\", server version \"%s\" too old. min defined SQL ver: %s", metric, vme.VersionStr, min_ver.String())
		}
		return MetricVersionProperties{}, fmt.Errorf("no suitable SQL found for metric \"%s\", version \"%s\"", metric, vme.VersionStr)
	}

	ret := mdm[metric][best_ver]

	// check if SQL def. override defined for some specific extension version and replace the metric SQL-s if so
	if ret.MetricAttrs.ExtensionVersionOverrides != nil && len(ret.MetricAttrs.ExtensionVersionOverrides) > 0 {
		if vme.Extensions != nil && len(vme.Extensions) > 0 {
			log.Debugf("[%s] extension version based override request found: %+v", metric, ret.MetricAttrs.ExtensionVersionOverrides)
			for _, extOverride := range ret.MetricAttrs.ExtensionVersionOverrides {
				var matching = true
				for _, extVer := range extOverride.ExpectedExtensionVersions { // "natural" sorting of metric definition assumed
					installedExtVer, ok := vme.Extensions[extVer.ExtName]
					if !ok || !installedExtVer.GreaterThanOrEqual(extVer.ExtMinVersion) {
						matching = false
					}
				}
				if matching { // all defined extensions / versions (if many) need to match
					_, ok := mdm[extOverride.TargetMetric]
					if !ok {
						log.Warningf("extension based override metric not found for metric %s. substitute metric name: %s", metric, extOverride.TargetMetric)
						continue
					}
					mvp, err := GetMetricVersionProperties(extOverride.TargetMetric, vme, mdm)
					if err != nil {
						log.Warningf("undefined extension based override for metric %s, substitute metric name: %s, version: %s not found", metric, extOverride.TargetMetric, best_ver)
						continue
					}
					log.Debugf("overriding metric %s based on the extension_version_based_overrides metric attribute with %s:%s", metric, extOverride.TargetMetric, best_ver)
					if mvp.Sql != "" {
						ret.Sql = mvp.Sql
					}
					if mvp.SqlSU != "" {
						ret.SqlSU = mvp.SqlSU
					}
				}
			}
		}
	}
	return ret, nil
}

func GetAllRecoMetricsForVersion(vme DBVersionMapEntry) map[string]MetricVersionProperties {
	mvp_map := make(map[string]MetricVersionProperties)

	metric_def_map_lock.RLock()
	defer metric_def_map_lock.RUnlock()
	for m := range metric_def_map {
		if strings.HasPrefix(m, RECO_PREFIX) {
			mvp, err := GetMetricVersionProperties(m, vme, metric_def_map)
			if err != nil {
				log.Warningf("Could not get SQL definition for metric \"%s\", PG %s", m, vme.VersionStr)
			} else if !mvp.MetricAttrs.IsPrivate {
				mvp_map[m] = mvp
			}
		}
	}
	return mvp_map
}

func GetRecommendations(dbUnique string, vme DBVersionMapEntry) ([]map[string]interface{}, error, time.Duration) {
	ret_data := make([]map[string]interface{}, 0)
	var total_duration time.Duration
	start_time_epoch_ns := time.Now().UnixNano()

	reco_metrics := GetAllRecoMetricsForVersion(vme)
	log.Debugf("Processing %d recommendation metrics for \"%s\"", len(reco_metrics), dbUnique)

	for m, mvp := range reco_metrics {
		data, err, duration := DBExecReadByDbUniqueName(dbUnique, m, mvp.MetricAttrs.StatementTimeoutSeconds, mvp.Sql)
		total_duration += duration
		if err != nil {
			if strings.Contains(err.Error(), "does not exist") { // some more exotic extensions missing is expected, don't pollute the error log
				log.Infof("[%s:%s] Could not execute recommendations SQL: %v", dbUnique, m, err)
			} else {
				log.Errorf("[%s:%s] Could not execute recommendations SQL: %v", dbUnique, m, err)
			}
			continue
		}
		for _, d := range data {
			d[EPOCH_COLUMN_NAME] = start_time_epoch_ns
			d["major_ver"] = PgVersionDecimalToMajorVerFloat(dbUnique, vme.Version)
			ret_data = append(ret_data, d)
		}
	}
	if len(ret_data) == 0 { // insert a dummy entry minimally so that Grafana can show at least a dropdown
		dummy := make(map[string]interface{})
		dummy["tag_reco_topic"] = "dummy"
		dummy["tag_object_name"] = "-"
		dummy["recommendation"] = "no recommendations"
		dummy[EPOCH_COLUMN_NAME] = start_time_epoch_ns
		dummy["major_ver"] = PgVersionDecimalToMajorVerFloat(dbUnique, vme.Version)
		ret_data = append(ret_data, dummy)
	}
	return ret_data, nil, total_duration
}

func PgVersionDecimalToMajorVerFloat(dbUnique string, pgVer decimal.Decimal) float64 {
	ver_float, _ := pgVer.Float64()

	if ver_float >= 10 {
		return math.Floor(ver_float)
	} else {
		return ver_float
	}
}

func FilterPgbouncerData(data []map[string]interface{}, databaseToKeep string, vme DBVersionMapEntry) []map[string]interface{} {
	filtered_data := make([]map[string]interface{}, 0)

	for _, dr := range data {
		//log.Debugf("bouncer dr: %+v", dr)
		if _, ok := dr["database"]; !ok {
			log.Warning("Expected 'database' key not found from pgbouncer_stats, not storing data")
			continue
		}
		if (len(databaseToKeep) > 0 && dr["database"] != databaseToKeep) || dr["database"] == "pgbouncer" { // always ignore the internal 'pgbouncer' DB
			log.Debugf("Skipping bouncer stats for pool entry %v as not the specified DBName of %s", dr["database"], databaseToKeep)
			continue // and all others also if a DB / pool name was specified in config
		}

		dr["tag_database"] = dr["database"] // support multiple databases / pools via tags if DbName left empty
		delete(dr, "database")              // remove the original pool name

		if vme.Version.GreaterThanOrEqual(pgBouncerNumericCountersStartVersion) { // v1.12 counters are of type numeric instead of int64
			for k, v := range dr {
				if k == "tag_database" {
					continue
				}
				decimalCounter, err := decimal.NewFromString(string(v.([]uint8)))
				if err != nil {
					log.Errorf("Could not parse \"%+v\" to Decimal: %s", string(v.([]uint8)), err)
					return filtered_data
				}
				dr[k] = decimalCounter.IntPart() // technically could cause overflow...but highly unlikely for 2^63
			}
		}
		filtered_data = append(filtered_data, dr)
	}

	return filtered_data
}

func FetchMetrics(msg MetricFetchMessage, host_state map[string]map[string]string, storage_ch chan<- []MetricStoreMessage, context string) ([]MetricStoreMessage, error) {
	var vme DBVersionMapEntry
	var db_pg_version decimal.Decimal
	var err, firstErr error
	var sql string
	var retryWithSuperuserSQL = true
	var data, cachedData []map[string]interface{}
	var duration time.Duration
	var md MonitoredDatabase
	var fromCache, isCacheable bool

	vme, err = DBGetPGVersion(msg.DBUniqueName, msg.DBType, false)
	if err != nil {
		log.Error("failed to fetch pg version for ", msg.DBUniqueName, msg.MetricName, err)
		return nil, err
	}
	if msg.MetricName == SPECIAL_METRIC_DB_SIZE || msg.MetricName == SPECIAL_METRIC_TABLE_STATS {
		if vme.ExecEnv == EXEC_ENV_AZURE_SINGLE && vme.ApproxDBSizeB > 1e12 { // 1TB
			subsMetricName := msg.MetricName + "_approx"
			mvp_approx, err := GetMetricVersionProperties(subsMetricName, vme, nil)
			if err == nil && mvp_approx.MetricAttrs.MetricStorageName == msg.MetricName {
				log.Infof("[%s:%s] Transparently swapping metric to %s due to hard-coded rules...", msg.DBUniqueName, msg.MetricName, subsMetricName)
				msg.MetricName = subsMetricName
			}
		}
	}
	db_pg_version = vme.Version

	if msg.DBType == config.DBTYPE_BOUNCER {
		db_pg_version = decimal.Decimal{} // version is 0.0 for all pgbouncer sql per convention
	}

	mvp, err := GetMetricVersionProperties(msg.MetricName, vme, nil)
	if err != nil && msg.MetricName != RECO_METRIC_NAME {
		epoch, ok := last_sql_fetch_error.Load(msg.MetricName + DB_METRIC_JOIN_STR + db_pg_version.String())
		if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
			log.Infof("Failed to get SQL for metric '%s', version '%s': %v", msg.MetricName, vme.VersionStr, err)
			last_sql_fetch_error.Store(msg.MetricName+DB_METRIC_JOIN_STR+db_pg_version.String(), time.Now().Unix())
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
			goto send_to_storage_channel
		}
	}

retry_with_superuser_sql: // if 1st fetch with normal SQL fails, try with SU SQL if it's defined

	sql = mvp.Sql

	if opts.Metric.NoHelperFunctions && mvp.CallsHelperFunctions && mvp.SqlSU != "" {
		log.Debugf("[%s:%s] Using SU SQL instead of normal one due to --no-helper-functions input", msg.DBUniqueName, msg.MetricName)
		sql = mvp.SqlSU
		retryWithSuperuserSQL = false
	}

	if (vme.IsSuperuser || (retryWithSuperuserSQL && firstErr != nil)) && mvp.SqlSU != "" {
		sql = mvp.SqlSU
		retryWithSuperuserSQL = false
	}
	if sql == "" && !(msg.MetricName == SPECIAL_METRIC_CHANGE_EVENTS || msg.MetricName == RECO_METRIC_NAME) {
		// let's ignore dummy SQL-s
		log.Debugf("[%s:%s] Ignoring fetch message - got an empty/dummy SQL string", msg.DBUniqueName, msg.MetricName)
		return nil, nil
	}

	if (mvp.MasterOnly && vme.IsInRecovery) || (mvp.StandbyOnly && !vme.IsInRecovery) {
		log.Debugf("[%s:%s] Skipping fetching of  as server not in wanted state (IsInRecovery=%v)", msg.DBUniqueName, msg.MetricName, vme.IsInRecovery)
		return nil, nil
	}

	if msg.MetricName == SPECIAL_METRIC_CHANGE_EVENTS && context != CONTEXT_PROMETHEUS_SCRAPE { // special handling, multiple queries + stateful
		CheckForPGObjectChangesAndStore(msg.DBUniqueName, vme, storage_ch, host_state) // TODO no host_state for Prometheus currently
	} else if msg.MetricName == RECO_METRIC_NAME && context != CONTEXT_PROMETHEUS_SCRAPE {
		data, _, _ = GetRecommendations(msg.DBUniqueName, vme)
	} else if msg.DBType == config.DBTYPE_PGPOOL {
		data, _, _ = FetchMetricsPgpool(msg, vme, mvp)
	} else {
		data, err, duration = DBExecReadByDbUniqueName(msg.DBUniqueName, msg.MetricName, mvp.MetricAttrs.StatementTimeoutSeconds, sql)

		if err != nil {
			// let's soften errors to "info" from functions that expect the server to be a primary to reduce noise
			if strings.Contains(err.Error(), "recovery is in progress") {
				db_pg_version_map_lock.RLock()
				ver := db_pg_version_map[msg.DBUniqueName]
				db_pg_version_map_lock.RUnlock()
				if ver.IsInRecovery {
					log.Debugf("[%s:%s] failed to fetch metrics: %s", msg.DBUniqueName, msg.MetricName, err)
					return nil, err
				}
			}

			if msg.MetricName == SPECIAL_METRIC_INSTANCE_UP {
				log.Debugf("[%s:%s] failed to fetch metrics. marking instance as not up: %s", msg.DBUniqueName, msg.MetricName, err)
				data = make([]map[string]interface{}, 1)
				data[0] = map[string]interface{}{"epoch_ns": time.Now().UnixNano(), "is_up": 0} // NB! should be updated if the "instance_up" metric definition is changed
				goto send_to_storage_channel
			}

			if strings.Contains(err.Error(), "connection refused") {
				SetDBUnreachableState(msg.DBUniqueName)
			}

			if retryWithSuperuserSQL && mvp.SqlSU != "" {
				firstErr = err
				log.Infof("[%s:%s] Normal fetch failed, re-trying to fetch with SU SQL", msg.DBUniqueName, msg.MetricName)
				goto retry_with_superuser_sql
			} else {
				if firstErr != nil {
					log.Infof("[%s:%s] failed to fetch metrics also with SU SQL so initial error will be returned. Current err: %s", msg.DBUniqueName, msg.MetricName, err)
					return nil, firstErr // returning the initial error
				} else {
					log.Infof("[%s:%s] failed to fetch metrics: %s", msg.DBUniqueName, msg.MetricName, err)
				}
			}
			return nil, err
		} else {
			md, err = GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
			if err != nil {
				log.Errorf("[%s:%s] could not get monitored DB details", msg.DBUniqueName, err)
				return nil, err
			}

			log.Infof("[%s:%s] fetched %d rows in %.1f ms", msg.DBUniqueName, msg.MetricName, len(data), float64(duration.Nanoseconds())/1000000)
			if regexIsPgbouncerMetrics.MatchString(msg.MetricName) { // clean unwanted pgbouncer pool stats here as not possible in SQL
				data = FilterPgbouncerData(data, md.DBName, vme)
			}

			ClearDBUnreachableStateIfAny(msg.DBUniqueName)
		}
	}

	if isCacheable && opts.InstanceLevelCacheMaxSeconds > 0 && msg.Interval.Seconds() > float64(opts.InstanceLevelCacheMaxSeconds) {
		PutToInstanceCache(msg, data)
	}

send_to_storage_channel:

	if (opts.AddRealDbname || opts.AddSystemIdentifier) && msg.DBType == config.DBTYPE_PG {
		db_pg_version_map_lock.RLock()
		ver := db_pg_version_map[msg.DBUniqueName]
		db_pg_version_map_lock.RUnlock()
		data = AddDbnameSysinfoIfNotExistsToQueryResultData(msg, data, ver)
	}

	if mvp.MetricAttrs.MetricStorageName != "" {
		log.Debugf("[%s] rerouting metric %s data to %s based on metric attributes", msg.DBUniqueName, msg.MetricName, mvp.MetricAttrs.MetricStorageName)
		msg.MetricName = mvp.MetricAttrs.MetricStorageName
	}
	if fromCache {
		md, err = GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
		if err != nil {
			log.Errorf("[%s:%s] could not get monitored DB details", msg.DBUniqueName, err)
			return nil, err
		}
		log.Infof("[%s:%s] loaded %d rows from the instance cache", msg.DBUniqueName, msg.MetricName, len(cachedData))
		atomic.AddUint64(&totalMetricsReusedFromCacheCounter, uint64(len(cachedData)))
		return []MetricStoreMessage{{DBUniqueName: msg.DBUniqueName, MetricName: msg.MetricName, Data: cachedData, CustomTags: md.CustomTags,
			MetricDefinitionDetails: mvp, RealDbname: vme.RealDbname, SystemIdentifier: vme.SystemIdentifier}}, nil
	} else {
		atomic.AddUint64(&totalMetricsFetchedCounter, uint64(len(data)))
		return []MetricStoreMessage{{DBUniqueName: msg.DBUniqueName, MetricName: msg.MetricName, Data: data, CustomTags: md.CustomTags,
			MetricDefinitionDetails: mvp, RealDbname: vme.RealDbname, SystemIdentifier: vme.SystemIdentifier}}, nil
	}
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

func PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, metric string) {
	if opts.Metric.PrometheusAsyncMode {
		promAsyncMetricCacheLock.Lock()
		defer promAsyncMetricCacheLock.Unlock()

		if metric == "" {
			delete(promAsyncMetricCache, dbUnique) // whole host removed from config
		} else {
			delete(promAsyncMetricCache[dbUnique], metric)
		}
	}
}

func GetFromInstanceCacheIfNotOlderThanSeconds(msg MetricFetchMessage, maxAgeSeconds int64) []map[string]interface{} {
	var clonedData []map[string]interface{}
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

	log.Debugf("[%s:%s] reading metric data from instance cache of \"%s\"", msg.DBUniqueName, msg.MetricName, msg.DBUniqueNameOrig)
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

func PutToInstanceCache(msg MetricFetchMessage, data []map[string]interface{}) {
	if len(data) == 0 {
		return
	}
	dataCopy := deepCopyMetricData(data)
	log.Debugf("[%s:%s] filling instance cache", msg.DBUniqueNameOrig, msg.MetricName)
	instanceMetricCacheLock.Lock()
	instanceMetricCache[msg.DBUniqueNameOrig+msg.MetricName] = dataCopy
	instanceMetricCacheLock.Unlock()

	instanceMetricCacheTimestampLock.Lock()
	instanceMetricCacheTimestamp[msg.DBUniqueNameOrig+msg.MetricName] = time.Now()
	instanceMetricCacheTimestampLock.Unlock()
}

func IsCacheableMetric(msg MetricFetchMessage, mvp MetricVersionProperties) bool {
	if !(msg.DBType == config.DBTYPE_PG_CONT || msg.DBType == config.DBTYPE_PATRONI_CONT) {
		return false
	}
	return mvp.MetricAttrs.IsInstanceLevel
}

func AddDbnameSysinfoIfNotExistsToQueryResultData(msg MetricFetchMessage, data []map[string]interface{}, ver DBVersionMapEntry) []map[string]interface{} {
	enriched_data := make([]map[string]interface{}, 0)

	log.Debugf("Enriching all rows of [%s:%s] with sysinfo (%s) / real dbname (%s) if set. ", msg.DBUniqueName, msg.MetricName, ver.SystemIdentifier, ver.RealDbname)
	for _, dr := range data {
		if opts.AddRealDbname && ver.RealDbname != "" {
			old, ok := dr[TAG_PREFIX+opts.RealDbnameField]
			if !ok || old == "" {
				dr[TAG_PREFIX+opts.RealDbnameField] = ver.RealDbname
			}
		}
		if opts.AddSystemIdentifier && ver.SystemIdentifier != "" {
			old, ok := dr[TAG_PREFIX+opts.SystemIdentifierField]
			if !ok || old == "" {
				dr[TAG_PREFIX+opts.SystemIdentifierField] = ver.SystemIdentifier
			}
		}
		enriched_data = append(enriched_data, dr)
	}
	return enriched_data
}

func StoreMetrics(metrics []MetricStoreMessage, storage_ch chan<- []MetricStoreMessage) (int, error) {

	if len(metrics) > 0 {
		atomic.AddUint64(&totalDatasetsFetchedCounter, 1)
		storage_ch <- metrics
		return len(metrics), nil
	}

	return 0, nil
}

func deepCopyMetricStoreMessages(metricStoreMessages []MetricStoreMessage) []MetricStoreMessage {
	new := make([]MetricStoreMessage, 0)
	for _, msm := range metricStoreMessages {
		data_new := make([]map[string]interface{}, 0)
		for _, dr := range msm.Data {
			dr_new := make(map[string]interface{})
			for k, v := range dr {
				dr_new[k] = v
			}
			data_new = append(data_new, dr_new)
		}
		tag_data_new := make(map[string]string)
		for k, v := range msm.CustomTags {
			tag_data_new[k] = v
		}

		m := MetricStoreMessage{DBUniqueName: msm.DBUniqueName, MetricName: msm.MetricName, DBType: msm.DBType,
			Data: data_new, CustomTags: tag_data_new}
		new = append(new, m)
	}
	return new
}

func deepCopyMetricData(data []map[string]interface{}) []map[string]interface{} {
	newData := make([]map[string]interface{}, len(data))

	for i, dr := range data {
		newRow := make(map[string]interface{})
		for k, v := range dr {
			newRow[k] = v
		}
		newData[i] = newRow
	}

	return newData
}

func deepCopyMetricDefinitionMap(mdm map[string]map[decimal.Decimal]MetricVersionProperties) map[string]map[decimal.Decimal]MetricVersionProperties {
	newMdm := make(map[string]map[decimal.Decimal]MetricVersionProperties)

	for metric, verMap := range mdm {
		newMdm[metric] = make(map[decimal.Decimal]MetricVersionProperties)
		for ver, mvp := range verMap {
			newMdm[metric][ver] = mvp
		}
	}
	return newMdm
}

// ControlMessage notifies of shutdown + interval change
func MetricGathererLoop(dbUniqueName, dbUniqueNameOrig, dbType, metricName string, config_map map[string]float64, control_ch <-chan ControlMessage, store_ch chan<- []MetricStoreMessage) {
	config := config_map
	interval := config[metricName]
	ticker := time.NewTicker(time.Millisecond * time.Duration(interval*1000))
	host_state := make(map[string]map[string]string)
	var last_uptime_s int64 = -1 // used for "server restarted" event detection
	var last_error_notification_time time.Time
	var vme DBVersionMapEntry
	var mvp MetricVersionProperties
	var err error
	failed_fetches := 0
	metricNameForStorage := metricName
	lastDBVersionFetchTime := time.Unix(0, 0) // check DB ver. ev. 5 min
	var stmtTimeoutOverride int64

	if opts.TestdataDays != 0 {
		if metricName == SPECIAL_METRIC_SERVER_LOG_EVENT_COUNTS || metricName == SPECIAL_METRIC_CHANGE_EVENTS {
			return
		}
		testDataGenerationModeWG.Add(1)
	}
	if opts.Metric.Datastore == DATASTORE_POSTGRES && opts.TestdataDays == 0 {
		if _, is_special_metric := specialMetrics[metricName]; !is_special_metric {
			vme, err := DBGetPGVersion(dbUniqueName, dbType, false)
			if err != nil {
				log.Warningf("[%s][%s] Failed to determine possible re-routing name, Grafana dashboards with re-routed metrics might not show all hosts", dbUniqueName, metricName)
			} else {
				mvp, err := GetMetricVersionProperties(metricName, vme, nil)
				if err != nil && !strings.Contains(err.Error(), "too old") {
					log.Warningf("[%s][%s] Failed to determine possible re-routing name, Grafana dashboards with re-routed metrics might not show all hosts", dbUniqueName, metricName)
				} else if mvp.MetricAttrs.MetricStorageName != "" {
					metricNameForStorage = mvp.MetricAttrs.MetricStorageName
				}
			}
		}

		err := AddDBUniqueMetricToListingTable(dbUniqueName, metricNameForStorage)
		if err != nil {
			log.Errorf("Could not add newly found gatherer [%s:%s] to the 'all_distinct_dbname_metrics' listing table: %v", dbUniqueName, metricName, err)
		}

		EnsureMetricDummy(metricNameForStorage) // ensure that there is at least an empty top-level table not to get ugly Grafana notifications
	}

	if metricName == SPECIAL_METRIC_SERVER_LOG_EVENT_COUNTS {
		logparseLoop(dbUniqueName, metricName, config_map, control_ch, store_ch) // no return
		return
	}

	for {
		if lastDBVersionFetchTime.Add(time.Minute * time.Duration(5)).Before(time.Now()) {
			vme, err = DBGetPGVersion(dbUniqueName, dbType, false) // in case of errors just ignore metric "disabled" time ranges
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

		metricCurrentlyDisabled := IsMetricCurrentlyDisabledForHost(metricName, vme, dbUniqueName)
		if metricCurrentlyDisabled && opts.TestdataDays == 0 {
			log.Debugf("[%s][%s] Ignoring fetch as metric disabled for current time range", dbUniqueName, metricName)
		} else {
			var metricStoreMessages []MetricStoreMessage
			var err error
			mfm := MetricFetchMessage{DBUniqueName: dbUniqueName, DBUniqueNameOrig: dbUniqueNameOrig, MetricName: metricName, DBType: dbType, Interval: time.Second * time.Duration(interval), StmtTimeoutOverride: stmtTimeoutOverride}

			// 1st try local overrides for some metrics if operating in push mode
			if opts.DirectOSStats && IsDirectlyFetchableMetric(metricName) {
				metricStoreMessages, err = FetchStatsDirectlyFromOS(mfm, vme, mvp)
				if err != nil {
					log.Errorf("[%s][%s] Could not reader metric directly from OS: %v", dbUniqueName, metricName, err)
				}
			}
			t1 := time.Now()
			if metricStoreMessages == nil {
				metricStoreMessages, err = FetchMetrics(
					mfm,
					host_state,
					store_ch,
					"")
			}
			t2 := time.Now()

			if t2.Sub(t1) > (time.Second * time.Duration(interval)) {
				log.Warningf("Total fetching time of %vs bigger than %vs interval for [%s:%s]", t2.Sub(t1).Truncate(time.Millisecond*100).Seconds(), interval, dbUniqueName, metricName)
			}

			if err != nil {
				failed_fetches++
				// complain only 1x per 10min per host/metric...
				if last_error_notification_time.IsZero() || last_error_notification_time.Add(time.Second*time.Duration(600)).Before(time.Now()) {
					log.Errorf("Failed to fetch metric data for [%s:%s]: %v", dbUniqueName, metricName, err)
					if failed_fetches > 1 {
						log.Errorf("Total failed fetches for [%s:%s]: %d", dbUniqueName, metricName, failed_fetches)
					}
					last_error_notification_time = time.Now()
				}
			} else if metricStoreMessages != nil {
				if opts.Metric.Datastore == DATASTORE_PROMETHEUS && opts.Metric.PrometheusAsyncMode && len(metricStoreMessages[0].Data) == 0 {
					PurgeMetricsFromPromAsyncCacheIfAny(dbUniqueName, metricName)
				}
				if len(metricStoreMessages[0].Data) > 0 {

					// pick up "server restarted" events here to avoid doing extra selects from CheckForPGObjectChangesAndStore code
					if metricName == "db_stats" {
						postmaster_uptime_s, ok := (metricStoreMessages[0].Data)[0]["postmaster_uptime_s"]
						if ok {
							if last_uptime_s != -1 {
								if postmaster_uptime_s.(int64) < last_uptime_s { // restart (or possibly also failover when host is routed) happened
									message := "Detected server restart (or failover) of \"" + dbUniqueName + "\""
									log.Warning(message)
									detected_changes_summary := make([](map[string]interface{}), 0)
									entry := map[string]interface{}{"details": message, "epoch_ns": (metricStoreMessages[0].Data)[0]["epoch_ns"]}
									detected_changes_summary = append(detected_changes_summary, entry)
									metricStoreMessages = append(metricStoreMessages,
										MetricStoreMessage{DBUniqueName: dbUniqueName, DBType: dbType,
											MetricName: "object_changes", Data: detected_changes_summary, CustomTags: metricStoreMessages[0].CustomTags})
								}
							}
							last_uptime_s = postmaster_uptime_s.(int64)
						}
					}

					if opts.TestdataDays != 0 {
						orig_msms := deepCopyMetricStoreMessages(metricStoreMessages)
						log.Warningf("Generating %d days of data for [%s:%s]", opts.TestdataDays, dbUniqueName, metricName)
						test_metrics_stored := 0
						simulated_time := t1
						end_time := t1.Add(time.Hour * time.Duration(opts.TestdataDays*24))

						if opts.TestdataDays < 0 {
							simulated_time, end_time = end_time, simulated_time
						}

						for simulated_time.Before(end_time) {
							log.Debugf("Metric [%s], simulating time: %v", metricName, simulated_time)
							for host_nr := 1; host_nr <= opts.TestdataMultiplier; host_nr++ {
								fake_dbname := fmt.Sprintf("%s-%d", dbUniqueName, host_nr)
								msgs_copy_tmp := deepCopyMetricStoreMessages(orig_msms)

								for i := 0; i < len(msgs_copy_tmp[0].Data); i++ {
									(msgs_copy_tmp[0].Data)[i][EPOCH_COLUMN_NAME] = (simulated_time.UnixNano() + int64(1000*i))
								}
								msgs_copy_tmp[0].DBUniqueName = fake_dbname
								//log.Debugf("fake data for [%s:%s]: %v", metricName, fake_dbname, msgs_copy_tmp[0].Data)
								_, _ = StoreMetrics(msgs_copy_tmp, store_ch)
								test_metrics_stored += len(msgs_copy_tmp[0].Data)
							}
							time.Sleep(time.Duration(opts.TestdataMultiplier * 10000000)) // 10ms * multiplier (in nanosec).
							// would generate more metrics than persister can write and eat up RAM
							simulated_time = simulated_time.Add(time.Second * time.Duration(interval))
						}
						log.Warningf("exiting MetricGathererLoop for [%s], %d total data points generated for %d hosts",
							metricName, test_metrics_stored, opts.TestdataMultiplier)
						testDataGenerationModeWG.Done()
						return
					} else {
						_, _ = StoreMetrics(metricStoreMessages, store_ch)
					}
				}
			}

			if opts.TestdataDays != 0 { // covers errors & no data
				testDataGenerationModeWG.Done()
				return
			}

		}
		select {
		case msg := <-control_ch:
			log.Debug("got control msg", dbUniqueName, metricName, msg)
			if msg.Action == GATHERER_STATUS_START {
				config = msg.Config
				interval = config[metricName]
				if ticker != nil {
					ticker.Stop()
				}
				ticker = time.NewTicker(time.Millisecond * time.Duration(interval*1000))
				log.Debug("started MetricGathererLoop for ", dbUniqueName, metricName, " interval:", interval)
			} else if msg.Action == GATHERER_STATUS_STOP {
				log.Debug("exiting MetricGathererLoop for ", dbUniqueName, metricName, " interval:", interval)
				return
			}
		case <-ticker.C:
			log.Debugf("MetricGathererLoop for [%s:%s] slept for %s", dbUniqueName, metricName, time.Second*time.Duration(interval))
		}

	}
}

func FetchStatsDirectlyFromOS(msg MetricFetchMessage, vme DBVersionMapEntry, mvp MetricVersionProperties) ([]MetricStoreMessage, error) {
	var data []map[string]interface{}
	var err error

	if msg.MetricName == METRIC_CPU_LOAD { // could function pointers work here?
		data, err = GetLoadAvgLocal()
	} else if msg.MetricName == METRIC_PSUTIL_CPU {
		data, err = GetGoPsutilCPU(msg.Interval)
	} else if msg.MetricName == METRIC_PSUTIL_DISK {
		data, err = GetGoPsutilDiskPG(msg.DBUniqueName)
	} else if msg.MetricName == METRIC_PSUTIL_DISK_IO_TOTAL {
		data, err = GetGoPsutilDiskTotals()
	} else if msg.MetricName == METRIC_PSUTIL_MEM {
		data, err = GetGoPsutilMem()
	}
	if err != nil {
		return nil, err
	}

	msm := DatarowsToMetricstoreMessage(data, msg, vme, mvp)
	return []MetricStoreMessage{msm}, nil
}

// data + custom tags + counters
func DatarowsToMetricstoreMessage(data []map[string]interface{}, msg MetricFetchMessage, vme DBVersionMapEntry, mvp MetricVersionProperties) MetricStoreMessage {
	md, err := GetMonitoredDatabaseByUniqueName(msg.DBUniqueName)
	if err != nil {
		log.Errorf("Could not resolve DBUniqueName %s, cannot set custom attributes for gathered data: %v", msg.DBUniqueName, err)
	}

	atomic.AddUint64(&totalMetricsFetchedCounter, uint64(len(data)))

	return MetricStoreMessage{
		DBUniqueName:            msg.DBUniqueName,
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
	if _, ok := directlyFetchableOSMetrics[metric]; ok {
		return true
	}
	return false
}

func IsStringInSlice(target string, slice []string) bool {
	for _, s := range slice {
		if target == s {
			return true
		}
	}
	return false
}

func IsMetricCurrentlyDisabledForHost(metricName string, vme DBVersionMapEntry, dbUniqueName string) bool {
	_, isSpecialMetric := specialMetrics[metricName]

	mvp, err := GetMetricVersionProperties(metricName, vme, nil)
	if err != nil {
		if isSpecialMetric || strings.Contains(err.Error(), "too old") {
			return false
		}
		log.Warningf("[%s][%s] Ignoring any possible time based gathering restrictions, could not get metric details", dbUniqueName, metricName)
		return false
	}

	md, err := GetMonitoredDatabaseByUniqueName(dbUniqueName) // TODO caching?
	if err != nil {
		log.Warningf("[%s][%s] Ignoring any possible time based gathering restrictions, could not get DB details", dbUniqueName, metricName)
		return false
	}

	if md.HostConfig.PerMetricDisabledTimes == nil && mvp.MetricAttrs.DisabledDays == "" && len(mvp.MetricAttrs.DisableTimes) == 0 {
		//log.Debugf("[%s][%s] No time based gathering restrictions defined", dbUniqueName, metricName)
		return false
	}

	metricHasOverrides := false
	if md.HostConfig.PerMetricDisabledTimes != nil {
		for _, hcdt := range md.HostConfig.PerMetricDisabledTimes {
			if IsStringInSlice(metricName, hcdt.Metrics) && (hcdt.DisabledDays != "" || len(hcdt.DisabledTimes) > 0) {
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
func IsInDaySpan(locTime time.Time, days, metric, dbUnique string) bool {
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
				log.Warningf("Ignoring invalid day range specification: %s. Check config", s)
				continue
			}
			startDay, err := strconv.Atoi(dayRange[0])
			endDay, err2 := strconv.Atoi(dayRange[1])
			if err != nil || err2 != nil {
				log.Warningf("Ignoring invalid day range specification: %s. Check config", s)
				continue
			}
			for i := startDay; i <= endDay && i >= 0 && i <= 7; i++ {
				ret[i] = true
			}

		} else {
			day, err := strconv.Atoi(s)
			if err != nil {
				log.Warningf("Ignoring invalid day range specification: %s. Check config", days)
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
		log.Warningf("[%s][%s] invalid time range: %s. Check config", dbUnique, metric, timeRange)
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
		log.Warningf("[%s][%s] Ignoring invalid disabled time range: %s. Check config. Erorr: %v", dbUnique, metric, timeRange, err)
		return false
	}

	check, err := time.Parse("15:04 -0700", strconv.Itoa(checkTime.Hour())+":"+strconv.Itoa(checkTime.Minute())+" "+t1.Format("-0700")) // UTC by default
	if err != nil {
		log.Warningf("[%s][%s] Ignoring invalid disabled time range: %s. Check config. Error: %v", dbUnique, metric, timeRange, err)
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
		if IsStringInSlice(metric, hcdi.Metrics) {
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
			//log.Debugf("[%s][%s] MetricAttrs ignored time/day match, skipping fetch", dbUnique, metric)
			return true
		}
	}

	return false
}

func UpdateMetricDefinitionMap(newMetrics map[string]map[decimal.Decimal]MetricVersionProperties) {
	metric_def_map_lock.Lock()
	metric_def_map = newMetrics
	metric_def_map_lock.Unlock()
	//log.Debug("metric_def_map:", metric_def_map)
	log.Debug("metrics definitions refreshed - nr. found:", len(newMetrics))
}

func jsonTextToMap(jsonText string) (map[string]float64, error) {
	retmap := make(map[string]float64)
	if jsonText == "" {
		return retmap, nil
	}
	var host_config map[string]interface{}
	if err := json.Unmarshal([]byte(jsonText), &host_config); err != nil {
		return nil, err
	}
	for k, v := range host_config {
		retmap[k] = v.(float64)
	}
	return retmap, nil
}

func jsonTextToStringMap(jsonText string) (map[string]string, error) {
	retmap := make(map[string]string)
	if jsonText == "" {
		return retmap, nil
	}
	var iMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonText), &iMap); err != nil {
		return nil, err
	}
	for k, v := range iMap {
		retmap[k] = fmt.Sprintf("%v", v)
	}
	return retmap, nil
}

func mapToJson(metricsMap map[string]interface{}) ([]byte, error) {
	return json.Marshal(metricsMap)
}

// Expects "preset metrics" definition file named preset-config.yaml to be present in provided --metrics folder
func ReadPresetMetricsConfigFromFolder(folder string, failOnError bool) (map[string]map[string]float64, error) {
	pmm := make(map[string]map[string]float64)

	log.Infof("Reading preset metric config from path %s ...", path.Join(folder, PRESET_CONFIG_YAML_FILE))
	preset_metrics, err := os.ReadFile(path.Join(folder, PRESET_CONFIG_YAML_FILE))
	if err != nil {
		log.Errorf("Failed to read preset metric config definition at: %s", folder)
		return pmm, err
	}
	pcs := make([]PresetConfig, 0)
	err = yaml.Unmarshal(preset_metrics, &pcs)
	if err != nil {
		log.Errorf("Unmarshaling error reading preset metric config: %v", err)
		return pmm, err
	}
	for _, pc := range pcs {
		pmm[pc.Name] = pc.Metrics
	}
	log.Infof("%d preset metric definitions found", len(pcs))
	return pmm, err
}

func ParseMetricColumnAttrsFromYAML(yamlPath string) MetricColumnAttrs {
	c := MetricColumnAttrs{}

	yamlFile, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Errorf("Error reading file %s: %s", yamlFile, err)
		return c
	}

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		log.Errorf("Unmarshaling error: %v", err)
	}
	return c
}

func ParseMetricAttrsFromYAML(yamlPath string) MetricAttrs {
	c := MetricAttrs{}

	yamlFile, err := os.ReadFile(yamlPath)
	if err != nil {
		log.Errorf("Error reading file %s: %s", yamlFile, err)
		return c
	}

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		log.Errorf("Unmarshaling error: %v", err)
	}
	return c
}

func ParseMetricColumnAttrsFromString(jsonAttrs string) MetricColumnAttrs {
	c := MetricColumnAttrs{}

	err := yaml.Unmarshal([]byte(jsonAttrs), &c)
	if err != nil {
		log.Errorf("Unmarshaling error: %v", err)
	}
	return c
}

func ParseMetricAttrsFromString(jsonAttrs string) MetricAttrs {
	c := MetricAttrs{}

	err := yaml.Unmarshal([]byte(jsonAttrs), &c)
	if err != nil {
		log.Errorf("Unmarshaling error: %v", err)
	}
	return c
}

// expected is following structure: metric_name/pg_ver/metric(_master|standby).sql
func ReadMetricsFromFolder(folder string, failOnError bool) (map[string]map[decimal.Decimal]MetricVersionProperties, error) {
	metrics_map := make(map[string]map[decimal.Decimal]MetricVersionProperties)
	metricNameRemapsNew := make(map[string]string)
	rIsDigitOrPunctuation := regexp.MustCompile(`^[\d\.]+$`)
	metricNamePattern := `^[a-z0-9_\.]+$`
	rMetricNameFilter := regexp.MustCompile(metricNamePattern)

	log.Infof("Searching for metrics from path %s ...", folder)
	metric_folders, err := os.ReadDir(folder)
	if err != nil {
		if failOnError {
			log.Fatalf("Could not read path %s: %s", folder, err)
		}
		log.Error(err)
		return metrics_map, err
	}

	for _, f := range metric_folders {
		if f.IsDir() {
			if f.Name() == FILE_BASED_METRIC_HELPERS_DIR {
				continue // helpers are pulled in when needed
			}
			if !rMetricNameFilter.MatchString(f.Name()) {
				log.Warningf("Ignoring metric '%s' as name not fitting pattern: %s", f.Name(), metricNamePattern)
				continue
			}
			//log.Debugf("Processing metric: %s", f.Name())
			pgVers, err := os.ReadDir(path.Join(folder, f.Name()))
			if err != nil {
				log.Error(err)
				return metrics_map, err
			}

			var metricAttrs MetricAttrs
			if _, err = os.Stat(path.Join(folder, f.Name(), "metric_attrs.yaml")); err == nil {
				metricAttrs = ParseMetricAttrsFromYAML(path.Join(folder, f.Name(), "metric_attrs.yaml"))
				//log.Debugf("Discovered following metric attributes for metric %s: %v", f.Name(), metricAttrs)
				if metricAttrs.MetricStorageName != "" {
					metricNameRemapsNew[f.Name()] = metricAttrs.MetricStorageName
				}
			}

			var metricColumnAttrs MetricColumnAttrs
			if _, err = os.Stat(path.Join(folder, f.Name(), "column_attrs.yaml")); err == nil {
				metricColumnAttrs = ParseMetricColumnAttrsFromYAML(path.Join(folder, f.Name(), "column_attrs.yaml"))
				//log.Debugf("Discovered following column attributes for metric %s: %v", f.Name(), metricColumnAttrs)
			}

			for _, pgVer := range pgVers {
				if strings.HasSuffix(pgVer.Name(), ".md") || pgVer.Name() == "column_attrs.yaml" || pgVer.Name() == "metric_attrs.yaml" {
					continue
				}
				if !rIsDigitOrPunctuation.MatchString(pgVer.Name()) {
					log.Warningf("Invalid metric structure - version folder names should consist of only numerics/dots, found: %s", pgVer.Name())
					continue
				}
				dirName, err := decimal.NewFromString(pgVer.Name())
				if err != nil {
					log.Errorf("Could not parse \"%s\" to Decimal: %s", pgVer.Name(), err)
					continue
				}
				//log.Debugf("Found %s", pgVer.Name())

				metricDefs, err := os.ReadDir(path.Join(folder, f.Name(), pgVer.Name()))
				if err != nil {
					log.Error(err)
					continue
				}

				foundMetricDefFiles := make(map[string]bool) // to warn on accidental duplicates
				for _, md := range metricDefs {
					if strings.HasPrefix(md.Name(), "metric") && strings.HasSuffix(md.Name(), ".sql") {
						p := path.Join(folder, f.Name(), pgVer.Name(), md.Name())
						metric_sql, err := os.ReadFile(p)
						if err != nil {
							log.Errorf("Failed to read metric definition at: %s", p)
							continue
						}
						_, exists := foundMetricDefFiles[md.Name()]
						if exists {
							log.Warningf("Multiple definitions found for metric [%s:%s], using the last one (%s)...", f.Name(), pgVer.Name(), md.Name())
						}
						foundMetricDefFiles[md.Name()] = true

						//log.Debugf("Metric definition for \"%s\" ver %s: %s", f.Name(), pgVer.Name(), metric_sql)
						mvpVer, ok := metrics_map[f.Name()]
						var mvp MetricVersionProperties
						if !ok {
							metrics_map[f.Name()] = make(map[decimal.Decimal]MetricVersionProperties)
						}
						mvp, ok = mvpVer[dirName]
						if !ok {
							mvp = MetricVersionProperties{Sql: string(metric_sql[:]), ColumnAttrs: metricColumnAttrs, MetricAttrs: metricAttrs}
						}
						mvp.CallsHelperFunctions = DoesMetricDefinitionCallHelperFunctions(mvp.Sql)
						if strings.Contains(md.Name(), "_master") {
							mvp.MasterOnly = true
						}
						if strings.Contains(md.Name(), "_standby") {
							mvp.StandbyOnly = true
						}
						if strings.Contains(md.Name(), "_su") {
							mvp.SqlSU = string(metric_sql[:])
						}
						metrics_map[f.Name()][dirName] = mvp
					}
				}
			}
		}
	}

	metricNameRemapLock.Lock()
	metricNameRemaps = metricNameRemapsNew
	metricNameRemapLock.Unlock()

	return metrics_map, nil
}

func ExpandEnvVarsForConfigEntryIfStartsWithDollar(md MonitoredDatabase) (MonitoredDatabase, int) {
	var changed int

	if strings.HasPrefix(md.DBName, "$") {
		md.DBName = os.ExpandEnv(md.DBName)
		changed++
	}
	if strings.HasPrefix(md.User, "$") {
		md.User = os.ExpandEnv(md.User)
		changed++
	}
	if strings.HasPrefix(md.Password, "$") {
		md.Password = os.ExpandEnv(md.Password)
		changed++
	}
	if strings.HasPrefix(md.PasswordType, "$") {
		md.PasswordType = os.ExpandEnv(md.PasswordType)
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
	if strings.HasPrefix(md.SslMode, "$") {
		md.SslMode = os.ExpandEnv(md.SslMode)
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

	log.Debugf("Converting monitoring YAML config to MonitoredDatabase from path %s ...", configFilePath)
	yamlFile, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("Error reading file %s: %s", configFilePath, err)
		return hostList, err
	}
	// TODO check mod timestamp or hash, from a global "caching map"
	c := make([]MonitoredDatabase, 0) // there can be multiple configs in a single file
	yamlFile = []byte(string(yamlFile))
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		log.Errorf("Unmarshaling error: %v", err)
		return hostList, err
	}
	for _, v := range c {
		if v.Port == "" {
			v.Port = "5432"
		}
		if v.DBType == "" {
			v.DBType = config.DBTYPE_PG
		}
		if v.IsEnabled {
			log.Debugf("Found active monitoring config entry: %#v", v)
			if v.Group == "" {
				v.Group = "default"
			}
			if v.StmtTimeout == 0 {
				v.StmtTimeout = 5
			}
			vExp, changed := ExpandEnvVarsForConfigEntryIfStartsWithDollar(v)
			if changed > 0 {
				log.Debugf("[%s] %d config attributes expanded from ENV", vExp.DBUniqueName, changed)
			}
			hostList = append(hostList, vExp)
		}
	}
	if len(hostList) == 0 {
		log.Warningf("Could not find any valid monitoring configs from file: %s", configFilePath)
	}
	return hostList, nil
}

// reads through the YAML files containing descriptions on which hosts to monitor
func ReadMonitoringConfigFromFileOrFolder(fileOrFolder string) ([]MonitoredDatabase, error) {
	hostList := make([]MonitoredDatabase, 0)

	fi, err := os.Stat(fileOrFolder)
	if err != nil {
		log.Errorf("Could not Stat() path: %s", fileOrFolder)
		return hostList, err
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		log.Infof("Reading monitoring config from path %s ...", fileOrFolder)

		err := filepath.Walk(fileOrFolder, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err // abort on first failure
			}
			if info.Mode().IsRegular() && (strings.HasSuffix(strings.ToLower(info.Name()), ".yaml") || strings.HasSuffix(strings.ToLower(info.Name()), ".yml")) {
				log.Debug("Found YAML config file:", info.Name())
				mdbs, err := ConfigFileToMonitoredDatabases(path)
				if err == nil {
					hostList = append(hostList, mdbs...)
				}
			}
			return nil
		})
		if err != nil {
			log.Errorf("Could not successfully Walk() path %s: %s", fileOrFolder, err)
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
			mdef, ok := preset_metric_def_map[e.PresetMetrics]
			if !ok {
				log.Errorf("Failed to resolve preset config \"%s\" for \"%s\"", e.PresetMetrics, e.DBUniqueName)
				continue
			}
			e.Metrics = mdef
		}
		if _, ok := dbTypeMap[e.DBType]; !ok {
			log.Warningf("Ignoring host \"%s\" - unknown dbtype: %s. Expected one of: %+v", e.DBUniqueName, e.DBType, dbTypes)
			continue
		}
		if e.IsEnabled && e.PasswordType == "aes-gcm-256" && opts.AesGcmKeyphrase != "" {
			e.Password = decrypt(e.DBUniqueName, opts.AesGcmKeyphrase, e.Password)
		}
		if e.DBType == config.DBTYPE_PATRONI && e.DBName == "" {
			log.Warningf("Ignoring host \"%s\" as \"dbname\" attribute not specified but required by dbtype=patroni", e.DBUniqueName)
			continue
		}
		if e.DBType == config.DBTYPE_PG && e.DBName == "" {
			log.Warningf("Ignoring host \"%s\" as \"dbname\" attribute not specified but required by dbtype=postgres", e.DBUniqueName)
			continue
		}
		if len(e.DBName) == 0 || e.DBType == config.DBTYPE_PG_CONT || e.DBType == config.DBTYPE_PATRONI || e.DBType == config.DBTYPE_PATRONI_CONT || e.DBType == config.DBTYPE_PATRONI_NAMESPACE_DISCOVERY {
			if e.DBType == config.DBTYPE_PG_CONT {
				log.Debugf("Adding \"%s\" (host=%s, port=%s) to continuous monitoring ...", e.DBUniqueName, e.Host, e.Port)
			}
			var found_dbs []MonitoredDatabase
			var err error

			if e.DBType == config.DBTYPE_PATRONI || e.DBType == config.DBTYPE_PATRONI_CONT || e.DBType == config.DBTYPE_PATRONI_NAMESPACE_DISCOVERY {
				found_dbs, err = ResolveDatabasesFromPatroni(e)
			} else {
				found_dbs, err = ResolveDatabasesFromConfigEntry(e)
			}
			if err != nil {
				log.Errorf("Failed to resolve DBs for \"%s\": %s", e.DBUniqueName, err)
				continue
			}
			temp_arr := make([]string, 0)
			for _, r := range found_dbs {
				md = append(md, r)
				temp_arr = append(temp_arr, r.DBName)
			}
			log.Debugf("Resolved %d DBs with prefix \"%s\": [%s]", len(found_dbs), e.DBUniqueName, strings.Join(temp_arr, ", "))
		} else {
			md = append(md, e)
		}
	}
	return md
}

func StatsServerHandler(w http.ResponseWriter, req *http.Request) {
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

func StartStatsServer(port int64) {
	http.HandleFunc("/", StatsServerHandler)
	for {
		log.Errorf("Failure in StatsServerHandler:", http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
		time.Sleep(time.Second * 60)
	}
}

// Calculates 1min avg metric fetching statistics for last 5min for StatsServerHandler to display
func StatsSummarizer() {
	var prevMetricsCounterValue uint64
	var currentMetricsCounterValue uint64
	ticker := time.NewTicker(time.Minute * 5)
	lastSummarization := gathererStartTime
	for now := range ticker.C {
		currentMetricsCounterValue = atomic.LoadUint64(&totalMetricsFetchedCounter)
		atomic.StoreInt64(&metricPointsPerMinuteLast5MinAvg, int64(math.Round(float64(currentMetricsCounterValue-prevMetricsCounterValue)*60/now.Sub(lastSummarization).Seconds())))
		prevMetricsCounterValue = currentMetricsCounterValue
		lastSummarization = now
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
		log.Warningf("Aes-gcm-256 encrypted password for \"%s\" should consist of 3 parts - using 'as is'", dbUnique)
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

func SyncMonitoredDBsToDatastore(monitored_dbs []MonitoredDatabase, persistence_channel chan []MetricStoreMessage) {
	if len(monitored_dbs) > 0 {
		msms := make([]MetricStoreMessage, len(monitored_dbs))
		now := time.Now()

		for _, mdb := range monitored_dbs {
			var db = make(map[string]interface{})
			db["tag_group"] = mdb.Group
			db["master_only"] = mdb.OnlyIfMaster
			db["epoch_ns"] = now.UnixNano()
			db["continuous_discovery_prefix"] = mdb.DBUniqueNameOrig
			for k, v := range mdb.CustomTags {
				db["tag_"+k] = v
			}
			var data = [](map[string]interface{}){db}
			msms = append(msms, MetricStoreMessage{DBUniqueName: mdb.DBUniqueName, MetricName: MONITORED_DBS_DATASTORE_SYNC_METRIC_NAME,
				Data: data})
		}
		persistence_channel <- msms
	}
}

func CheckFolderExistsAndReadable(path string) bool {
	if _, err := os.ReadDir(path); err != nil {
		return false
	}
	return true
}

func goPsutilCalcCPUUtilization(probe0, probe1 cpu.TimesStat) float64 {
	return 100 - (100.0 * (probe1.Idle - probe0.Idle + probe1.Iowait - probe0.Iowait + probe1.Steal - probe0.Steal) / (probe1.Total() - probe0.Total()))
}

// Simulates "psutil" metric output. Assumes the result from last call as input, otherwise uses a 1s measurement
// https://github.com/cybertec-postgresql/pgwatch3/blob/master/pgwatch3/metrics/psutil_cpu/9.0/metric.sql
func GetGoPsutilCPU(interval time.Duration) ([]map[string]interface{}, error) {
	prevCPULoadTimeStatsLock.RLock()
	prevTime := prevCPULoadTimestamp
	prevTimeStat := prevCPULoadTimeStats
	prevCPULoadTimeStatsLock.RUnlock()

	if prevTime.IsZero() || (time.Now().UnixNano()-prevTime.UnixNano()) < 1e9 { // give "short" stats on first run, based on a 1s probe
		probe0, err := cpu.Times(false)
		if err != nil {
			return nil, err
		}
		prevTimeStat = probe0[0]
		time.Sleep(1e9)
	}

	curCallStats, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if prevTime.IsZero() || time.Now().UnixNano()-prevTime.UnixNano() < 1e9 || time.Now().Unix()-prevTime.Unix() >= int64(interval.Seconds()) {
		prevCPULoadTimeStatsLock.Lock() // update the cache
		prevCPULoadTimeStats = curCallStats[0]
		prevCPULoadTimestamp = time.Now()
		prevCPULoadTimeStatsLock.Unlock()
	}

	la, err := load.Avg()
	if err != nil {
		return nil, err
	}

	cpus, err := cpu.Counts(true)
	if err != nil {
		return nil, err
	}

	retMap := make(map[string]interface{})
	retMap["epoch_ns"] = time.Now().UnixNano()
	retMap["cpu_utilization"] = math.Round(100*goPsutilCalcCPUUtilization(prevTimeStat, curCallStats[0])) / 100
	retMap["load_1m_norm"] = math.Round(100*la.Load1/float64(cpus)) / 100
	retMap["load_1m"] = math.Round(100*la.Load1) / 100
	retMap["load_5m_norm"] = math.Round(100*la.Load5/float64(cpus)) / 100
	retMap["load_5m"] = math.Round(100*la.Load5) / 100
	retMap["user"] = math.Round(10000.0*(curCallStats[0].User-prevTimeStat.User)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["system"] = math.Round(10000.0*(curCallStats[0].System-prevTimeStat.System)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["idle"] = math.Round(10000.0*(curCallStats[0].Idle-prevTimeStat.Idle)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["iowait"] = math.Round(10000.0*(curCallStats[0].Iowait-prevTimeStat.Iowait)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["irqs"] = math.Round(10000.0*(curCallStats[0].Irq-prevTimeStat.Irq+curCallStats[0].Softirq-prevTimeStat.Softirq)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100
	retMap["other"] = math.Round(10000.0*(curCallStats[0].Steal-prevTimeStat.Steal+curCallStats[0].Guest-prevTimeStat.Guest+curCallStats[0].GuestNice-prevTimeStat.GuestNice)/(curCallStats[0].Total()-prevTimeStat.Total())) / 100

	return []map[string]interface{}{retMap}, nil
}

func GetGoPsutilMem() ([]map[string]interface{}, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	retMap := make(map[string]interface{})
	retMap["epoch_ns"] = time.Now().UnixNano()
	retMap["total"] = int64(vm.Total)
	retMap["used"] = int64(vm.Used)
	retMap["free"] = int64(vm.Free)
	retMap["buff_cache"] = int64(vm.Buffers)
	retMap["available"] = int64(vm.Available)
	retMap["percent"] = math.Round(100*vm.UsedPercent) / 100
	retMap["swap_total"] = int64(vm.SwapTotal)
	retMap["swap_used"] = int64(vm.SwapCached)
	retMap["swap_free"] = int64(vm.SwapFree)
	retMap["swap_percent"] = math.Round(100*float64(vm.SwapCached)/float64(vm.SwapTotal)) / 100

	return []map[string]interface{}{retMap}, nil
}

func GetGoPsutilDiskTotals() ([]map[string]interface{}, error) {
	d, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}

	retMap := make(map[string]interface{})
	var readBytes, writeBytes, reads, writes float64

	retMap["epoch_ns"] = time.Now().UnixNano()
	for _, v := range d { // summarize all disk devices
		readBytes += float64(v.ReadBytes) // datatype float is just an oversight in the original psutil helper
		// but can't change it without causing problems on storage level (InfluxDB)
		writeBytes += float64(v.WriteBytes)
		reads += float64(v.ReadCount)
		writes += float64(v.WriteCount)
	}
	retMap["read_bytes"] = readBytes
	retMap["write_bytes"] = writeBytes
	retMap["read_count"] = reads
	retMap["write_count"] = writes

	return []map[string]interface{}{retMap}, nil
}

func GetLoadAvgLocal() ([]map[string]interface{}, error) {
	la, err := load.Avg()
	if err != nil {
		log.Errorf("Could not inquiry local system load average: %v", err)
		return nil, err
	}

	row := make(map[string]interface{})
	row["epoch_ns"] = time.Now().UnixNano()
	row["load_1min"] = la.Load1
	row["load_5min"] = la.Load5
	row["load_15min"] = la.Load15

	return []map[string]interface{}{row}, nil
}

func shouldDbBeMonitoredBasedOnCurrentState(md MonitoredDatabase) bool {
	return !IsDBDormant(md.DBUniqueName)
}

func ControlChannelsMapToList(control_channels map[string]chan ControlMessage) []string {
	control_channel_list := make([]string, len(control_channels))
	i := 0
	for key := range control_channels {
		control_channel_list[i] = key
		i++
	}
	return control_channel_list
}

func DoCloseResourcesForRemovedMonitoredDBIfAny(dbUnique string) {

	CloseOrLimitSQLConnPoolForMonitoredDBIfAny(dbUnique)

	PurgeMetricsFromPromAsyncCacheIfAny(dbUnique, "")
}

func CloseResourcesForRemovedMonitoredDBs(currentDBs, prevLoopDBs []MonitoredDatabase, shutDownDueToRoleChange map[string]bool) {
	var curDBsMap = make(map[string]bool)

	for _, curDB := range currentDBs {
		curDBsMap[curDB.DBUniqueName] = true
	}

	for _, prevDB := range prevLoopDBs {
		if _, ok := curDBsMap[prevDB.DBUniqueName]; !ok { // removed from config
			DoCloseResourcesForRemovedMonitoredDBIfAny(prevDB.DBUniqueName)
		}
	}

	// or to be ignored due to current instance state
	for roleChangedDB := range shutDownDueToRoleChange {
		DoCloseResourcesForRemovedMonitoredDBIfAny(roleChangedDB)
	}
}

func PromAsyncCacheInitIfRequired(dbUnique, metric string) { // cache structure: [dbUnique][metric]lastly_fetched_data
	if opts.Metric.Datastore == DATASTORE_PROMETHEUS && opts.Metric.PrometheusAsyncMode {
		promAsyncMetricCacheLock.Lock()
		defer promAsyncMetricCacheLock.Unlock()
		if _, ok := promAsyncMetricCache[dbUnique]; !ok {
			metricMap := make(map[string][]MetricStoreMessage)
			promAsyncMetricCache[dbUnique] = metricMap
		}
	}
}

func PromAsyncCacheAddMetricData(dbUnique, metric string, msgArr []MetricStoreMessage) { // cache structure: [dbUnique][metric]lastly_fetched_data
	promAsyncMetricCacheLock.Lock()
	defer promAsyncMetricCacheLock.Unlock()
	if _, ok := promAsyncMetricCache[dbUnique]; ok {
		promAsyncMetricCache[dbUnique][metric] = msgArr
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
	// NB! After creating the file it can still take up to --servers-refresh-loop-seconds (2min def.) for change to take effect!
	if opts.EmergencyPauseTriggerfile == "" {
		return false
	}
	_, err := os.Stat(opts.EmergencyPauseTriggerfile)
	return err == nil
}

func DoesMetricDefinitionCallHelperFunctions(sqlDefinition string) bool {
	if !opts.Metric.NoHelperFunctions { // save on regex matching --no-helper-functions param not set, information will not be used then anyways
		return false
	}
	return regexSQLHelperFunctionCalled.MatchString(sqlDefinition)
}

var opts config.CmdOptions

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
		cancel()
	}()
	exitCode = ExitCodeUserCancel
}

const (
	ExitCodeOK int = iota
	ExitCodeConfigError
	ExitCodeWebUIError
	ExitCodeUpgradeError
	ExitCodeUserCancel
	ExitCodeShutdownCommand
)

var exitCode = ExitCodeOK

func main() {
	defer func() { os.Exit(exitCode) }()

	ctx, cancel := context.WithCancel(context.Background())
	SetupCloseHandler(cancel)
	defer cancel()

	opts, err := config.NewConfig(os.Stdout)
	if err != nil {
		if opts != nil && opts.VersionOnly() {
			printVersion()
			return
		}
		fmt.Println("Configuration error: ", err)
		exitCode = ExitCodeConfigError
		return
	}

	uifs, _ := fs.Sub(webuifs, "webui/build")
	ui := webserver.Init(":8080", uifs, uiapi)
	if ui == nil {
		exitCode = ExitCodeWebUIError
		return
	}

	if strings.ToUpper(opts.Verbose) == "DEBUG" {
		logging.SetLevel(logging.DEBUG, "main")
	} else if strings.ToUpper(opts.Verbose) == "INFO" {
		logging.SetLevel(logging.INFO, "main")
	} else if strings.HasPrefix(strings.ToUpper(opts.Verbose), "WARN") {
		logging.SetLevel(logging.WARNING, "main")
	} else {
		if len(opts.Verbose) >= 2 {
			logging.SetLevel(logging.DEBUG, "main")
		} else if len(opts.Verbose) == 1 {
			logging.SetLevel(logging.INFO, "main")
		} else {
			logging.SetLevel(logging.WARNING, "main")
		}
	}
	logging.SetFormatter(logging.MustStringFormatter(`%{level:.4s} %{shortfunc}: %{message}`))

	log.Debugf("opts: %+v", opts)

	if opts.AesGcmPasswordToEncrypt > "" { // special flag - encrypt and exit
		fmt.Println(encrypt(opts.AesGcmKeyphrase, opts.AesGcmPasswordToEncrypt))
		os.Exit(0)
	}

	if opts.IsAdHocMode() && opts.AdHocUniqueName == "adhoc" {
		log.Warning("In ad-hoc mode: using default unique name 'adhoc' for metrics storage. use --adhoc-name to override.")
	}

	// running in config file based mode?
	if len(opts.Config) > 0 {
		if opts.Metric.MetricsFolder == "" && CheckFolderExistsAndReadable(DEFAULT_METRICS_DEFINITION_PATH_PKG) {
			opts.Metric.MetricsFolder = DEFAULT_METRICS_DEFINITION_PATH_PKG
			log.Warningf("--metrics-folder path not specified, using %s", opts.Metric.MetricsFolder)
		} else if opts.Metric.MetricsFolder == "" && CheckFolderExistsAndReadable(DEFAULT_METRICS_DEFINITION_PATH_DOCKER) {
			opts.Metric.MetricsFolder = DEFAULT_METRICS_DEFINITION_PATH_DOCKER
			log.Warningf("--metrics-folder path not specified, using %s", opts.Metric.MetricsFolder)
		} else {
			if !CheckFolderExistsAndReadable(opts.Metric.MetricsFolder) {
				log.Fatalf("Could not read --metrics-folder path %s", opts.Metric.MetricsFolder)
			}
		}

		if !opts.IsAdHocMode() {
			fi, err := os.Stat(opts.Config)
			if err != nil {
				log.Fatalf("Could not Stat() path %s: %s", opts.Config, err)
			}
			switch mode := fi.Mode(); {
			case mode.IsDir():
				_, err := os.ReadDir(opts.Config)
				if err != nil {
					log.Fatalf("Could not read path %s: %s", opts.Config, err)
				}
			case mode.IsRegular():
				_, err := os.ReadFile(opts.Config)
				if err != nil {
					log.Fatalf("Could not read path %s: %s", opts.Config, err)
				}
			}
		}

		fileBasedMetrics = true
	} else if opts.IsAdHocMode() && opts.Metric.MetricsFolder != "" && CheckFolderExistsAndReadable(opts.Metric.MetricsFolder) {
		// don't need the Config DB connection actually for ad-hoc mode if metric definitions are there
		fileBasedMetrics = true
	} else if opts.IsAdHocMode() && opts.Metric.MetricsFolder == "" && (CheckFolderExistsAndReadable(DEFAULT_METRICS_DEFINITION_PATH_PKG) || CheckFolderExistsAndReadable(DEFAULT_METRICS_DEFINITION_PATH_DOCKER)) {
		if CheckFolderExistsAndReadable(DEFAULT_METRICS_DEFINITION_PATH_PKG) {
			opts.Metric.MetricsFolder = DEFAULT_METRICS_DEFINITION_PATH_PKG
		} else if CheckFolderExistsAndReadable(DEFAULT_METRICS_DEFINITION_PATH_DOCKER) {
			opts.Metric.MetricsFolder = DEFAULT_METRICS_DEFINITION_PATH_DOCKER
		}
		log.Warningf("--metrics-folder path not specified, using %s", opts.Metric.MetricsFolder)
		fileBasedMetrics = true
	} else { // normal "Config DB" mode
		// make sure all PG params are there
		if opts.Connection.User == "" {
			opts.Connection.User = os.Getenv("USER")
		}
		if opts.Connection.Host == "" || opts.Connection.Port == "" || opts.Connection.Dbname == "" || opts.Connection.User == "" {
			fmt.Println("Check config DB parameters")
			return
		}

		_ = InitAndTestConfigStoreConnection(ctx, opts.Connection.Host,
			opts.Connection.Port, opts.Connection.Dbname, opts.Connection.User, opts.Connection.Password,
			opts.Connection.PgRequireSSL, true)
	}

	pgBouncerNumericCountersStartVersion, _ = decimal.NewFromString("1.12")

	if opts.InternalStatsPort > 0 && !opts.Ping {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", opts.InternalStatsPort))
		if err != nil {
			log.Fatalf("Could not start the internal statistics interface on port %d. Set --internal-stats-port to an open port or to 0 to disable. Err: %v", opts.InternalStatsPort, err)
		}
		err = l.Close()
		if err != nil {
			log.Fatalf("Could not cleanly stop the temporary listener on port %d: %v", opts.InternalStatsPort, err)
		}
		log.Infof("Starting the internal statistics interface on port %d...", opts.InternalStatsPort)
		go StartStatsServer(opts.InternalStatsPort)
		go StatsSummarizer()
	}

	if opts.Metric.PrometheusAsyncMode {
		opts.BatchingDelayMs = 0 // using internal cache, no batching for storage smoothing needed
	}

	control_channels := make(map[string](chan ControlMessage)) // [db1+metric1]=chan
	persist_ch := make(chan []MetricStoreMessage, 10000)
	var buffered_persist_ch chan []MetricStoreMessage

	if !opts.Ping {

		if opts.BatchingDelayMs > 0 && opts.Metric.Datastore != DATASTORE_PROMETHEUS {
			buffered_persist_ch = make(chan []MetricStoreMessage, 10000) // "staging area" for metric storage batching, when enabled
			log.Info("starting MetricsBatcher...")
			go MetricsBatcher(opts.BatchingDelayMs, buffered_persist_ch, persist_ch)
		}

		if opts.Metric.Datastore == DATASTORE_GRAPHITE {
			if opts.Metric.GraphiteHost == "" || opts.Metric.GraphitePort == "" {
				log.Fatal("--graphite-host/port needed!")
			}
			port, _ := strconv.ParseInt(opts.Metric.GraphitePort, 10, 32)
			graphite_host = opts.Metric.GraphiteHost
			graphite_port = int(port)
			InitGraphiteConnection(graphite_host, graphite_port)
			log.Info("starting GraphitePersister...")
			go MetricsPersister(DATASTORE_GRAPHITE, persist_ch)
		} else if opts.Metric.Datastore == DATASTORE_JSON {
			if len(opts.Metric.JSONStorageFile) == 0 {
				log.Fatal("--datastore=json requires --json-storage-file to be set")
			}
			jsonOutFile, err := os.Create(opts.Metric.JSONStorageFile) // test file path writeability
			if err != nil {
				log.Fatalf("Could not create JSON storage file: %s", err)
			}
			err = jsonOutFile.Close()
			if err != nil {
				log.Fatal(err)
			}
			log.Warningf("In JSON output mode. Gathered metrics will be written to \"%s\"...", opts.Metric.JSONStorageFile)
			go MetricsPersister(DATASTORE_JSON, persist_ch)
		} else if opts.Metric.Datastore == DATASTORE_POSTGRES {
			if len(opts.Metric.PGMetricStoreConnStr) == 0 {
				log.Fatal("--datastore=postgres requires --pg-metric-store-conn-str to be set")
			}

			_ = InitAndTestMetricStoreConnection(opts.Metric.PGMetricStoreConnStr, true)

			PGSchemaType = CheckIfPGSchemaInitializedOrFail()

			log.Info("starting PostgresPersister...")
			go MetricsPersister(DATASTORE_POSTGRES, persist_ch)

			log.Info("starting UniqueDbnamesListingMaintainer...")
			go UniqueDbnamesListingMaintainer(true)

			if opts.Metric.PGRetentionDays > 0 && PGSchemaType != "custom" && opts.TestdataDays == 0 {
				log.Info("starting old Postgres metrics cleanup job...")
				go OldPostgresMetricsDeleter(opts.Metric.PGRetentionDays, PGSchemaType)
			}

		} else if opts.Metric.Datastore == DATASTORE_PROMETHEUS {
			if opts.TestdataDays != 0 || opts.TestdataMultiplier > 0 {
				log.Fatal("Test data generation mode cannot be used with Prometheus data store")
			}

			if opts.Metric.PrometheusAsyncMode {
				log.Info("starting Prometheus Cache Persister...")
				go MetricsPersister(DATASTORE_PROMETHEUS, persist_ch)
			}
			go StartPrometheusExporter()
		} else {
			log.Fatal("Unknown datastore. Check the --datastore param")
		}

		_, _ = daemon.SdNotify(false, "READY=1") // Notify systemd, does nothing outside of systemd
	}

	first_loop := true
	mainLoopCount := 0
	var monitored_dbs []MonitoredDatabase
	var last_metrics_refresh_time int64
	var metrics map[string]map[decimal.Decimal]MetricVersionProperties
	var hostLastKnownStatusInRecovery = make(map[string]bool) // isInRecovery
	var metric_config map[string]float64                      // set to host.Metrics or host.MetricsStandby (in case optional config defined and in recovery state

	for { //main loop
		hostsToShutDownDueToRoleChange := make(map[string]bool) // hosts went from master to standby and have "only if master" set
		var control_channel_name_list []string
		gatherers_shut_down := 0

		if time.Now().Unix()-last_metrics_refresh_time > METRIC_DEFINITION_REFRESH_TIME {
			//metrics
			if fileBasedMetrics {
				metrics, err = ReadMetricsFromFolder(opts.Metric.MetricsFolder, first_loop)
			} else {
				metrics, err = ReadMetricDefinitionMapFromPostgres(first_loop)
			}
			if err == nil {
				UpdateMetricDefinitionMap(metrics)
				last_metrics_refresh_time = time.Now().Unix()
			} else {
				log.Errorf("Could not refresh metric definitions: %s", err)
			}
		}

		if fileBasedMetrics {
			pmc, err := ReadPresetMetricsConfigFromFolder(opts.Metric.MetricsFolder, false)
			if err != nil {
				if first_loop {
					log.Fatalf("Could not read preset metric config from \"%s\": %s", path.Join(opts.Metric.MetricsFolder, PRESET_CONFIG_YAML_FILE), err)
				} else {
					log.Errorf("Could not read preset metric config from \"%s\": %s", path.Join(opts.Metric.MetricsFolder, PRESET_CONFIG_YAML_FILE), err)
				}
			} else {
				preset_metric_def_map = pmc
				log.Debugf("Loaded preset metric config: %#v", pmc)
			}

			if opts.IsAdHocMode() {
				adhocconfig, ok := pmc[opts.AdHocConfig]
				if !ok {
					log.Warningf("Could not find a preset metric config named \"%s\", assuming JSON config...", opts.AdHocConfig)
					adhocconfig, err = jsonTextToMap(opts.AdHocConfig)
					if err != nil {
						log.Fatalf("Could not parse --adhoc-config(%s): %v", opts.AdHocConfig, err)
					}
				}
				md := MonitoredDatabase{DBUniqueName: opts.AdHocUniqueName, DBType: opts.AdHocDBType, Metrics: adhocconfig, LibPQConnStr: opts.AdHocConnString}
				if opts.AdHocDBType == config.DBTYPE_PG {
					monitored_dbs = []MonitoredDatabase{md}
				} else {
					resolved, err := ResolveDatabasesFromConfigEntry(md)
					if err != nil {
						if first_loop {
							log.Fatalf("Failed to resolve DBs for ConnStr \"%s\": %s", opts.AdHocConnString, err)
						} else { // keep previously found list
							log.Errorf("Failed to resolve DBs for ConnStr \"%s\": %s", opts.AdHocConnString, err)
						}
					} else {
						monitored_dbs = resolved
					}
				}
			} else {
				mc, err := ReadMonitoringConfigFromFileOrFolder(opts.Config)
				if err == nil {
					log.Debugf("Found %d monitoring config entries", len(mc))
					if len(opts.Metric.Group) > 0 {
						var removed_count int
						mc, removed_count = FilterMonitoredDatabasesByGroup(mc, opts.Metric.Group)
						log.Infof("Filtered out %d config entries based on --groups=%s", removed_count, opts.Metric.Group)
					}
					monitored_dbs = GetMonitoredDatabasesFromMonitoringConfig(mc)
					log.Debugf("Found %d databases to monitor from %d config items...", len(monitored_dbs), len(mc))
				} else {
					if first_loop {
						log.Fatalf("Could not read/parse monitoring config from path: %s. err: %v", opts.Config, err)
					} else {
						log.Errorf("Could not read/parse monitoring config from path: %s. using last valid config data. err: %v", opts.Config, err)
					}
					time.Sleep(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds))
					continue
				}
			}
		} else {
			monitored_dbs, err = GetMonitoredDatabasesFromConfigDB()
			if err != nil {
				if first_loop {
					log.Fatal("could not fetch active hosts - check config!", err)
				} else {
					log.Error("could not fetch active hosts, using last valid config data. err:", err)
					time.Sleep(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds))
					continue
				}
			}
		}

		if DoesEmergencyTriggerfileExist() {
			log.Warningf("Emergency pause triggerfile detected at %s, ignoring currently configured DBs", opts.EmergencyPauseTriggerfile)
			monitored_dbs = make([]MonitoredDatabase, 0)
		}

		UpdateMonitoredDBCache(monitored_dbs)

		if lastMonitoredDBsUpdate.IsZero() || lastMonitoredDBsUpdate.Before(time.Now().Add(-1*time.Second*MONITORED_DBS_DATASTORE_SYNC_INTERVAL_SECONDS)) {
			monitored_dbs_copy := make([]MonitoredDatabase, len(monitored_dbs))
			copy(monitored_dbs_copy, monitored_dbs)
			if opts.BatchingDelayMs > 0 {
				go SyncMonitoredDBsToDatastore(monitored_dbs_copy, buffered_persist_ch)
			} else {
				go SyncMonitoredDBsToDatastore(monitored_dbs_copy, persist_ch)
			}
			lastMonitoredDBsUpdate = time.Now()
		}

		if first_loop && (len(monitored_dbs) == 0 || len(metric_def_map) == 0) {
			log.Warningf("host info refreshed, nr. of enabled entries in configuration: %d, nr. of distinct metrics: %d", len(monitored_dbs), len(metric_def_map))
		} else {
			log.Infof("host info refreshed, nr. of enabled entries in configuration: %d, nr. of distinct metrics: %d", len(monitored_dbs), len(metric_def_map))
		}

		if first_loop {
			first_loop = false // only used for failing when 1st config reading fails
		}

		for _, host := range monitored_dbs {
			log.Debugf("processing database: %s, metric config: %v, custom tags: %v, host config: %#v", host.DBUniqueName, host.Metrics, host.CustomTags, host.HostConfig)

			db_unique := host.DBUniqueName
			db_unique_orig := host.DBUniqueNameOrig
			db_type := host.DBType
			metric_config = host.Metrics
			wasInstancePreviouslyDormant := IsDBDormant(db_unique)

			if host.PasswordType == "aes-gcm-256" && len(opts.AesGcmKeyphrase) == 0 && len(opts.AesGcmKeyphraseFile) == 0 {
				// Warn if any encrypted hosts found but no keyphrase given
				log.Warningf("Encrypted password type found for host \"%s\", but no decryption keyphrase specified. Use --aes-gcm-keyphrase or --aes-gcm-keyphrase-file params", db_unique)
			}

			err := InitSQLConnPoolForMonitoredDBIfNil(host)
			if err != nil {
				log.Warningf("Could not init SQL connection pool for %s, retrying on next main loop. Err: %v", db_unique, err)
				continue
			}

			InitPGVersionInfoFetchingLockIfNil(host)

			_, connectFailedSoFar := failedInitialConnectHosts[db_unique]

			if connectFailedSoFar { // idea is not to spwan any runners before we've successfully pinged the DB
				var err error
				var ver DBVersionMapEntry

				if connectFailedSoFar {
					log.Infof("retrying to connect to uninitialized DB \"%s\"...", db_unique)
				} else {
					log.Infof("new host \"%s\" found, checking connectivity...", db_unique)
				}

				ver, err = DBGetPGVersion(db_unique, db_type, true)
				if err != nil {
					log.Errorf("could not start metric gathering for DB \"%s\" due to connection problem: %s", db_unique, err)
					if opts.AdHocConnString != "" {
						log.Errorf("will retry in %ds...", opts.Connection.ServersRefreshLoopSeconds)
					}
					failedInitialConnectHosts[db_unique] = true
					continue
				} else {
					log.Infof("Connect OK. [%s] is on version %s (in recovery: %v)", db_unique, ver.VersionStr, ver.IsInRecovery)
					if connectFailedSoFar {
						delete(failedInitialConnectHosts, db_unique)
					}
					if ver.IsInRecovery && host.OnlyIfMaster {
						log.Infof("[%s] not added to monitoring due to 'master only' property", db_unique)
						continue
					}
					metric_config = host.Metrics
					hostLastKnownStatusInRecovery[db_unique] = ver.IsInRecovery
					if ver.IsInRecovery && len(host.MetricsStandby) > 0 {
						metric_config = host.MetricsStandby
					}
				}

				if !opts.Ping && (host.IsSuperuser || opts.IsAdHocMode() && opts.AdHocCreateHelpers) && IsPostgresDBType(db_type) && !ver.IsInRecovery {
					if opts.Metric.NoHelperFunctions {
						log.Infof("[%s] Skipping rollout out helper functions due to the --no-helper-functions flag ...", db_unique)
					} else {
						log.Infof("Trying to create helper functions if missing for \"%s\"...", db_unique)
						_ = TryCreateMetricsFetchingHelpers(db_unique)
					}
				}

				if !(opts.Ping || (opts.Metric.Datastore == DATASTORE_PROMETHEUS && !opts.Metric.PrometheusAsyncMode)) {
					time.Sleep(time.Millisecond * 100) // not to cause a huge load spike when starting the daemon with 100+ monitored DBs
				}
			}

			if IsPostgresDBType(host.DBType) {
				var DBSizeMB int64

				if opts.MinDbSizeMB >= 8 { // an empty DB is a bit less than 8MB
					DBSizeMB, _ = DBGetSizeMB(db_unique) // ignore errors, i.e. only remove from monitoring when we're certain it's under the threshold
					if DBSizeMB != 0 {
						if DBSizeMB < opts.MinDbSizeMB {
							log.Infof("[%s] DB will be ignored due to the --min-db-size-mb filter. Current (up to %v cached) DB size = %d MB", db_unique, DB_SIZE_CACHING_INTERVAL, DBSizeMB)
							hostsToShutDownDueToRoleChange[db_unique] = true // for the case when DB size was previosly above the threshold
							SetUndersizedDBState(db_unique, true)
							continue
						} else {
							SetUndersizedDBState(db_unique, false)
						}
					}
				}
				ver, err := DBGetPGVersion(db_unique, host.DBType, false)
				if err == nil { // ok to ignore error, re-tried on next loop
					lastKnownStatusInRecovery := hostLastKnownStatusInRecovery[db_unique]
					if ver.IsInRecovery && host.OnlyIfMaster {
						log.Infof("[%s] to be removed from monitoring due to 'master only' property and status change", db_unique)
						hostsToShutDownDueToRoleChange[db_unique] = true
						SetRecoveryIgnoredDBState(db_unique, true)
						continue
					} else if lastKnownStatusInRecovery != ver.IsInRecovery {
						if ver.IsInRecovery && len(host.MetricsStandby) > 0 {
							log.Warningf("Switching metrics collection for \"%s\" to standby config...", db_unique)
							metric_config = host.MetricsStandby
							hostLastKnownStatusInRecovery[db_unique] = true
						} else {
							log.Warningf("Switching metrics collection for \"%s\" to primary config...", db_unique)
							metric_config = host.Metrics
							hostLastKnownStatusInRecovery[db_unique] = false
							SetRecoveryIgnoredDBState(db_unique, false)
						}
					}
				}

				if wasInstancePreviouslyDormant && !IsDBDormant(db_unique) {
					RestoreSqlConnPoolLimitsForPreviouslyDormantDB(db_unique)
				}

				if mainLoopCount == 0 && opts.TryCreateListedExtsIfMissing != "" && !ver.IsInRecovery {
					extsToCreate := strings.Split(opts.TryCreateListedExtsIfMissing, ",")
					extsCreated := TryCreateMissingExtensions(db_unique, extsToCreate, ver.Extensions)
					log.Infof("[%s] %d/%d extensions created based on --try-create-listed-exts-if-missing input %v", db_unique, len(extsCreated), len(extsToCreate), extsCreated)
				}
			}

			if opts.Ping {
				continue // don't launch metric fetching threads
			}

			for metric_name := range metric_config {
				if opts.Metric.Datastore == DATASTORE_PROMETHEUS && !opts.Metric.PrometheusAsyncMode {
					continue // normal (non-async, no background fetching) Prom mode means only per-scrape fetching
				}
				metric := metric_name
				metric_def_ok := false

				if strings.HasPrefix(metric, RECO_PREFIX) {
					metric = RECO_METRIC_NAME
				}
				interval := metric_config[metric]

				if metric == RECO_METRIC_NAME {
					metric_def_ok = true
				} else {
					metric_def_map_lock.RLock()
					_, metric_def_ok = metric_def_map[metric]
					metric_def_map_lock.RUnlock()
				}

				db_metric := db_unique + DB_METRIC_JOIN_STR + metric
				_, ch_ok := control_channels[db_metric]

				if metric_def_ok && !ch_ok { // initialize a new per db/per metric control channel
					if interval > 0 {
						host_metric_interval_map[db_metric] = interval
						log.Infof("starting gatherer for [%s:%s] with interval %v s", db_unique, metric, interval)
						control_channels[db_metric] = make(chan ControlMessage, 1)
						PromAsyncCacheInitIfRequired(db_unique, metric)
						if opts.BatchingDelayMs > 0 {
							go MetricGathererLoop(db_unique, db_unique_orig, db_type, metric, metric_config, control_channels[db_metric], buffered_persist_ch)
						} else {
							go MetricGathererLoop(db_unique, db_unique_orig, db_type, metric, metric_config, control_channels[db_metric], persist_ch)
						}
					}
				} else if (!metric_def_ok && ch_ok) || interval <= 0 {
					// metric definition files were recently removed or interval set to zero
					log.Warning("shutting down metric", metric, "for", host.DBUniqueName)
					control_channels[db_metric] <- ControlMessage{Action: GATHERER_STATUS_STOP}
					delete(control_channels, db_metric)
				} else if !metric_def_ok {
					epoch, ok := last_sql_fetch_error.Load(metric)
					if !ok || ((time.Now().Unix() - epoch.(int64)) > 3600) { // complain only 1x per hour
						log.Warningf("metric definition \"%s\" not found for \"%s\"", metric, db_unique)
						last_sql_fetch_error.Store(metric, time.Now().Unix())
					}
				} else {
					// check if interval has changed
					if host_metric_interval_map[db_metric] != interval {
						log.Warning("sending interval update for", db_unique, metric)
						control_channels[db_metric] <- ControlMessage{Action: GATHERER_STATUS_START, Config: metric_config}
						host_metric_interval_map[db_metric] = interval
					}
				}
			}
		}

		atomic.StoreInt32(&mainLoopInitialized, 1) // to hold off scraping until metric fetching runners have been initialized

		if opts.Ping {
			if len(failedInitialConnectHosts) > 0 {
				log.Errorf("Could not reach %d configured DB host out of %d", len(failedInitialConnectHosts), len(monitored_dbs))
				os.Exit(len(failedInitialConnectHosts))
			}
			log.Infof("All configured %d DB hosts were reachable", len(monitored_dbs))
			os.Exit(0)
		}

		if opts.TestdataDays != 0 {
			log.Info("Waiting for all metrics generation goroutines to stop ...")
			time.Sleep(time.Second * 10) // with that time all different metric fetchers should have started
			testDataGenerationModeWG.Wait()
			for {
				pqlen := len(persist_ch)
				if pqlen == 0 {
					if opts.Metric.Datastore == DATASTORE_POSTGRES {
						UniqueDbnamesListingMaintainer(false) // refresh Grafana listing table
					}
					log.Warning("All generators have exited and data stored. Exit")
					os.Exit(0)
				}
				log.Infof("Waiting for generated metrics to be stored (%d still in queue) ...", pqlen)
				time.Sleep(time.Second * 1)
			}
		}

		if mainLoopCount == 0 {
			goto MainLoopSleep
		}

		// loop over existing channels and stop workers if DB or metric removed from config
		// or state change makes it uninteresting
		log.Debug("checking if any workers need to be shut down...")
		control_channel_name_list = ControlChannelsMapToList(control_channels)

		for _, db_metric := range control_channel_name_list {
			var currentMetricConfig map[string]float64
			var dbInfo MonitoredDatabase
			var ok, dbRemovedFromConfig bool
			singleMetricDisabled := false
			splits := strings.Split(db_metric, DB_METRIC_JOIN_STR)
			db := splits[0]
			metric := splits[1]
			//log.Debugf("Checking if need to shut down worker for [%s:%s]...", db, metric)

			_, wholeDbShutDownDueToRoleChange := hostsToShutDownDueToRoleChange[db]
			if !wholeDbShutDownDueToRoleChange {
				monitored_db_cache_lock.RLock()
				dbInfo, ok = monitored_db_cache[db]
				monitored_db_cache_lock.RUnlock()
				if !ok { // normal removing of DB from config
					dbRemovedFromConfig = true
					log.Debugf("DB %s removed from config, shutting down all metric worker processes...", db)
				}
			}

			if !(wholeDbShutDownDueToRoleChange || dbRemovedFromConfig) { // maybe some single metric was disabled
				db_pg_version_map_lock.RLock()
				verInfo, ok := db_pg_version_map[db]
				db_pg_version_map_lock.RUnlock()
				if !ok {
					log.Warningf("Could not find PG version info for DB %s, skipping shutdown check of metric worker process for %s", db, metric)
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

			if wholeDbShutDownDueToRoleChange || dbRemovedFromConfig || singleMetricDisabled {
				log.Infof("shutting down gatherer for [%s:%s] ...", db, metric)
				control_channels[db_metric] <- ControlMessage{Action: GATHERER_STATUS_STOP}
				delete(control_channels, db_metric)
				log.Debugf("control channel for [%s:%s] deleted", db, metric)
				gatherers_shut_down++
				ClearDBUnreachableStateIfAny(db)
				PurgeMetricsFromPromAsyncCacheIfAny(db, metric)
			}
		}

		if gatherers_shut_down > 0 {
			log.Warningf("sent STOP message to %d gatherers (it might take some minutes for them to stop though)", gatherers_shut_down)
		}

		// Destroy conn pools, Prom async cache
		CloseResourcesForRemovedMonitoredDBs(monitored_dbs, prevLoopMonitoredDBs, hostsToShutDownDueToRoleChange)

	MainLoopSleep:
		mainLoopCount++
		prevLoopMonitoredDBs = monitored_dbs

		log.Debugf("main sleeping %ds...", opts.Connection.ServersRefreshLoopSeconds)
		select {
		case <-time.After(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds)):
			// pass
		case <-ctx.Done():
			return
		}
	}

}
