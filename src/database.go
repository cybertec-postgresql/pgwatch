package main

import (
	"context"
	"encoding/json"
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
	"github.com/cybertec-postgresql/pgwatch3/psutil"
	"github.com/jackc/pgx/v5"
)

var configDb db.PgxPoolIface
var metricDb db.PgxPoolIface
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

	if md.DBType == config.DbTypeBouncer {
		md.DBName = "pgbouncer"
	}

	conn, err := db.GetPostgresDBConnection(mainContext, md.LibPQConnStr, md.Host, md.Port, md.DBName, md.User, md.Password,
		md.SslMode, md.SslRootCAPath, md.SslClientCertPath, md.SslClientKeyPath)
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

func DBExecRead(ctx context.Context, conn db.PgxIface, sql string, args ...any) (MetricData, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err == nil {
		return pgx.CollectRows(rows, pgx.RowToMap)
	}
	return nil, err
}

func DBExecReadByDbUniqueName(ctx context.Context, dbUnique string, stmtTimeoutOverride int64, sql string, args ...any) (MetricData, error) {
	var conn db.PgxIface
	var md MonitoredDatabase
	var data MetricData
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
	if !opts.IsAdHocMode() && IsPostgresDBType(md.DBType) {
		stmtTimeout := md.StmtTimeout
		if stmtTimeoutOverride > 0 {
			stmtTimeout = stmtTimeoutOverride
		}
		if stmtTimeout > 0 { // 0 = don't change, use DB level settings
			_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout TO '%ds'", stmtTimeout))
		}
		if err != nil {
			atomic.AddUint64(&totalMetricFetchFailuresCounter, 1)
			return nil, err
		}
	}
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

func GetAllActiveHostsFromConfigDB() (MetricData, error) {
	sqlLatest := `
		select /* pgwatch3_generated */
		  md_unique_name, md_group, md_dbtype, md_hostname, md_port, md_dbname, md_user, coalesce(md_password, '') as md_password,
		  coalesce(p.pc_config, md_config)::text as md_config, coalesce(s.pc_config, md_config_standby, '{}'::jsonb)::text as md_config_standby,
		  md_statement_timeout_seconds, md_sslmode, md_is_superuser,
		  coalesce(md_include_pattern, '') as md_include_pattern, coalesce(md_exclude_pattern, '') as md_exclude_pattern,
		  coalesce(md_custom_tags::text, '{}') as md_custom_tags, md_root_ca_path, md_client_cert_path, md_client_key_path,
		  md_password_type, coalesce(md_host_config, '{}')::text as md_host_config, md_only_if_master
		from
		  pgwatch3.monitored_db
	          left join
		  pgwatch3.preset_config p on p.pc_name = md_preset_config_name /* primary preset if any */
	          left join
		  pgwatch3.preset_config s on s.pc_name = md_preset_config_name_standby /* standby preset if any */
		where
		  md_is_enabled
	`
	sqlPrev := `
		select /* pgwatch3_generated */
		  md_unique_name, md_group, md_dbtype, md_hostname, md_port, md_dbname, md_user, coalesce(md_password, '') as md_password,
		  coalesce(pc_config, md_config)::text as md_config, md_statement_timeout_seconds, md_sslmode, md_is_superuser,
		  coalesce(md_include_pattern, '') as md_include_pattern, coalesce(md_exclude_pattern, '') as md_exclude_pattern,
		  coalesce(md_custom_tags::text, '{}') as md_custom_tags, md_root_ca_path, md_client_cert_path, md_client_key_path,
		  md_password_type, coalesce(md_host_config, '{}')::text as md_host_config, md_only_if_master
		from
		  pgwatch3.monitored_db
	          left join
		  pgwatch3.preset_config on pc_name = md_preset_config_name
		where
		  md_is_enabled
	`
	data, err := DBExecRead(mainContext, configDb, sqlLatest)
	if err != nil {
		err1 := err
		logger.Debugf("Failed to query the monitored DB-s config with latest SQL: %v ", err1)
		data, err = DBExecRead(mainContext, configDb, sqlPrev)
		if err == nil {
			logger.Warning("Fetching monitored DB-s config succeeded with SQL from previous schema version - gatherer update required!")
		} else {
			logger.Errorf("Failed to query the monitored DB-s config: %v", err1) // show the original error
		}
	}
	return data, err
}

func OldPostgresMetricsDeleter(ctx context.Context, metricAgeDaysThreshold int64, schemaType db.MetricSchemaType) {
	sqlDoesOldPartListingFuncExist := `SELECT count(*) FROM information_schema.routines WHERE routine_schema = 'admin' AND routine_name = 'get_old_time_partitions'`
	oldPartListingFuncExists := false // if func existing (>v1.8.1) then use it to drop old partitions in smaller batches
	// as for large setup (50+ DBs) one could reach the default "max_locks_per_transaction" otherwise

	ret, err := DBExecRead(mainContext, metricDb, sqlDoesOldPartListingFuncExist)
	if err == nil && len(ret) > 0 && ret[0]["count"].(int64) > 0 {
		oldPartListingFuncExists = true
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Hour):
		// to reduce distracting log messages at startup
	}

	for {
		if schemaType == db.MetricSchemaTimescale || (!oldPartListingFuncExists && schemaType == db.MetricSchemaPostgres) {
			partsDropped, err := DropOldTimePartitions(metricAgeDaysThreshold)

			if err != nil {
				logger.Errorf("Failed to drop old partitions (>%d days) from Postgres: %v", metricAgeDaysThreshold, err)
				time.Sleep(time.Second * 300)
				continue
			}
			logger.Infof("Dropped %d old metric partitions...", partsDropped)
		} else if oldPartListingFuncExists && schemaType == db.MetricSchemaPostgres {
			partsToDrop, err := GetOldTimePartitions(metricAgeDaysThreshold)
			if err != nil {
				logger.Errorf("Failed to get a listing of old (>%d days) time partitions from Postgres metrics DB - check that the admin.get_old_time_partitions() function is rolled out: %v", metricAgeDaysThreshold, err)
				time.Sleep(time.Second * 300)
				continue
			}
			if len(partsToDrop) > 0 {
				logger.Infof("Dropping %d old metric partitions one by one...", len(partsToDrop))
				for _, toDrop := range partsToDrop {
					sqlDropTable := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, toDrop)
					logger.Debugf("Dropping old metric data partition: %s", toDrop)
					_, err := DBExecRead(mainContext, metricDb, sqlDropTable)
					if err != nil {
						logger.Errorf("Failed to drop old partition %s from Postgres metrics DB: %v", toDrop, err)
						time.Sleep(time.Second * 300)
					} else {
						time.Sleep(time.Second * 5)
					}
				}
			} else {
				logger.Infof("No old metric partitions found to drop...")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour * 12):
			// every 12 hours
		}
	}
}

func DropOldTimePartitions(metricAgeDaysThreshold int64) (int, error) {
	partsDropped := 0
	var err error
	sqlOldPart := `select admin.drop_old_time_partitions($1, $2)`

	ret, err := DBExecRead(mainContext, metricDb, sqlOldPart, metricAgeDaysThreshold, false)
	if err != nil {
		logger.Error("Failed to drop old time partitions from Postgres metricDB:", err)
		return partsDropped, err
	}
	partsDropped = int(ret[0]["drop_old_time_partitions"].(int64))

	return partsDropped, err
}

func GetOldTimePartitions(metricAgeDaysThreshold int64) ([]string, error) {
	partsToDrop := make([]string, 0)
	var err error
	sqlGetOldParts := `select admin.get_old_time_partitions($1)`

	ret, err := DBExecRead(mainContext, metricDb, sqlGetOldParts, metricAgeDaysThreshold)
	if err != nil {
		logger.Error("Failed to get a listing of old time partitions from Postgres metricDB:", err)
		return partsToDrop, err
	}
	for _, row := range ret {
		partsToDrop = append(partsToDrop, row["get_old_time_partitions"].(string))
	}

	return partsToDrop, nil
}

func AddDBUniqueMetricToListingTable(dbUnique, metric string) error {
	sql := `insert into admin.all_distinct_dbname_metrics
			select $1, $2
			where not exists (
				select * from admin.all_distinct_dbname_metrics where dbname = $1 and metric = $2
			)`
	_, err := DBExecRead(mainContext, metricDb, sql, dbUnique, metric)
	return err
}

func UniqueDbnamesListingMaintainer(ctx context.Context, daemonMode bool) {
	// due to metrics deletion the listing can go out of sync (a trigger not really wanted)
	sqlGetAdvisoryLock := `SELECT pg_try_advisory_lock(1571543679778230000) AS have_lock` // 1571543679778230000 is just a random bigint
	sqlTopLevelMetrics := `SELECT table_name FROM admin.get_top_level_metric_tables()`
	sqlDistinct := `
	WITH RECURSIVE t(dbname) AS (
		SELECT MIN(dbname) AS dbname FROM %s
		UNION
		SELECT (SELECT MIN(dbname) FROM %s WHERE dbname > t.dbname) FROM t )
	SELECT dbname FROM t WHERE dbname NOTNULL ORDER BY 1
	`
	sqlDelete := `DELETE FROM admin.all_distinct_dbname_metrics WHERE NOT dbname = ANY($1) and metric = $2 RETURNING *`
	sqlDeleteAll := `DELETE FROM admin.all_distinct_dbname_metrics WHERE metric = $1 RETURNING *`
	sqlAdd := `
		INSERT INTO admin.all_distinct_dbname_metrics SELECT u, $2 FROM (select unnest($1::text[]) as u) x
		WHERE NOT EXISTS (select * from admin.all_distinct_dbname_metrics where dbname = u and metric = $2)
		RETURNING *;
	`

	for {
		if daemonMode {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour * 24):
				// go for it
			}
		}

		logger.Infof("Trying to get metricsDb listing maintaner advisory lock...") // to only have one "maintainer" in case of a "push" setup, as can get costly
		lock, err := DBExecRead(mainContext, metricDb, sqlGetAdvisoryLock)
		if err != nil {
			logger.Error("Getting metricsDb listing maintaner advisory lock failed:", err)
			continue
		}
		if !(lock[0]["have_lock"].(bool)) {
			logger.Info("Skipping admin.all_distinct_dbname_metrics maintenance as another instance has the advisory lock...")
			continue
		}

		logger.Infof("Refreshing admin.all_distinct_dbname_metrics listing table...")
		allDistinctMetricTables, err := DBExecRead(mainContext, metricDb, sqlTopLevelMetrics)
		if err != nil {
			logger.Error("Could not refresh Postgres dbnames listing table:", err)
		} else {
			for _, dr := range allDistinctMetricTables {
				foundDbnamesMap := make(map[string]bool)
				foundDbnamesArr := make([]string, 0)
				metricName := strings.Replace(dr["table_name"].(string), "public.", "", 1)

				logger.Debugf("Refreshing all_distinct_dbname_metrics listing for metric: %s", metricName)
				ret, err := DBExecRead(mainContext, metricDb, fmt.Sprintf(sqlDistinct, dr["table_name"], dr["table_name"]))
				if err != nil {
					logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for '%s': %s", metricName, err)
					break
				}
				for _, drDbname := range ret {
					foundDbnamesMap[drDbname["dbname"].(string)] = true // "set" behaviour, don't want duplicates
				}

				// delete all that are not known and add all that are not there
				for k := range foundDbnamesMap {
					foundDbnamesArr = append(foundDbnamesArr, k)
				}
				if len(foundDbnamesArr) == 0 { // delete all entries for given metric
					logger.Debugf("Deleting Postgres all_distinct_dbname_metrics listing table entries for metric '%s':", metricName)
					_, err = DBExecRead(mainContext, metricDb, sqlDeleteAll, metricName)
					if err != nil {
						logger.Errorf("Could not delete Postgres all_distinct_dbname_metrics listing table entries for metric '%s': %s", metricName, err)
					}
					continue
				}
				ret, err = DBExecRead(mainContext, metricDb, sqlDelete, foundDbnamesArr, metricName)
				if err != nil {
					logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metricName, err)
				} else if len(ret) > 0 {
					logger.Infof("Removed %d stale entries from all_distinct_dbname_metrics listing table for metric: %s", len(ret), metricName)
				}
				ret, err = DBExecRead(mainContext, metricDb, sqlAdd, foundDbnamesArr, metricName)
				if err != nil {
					logger.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metricName, err)
				} else if len(ret) > 0 {
					logger.Infof("Added %d entry to the Postgres all_distinct_dbname_metrics listing table for metric: %s", len(ret), metricName)
				}
				if daemonMode {
					time.Sleep(time.Minute)
				}
			}
		}
		if !daemonMode {
			return
		}
	}
}

func EnsureMetricDummy(metric string) {
	if opts.Metric.Datastore != datastorePostgres {
		return
	}
	sqlEnsure := "select admin.ensure_dummy_metrics_table($1) as created"
	PGDummyMetricTablesLock.Lock()
	defer PGDummyMetricTablesLock.Unlock()
	lastEnsureCall, ok := PGDummyMetricTables[metric]
	if ok && lastEnsureCall.After(time.Now().Add(-1*time.Hour)) {
		return
	}
	if err := metricDb.QueryRow(mainContext, sqlEnsure, metric).Scan(&ok); err != nil {
		logger.Error(err)
		return
	}
	if ok {
		logger.WithField("metric", metric).Info("Created a dummy partition of metric")
		PGDummyMetricTables[metric] = time.Now()
	}
}

func EnsureMetric(pgPartBounds map[string]ExistingPartitionInfo, force bool) error {

	sqlEnsure := `
	select * from admin.ensure_partition_metric($1)
	`
	for metric := range pgPartBounds {

		_, ok := partitionMapMetric[metric] // sequential access currently so no lock needed
		if !ok || force {
			_, err := DBExecRead(mainContext, metricDb, sqlEnsure, metric)
			if err != nil {
				logger.Errorf("Failed to create partition on metric '%s': %v", metric, err)
				return err
			}
			partitionMapMetric[metric] = ExistingPartitionInfo{}
		}
	}
	return nil
}

func EnsureMetricTimescale(pgPartBounds map[string]ExistingPartitionInfo, force bool) error {
	var err error
	sqlEnsure := `
	select * from admin.ensure_partition_timescale($1)
	`
	for metric := range pgPartBounds {
		if strings.HasSuffix(metric, "_realtime") {
			continue
		}
		_, ok := partitionMapMetric[metric]
		if !ok {
			_, err = DBExecRead(mainContext, metricDb, sqlEnsure, metric)
			if err != nil {
				logger.Errorf("Failed to create a TimescaleDB table for metric '%s': %v", metric, err)
				return err
			}
			partitionMapMetric[metric] = ExistingPartitionInfo{}
		}
	}

	err = EnsureMetricTime(pgPartBounds, force, true)
	if err != nil {
		return err
	}
	return nil
}

func EnsureMetricTime(pgPartBounds map[string]ExistingPartitionInfo, force bool, realtimeOnly bool) error {
	// TODO if less < 1d to part. end, precreate ?
	sqlEnsure := `
	select * from admin.ensure_partition_metric_time($1, $2)
	`

	for metric, pb := range pgPartBounds {
		if realtimeOnly && !strings.HasSuffix(metric, "_realtime") {
			continue
		}
		if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
			return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
		}

		partInfo, ok := partitionMapMetric[metric]
		if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || force {
			ret, err := DBExecRead(mainContext, metricDb, sqlEnsure, metric, pb.StartTime)
			if err != nil {
				logger.Error("Failed to create partition on 'metrics':", err)
				return err
			}
			if !ok {
				partInfo = ExistingPartitionInfo{}
			}
			partInfo.StartTime = ret[0]["part_available_from"].(time.Time)
			partInfo.EndTime = ret[0]["part_available_to"].(time.Time)
			partitionMapMetric[metric] = partInfo
		}
		if pb.EndTime.After(partInfo.EndTime) || pb.EndTime.Equal(partInfo.EndTime) || force {
			ret, err := DBExecRead(mainContext, metricDb, sqlEnsure, metric, pb.EndTime)
			if err != nil {
				logger.Error("Failed to create partition on 'metrics':", err)
				return err
			}
			partInfo.EndTime = ret[0]["part_available_to"].(time.Time)
			partitionMapMetric[metric] = partInfo
		}
	}
	return nil
}

func EnsureMetricDbnameTime(metricDbnamePartBounds map[string]map[string]ExistingPartitionInfo, force bool) error {
	// TODO if less < 1d to part. end, precreate ?
	sqlEnsure := `
	select * from admin.ensure_partition_metric_dbname_time($1, $2, $3)
	`

	for metric, dbnameTimestampMap := range metricDbnamePartBounds {
		_, ok := partitionMapMetricDbname[metric]
		if !ok {
			partitionMapMetricDbname[metric] = make(map[string]ExistingPartitionInfo)
		}

		for dbname, pb := range dbnameTimestampMap {

			if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
				return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
			}

			partInfo, ok := partitionMapMetricDbname[metric][dbname]
			if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || force {
				ret, err := DBExecRead(mainContext, metricDb, sqlEnsure, metric, dbname, pb.StartTime)
				if err != nil {
					logger.Errorf("Failed to create partition for [%s:%s]: %v", metric, dbname, err)
					return err
				}
				if !ok {
					partInfo = ExistingPartitionInfo{}
				}
				partInfo.StartTime = ret[0]["part_available_from"].(time.Time)
				partInfo.EndTime = ret[0]["part_available_to"].(time.Time)
				partitionMapMetricDbname[metric][dbname] = partInfo
			}
			if pb.EndTime.After(partInfo.EndTime) || pb.EndTime.Equal(partInfo.EndTime) || force {
				ret, err := DBExecRead(mainContext, metricDb, sqlEnsure, metric, dbname, pb.EndTime)
				if err != nil {
					logger.Errorf("Failed to create partition for [%s:%s]: %v", metric, dbname, err)
					return err
				}
				partInfo.EndTime = ret[0]["part_available_to"].(time.Time)
				partitionMapMetricDbname[metric][dbname] = partInfo
			}
		}
	}
	return nil
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

			data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 300, sqlDbSize) // can take some time on ancient FS, use 300s stmt timeout
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
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sqlPGExecEnv)
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
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sqlApproxDBSize)
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
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, 0, "show version")
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
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, 0, pgpoolVersion)
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
		data, err := DBExecReadByDbUniqueName(ctx, dbUnique, 0, sql)
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

		if verNew.Version > VersionToInt("10.0") && opts.AddSystemIdentifier {
			logger.Debugf("[%s] determining system identifier version (pg ver: %v)", dbUnique, verNew.VersionStr)
			data, err := DBExecReadByDbUniqueName(ctx, dbUnique, 0, sqlSysid)
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
		data, err = DBExecReadByDbUniqueName(ctx, dbUnique, 0, sqlSu)
		if err == nil {
			verNew.IsSuperuser = data[0]["rolsuper"].(bool)
		}
		logger.Debugf("[%s] superuser=%v", dbUnique, verNew.IsSuperuser)

		if verNew.Version >= MinExtensionInfoAvailable {
			//log.Debugf("[%s] determining installed extensions info...", dbUnique)
			data, err = DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sqlExtensions)
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

func DetectSprocChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(MetricData, 0)
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

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.MetricAttrs.StatementTimeoutSeconds, mvp.SQL)
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
				influxEntry := make(MetricEntry)
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
		storageCh <- []MetricStoreMessage{{DBUniqueName: dbUnique, MetricName: "sproc_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	} else if opts.Metric.Datastore == datastorePostgres && firstRun {
		EnsureMetricDummy("sproc_changes")
	}

	return changeCounts
}

func DetectTableChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(MetricData, 0)
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

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.MetricAttrs.StatementTimeoutSeconds, mvp.SQL)
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
				influxEntry := make(MetricEntry)
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
		storageCh <- []MetricStoreMessage{{DBUniqueName: dbUnique, MetricName: "table_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	} else if opts.Metric.Datastore == datastorePostgres && firstRun {
		EnsureMetricDummy("table_changes")
	}

	return changeCounts
}

func DetectIndexChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(MetricData, 0)
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

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.MetricAttrs.StatementTimeoutSeconds, mvp.SQL)
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
				influxEntry := make(MetricEntry)
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
		storageCh <- []MetricStoreMessage{{DBUniqueName: dbUnique, MetricName: "index_changes", Data: detectedChanges, CustomTags: md.CustomTags}}
	} else if opts.Metric.Datastore == datastorePostgres && firstRun {
		EnsureMetricDummy("index_changes")
	}

	return changeCounts
}

func DetectPrivilegeChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(MetricData, 0)
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
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.MetricAttrs.StatementTimeoutSeconds, mvp.SQL)
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
				revokeEntry := make(MetricEntry)
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

	if opts.Metric.Datastore == datastorePostgres && firstRun {
		EnsureMetricDummy("privilege_changes")
	}
	logger.Debugf("[%s][%s] detected %d object privilege changes...", dbUnique, specialMetricChangeEvents, len(detectedChanges))
	if len(detectedChanges) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []MetricStoreMessage{
			{
				DBUniqueName: dbUnique,
				MetricName:   "privilege_changes",
				Data:         detectedChanges,
				CustomTags:   md.CustomTags,
			}}
	}

	return changeCounts
}

func DetectConfigurationChanges(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []MetricStoreMessage, hostState map[string]map[string]string) ChangeDetectionResults {
	detectedChanges := make(MetricData, 0)
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

	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, mvp.MetricAttrs.StatementTimeoutSeconds, mvp.SQL)
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
		storageCh <- []MetricStoreMessage{{
			DBUniqueName: dbUnique,
			MetricName:   "configuration_changes",
			Data:         detectedChanges,
			CustomTags:   md.CustomTags,
		}}
	} else if opts.Metric.Datastore == datastorePostgres {
		EnsureMetricDummy("configuration_changes")
	}

	return changeCounts
}

func CheckForPGObjectChangesAndStore(dbUnique string, vme DBVersionMapEntry, storageCh chan<- []MetricStoreMessage, hostState map[string]map[string]string) {
	sprocСounts := DetectSprocChanges(dbUnique, vme, storageCh, hostState) // TODO some of Detect*() code could be unified...
	tableСounts := DetectTableChanges(dbUnique, vme, storageCh, hostState)
	indexСounts := DetectIndexChanges(dbUnique, vme, storageCh, hostState)
	confСounts := DetectConfigurationChanges(dbUnique, vme, storageCh, hostState)
	privСhangeCounts := DetectPrivilegeChanges(dbUnique, vme, storageCh, hostState)

	if opts.Metric.Datastore == datastorePostgres {
		EnsureMetricDummy("object_changes")
	}

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
		detectedChangesSummary := make(MetricData, 0)
		influxEntry := make(MetricEntry)
		influxEntry["details"] = message
		influxEntry["epoch_ns"] = time.Now().UnixNano()
		detectedChangesSummary = append(detectedChangesSummary, influxEntry)
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storageCh <- []MetricStoreMessage{{DBUniqueName: dbUnique,
			DBType:     md.DBType,
			MetricName: "object_changes",
			Data:       detectedChangesSummary,
			CustomTags: md.CustomTags,
		}}

	}
}

// some extra work needed as pgpool SHOW commands don't specify the return data types for some reason
func FetchMetricsPgpool(msg MetricFetchMessage, _ DBVersionMapEntry, mvp MetricVersionProperties) (MetricData, error) {
	var retData = make(MetricData, 0)
	epochNs := time.Now().UnixNano()

	sqlLines := strings.Split(strings.ToUpper(mvp.SQL), "\n")

	for _, sql := range sqlLines {
		if strings.HasPrefix(sql, "SHOW POOL_NODES") {
			data, err := DBExecReadByDbUniqueName(mainContext, msg.DBUniqueName, 0, sql)
			if err != nil {
				logger.Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				return data, err
			}

			for _, row := range data {
				retRow := make(MetricEntry)
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

			data, err := DBExecReadByDbUniqueName(mainContext, msg.DBUniqueName, 0, sql)
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

func ReadMetricDefinitionMapFromPostgres(failOnError bool) (map[string]map[uint]MetricVersionProperties, error) {
	metricDefMapNew := make(map[string]map[uint]MetricVersionProperties)
	metricNameRemapsNew := make(map[string]string)
	sql := `select /* pgwatch3_generated */ m_name, m_pg_version_from::text, m_sql, m_master_only, m_standby_only,
			  coalesce(m_column_attrs::text, '') as m_column_attrs, coalesce(m_column_attrs::text, '') as m_column_attrs,
			  coalesce(ma_metric_attrs::text, '') as ma_metric_attrs, m_sql_su
			from
              pgwatch3.metric
              left join
              pgwatch3.metric_attribute on (ma_metric_name = m_name)
			where
              m_is_active
		    order by
		      1, 2`

	logger.Info("updating metrics definitons from ConfigDB...")
	data, err := DBExecRead(mainContext, configDb, sql)
	if err != nil {
		if failOnError {
			logger.Fatal(err)
		} else {
			logger.Error(err)
			return metricDefinitionMap, err
		}
	}
	if len(data) == 0 {
		logger.Warning("no active metric definitions found from config DB")
		return metricDefMapNew, err
	}

	logger.WithField("metrics", len(data)).Debug("Active metrics found in config database (pgwatch3.metric)")
	for _, row := range data {
		_, ok := metricDefMapNew[row["m_name"].(string)]
		if !ok {
			metricDefMapNew[row["m_name"].(string)] = make(map[uint]MetricVersionProperties)
		}
		d := VersionToInt(row["m_pg_version_from"].(string))
		ca := MetricColumnAttrs{}
		if row["m_column_attrs"].(string) != "" {
			ca = ParseMetricColumnAttrsFromString(row["m_column_attrs"].(string))
		}
		ma := MetricAttrs{}
		if row["ma_metric_attrs"].(string) != "" {
			ma = ParseMetricAttrsFromString(row["ma_metric_attrs"].(string))
			if ma.MetricStorageName != "" {
				metricNameRemapsNew[row["m_name"].(string)] = ma.MetricStorageName
			}
		}
		metricDefMapNew[row["m_name"].(string)][d] = MetricVersionProperties{
			SQL:                  row["m_sql"].(string),
			SQLSU:                row["m_sql_su"].(string),
			MasterOnly:           row["m_master_only"].(bool),
			StandbyOnly:          row["m_standby_only"].(bool),
			ColumnAttrs:          ca,
			MetricAttrs:          ma,
			CallsHelperFunctions: DoesMetricDefinitionCallHelperFunctions(row["m_sql"].(string)),
		}
	}

	metricNameRemapLock.Lock()
	metricNameRemaps = metricNameRemapsNew
	metricNameRemapLock.Unlock()

	return metricDefMapNew, err
}

func DoesFunctionExists(dbUnique, functionName string) bool {
	logger.Debug("Checking for function existence", dbUnique, functionName)
	sql := fmt.Sprintf("select /* pgwatch3_generated */ 1 from pg_proc join pg_namespace n on pronamespace = n.oid where proname = '%s' and n.nspname = 'public'", functionName)
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sql)
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
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sqlAvailable)
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
			_, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sqlCreateExt)
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
		helpers, err := ReadMetricsFromFolder(path.Join(opts.Metric.MetricsFolder, fileBasedMetricHelpersDir), false)
		if err != nil {
			logger.Errorf("Failed to fetch helpers from \"%s\": %s", path.Join(opts.Metric.MetricsFolder, fileBasedMetricHelpersDir), err)
			return err
		}
		logger.Debug("%d helper definitions found from \"%s\"...", len(helpers), path.Join(opts.Metric.MetricsFolder, fileBasedMetricHelpersDir))

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
				_, err = DBExecReadByDbUniqueName(mainContext, dbUnique, 0, mvp.SQL)
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
				_, err = DBExecReadByDbUniqueName(mainContext, dbUnique, 0, mvp.SQL)
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

	// some cloud providers limit access to template1 for some reason, so try with postgres and defaultdb (Aiven)
	templateDBsToTry := []string{"template1", "postgres", "defaultdb"}

	for _, templateDB := range templateDBsToTry {
		c, err = db.GetPostgresDBConnection(mainContext, ce.LibPQConnStr, ce.Host, ce.Port, templateDB, ce.User, ce.Password,
			ce.SslMode, ce.SslRootCAPath, ce.SslClientCertPath, ce.SslClientKeyPath)
		if err != nil {
			return md, err
		}
		err = c.Ping(mainContext)
		if err == nil {
			break
		}
		c.Close()
	}
	if err != nil {
		return md, fmt.Errorf("Failed to connect to any of the template DBs: %v", templateDBsToTry)
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
			DBName:               d["datname"].(string),
			Host:                 ce.Host,
			Port:                 ce.Port,
			User:                 ce.User,
			Password:             ce.Password,
			PasswordType:         ce.PasswordType,
			SslMode:              ce.SslMode,
			SslRootCAPath:        ce.SslRootCAPath,
			SslClientCertPath:    ce.SslClientCertPath,
			SslClientKeyPath:     ce.SslClientKeyPath,
			StmtTimeout:          ce.StmtTimeout,
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
func GetGoPsutilDiskPG(dbUnique string) (MetricData, error) {
	sql := `select current_setting('data_directory') as dd, current_setting('log_directory') as ld, current_setting('server_version_num')::int as pgver`
	sqlTS := `select spcname::text as name, pg_catalog.pg_tablespace_location(oid) as location from pg_catalog.pg_tablespace where not spcname like any(array[E'pg\\_%'])`
	data, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sql)
	if err != nil || len(data) == 0 {
		logger.Errorf("Failed to determine relevant PG disk paths via SQL: %v", err)
		return nil, err
	}
	dataTblsp, err := DBExecReadByDbUniqueName(mainContext, dbUnique, 0, sqlTS)
	if err != nil {
		logger.Infof("Failed to determine relevant PG tablespace paths via SQL: %v", err)
	}
	return psutil.GetGoPsutilDiskPG(data, dataTblsp)
}

func SendToPostgres(storeMessages []MetricStoreMessage) error {
	if len(storeMessages) == 0 {
		return nil
	}
	tsWarningPrinted := false
	metricsToStorePerMetric := make(map[string][]MetricStoreMessagePostgres)
	rowsBatched := 0
	totalRows := 0
	pgPartBounds := make(map[string]ExistingPartitionInfo)                  // metric=min/max
	pgPartBoundsDbName := make(map[string]map[string]ExistingPartitionInfo) // metric=[dbname=min/max]
	var err error

	for _, msg := range storeMessages {
		if msg.Data == nil || len(msg.Data) == 0 {
			continue
		}
		logger.WithField("data", msg.Data).WithField("len", len(msg.Data)).Debug("Sending To Postgres")

		for _, dr := range msg.Data {
			var epochTime time.Time
			var epochNs int64

			tags := make(map[string]any)
			fields := make(map[string]any)

			totalRows++

			if msg.CustomTags != nil {
				for k, v := range msg.CustomTags {
					tags[k] = fmt.Sprintf("%v", v)
				}
			}

			for k, v := range dr {
				if v == nil || v == "" {
					continue // not storing NULLs
				}
				if k == epochColumnName {
					epochNs = v.(int64)
				} else if strings.HasPrefix(k, tagPrefix) {
					tag := k[4:]
					tags[tag] = fmt.Sprintf("%v", v)
				} else {
					fields[k] = v
				}
			}

			if epochNs == 0 {
				if !tsWarningPrinted && !regexIsPgbouncerMetrics.MatchString(msg.MetricName) {
					logger.Warning("No timestamp_ns found, server time will be used. measurement:", msg.MetricName)
					tsWarningPrinted = true
				}
				epochTime = time.Now()
			} else {
				epochTime = time.Unix(0, epochNs)
			}

			var metricsArr []MetricStoreMessagePostgres
			var ok bool

			metricNameTemp := msg.MetricName

			metricsArr, ok = metricsToStorePerMetric[metricNameTemp]
			if !ok {
				metricsToStorePerMetric[metricNameTemp] = make([]MetricStoreMessagePostgres, 0)
			}
			metricsArr = append(metricsArr, MetricStoreMessagePostgres{Time: epochTime, DBName: msg.DBUniqueName,
				Metric: msg.MetricName, Data: fields, TagData: tags})
			metricsToStorePerMetric[metricNameTemp] = metricsArr

			rowsBatched++

			if MetricSchema == db.MetricSchemaTimescale {
				// set min/max timestamps to check/create partitions
				bounds, ok := pgPartBounds[msg.MetricName]
				if !ok || (ok && epochTime.Before(bounds.StartTime)) {
					bounds.StartTime = epochTime
					pgPartBounds[msg.MetricName] = bounds
				}
				if !ok || (ok && epochTime.After(bounds.EndTime)) {
					bounds.EndTime = epochTime
					pgPartBounds[msg.MetricName] = bounds
				}
			} else if MetricSchema == db.MetricSchemaPostgres {
				_, ok := pgPartBoundsDbName[msg.MetricName]
				if !ok {
					pgPartBoundsDbName[msg.MetricName] = make(map[string]ExistingPartitionInfo)
				}
				bounds, ok := pgPartBoundsDbName[msg.MetricName][msg.DBUniqueName]
				if !ok || (ok && epochTime.Before(bounds.StartTime)) {
					bounds.StartTime = epochTime
					pgPartBoundsDbName[msg.MetricName][msg.DBUniqueName] = bounds
				}
				if !ok || (ok && epochTime.After(bounds.EndTime)) {
					bounds.EndTime = epochTime
					pgPartBoundsDbName[msg.MetricName][msg.DBUniqueName] = bounds
				}
			}
		}
	}

	if MetricSchema == db.MetricSchemaPostgres {
		err = EnsureMetricDbnameTime(pgPartBoundsDbName, forceRecreatePGMetricPartitions)
	} else if MetricSchema == db.MetricSchemaTimescale {
		err = EnsureMetricTimescale(pgPartBounds, forceRecreatePGMetricPartitions)
	} else {
		logger.Fatal("should never happen...")
	}
	if forceRecreatePGMetricPartitions {
		forceRecreatePGMetricPartitions = false
	}
	if err != nil {
		atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
		return err
	}

	// send data to PG, with a separate COPY for all metrics
	logger.Debugf("COPY-ing %d metrics to Postgres metricsDB...", rowsBatched)
	t1 := time.Now()

	for metricName, metrics := range metricsToStorePerMetric {

		getTargetTable := func() pgx.Identifier {
			return pgx.Identifier{metricName}
		}

		getTargetColumns := func() []string {
			return []string{"time", "dbname", "data", "tag_data"}
		}

		for _, m := range metrics {
			l := logger.WithField("db", m.DBName).WithField("metric", m.Metric)
			jsonBytes, err := json.Marshal(m.Data)
			if err != nil {
				logger.Errorf("Skipping 1 metric for [%s:%s] due to JSON conversion error: %s", m.DBName, m.Metric, err)
				atomic.AddUint64(&totalMetricsDroppedCounter, 1)
				continue
			}

			getTagData := func() any {
				if len(m.TagData) > 0 {
					jsonBytesTags, err := json.Marshal(m.TagData)
					if err != nil {
						l.Error(err)
						atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
						return nil
					}
					return string(jsonBytesTags)
				}
				return nil
			}

			rows := [][]any{{m.Time, m.DBName, string(jsonBytes), getTagData()}}

			if _, err = metricDb.CopyFrom(context.Background(), getTargetTable(), getTargetColumns(), pgx.CopyFromRows(rows)); err != nil {
				l.Error(err)
				atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
				forceRecreatePGMetricPartitions = strings.Contains(err.Error(), "no partition")
				if forceRecreatePGMetricPartitions {
					logger.Warning("Some metric partitions might have been removed, halting all metric storage. Trying to re-create all needed partitions on next run")
				}
			}
		}
	}

	diff := time.Since(t1)
	if err == nil {
		if len(storeMessages) == 1 {
			logger.Infof("wrote %d/%d rows to Postgres for [%s:%s] in %.1f ms", rowsBatched, totalRows,
				storeMessages[0].DBUniqueName, storeMessages[0].MetricName, float64(diff.Nanoseconds())/1000000)
		} else {
			logger.Infof("wrote %d/%d rows from %d metric sets to Postgres in %.1f ms", rowsBatched, totalRows,
				len(storeMessages), float64(diff.Nanoseconds())/1000000)
		}
		atomic.StoreInt64(&lastSuccessfulDatastoreWriteTimeEpoch, t1.Unix())
		atomic.AddUint64(&datastoreTotalWriteTimeMicroseconds, uint64(diff.Microseconds()))
		atomic.AddUint64(&datastoreWriteSuccessCounter, 1)
	}
	return err
}
