package reaper

import (
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

const (
	epochColumnName     string = "epoch_ns" // this column (epoch in nanoseconds) is expected in every metric query
	tagPrefix           string = "tag_"
	persistQueueMaxSize        = 10000 // storage queue max elements. when reaching the limit, older metrics will be dropped.

	gathererStatusStart     = "START"
	gathererStatusStop      = "STOP"
	metricdbIdent           = "metricDb"
	configdbIdent           = "configDb"
	contextPrometheusScrape = "prometheus-scrape"

	monitoredDbsDatastoreSyncIntervalSeconds = 600              // write actively monitored DBs listing to metrics store after so many seconds
	monitoredDbsDatastoreSyncMetricName      = "configured_dbs" // FYI - for Postgres datastore there's also the admin.all_unique_dbnames table with all recent DB unique names with some metric data

	dbSizeCachingInterval = 30 * time.Minute
	dbMetricJoinStr       = "¤¤¤" // just some unlikely string for a DB name to avoid using maps of maps for DB+metric data

)

type MetricFetchConfig struct {
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

type MonitoredDatabaseSettings struct {
	LastCheckedOn    time.Time
	IsInRecovery     bool
	VersionStr       string
	Version          int
	RealDbname       string
	SystemIdentifier string
	IsSuperuser      bool // if true and no helpers are installed, use superuser SQL version of metric if available
	Extensions       map[string]int
	ExecEnv          string
	ApproxDBSizeB    int64
}

type ExistingPartitionInfo struct {
	StartTime time.Time
	EndTime   time.Time
}
