package reaper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/db"
	"github.com/cybertec-postgresql/pgwatch/log"
	"github.com/cybertec-postgresql/pgwatch/metrics"
	"github.com/cybertec-postgresql/pgwatch/metrics/psutil"
	"github.com/cybertec-postgresql/pgwatch/sinks"
	"github.com/cybertec-postgresql/pgwatch/sources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// every DB under monitoring should have exactly 1 sql.DB connection assigned, that will internally limit parallel access
func InitSQLConnPoolForMonitoredDBIfNil(ctx context.Context, md *sources.MonitoredDatabase, maxConns int) (err error) {
	conn := md.Conn
	if conn != nil {
		return nil
	}

	md.Conn, err = db.New(ctx, md.ConnStr, func(conf *pgxpool.Config) error {
		conf.MaxConns = int32(maxConns)
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func DBExecRead(ctx context.Context, conn db.PgxIface, sql string, args ...any) (metrics.Measurements, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err == nil {
		return pgx.CollectRows(rows, pgx.RowToMap)
	}
	return nil, err
}

func GetConnByUniqueName(dbUnique string) db.PgxIface {
	if md, err := GetMonitoredDatabaseByUniqueName(dbUnique); err == nil {
		return md.Conn
	}
	return nil
}

func DBExecReadByDbUniqueName(ctx context.Context, dbUnique string, sql string, args ...any) (metrics.Measurements, error) {
	var conn db.PgxIface
	var md *sources.MonitoredDatabase
	var err error
	var tx pgx.Tx
	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL")
	}
	if md, err = GetMonitoredDatabaseByUniqueName(dbUnique); err != nil {
		return nil, err
	}
	if conn = GetConnByUniqueName(dbUnique); conn == nil {
		log.GetLogger(ctx).Errorf("SQL connection for dbUnique %s not found or nil", dbUnique) // Should always be initialized in the main loop DB discovery code ...
		return nil, errors.New("SQL connection not found or nil")
	}
	if tx, err = conn.Begin(ctx); err != nil {
		return nil, err
	}
	defer func() { _ = tx.Commit(ctx) }()
	if md.IsPostgresSource() {
		_, err = tx.Exec(ctx, "SET LOCAL lock_timeout TO '100ms'")
		if err != nil {
			return nil, err
		}
	}
	return DBExecRead(ctx, tx, sql, args...)
}

const (
	execEnvUnknown       = "UNKNOWN"
	execEnvAzureSingle   = "AZURE_SINGLE"
	execEnvAzureFlexible = "AZURE_FLEXIBLE"
	execEnvGoogle        = "GOOGLE"
)

func DBGetSizeMB(ctx context.Context, dbUnique string) (int64, error) {
	sqlDbSize := `select /* pgwatch3_generated */ pg_database_size(current_database());`
	var sizeMB int64

	lastDBSizeCheckLock.RLock()
	lastDBSizeCheckTime := lastDBSizeFetchTime[dbUnique]
	lastDBSize, ok := lastDBSizeMB[dbUnique]
	lastDBSizeCheckLock.RUnlock()

	if !ok || lastDBSizeCheckTime.Add(dbSizeCachingInterval).Before(time.Now()) {
		ver, err := GetMonitoredDatabaseSettings(ctx, dbUnique, sources.SourcePostgres, false)
		if err != nil || (ver.ExecEnv != execEnvAzureSingle) || (ver.ExecEnv == execEnvAzureSingle && ver.ApproxDBSizeB < 1e12) {
			log.GetLogger(ctx).Debugf("[%s] determining DB size ...", dbUnique)

			data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlDbSize) // can take some time on ancient FS, use 300s stmt timeout
			if err != nil {
				log.GetLogger(ctx).Errorf("[%s] failed to determine DB size...cannot apply --min-db-size-mb flag. err: %v ...", dbUnique, err)
				return 0, err
			}
			sizeMB = data[0]["pg_database_size"].(int64) / 1048576
		} else {
			log.GetLogger(ctx).Debugf("[%s] Using approx DB size for the --min-db-size-mb filter ...", dbUnique)
			sizeMB = ver.ApproxDBSizeB / 1048576
		}

		log.GetLogger(ctx).Debugf("[%s] DB size = %d MB, caching for %v ...", dbUnique, sizeMB, dbSizeCachingInterval)

		lastDBSizeCheckLock.Lock()
		lastDBSizeFetchTime[dbUnique] = time.Now()
		lastDBSizeMB[dbUnique] = sizeMB
		lastDBSizeCheckLock.Unlock()

		return sizeMB, nil

	}
	log.GetLogger(ctx).Debugf("[%s] using cached DBsize %d MB for the --min-db-size-mb filter check", dbUnique, lastDBSize)
	return lastDBSize, nil
}

func TryDiscoverExecutionEnv(ctx context.Context, dbUnique string) (execEnv string) {
	sql := `select /* pgwatch3_generated */
	case
	  when exists (select * from pg_settings where name = 'pg_qs.host_database' and setting = 'azure_sys') and version() ~* 'compiled by Visual C' then 'AZURE_SINGLE'
	  when exists (select * from pg_settings where name = 'pg_qs.host_database' and setting = 'azure_sys') and version() ~* 'compiled by gcc' then 'AZURE_FLEXIBLE'
	  when exists (select * from pg_settings where name = 'cloudsql.supported_extensions') then 'GOOGLE'
	else
	  'UNKNOWN'
	end as exec_env`
	_ = GetConnByUniqueName(dbUnique).QueryRow(ctx, sql).Scan(&execEnv)
	return
}

func GetDBTotalApproxSize(ctx context.Context, dbUnique string) (int64, error) {
	sqlApproxDBSize := `
	select /* pgwatch3_generated */
		current_setting('block_size')::int8 * sum(relpages) as db_size_approx
	from
		pg_class c
	where	/* works only for v9.1+*/
		c.relpersistence != 't';
	`
	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlApproxDBSize)
	if err != nil {
		return 0, err
	}
	return data[0]["db_size_approx"].(int64), nil
}

// VersionToInt parses a given version and returns an integer  or
// an error if unable to parse the version. Only parses valid semantic versions.
// Performs checking that can find errors within the version.
// Examples: v1.2 -> 01_02_00, v9.6.3 -> 09_06_03, v11 -> 11_00_00
var regVer = regexp.MustCompile(`(\d+).?(\d*).?(\d*)`)

func VersionToInt(version string) (v int) {
	if matches := regVer.FindStringSubmatch(version); len(matches) > 1 {
		for i, match := range matches[1:] {
			v += func() (m int) { m, _ = strconv.Atoi(match); return }() * int(math.Pow10(4-i*2))
		}
	}
	return
}

var rBouncerAndPgpoolVerMatch = regexp.MustCompile(`\d+\.+\d+`) // extract $major.minor from "4.1.2 (karasukiboshi)" or "PgBouncer 1.12.0"

func GetMonitoredDatabaseSettings(ctx context.Context, dbUnique string, srcType sources.Kind, noCache bool) (MonitoredDatabaseSettings, error) {
	var dbSettings MonitoredDatabaseSettings
	var dbNewSettings MonitoredDatabaseSettings
	var ok bool

	l := log.GetLogger(ctx).WithField("source", dbUnique).WithField("kind", srcType)

	sqlExtensions := `select /* pgwatch3_generated */ extname::text, (regexp_matches(extversion, $$\d+\.?\d+?$$))[1]::text as extversion from pg_extension order by 1;`

	MonitoredDatabasesSettingsLock.Lock()
	getVerLock, ok := MonitoredDatabasesSettingsGetLock[dbUnique]
	if !ok {
		MonitoredDatabasesSettingsGetLock[dbUnique] = &sync.RWMutex{}
		getVerLock = MonitoredDatabasesSettingsGetLock[dbUnique]
	}
	dbSettings, ok = MonitoredDatabasesSettings[dbUnique]
	MonitoredDatabasesSettingsLock.Unlock()

	if !noCache && ok && dbSettings.LastCheckedOn.After(time.Now().Add(time.Minute*-2)) { // use cached version for 2 min
		//log.Debugf("using cached postgres version %s for %s", ver.Version.String(), dbUnique)
		return dbSettings, nil
	}
	getVerLock.Lock() // limit to 1 concurrent version info fetch per DB
	defer getVerLock.Unlock()
	l.Debug("determining DB version and recovery status...")

	if dbNewSettings.Extensions == nil {
		dbNewSettings.Extensions = make(map[string]int)
	}

	switch srcType {
	case sources.SourcePgBouncer:
		if err := GetConnByUniqueName(dbUnique).QueryRow(ctx, "SHOW VERSION").Scan(&dbNewSettings.VersionStr); err != nil {
			return dbNewSettings, err
		}
		matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(dbNewSettings.VersionStr)
		if len(matches) != 1 {
			return dbSettings, fmt.Errorf("Unexpected PgBouncer version input: %s", dbNewSettings.VersionStr)
		}
		dbNewSettings.Version = VersionToInt(matches[0])
	case sources.SourcePgPool:
		if err := GetConnByUniqueName(dbUnique).QueryRow(ctx, "SHOW POOL_VERSION").Scan(&dbNewSettings.VersionStr); err != nil {
			return dbNewSettings, err
		}

		matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(dbNewSettings.VersionStr)
		if len(matches) != 1 {
			return dbSettings, fmt.Errorf("Unexpected PgPool version input: %s", dbNewSettings.VersionStr)
		}
		dbNewSettings.Version = VersionToInt(matches[0])
	default:
		sql := `select /* pgwatch3_generated */ 
	current_setting('server_version_num')::int / 10000 as ver, 
	version(), 
	pg_is_in_recovery(), 
	current_database()::TEXT,
	system_identifier
FROM
	pg_control_system()`

		err := GetConnByUniqueName(dbUnique).QueryRow(ctx, sql).
			Scan(&dbNewSettings.Version, &dbNewSettings.VersionStr,
				&dbNewSettings.IsInRecovery, &dbNewSettings.RealDbname,
				&dbNewSettings.SystemIdentifier)
		if err != nil {
			if noCache {
				return dbSettings, err
			}
			l.Error("DBGetPGVersion failed, using old cached value: ", err)
			return dbSettings, nil
		}

		if dbSettings.ExecEnv != "" {
			dbNewSettings.ExecEnv = dbSettings.ExecEnv // carry over as not likely to change ever
		} else {
			l.Debugf("determining the execution env...")
			dbNewSettings.ExecEnv = TryDiscoverExecutionEnv(ctx, dbUnique)
		}

		// to work around poor Azure Single Server FS functions performance for some metrics + the --min-db-size-mb filter
		if dbNewSettings.ExecEnv == execEnvAzureSingle {
			if approxSize, err := GetDBTotalApproxSize(ctx, dbUnique); err == nil {
				dbNewSettings.ApproxDBSizeB = approxSize
			} else {
				dbNewSettings.ApproxDBSizeB = dbSettings.ApproxDBSizeB
			}
		}

		l.Debugf("[%s] determining if monitoring user is a superuser...", dbUnique)
		sqlSu := `select /* pgwatch3_generated */ rolsuper from pg_roles r where rolname = session_user`

		if err = GetConnByUniqueName(dbUnique).QueryRow(ctx, sqlSu).Scan(&dbNewSettings.IsSuperuser); err != nil {
			l.Errorf("[%s] failed to determine if monitoring user is a superuser: %v", dbUnique, err)
		}

		l.Debugf("[%s] determining installed extensions info...", dbUnique)
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlExtensions)
		if err != nil {
			l.Errorf("[%s] failed to determine installed extensions info: %v", dbUnique, err)
		} else {
			for _, dr := range data {
				extver := VersionToInt(dr["extversion"].(string))
				if extver == 0 {
					l.Error("failed to determine extension version info for extension: ", dr["extname"])
					continue
				}
				dbNewSettings.Extensions[dr["extname"].(string)] = extver
			}
			l.Debugf("[%s] installed extensions: %+v", dbUnique, dbNewSettings.Extensions)
		}

	}

	dbNewSettings.LastCheckedOn = time.Now()
	MonitoredDatabasesSettingsLock.Lock()
	MonitoredDatabasesSettings[dbUnique] = dbNewSettings
	MonitoredDatabasesSettingsLock.Unlock()

	return dbNewSettings, nil
}

func DetectSprocChanges(ctx context.Context, dbUnique string, vme MonitoredDatabaseSettings, storageCh chan<- []metrics.MeasurementMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for sproc changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["sproc_hashes"]; !ok {
		firstRun = true
		hostState["sproc_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("sproc_hashes", vme, nil)
	if err != nil {
		log.GetLogger(ctx).Error("could not get sproc_hashes sql:", err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, mvp.GetSQL(int(vme.Version)))
	if err != nil {
		log.GetLogger(ctx).Error("could not read sproc_hashes from monitored host: ", dbUnique, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_sproc"].(string) + dbMetricJoinStr + dr["tag_oid"].(string)
		prevHash, ok := hostState["sproc_hashes"][objIdent]
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				log.GetLogger(ctx).Info("detected change in sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["sproc_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				log.GetLogger(ctx).Info("detected new sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
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
				log.GetLogger(ctx).Info("detected delete of sproc:", splits[0], ", oid:", splits[1])
				influxEntry := make(metrics.Measurement)
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
	log.GetLogger(ctx).Debugf("[%s][%s] detected %d sproc changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MeasurementMessage{{DBName: dbUnique, MetricName: "sproc_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	}

	return changeCounts
}

func DetectTableChanges(ctx context.Context, dbUnique string, vme MonitoredDatabaseSettings, storageCh chan<- []metrics.MeasurementMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for table changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["table_hashes"]; !ok {
		firstRun = true
		hostState["table_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("table_hashes", vme, nil)
	if err != nil {
		log.GetLogger(ctx).Error("could not get table_hashes sql:", err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, mvp.GetSQL(int(vme.Version)))
	if err != nil {
		log.GetLogger(ctx).Error("could not read table_hashes from monitored host:", dbUnique, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_table"].(string)
		prevHash, ok := hostState["table_hashes"][objIdent]
		//log.Debug("inspecting table:", objIdent, "hash:", prev_hash)
		if ok { // we have existing state
			if prevHash != dr["md5"].(string) {
				log.GetLogger(ctx).Info("detected DDL change in table:", dr["tag_table"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["table_hashes"][objIdent] = dr["md5"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				log.GetLogger(ctx).Info("detected new table:", dr["tag_table"])
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
				log.GetLogger(ctx).Info("detected drop of table:", table)
				influxEntry := make(metrics.Measurement)
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

	log.GetLogger(ctx).Debugf("[%s][%s] detected %d table changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MeasurementMessage{{DBName: dbUnique, MetricName: "table_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	}

	return changeCounts
}

func DetectIndexChanges(ctx context.Context, dbUnique string, vme MonitoredDatabaseSettings, storageCh chan<- []metrics.MeasurementMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for index changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["index_hashes"]; !ok {
		firstRun = true
		hostState["index_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("index_hashes", vme, nil)
	if err != nil {
		log.GetLogger(ctx).Error("could not get index_hashes sql:", err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, mvp.GetSQL(int(vme.Version)))
	if err != nil {
		log.GetLogger(ctx).Error("could not read index_hashes from monitored host:", dbUnique, ", err:", err)
		return changeCounts
	}

	for _, dr := range data {
		objIdent := dr["tag_index"].(string)
		prevHash, ok := hostState["index_hashes"][objIdent]
		if ok { // we have existing state
			if prevHash != (dr["md5"].(string) + dr["is_valid"].(string)) {
				log.GetLogger(ctx).Info("detected index change:", dr["tag_index"], ", table:", dr["table"])
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["index_hashes"][objIdent] = dr["md5"].(string) + dr["is_valid"].(string)
				changeCounts.Altered++
			}
		} else { // check for new / delete
			if !firstRun {
				log.GetLogger(ctx).Info("detected new index:", dr["tag_index"])
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
				log.GetLogger(ctx).Info("detected drop of index_name:", indexName)
				influxEntry := make(metrics.Measurement)
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
	log.GetLogger(ctx).Debugf("[%s][%s] detected %d index changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MeasurementMessage{{DBName: dbUnique, MetricName: "index_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	}

	return changeCounts
}

func DetectPrivilegeChanges(ctx context.Context, dbUnique string, vme MonitoredDatabaseSettings, storageCh chan<- []metrics.MeasurementMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking object privilege changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["object_privileges"]; !ok {
		firstRun = true
		hostState["object_privileges"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("privilege_changes", vme, nil)
	if err != nil || mvp.GetSQL(int(vme.Version)) == "" {
		log.GetLogger(ctx).Warningf("[%s][%s] could not get SQL for 'privilege_changes'. cannot detect privilege changes", dbUnique, specialMetricChangeEvents)
		return changeCounts
	}

	// returns rows of: object_type, tag_role, tag_object, privilege_type
	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, mvp.GetSQL(int(vme.Version)))
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s][%s] failed to fetch object privileges info: %v", dbUnique, specialMetricChangeEvents, err)
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
				log.GetLogger(ctx).Infof("[%s][%s] detected new object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
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
				log.GetLogger(ctx).Infof("[%s][%s] detected removed object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					dbUnique, specialMetricChangeEvents, splits[1], splits[0], splits[2], splits[3])
				revokeEntry := make(metrics.Measurement)
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

	log.GetLogger(ctx).Debugf("[%s][%s] detected %d object privilege changes...", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MeasurementMessage{
			{
				DBName:     dbUnique,
				MetricName: "privilege_changes",
				Data:       detectedChanges,
				CustomTags: md.CustomTags,
			}}
	}

	return changeCounts
}

func DetectConfigurationChanges(ctx context.Context, dbUnique string, vme MonitoredDatabaseSettings, storageCh chan<- []metrics.MeasurementMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(metrics.Measurements, 0)
	var firstRun bool
	var changeCounts ChangeDetectionResults

	log.GetLogger(ctx).Debugf("[%s][%s] checking for configuration changes...", dbUnique, specialMetricChangeEvents)
	if _, ok := hostState["configuration_hashes"]; !ok {
		firstRun = true
		hostState["configuration_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("configuration_hashes", vme, nil)
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s][%s] could not get configuration_hashes sql: %v", dbUnique, specialMetricChangeEvents, err)
		return changeCounts
	}

	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, mvp.GetSQL(int(vme.Version)))
	if err != nil {
		log.GetLogger(ctx).Errorf("[%s][%s] could not read configuration_hashes from monitored host: %v", dbUnique, specialMetricChangeEvents, err)
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
				log.GetLogger(ctx).Warningf("[%s][%s] detected settings change: %s = %s (prev: %s)",
					dbUnique, specialMetricChangeEvents, objIdent, objValue, prevРash)
				dr["event"] = "alter"
				detectedChanges = append(detectedChanges, dr)
				hostState["configuration_hashes"][objIdent] = objValue
				changeCounts.Altered++
			}
		} else { // check for new, delete not relevant here (pg_upgrade)
			if !firstRun {
				log.GetLogger(ctx).Warningf("[%s][%s] detected new setting: %s", dbUnique, specialMetricChangeEvents, objIdent)
				dr["event"] = "create"
				detectedChanges = append(detectedChanges, dr)
				changeCounts.Created++
			}
			hostState["configuration_hashes"][objIdent] = objValue
		}
	}

	log.GetLogger(ctx).Debugf("[%s][%s] detected %d configuration changes", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MeasurementMessage{{
			DBName:     dbUnique,
			MetricName: "configuration_changes",
			Data:       detectedChanges,
			CustomTags: md.CustomTags,
		}}
	}

	return changeCounts
}

func CheckForPGObjectChangesAndStore(ctx context.Context, dbUnique string, vme MonitoredDatabaseSettings, storageCh chan<- []metrics.MeasurementMessage, hostState map[string]map[string]string) {
	sprocCounts := DetectSprocChanges(ctx, dbUnique, vme, storageCh, hostState) // TODO some of Detect*() code could be unified...
	tableCounts := DetectTableChanges(ctx, dbUnique, vme, storageCh, hostState)
	indexCounts := DetectIndexChanges(ctx, dbUnique, vme, storageCh, hostState)
	confCounts := DetectConfigurationChanges(ctx, dbUnique, vme, storageCh, hostState)
	privChangeCounts := DetectPrivilegeChanges(ctx, dbUnique, vme, storageCh, hostState)

	// need to send info on all object changes as one message as Grafana applies "last wins" for annotations with similar timestamp
	message := ""
	if sprocCounts.Altered > 0 || sprocCounts.Created > 0 || sprocCounts.Dropped > 0 {
		message += fmt.Sprintf(" sprocs %d/%d/%d", sprocCounts.Created, sprocCounts.Altered, sprocCounts.Dropped)
	}
	if tableCounts.Altered > 0 || tableCounts.Created > 0 || tableCounts.Dropped > 0 {
		message += fmt.Sprintf(" tables/views %d/%d/%d", tableCounts.Created, tableCounts.Altered, tableCounts.Dropped)
	}
	if indexCounts.Altered > 0 || indexCounts.Created > 0 || indexCounts.Dropped > 0 {
		message += fmt.Sprintf(" indexes %d/%d/%d", indexCounts.Created, indexCounts.Altered, indexCounts.Dropped)
	}
	if confCounts.Altered > 0 || confCounts.Created > 0 {
		message += fmt.Sprintf(" configuration %d/%d/%d", confCounts.Created, confCounts.Altered, confCounts.Dropped)
	}
	if privChangeCounts.Dropped > 0 || privChangeCounts.Created > 0 {
		message += fmt.Sprintf(" privileges %d/%d/%d", privChangeCounts.Created, privChangeCounts.Altered, privChangeCounts.Dropped)
	}

	if message > "" {
		message = "Detected changes for \"" + dbUnique + "\" [Created/Altered/Dropped]:" + message
		log.GetLogger(ctx).Info(message)
		detectedChangesSummary := make(metrics.Measurements, 0)
		influxEntry := make(metrics.Measurement)
		influxEntry["details"] = message
		influxEntry["epoch_ns"] = time.Now().UnixNano()
		detectedChangesSummary = append(detectedChangesSummary, influxEntry)
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []metrics.MeasurementMessage{{DBName: dbUnique,
			SourceType: string(md.Kind),
			MetricName: "object_changes",
			Data:       detectedChangesSummary,
			CustomTags: md.CustomTags,
		}}

	}
}

// some extra work needed as pgpool SHOW commands don't specify the return data types for some reason
func FetchMetricsPgpool(ctx context.Context, msg MetricFetchConfig, vme MonitoredDatabaseSettings, mvp metrics.Metric) (metrics.Measurements, error) {
	var retData = make(metrics.Measurements, 0)
	epochNs := time.Now().UnixNano()

	sqlLines := strings.Split(strings.ToUpper(mvp.GetSQL(int(vme.Version))), "\n")

	for _, sql := range sqlLines {
		if strings.HasPrefix(sql, "SHOW POOL_NODES") {
			data, err := DBExecReadByDbUniqueName(ctx, msg.DBUniqueName, sql)
			if err != nil {
				log.GetLogger(ctx).Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				return data, err
			}

			for _, row := range data {
				retRow := make(metrics.Measurement)
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
				log.GetLogger(ctx).Warningf("[%s][%s] SHOW POOL_NODES needs to be placed before SHOW POOL_PROCESSES. ignoring SHOW POOL_PROCESSES", msg.DBUniqueName, msg.MetricName)
				continue
			}

			data, err := DBExecReadByDbUniqueName(ctx, msg.DBUniqueName, sql)
			if err != nil {
				log.GetLogger(ctx).Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				continue
			}

			// summarize processesTotal / processes_active over all rows
			processesTotal := 0
			processesActive := 0
			for _, row := range data {
				processesTotal++
				v, ok := row["database"]
				if !ok {
					log.GetLogger(ctx).Infof("[%s][%s] column 'database' not found from data returned by SHOW POOL_PROCESSES, check pool version / SQL definition", msg.DBUniqueName, msg.MetricName)
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

func DoesFunctionExists(ctx context.Context, dbUnique, functionName string) bool {
	log.GetLogger(ctx).Debug("Checking for function existence", dbUnique, functionName)
	sql := fmt.Sprintf("select /* pgwatch3_generated */ 1 from pg_proc join pg_namespace n on pronamespace = n.oid where proname = '%s' and n.nspname = 'public'", functionName)
	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sql)
	if err != nil {
		log.GetLogger(ctx).Error("Failed to check for function existence", dbUnique, functionName, err)
		return false
	}
	if len(data) > 0 {
		log.GetLogger(ctx).Debugf("Function %s exists on %s", functionName, dbUnique)
		return true
	}
	return false
}

// Called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
// With newer Postgres version can even succeed if the user is not a real superuser due to some cloud-specific
// whitelisting or "trusted extensions" (a feature from v13). Ignores errors.
func TryCreateMissingExtensions(ctx context.Context, dbUnique string, extensionNames []string, existingExtensions map[string]int) []string {
	sqlAvailable := `select name::text from pg_available_extensions`
	extsCreated := make([]string, 0)

	// For security reasons don't allow to execute random strings but check that it's an existing extension
	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlAvailable)
	if err != nil {
		log.GetLogger(ctx).Infof("[%s] Failed to get a list of available extensions: %v", dbUnique, err)
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
			log.GetLogger(ctx).Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", dbUnique, extToCreate)
		} else {
			sqlCreateExt := `create extension ` + extToCreate
			_, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlCreateExt)
			if err != nil {
				log.GetLogger(ctx).Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", dbUnique, extToCreate, err)
			}
			extsCreated = append(extsCreated, extToCreate)
		}
	}

	return extsCreated
}

// Called once on daemon startup to try to create "metric fething helper" functions automatically
func TryCreateMetricsFetchingHelpers(ctx context.Context, md *sources.MonitoredDatabase) (err error) {
	metricConfig := func() map[string]float64 {
		if len(md.Metrics) > 0 {
			return md.Metrics
		}
		if md.PresetMetrics > "" {
			return metricDefinitionMap.PresetDefs[md.PresetMetrics].Metrics
		}
		return nil
	}()
	conf, err := pgx.ParseConfig(md.ConnStr)
	if err != nil {
		return err
	}
	conf.DefaultQueryExecMode = pgx.QueryExecModeExec
	c, err := pgx.ConnectConfig(ctx, conf)
	if err != nil {
		return nil
	}
	defer c.Close(ctx)

	for metricName := range metricConfig {
		Metric := metricDefinitionMap.MetricDefs[metricName]
		if Metric.InitSQL == "" {
			continue
		}

		_, err = c.Exec(ctx, Metric.InitSQL)
		if err != nil {
			log.GetLogger(ctx).Warningf("Failed to create a metric fetching helper for %s in %s: %v", md.Name, metricName, err)
		} else {
			log.GetLogger(ctx).Info("Successfully created metric fetching helper for", md.Name, metricName)
		}
	}
	return nil
}

// connects actually to the instance to determine PG relevant disk paths / mounts
func GetGoPsutilDiskPG(ctx context.Context, dbUnique string) (metrics.Measurements, error) {
	sql := `select current_setting('data_directory') as dd, current_setting('log_directory') as ld, current_setting('server_version_num')::int as pgver`
	sqlTS := `select spcname::text as name, pg_catalog.pg_tablespace_location(oid) as location from pg_catalog.pg_tablespace where not spcname like any(array[E'pg\\_%'])`
	data, err := DBExecReadByDbUniqueName(ctx, dbUnique, sql)
	if err != nil || len(data) == 0 {
		log.GetLogger(ctx).Errorf("Failed to determine relevant PG disk paths via SQL: %v", err)
		return nil, err
	}
	dataTblsp, err := DBExecReadByDbUniqueName(ctx, dbUnique, sqlTS)
	if err != nil {
		log.GetLogger(ctx).Infof("Failed to determine relevant PG tablespace paths via SQL: %v", err)
	}
	return psutil.GetGoPsutilDiskPG(data, dataTblsp)
}

func CloseResourcesForRemovedMonitoredDBs(metricsWriter *sinks.MultiWriter, currentDBs, prevLoopDBs sources.MonitoredDatabases, shutDownDueToRoleChange map[string]bool) {
	var curDBsMap = make(map[string]bool)

	for _, curDB := range currentDBs {
		curDBsMap[curDB.Name] = true
	}

	for _, prevDB := range prevLoopDBs {
		if _, ok := curDBsMap[prevDB.Name]; !ok { // removed from config
			prevDB.Conn.Close()
			_ = metricsWriter.SyncMetrics(prevDB.Name, "", "remove")
		}
	}

	// or to be ignored due to current instance state
	for roleChangedDB := range shutDownDueToRoleChange {
		if db := currentDBs.GetMonitoredDatabase(roleChangedDB); db != nil {
			db.Conn.Close()
		}
		_ = metricsWriter.SyncMetrics(roleChangedDB, "", "remove")
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
	recoveryIgnored, ok := recoveryIgnoredDBs[dbUnique]
	if ok {
		return recoveryIgnored
	}
	return false
}

func IsDBDormant(dbUnique string) bool {
	return IsDBUndersized(dbUnique) || IsDBIgnoredBasedOnRecoveryState(dbUnique)
}
