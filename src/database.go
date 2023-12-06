package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/psutil"
	"github.com/jackc/pgx/v5"
)

var configDb db.PgxPoolIface
var monitoredDbConnCache map[string]db.PgxPoolIface = make(map[string]db.PgxPoolIface)

func IsPostgresDBType(dbType string) bool {
	if dbType == config.DbTypeBouncer || dbType == config.DbTypePgPOOL {
		return false
	}
	return true
}

// every DB under monitoring should have exactly 1 sql.DB connection assigned, that will internally limit parallel access
func InitSQLConnPoolForMonitoredDBIfNil(md MonitoredDatabase) error {
	monitoredDbConnCacheLock.Lock()
	defer monitoredDbConnCacheLock.Unlock()

	conn, ok := monitoredDbConnCache[md.DBUniqueName]
	if ok && conn != nil {
		return nil
	}

	conn, err := db.GetPostgresDBConnection(mainContext, md.ConnStr)
	if err != nil {
		return err
	}

	monitoredDbConnCache[md.DBUniqueName] = conn
	logger.Debugf("[%s] Connection pool initialized with max %d parallel connections. Conn pooling: %v", md.DBUniqueName, opts.MaxParallelConnectionsPerDb, opts.UseConnPooling)

	return nil
}

func CloseOrLimitSQLConnPoolForMonitoredDBIfAny(dbUnique string) {
	monitoredDbConnCacheLock.Lock()
	defer monitoredDbConnCacheLock.Unlock()

	conn, ok := monitoredDbConnCache[dbUnique]
	if !ok || conn == nil {
		return
	}

	if IsDBUndersized(dbUnique) || IsDBIgnoredBasedOnRecoveryState(dbUnique) {

		if opts.UseConnPooling {
			s := conn.Stat()
			if s.TotalConns() > 1 {
				logger.Debugf("[%s] Limiting SQL connection pool to max 1 connection due to dormant state ...", dbUnique)
				// conn.SetMaxIdleConns(1)
				// conn.SetMaxOpenConns(1)
			}
		}

	} else { // removed from config
		logger.Debugf("[%s] Closing SQL connection pool ...", dbUnique)
		conn.Close()
		delete(monitoredDbConnCache, dbUnique)
	}
}

func DBExecRead(ctx context.Context, conn db.PgxIface, sql string, args ...any) (metrics.MetricData, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err == nil {
		return pgx.CollectRows(rows, pgx.RowToMap)
	}
	return nil, err
}

func DBExecReadByDbUniqueName(ctx context.Context, dbUnique string, sql string, args ...any) (metrics.MetricData, error) {
	var conn db.PgxIface
	var md MonitoredDatabase
	var data metrics.MetricData
	var err error
	var tx pgx.Tx
	var exists bool
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL")
	}
	md, err = GetMonitoredDatabaseByUniqueName(dbUnique)
	if err != nil {
		return nil, err
	}
	monitoredDbConnCacheLock.RLock()
	// sqlx.DB itself is parallel safe
	conn, exists = monitoredDbConnCache[dbUnique]
	monitoredDbConnCacheLock.RUnlock()
	if !exists || conn == nil {
		logger.Errorf("SQL connection for dbUnique %s not found or nil", dbUnique) // Should always be initialized in the main loop DB discovery code ...
		return nil, errors.New("SQL connection not found or nil")
	}
	tx, err = conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Commit(ctx) }()
	if IsPostgresDBType(md.DBType) {
		_, err = tx.Exec(ctx, "SET LOCAL lock_timeout TO '100ms'")
		if err != nil {
			atomic.AddUint64(&totalMetricFetchFailuresCounter, 1)
			return nil, err
		}
	}
	if data, err = DBExecRead(ctx, tx, sql, args...); err != nil {
		atomic.AddUint64(&totalMetricFetchFailuresCounter, 1)
	}
	return data, err
}

func GetAllActiveHostsFromConfigDB() (metrics.MetricData, error) {
	sqlLatest := `
		select /* pgwatch3_generated */
		  md_name, md_group, md_dbtype, md_connstr,
		  coalesce(p.pc_config, md_config)::text as md_config, coalesce(s.pc_config, md_config_standby, '{}'::jsonb)::text as md_config_standby,
		  md_is_superuser,
		  coalesce(md_include_pattern, '') as md_include_pattern, coalesce(md_exclude_pattern, '') as md_exclude_pattern,
		  coalesce(md_custom_tags::text, '{}') as md_custom_tags, 
		  md_encryption, coalesce(md_host_config, '{}')::text as md_host_config, md_only_if_master
		from
		  pgwatch3.monitored_db
	          left join
		  pgwatch3.preset_config p on p.pc_name = md_preset_config_name /* primary preset if any */
	          left join
		  pgwatch3.preset_config s on s.pc_name = md_preset_config_name_standby /* standby preset if any */
		where
		  md_is_enabled
	`
	return DBExecRead(mainContext, configDb, sqlLatest)
}

func DBGetSizeMB(dbUnique string) (int64, error) {
	sqlDbSize := `select /* pgwatch3_generated */ pg_database_size(current_database());`
	var sizeMB int64

	lastDBSizeCheckLock.RLock()
	lastDBSizeCheckTime := lastDBSizeFetchTime[dbUnique]
	lastDBSize, ok := lastDBSizeMB[dbUnique]
	lastDBSizeCheckLock.RUnlock()

	if !ok || lastDBSizeCheckTime.Add(dbSizeCachingInterval).Before(time.Now()) {
		ver, err := DBGetPGVersion(mainContext, dbUnique, config.DbTypePg, false)
		if err != nil || (ver.ExecEnv != execEnvAzureSingle) || (ver.ExecEnv == execEnvAzureSingle && ver.ApproxDBSizeB < 1e12) {
			logger.Debugf("[%s] determining DB size ...", dbUnique)

			data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sqlDbSize) // can take some time on ancient FS, use 300s stmt timeout
			if err != nil {
				logger.Errorf("[%s] failed to determine DB size...cannot apply --min-db-size-mb flag. err: %v ...", dbUnique, err)
				return 0, err
			}
			sizeMB = data[0]["pg_database_size"].(int64) / 1048576
		} else {
			logger.Debugf("[%s] Using approx DB size for the --min-db-size-mb filter ...", dbUnique)
			sizeMB = ver.ApproxDBSizeB / 1048576
		}

		logger.Debugf("[%s] DB size = %d MB, caching for %v ...", dbUnique, sizeMB, dbSizeCachingInterval)

		lastDBSizeCheckLock.Lock()
		lastDBSizeFetchTime[dbUnique] = time.Now()
		lastDBSizeMB[dbUnique] = sizeMB
		lastDBSizeCheckLock.Unlock()

		return sizeMB, nil

	}
	logger.Debugf("[%s] using cached DBsize %d MB for the --min-db-size-mb filter check", dbUnique, lastDBSize)
	return lastDBSize, nil
}

func TryDiscoverExecutionEnv(dbUnique string) string {
	sqlPGExecEnv := `select /* pgwatch3_generated */
	case
	  when exists (select * from pg_settings where name = 'pg_qs.host_database' and setting = 'azure_sys') and version() ~* 'compiled by Visual C' then 'AZURE_SINGLE'
	  when exists (select * from pg_settings where name = 'pg_qs.host_database' and setting = 'azure_sys') and version() ~* 'compiled by gcc' then 'AZURE_FLEXIBLE'
	  when exists (select * from pg_settings where name = 'cloudsql.supported_extensions') then 'GOOGLE'
	else
	  'UNKNOWN'
	end as exec_env;
  `
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sqlPGExecEnv)
	if err != nil {
		return ""
	}
	return data[0]["exec_env"].(string)
}

func GetDBTotalApproxSize(dbUnique string) (int64, error) {
	sqlApproxDBSize := `
	select /* pgwatch3_generated */
		current_setting('block_size')::int8 * sum(relpages) as db_size_approx
	from
		pg_class c
	where	/* works only for v9.1+*/
		c.relpersistence != 't';
	`
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sqlApproxDBSize)
	if err != nil {
		return 0, err
	}
	return data[0]["db_size_approx"].(int64), nil
}

func DBGetPGVersion(ctx context.Context, dbUnique string, dbType string, noCache bool) (DBVersionMapEntry, error) {
	var ver DBVersionMapEntry
	var verNew DBVersionMapEntry
	var ok bool
	sql := `
		select /* pgwatch3_generated */ (regexp_matches(
			regexp_replace(current_setting('server_version'), '(beta|devel).*', '', 'g'),
			E'\\d+\\.?\\d+?')
			)[1]::text as ver, pg_is_in_recovery(), current_database()::text;
	`
	sqlSysid := `select /* pgwatch3_generated */ system_identifier::text from pg_control_system();`
	sqlSu := `select /* pgwatch3_generated */ rolsuper
			   from pg_roles r where rolname = session_user;`
	sqlExtensions := `select /* pgwatch3_generated */ extname::text, (regexp_matches(extversion, $$\d+\.?\d+?$$))[1]::text as extversion from pg_extension order by 1;`
	pgpoolVersion := `SHOW POOL_VERSION` // supported from pgpool2 v3.0

	dbPgVersionMapLock.Lock()
	getVerLock, ok := dbGetPgVersionMapLock[dbUnique]
	if !ok {
		dbGetPgVersionMapLock[dbUnique] = &sync.RWMutex{}
		getVerLock = dbGetPgVersionMapLock[dbUnique]
	}
	ver, ok = dbPgVersionMap[dbUnique]
	dbPgVersionMapLock.Unlock()

	if !noCache && ok && ver.LastCheckedOn.After(time.Now().Add(time.Minute*-2)) { // use cached version for 2 min
		//log.Debugf("using cached postgres version %s for %s", ver.Version.String(), dbUnique)
		return ver, nil
	}
	getVerLock.Lock() // limit to 1 concurrent version info fetch per DB
	defer getVerLock.Unlock()
	logger.WithField("database", dbUnique).
		WithField("type", dbType).Debug("determining DB version and recovery status...")

	if verNew.Extensions == nil {
		verNew.Extensions = make(map[string]uint)
	}

	if dbType == config.DbTypeBouncer {
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, "show version")
		if err != nil {
			return verNew, err
		}
		if len(data) == 0 {
			// surprisingly pgbouncer 'show version' outputs in pre v1.12 is emitted as 'NOTICE' which cannot be accessed from Go lib/pg
			verNew.Version = 0
			verNew.VersionStr = "0"
		} else {
			matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(data[0]["version"].(string))
			if len(matches) != 1 {
				logger.Errorf("[%s] Unexpected PgBouncer version input: %s", dbUnique, data[0]["version"].(string))
				return ver, fmt.Errorf("Unexpected PgBouncer version input: %s", data[0]["version"].(string))
			}
			verNew.VersionStr = matches[0]
			verNew.Version = VersionToInt(matches[0])
		}
	} else if dbType == config.DbTypePgPOOL {
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, pgpoolVersion)
		if err != nil {
			return verNew, err
		}
		if len(data) == 0 {
			verNew.Version = VersionToInt("3.0")
			verNew.VersionStr = "3.0"
		} else {
			matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(string(data[0]["pool_version"].([]byte)))
			if len(matches) != 1 {
				logger.Errorf("[%s] Unexpected PgPool version input: %s", dbUnique, data[0]["pool_version"].([]byte))
				return ver, fmt.Errorf("Unexpected PgPool version input: %s", data[0]["pool_version"].([]byte))
			}
			verNew.VersionStr = matches[0]
			verNew.Version = VersionToInt(matches[0])
		}
	} else {
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sql)
		if err != nil {
			if noCache {
				return ver, err
			}
			logger.Infof("[%s] DBGetPGVersion failed, using old cached value. err: %v", dbUnique, err)
			return ver, nil

		}
		verNew.Version = VersionToInt(data[0]["ver"].(string))
		verNew.VersionStr = data[0]["ver"].(string)
		verNew.IsInRecovery = data[0]["pg_is_in_recovery"].(bool)
		verNew.RealDbname = data[0]["current_database"].(string)

		if verNew.Version > VersionToInt("10.0") && opts.Metric.SystemIdentifierField > "" {
			logger.Debugf("[%s] determining system identifier version (pg ver: %v)", dbUnique, verNew.VersionStr)
			data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlSysid)
			if err == nil && len(data) > 0 {
				verNew.SystemIdentifier = data[0]["system_identifier"].(string)
			}
		}

		if ver.ExecEnv != "" {
			verNew.ExecEnv = ver.ExecEnv // carry over as not likely to change ever
		} else {
			logger.Debugf("[%s] determining the execution env...", dbUnique)
			execEnv := TryDiscoverExecutionEnv(dbUnique)
			if execEnv != "" {
				logger.Debugf("[%s] running on execution env: %s", dbUnique, execEnv)
				verNew.ExecEnv = execEnv
			}
		}

		// to work around poor Azure Single Server FS functions performance for some metrics + the --min-db-size-mb filter
		if verNew.ExecEnv == execEnvAzureSingle {
			approxSize, err := GetDBTotalApproxSize(dbUnique)
			if err == nil {
				verNew.ApproxDBSizeB = approxSize
			} else {
				verNew.ApproxDBSizeB = ver.ApproxDBSizeB
			}
		}

		logger.Debugf("[%s] determining if monitoring user is a superuser...", dbUnique)
		data, err = DBExecReadByDbUniqueName(ctx, dbUnique, sqlSu)
		if err == nil {
			verNew.IsSuperuser = data[0]["rolsuper"].(bool)
		}
		logger.Debugf("[%s] superuser=%v", dbUnique, verNew.IsSuperuser)

		if verNew.Version >= MinExtensionInfoAvailable {
			//log.Debugf("[%s] determining installed extensions info...", dbUnique)
			data, err = DBExecReadByDbUniqueName(mainContext, dbUnique, sqlExtensions)
			if err != nil {
				logger.Errorf("[%s] failed to determine installed extensions info: %v", dbUnique, err)
			} else {
				for _, dr := range data {
					extver := VersionToInt(dr["extversion"].(string))
					if extver == 0 {
						logger.Error("[%s] failed to determine extension version info for extension %s: %v", dbUnique, dr["extname"])
						continue
					}
					verNew.Extensions[dr["extname"].(string)] = extver
				}
				logger.Debugf("[%s] installed extensions: %+v", dbUnique, verNew.Extensions)
			}
		}
	}

	verNew.LastCheckedOn = time.Now()
	dbPgVersionMapLock.Lock()
	dbPgVersionMap[dbUnique] = verNew
	dbPgVersionMapLock.Unlock()

	return verNew, nil
}

func DetectSprocChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []metrics.MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.MetricData, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	logger.Debugf("[%s][%s] checking for sproc changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["sproc_hashes"]; !ok {
		firstRun = true
		hostState["sproc_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("sproc_hashes", vme, nil)
	if err != nil {
		logger.Error("could not get sproc_hashes sql:", err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
	if err != nil {
		logger.Error("could not read sproc_hashes from monitored host: ", dbUnique, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_sproc"].(string) + dbMetricJoinStr + dr["tag_oid"].(string)
		prevHash, ok := hostState["sproc_hashes"][objIdent]
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				logger.Info("detected change in sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["sproc_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				logger.Info("detected new sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["sproc_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(hostState["sproc_hashes"]) != len(data) {
		deletedSProcs := make([]string, 0)
		// turn resultset to map => [oid]=true for faster checks
		currentOidMap := make(map[string]bool)
		for _, dr := range data {
			currentOidMap[dr["tag_sproc"].(string)+dbMetricJoinStr+dr["tag_oid"].(string)] = true
		}
		for sprocIdent := range hostState["sproc_hashes"] {
			_, ok := currentOidMap[sprocIdent]
			if !ok {
				splits := strings.Split(sprocIdent, dbMetricJoinStr)
				logger.Info("detected delete of sproc:", splits[0], ", oid:", splits[1])
				influxEntry := make(metrics.MetricEntry)
				influxEntry["event"] = "drop"
				influxEntry["tag_sproc"] = splits[0]
				influxEntry["tag_oid"] = splits[1]
				if len(data) > 0 {
					influxEntry["epoch_ns"] = data[0]["epoch_ns"]
				} else {
					influxEntry["epoch_ns"] = time.Now().UnixNano()
				}
				detectedChanges = append(detectedChanges, influxEntry)
				deletedSProcs = append(deletedSProcs, sprocIdent)
				changeCounts.Dropped++
			}
		}
		for _, deletedSProc := range deletedSProcs {
			delete(hostState["sproc_hashes"], deletedSProc)
		}
	}
	logger.Debugf("[%s][%s] detected %d sproc changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MetricStoreMessage{{DBName: dbUnique, MetricName: "sproc_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	}

	return changeCounts
}

func DetectTableChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []metrics.MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.MetricData, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	logger.Debugf("[%s][%s] checking for table changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["table_hashes"]; !ok {
		firstRun = true
		hostState["table_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("table_hashes", vme, nil)
	if err != nil {
		logger.Error("could not get table_hashes sql:", err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
	if err != nil {
		logger.Error("could not read table_hashes from monitored host:", dbUnique, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_table"].(string)
		prevHash, ok := hostState["table_hashes"][objIdent]
		//log.Debug("inspecting table:", objIdent, "hash:", prev_hash)
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				logger.Info("detected DDL change in table:", dr["tag_table"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["table_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				logger.Info("detected new table:", dr["tag_table"])
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["table_hashes"][objIdent] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(hostState["table_hashes"]) != len(data) {
		deletedTables := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentTableMap := make(map[string]bool)
		for _, dr := range data {
			currentTableMap[dr["tag_table"].(string)] = true
		}
		for table := range hostState["table_hashes"] {
			_, ok := currentTableMap[table]
			if !ok {
				logger.Info("detected drop of table:", table)
				influxEntry := make(metrics.MetricEntry)
				influxEntry["event"] = "drop"
				influxEntry["tag_table"] = table
				if len(data) > 0 {
					influxEntry["epoch_ns"] = data[0]["epoch_ns"]
				} else {
					influxEntry["epoch_ns"] = time.Now().UnixNano()
				}
				detectedChanges = append(detectedChanges, influxEntry)
				deletedTables = append(deletedTables, table)
				changeCounts.Dropped++
			}
		}
		for _, deletedTable := range deletedTables {
			delete(hostState["table_hashes"], deletedTable)
		}
	}

	logger.Debugf("[%s][%s] detected %d table changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MetricStoreMessage{{DBName: dbUnique, MetricName: "table_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	}

	return changeCounts
}

func DetectIndexChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []metrics.MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.MetricData, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	logger.Debugf("[%s][%s] checking for index changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["index_hashes"]; !ok {
		firstRun = true
		hostState["index_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("index_hashes", vme, nil)
	if err != nil {
		logger.Error("could not get index_hashes sql:", err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
	if err != nil {
		logger.Error("could not read index_hashes from monitored host:", dbUnique, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_index"].(string)
		prevHash, ok := hostState["index_hashes"][objIdent]
		if ok { // we have existing state
			if prevHash != (dr["md5"].(string) + dr["is_valid"].(string)) {
				logger.Info("detected index change:", dr["tag_index"], ", table:", dr["table"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				logger.Info("detected new index:", dr["tag_index"])
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
		}
	}
	// detect deletes
	if !firstRun && len(hostState["index_hashes"]) != len(data) {
		deletedIndexes := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		currentIndexMap := make(map[string]bool)
		for _, dr := range data {
			currentIndexMap[dr["tag_index"].(string)] = true
		}
		for indexName := range hostState["index_hashes"] {
			_, ok := currentIndexMap[indexName]
			if !ok {
				logger.Info("detected drop of index_name:", indexName)
				influxEntry := make(metrics.MetricEntry)
				influxEntry["event"] = "drop"
				influxEntry["tag_index"] = indexName
				if len(data) > 0 {
					influxEntry["epoch_ns"] = data[0]["epoch_ns"]
				} else {
					influxEntry["epoch_ns"] = time.Now().UnixNano()
				}
				detectedChanges = append(detectedChanges, influxEntry)
				deletedIndexes = append(deletedIndexes, indexName)
				changeCounts.Dropped++
			}
		}
		for _, deletedIndex := range deletedIndexes {
			delete(hostState["index_hashes"], deletedIndex)
		}
	}
	logger.Debugf("[%s][%s] detected %d index changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MetricStoreMessage{{DBName: dbUnique, MetricName: "index_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	}

	return changeCounts
}

func DetectPrivilegeChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []metrics.MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.MetricData, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	logger.Debugf("[%s][%s] checking object privilege changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["object_privileges"]; !ok {
		firstRun = true
		hostState["object_privileges"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("privilege_changes", vme, nil)
	if err != nil || mvp.SQL == "" {
		logger.Warningf("[%s][%s] could not get SQL for 'privilege_changes'. cannot detect privilege changes", dbUnique, specialMetricChangeEvents)
		return changeCounts
	}

	// returns rows of: object_type, tag_role, tag_object, privilege_type
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
	if err != nil {
		logger.Errorf("[%s][%s] failed to fetch object privileges info: %v", dbUnique, specialMetricChangeEvents, err)
		return changeCounts
	}

	currentState := make(map[string]bool)
	for _, dr := range data {
		objIdent := fmt.Sprintf("%s#:#%s#:#%s#:#%s", dr["object_type"], dr["tag_role"], dr["tag_object"], dr["privilege_type"])
		if firstRun {
			hostState["object_privileges"][objIdent] = ""
		} else {
			_, ok := hostState["object_privileges"][objIdent]
			if !ok {
				logger.Infof("[%s][%s] detected new object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					dbUnique, specialMetricChangeEvents, dr["tag_role"], dr["object_type"], dr["tag_object"], dr["privilege_type"])
				dr["event"] = "GRANT"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
				hostState["object_privileges"][objIdent] = ""
			}
			currentState[objIdent] = true
		}
	}
	// check revokes - exists in old state only
	if !firstRun && len(currentState) > 0 {
		for objPrevRun := range hostState["object_privileges"] {
			if _, ok := currentState[objPrevRun]; !ok {
				splits := strings.Split(objPrevRun, "#:#")
				logger.Infof("[%s][%s] detected removed object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					dbUnique, specialMetricChangeEvents, splits[1], splits[0], splits[2], splits[3])
				revokeEntry := make(metrics.MetricEntry)
				if epochNs, ok := data[0]["epoch_ns"]; ok {
					revokeEntry["epoch_ns"] = epochNs
				} else {
					revokeEntry["epoch_ns"] = time.Now().UnixNano()
				}
				revokeEntry["object_type"] = splits[0]
				revokeEntry["tag_role"] = splits[1]
				revokeEntry["tag_object"] = splits[2]
				revokeEntry["privilege_type"] = splits[3]
				revokeEntry["event"] = "REVOKE"
				detectedChanges = append(detectedChanges, revokeEntry)
				changeCounts.Dropped++
				delete(hostState["object_privileges"], objPrevRun)
			}
		}
	}

	logger.Debugf("[%s][%s] detected %d object privilege changes...", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MetricStoreMessage{
			{
				DBName:     dbUnique,
				MetricName: "privilege_changes",
				Data:       detectedChanges,
				CustomTags: md.CustomTags,
			}}
	}

	return changeCounts
}

func DetectConfigurationChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []metrics.MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.MetricData, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	logger.Debugf("[%s][%s] checking for configuration changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["configuration_hashes"]; !ok {
		firstRun = true
		hostState["configuration_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("configuration_hashes", vme, nil)
	if err != nil {
		logger.Errorf("[%s][%s] could not get configuration_hashes sql: %v", dbUnique, specialMetricChangeEvents, err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
	if err != nil {
		logger.Errorf("[%s][%s] could not read configuration_hashes from monitored host: %v", dbUnique, specialMetricChangeEvents, err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_setting"].(string)
		objValue := dr["value"].(string)
		prevРash, ok := hostState["configuration_hashes"][objIdent]
		if ok { // we have existing state
			if prevРash != objValue {
				if objIdent == "connection_ID" {
					continue // ignore some weird Azure managed PG service setting
				}
				logger.Warningf("[%s][%s] detected settings change: %s = %s (prev: %s)",
					dbUnique, specialMetricChangeEvents, objIdent, objValue, prevРash)
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["configuration_hashes"][objIdent] = objValue
				changeCounts.Altered++
			}
		} else { // check for new, delete not relevant here (pg_upgrade)
			if !firstRun {
				logger.Warningf("[%s][%s] detected new setting: %s", dbUnique, specialMetricChangeEvents, objIdent)
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["configuration_hashes"][objIdent] = objValue
		}
	}

	logger.Debugf("[%s][%s] detected %d configuration changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MetricStoreMessage{{
			DBName:     dbUnique,
			MetricName: "configuration_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}}
	}

	return changeCounts
}

func CheckForPGObjectChangesAndStore(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []metrics.MetricStoreMessage, hostState map[string]map[string]string) {
	sprocСounts := DetectSprocChanges(dbUnique, vme, storageCh, hostState) // TODO some of Detect*() code could be unified...
	tableСounts := DetectTableChanges(dbUnique, vme, storageCh, hostState)
	indexСounts := DetectIndexChanges(dbUnique, vme, storageCh, hostState)
	confСounts := DetectConfigurationChanges(dbUnique, vme, storageCh, hostState)
	privСhangeCounts := DetectPrivilegeChanges(dbUnique, vme, storageCh, hostState)

	// need to send info on all object changes as one message as Grafana applies "last wins" for annotations with similar timestamp
	message := ""
	if sprocСounts.Altered > 0 || sprocСounts.Created > 0 || sprocСounts.Dropped > 0 {
		message += fmt.Sprintf(" sprocs %d/%d/%d", sprocСounts.Created, sprocСounts.Altered, sprocСounts.Dropped)
	}
	if tableСounts.Altered > 0 || tableСounts.Created > 0 || tableСounts.Dropped > 0 {
		message += fmt.Sprintf(" tables/views %d/%d/%d", tableСounts.Created, tableСounts.Altered, tableСounts.Dropped)
	}
	if indexСounts.Altered > 0 || indexСounts.Created > 0 || indexСounts.Dropped > 0 {
		message += fmt.Sprintf(" indexes %d/%d/%d", indexСounts.Created, indexСounts.Altered, indexСounts.Dropped)
	}
	if confСounts.Altered > 0 || confСounts.Created > 0 {
		message += fmt.Sprintf(" configuration %d/%d/%d", confСounts.Created, confСounts.Altered, confСounts.Dropped)
	}
	if privСhangeCounts.Dropped > 0 || privСhangeCounts.Created > 0 {
		message += fmt.Sprintf(" privileges %d/%d/%d", privСhangeCounts.Created, privСhangeCounts.Altered, privСhangeCounts.Dropped)
	}

	if message > "" {
		message = "Detected changes for \"" + dbUnique + "\" [Created/Altered/Dropped]:" + message
		logger.Info(message)
		detectedChangesSummary := make(metrics.MetricData, 0)
		influxEntry := make(metrics.MetricEntry)
		influxEntry["details"] = message
		influxEntry["epoch_ns"] = time.Now().UnixNano()
		detectedChangesSummary = append(detectedChangesSummary, influxEntry)
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MetricStoreMessage{{DBName: dbUnique,
			DBType:     md.DBType,
			MetricName: "object_changes",
			Data:       detectedChangesSummary,
			CustomTags: md.CustomTags,
		}}

	}
}

// some extra work needed as pgpool SHOW commands don't specify the return data types for some reason
func FetchMetricsPgpool(msg MetricFetchMessage, _ DBVersionMapEntry, mvp metrics.MetricProperties) (metrics.MetricData, error) {
	var retData = make(metrics.MetricData, 0)
	epochNs := time.Now().UnixNano()

	sqlLines := strings.Split(strings.ToUpper(mvp.SQL), "\n")

	for _, sql := range sqlLines {
		if strings.HasPrefix(sql, "SHOW POOL_NODES") {
			data, err := DBExecReadByDbUniqueName(mainContext, msg.DBUniqueName, sql)
			if err != nil {
				logger.Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				return data, err
			}

			for _, row := range data {
				retRow := make(metrics.MetricEntry)
				retRow[epochColumnName] = epochNs
				for k, v := range row {
					vs := string(v.([]byte))
					// need 1 tag so that Influx would not merge rows
					if k == "node_id" {
						retRow["tag_node_id"] = vs
						continue
					}

					retRow[k] = vs
					if k == "status" { // was changed from numeric to string at some pgpool version so leave the string
						// but also add "status_num" field
						if vs == "up" {
							retRow["status_num"] = 1
						} else if vs == "down" {
							retRow["status_num"] = 0
						} else {
							i, err := strconv.ParseInt(vs, 10, 64)
							if err == nil {
								retRow["status_num"] = i
							}
						}
						continue
					}
					// everything is returned as text, so try to convert all numerics into ints / floats
					if k != "lb_weight" {
						i, err := strconv.ParseInt(vs, 10, 64)
						if err == nil {
							retRow[k] = i
							continue
						}
					}
					f, err := strconv.ParseFloat(vs, 64)
					if err == nil {
						retRow[k] = f
						continue
					}
				}
				retData = append(retData, retRow)
			}
		} else if strings.HasPrefix(sql, "SHOW POOL_PROCESSES") {
			if len(retData) == 0 {
				logger.Warningf("[%s][%s] SHOW POOL_NODES needs to be placed before SHOW POOL_PROCESSES. ignoring SHOW POOL_PROCESSES", msg.DBUniqueName, msg.MetricName)
				continue
			}

			data, err := DBExecReadByDbUniqueName(mainContext, msg.DBUniqueName, sql)
			if err != nil {
				logger.Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				continue
			}

			// summarize processesTotal / processes_active over all rows
			processesTotal := 0
			processesActive := 0
			for _, row := range data {
				processesTotal++
				v, ok := row["database"]
				if !ok {
					logger.Infof("[%s][%s] column 'database' not found from data returned by SHOW POOL_PROCESSES, check pool version / SQL definition", msg.DBUniqueName, msg.MetricName)
					continue
				}
				if len(v.([]byte)) > 0 {
					processesActive++
				}
			}

			for _, retRow := range retData {
				retRow["processes_total"] = processesTotal
				retRow["processes_active"] = processesActive
			}
		}
	}
	return retData, nil
}

func DoesFunctionExists(dbUnique, functionName string) bool {
	logger.Debug("Checking for function existence", dbUnique, functionName)
	sql := fmt.Sprintf("select /* pgwatch3_generated */ 1 from pg_proc join pg_namespace n on pronamespace = n.oid where proname = '%s' and n.nspname = 'public'", functionName)
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sql)
	if err != nil {
		logger.Error("Failed to check for function existence", dbUnique, functionName, err)
		return false
	}
	if len(data) > 0 {
		logger.Debugf("Function %s exists on %s", functionName, dbUnique)
		return true
	}
	return false
}

// Called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
// With newer Postgres version can even succeed if the user is not a real superuser due to some cloud-specific
// whitelisting or "trusted extensions" (a feature from v13). Ignores errors.
func TryCreateMissingExtensions(dbUnique string, extensionNames []string, existingExtensions map[string]uint) []string {
	sqlAvailable := `select name::text from pg_available_extensions`
	extsCreated := make([]string, 0)

	// For security reasons don't allow to execute random strings but check that it's an existing extension
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sqlAvailable)
	if err != nil {
		logger.Infof("[%s] Failed to get a list of available extensions: %v", dbUnique, err)
		return extsCreated
	}

	availableExts := make(map[string]bool)
	for _, row := range data {
		availableExts[row["name"].(string)] = true
	}

	for _, extToCreate := range extensionNames {
		if _, ok := existingExtensions[extToCreate]; ok {
			continue
		}
		_, ok := availableExts[extToCreate]
		if !ok {
			logger.Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", dbUnique, extToCreate)
		} else {
			sqlCreateExt := `create extension ` + extToCreate
			_, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sqlCreateExt)
			if err != nil {
				logger.Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", dbUnique, extToCreate, err)
			}
			extsCreated = append(extsCreated, extToCreate)
		}
	}

	return extsCreated
}

// Called once on daemon startup to try to create "metric fething helper" functions automatically
func TryCreateMetricsFetchingHelpers(dbUnique string) error {
	dbPgVersion, err := DBGetPGVersion(mainContext, dbUnique, config.DbTypePg, false)
	if err != nil {
		logger.Errorf("Failed to fetch pg version for \"%s\": %s", dbUnique, err)
		return err
	}

	if fileBasedMetrics {
		helpers, _, err := metrics.ReadMetricsFromFolder(mainContext, path.Join(opts.Metric.MetricsFolder, metrics.FileBasedMetricHelpersDir))
		if err != nil {
			logger.Errorf("Failed to fetch helpers from \"%s\": %s", path.Join(opts.Metric.MetricsFolder, metrics.FileBasedMetricHelpersDir), err)
			return err
		}
		logger.Debug("%d helper definitions found from \"%s\"...", len(helpers), path.Join(opts.Metric.MetricsFolder, metrics.FileBasedMetricHelpersDir))

		for helperName := range helpers {
			if strings.Contains(helperName, "windows") {
				logger.Infof("Skipping %s rollout. Windows helpers need to be rolled out manually", helperName)
				continue
			}
			if !DoesFunctionExists(dbUnique, helperName) {

				logger.Debug("Trying to create metric fetching helpers for", dbUnique, helperName)
				mvp, err := GetMetricVersionProperties(helperName, dbPgVersion, helpers)
				if err != nil {
					logger.Warning("Could not find query text for", dbUnique, helperName)
					continue
				}
				_, err = DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
				if err != nil {
					logger.Warning("Failed to create a metric fetching helper for", dbUnique, helperName)
					logger.Warning(err)
				} else {
					logger.Info("Successfully created metric fetching helper for", dbUnique, helperName)
				}
			}
		}

	} else {
		sqlHelpers := "select /* pgwatch3_generated */ distinct m_name from pgwatch3.metric where m_is_active and m_is_helper" // m_name is a helper function name
		data, err := DBExecRead(mainContext, configDb, sqlHelpers)
		if err != nil {
			logger.Error(err)
			return err
		}
		for _, row := range data {
			metric := row["m_name"].(string)

			if strings.Contains(metric, "windows") {
				logger.Infof("Skipping %s rollout. Windows helpers need to be rolled out manually", metric)
				continue
			}
			if !DoesFunctionExists(dbUnique, metric) {

				logger.Debug("Trying to create metric fetching helpers for", dbUnique, metric)
				mvp, err := GetMetricVersionProperties(metric, dbPgVersion, nil)
				if err != nil {
					logger.Warning("Could not find query text for", dbUnique, metric)
					continue
				}
				_, err = DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.SQL)
				if err != nil {
					logger.Warning("Failed to create a metric fetching helper for", dbUnique, metric)
					logger.Warning(err)
				} else {
					logger.Warning("Successfully created metric fetching helper for", dbUnique, metric)
				}
			}
		}
	}
	return nil
}

// "resolving" reads all the DB names from the given host/port, additionally matching/not matching specified regex patterns
func ResolveDatabasesFromConfigEntry(ce MonitoredDatabase) ([]MonitoredDatabase, error) {
	var c db.PgxPoolIface
	var err error
	md := make([]MonitoredDatabase, 0)

	c, err = db.GetPostgresDBConnection(mainContext, ce.ConnStr)
	if err != nil {
		return md, err
	}
	defer c.Close()

	sql := `select /* pgwatch3_generated */ datname::text as datname,
		quote_ident(datname)::text as datname_escaped
		from pg_database
		where not datistemplate
		and datallowconn
		and has_database_privilege (datname, 'CONNECT')
		and case when length(trim($1)) > 0 then datname ~ $2 else true end
		and case when length(trim($3)) > 0 then not datname ~ $4 else true end`

	data, err := DBExecRead(mainContext, c, sql, ce.DBNameIncludePattern, ce.DBNameIncludePattern, ce.DBNameExcludePattern, ce.DBNameExcludePattern)
	if err != nil {
		return md, err
	}

	for _, d := range data {
		md = append(md, MonitoredDatabase{
			DBUniqueName:         ce.DBUniqueName + "_" + d["datname_escaped"].(string),
			DBUniqueNameOrig:     ce.DBUniqueName,
			ConnStr:              ce.ConnStr,
			Encryption:           ce.Encryption,
			Metrics:              ce.Metrics,
			MetricsStandby:       ce.MetricsStandby,
			PresetMetrics:        ce.PresetMetrics,
			PresetMetricsStandby: ce.PresetMetricsStandby,
			IsSuperuser:          ce.IsSuperuser,
			CustomTags:           ce.CustomTags,
			HostConfig:           ce.HostConfig,
			OnlyIfMaster:         ce.OnlyIfMaster,
			DBType:               ce.DBType})
	}

	return md, err
}

// connects actually to the instance to determine PG relevant disk paths / mounts
func GetGoPsutilDiskPG(dbUnique string) (metrics.MetricData, error) {
	sql := `select current_setting('data_directory') as dd, current_setting('log_directory') as ld, current_setting('server_version_num')::int as pgver`
	sqlTS := `select spcname::text as name, pg_catalog.pg_tablespace_location(oid) as location from pg_catalog.pg_tablespace where not spcname like any(array[E'pg\\_%'])`
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sql)
	if err != nil || len(data) == 0 {
		logger.Errorf("Failed to determine relevant PG disk paths via SQL: %v", err)
		return nil, err
	}
	dataTblsp, err := DBExecReadByDbUniqueName(mainContext, dbUnique, sqlTS)
	if err != nil {
		logger.Infof("Failed to determine relevant PG tablespace paths via SQL: %v", err)
	}
	return psutil.GetGoPsutilDiskPG(data, dataTblsp)
}
