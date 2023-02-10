package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shopspring/decimal"
)

var configDb *sqlx.DB
var metricDb *sqlx.DB
var monitored_db_conn_cache map[string]*sqlx.DB = make(map[string]*sqlx.DB)

func GetPostgresDBConnection(libPqConnString, host, port, dbname, user, password, sslmode, sslrootcert, sslcert, sslkey string) (*sqlx.DB, error) {
	var connStr string

	//log.Debug("Connecting to: ", host, port, dbname, user, password)
	if len(libPqConnString) > 0 {
		connStr = libPqConnString
		if !strings.Contains(strings.ToLower(connStr), "sslmode") {
			if strings.Contains(connStr, "postgresql://") || strings.Contains(connStr, "postgres://") { // JDBC style
				if strings.Contains(connStr, "?") { // has some extra params already
					connStr += "&sslmode=disable" // defaulting to "disable" as Go driver doesn't support "prefer"
				} else {
					connStr += "?sslmode=disable"
				}
			} else { // LibPQ style
				connStr += " sslmode=disable"
			}
		}
		if !strings.Contains(strings.ToLower(connStr), "connect_timeout") {
			if strings.Contains(connStr, "postgresql://") || strings.Contains(connStr, "postgres://") { // JDBC style
				if strings.Contains(connStr, "?") { // has some extra params already
					connStr += "&connect_timeout=5" // 5 seconds
				} else {
					connStr += "?connect_timeout=5"
				}
			} else { // LibPQ style
				connStr += " connect_timeout=5"
			}
		}
	} else {
		connStr = fmt.Sprintf("host=%s port=%s dbname='%s' sslmode=%s user=%s application_name=%s sslrootcert='%s' sslcert='%s' sslkey='%s' connect_timeout=5",
			host, port, dbname, sslmode, user, APPLICATION_NAME, sslrootcert, sslcert, sslkey)
		if password != "" { // having empty string as password effectively disables .pgpass so include only if password given
			connStr += fmt.Sprintf(" password='%s'", password)
		}
	}

	return sqlx.Open("postgres", connStr)
}

func InitAndTestConfigStoreConnection(host, port, dbname, user, password, requireSSL string, failOnErr bool) error {
	var err error
	SSLMode := "disable"
	var retries = 3 // ~15s

	if StringToBoolOrFail(requireSSL, "--pg-require-ssl") {
		SSLMode = "require"
	}

	for i := 0; i <= retries; i++ {
		// configDb is used by the main thread only. no verify-ca/verify-full support currently
		configDb, err = GetPostgresDBConnection("", host, port, dbname, user, password, SSLMode, "", "", "")
		if err != nil {
			if i < retries {
				log.Errorf("could not open metricDb connection. retrying in 5s. %d retries left. err: %v", retries-i, err)
				time.Sleep(time.Second * 5)
				continue
			}
			if failOnErr {
				log.Fatal("could not open configDb connection! exit.")
			} else {
				log.Error("could not open configDb connection!")
				return err
			}
		}

		err = configDb.Ping()

		if err != nil {
			if i < retries {
				log.Errorf("could not ping configDb! retrying in 5s. %d retries left. err: %v", retries-i, err)
				time.Sleep(time.Second * 5)
				continue
			}
			if failOnErr {
				log.Fatal("could not ping configDb! exit.", err)
			} else {
				log.Error("could not ping configDb!", err)
				return err
			}
		} else {
			log.Info("connect to configDb OK!")
			break
		}
	}
	configDb.SetMaxIdleConns(1)
	configDb.SetMaxOpenConns(2)
	configDb.SetConnMaxLifetime(time.Second * time.Duration(PG_CONN_RECYCLE_SECONDS))
	return nil
}

func InitAndTestMetricStoreConnection(connStr string, failOnErr bool) error {
	var err error
	var retries = 3 // ~15s

	for i := 0; i <= retries; i++ {

		metricDb, err = GetPostgresDBConnection(connStr, "", "", "", "", "", "", "", "", "")
		if err != nil {
			if i < retries {
				log.Errorf("could not open metricDb connection. retrying in 5s. %d retries left. err: %v", retries-i, err)
				time.Sleep(time.Second * 5)
				continue
			}
			if failOnErr {
				log.Fatal("could not open metricDb connection! exit. err:", err)
			} else {
				log.Error("could not open metricDb connection:", err)
				return err
			}
		}

		err = metricDb.Ping()

		if err != nil {
			if i < retries {
				log.Errorf("could not ping metricDb! retrying in 5s. %d retries left. err: %v", retries-i, err)
				time.Sleep(time.Second * 5)
				continue
			}
			if failOnErr {
				log.Fatal("could not ping metricDb! exit.", err)
			} else {
				return err
			}
		} else {
			log.Info("connect to metricDb OK!")
			break
		}
	}
	metricDb.SetMaxIdleConns(1)
	metricDb.SetMaxOpenConns(2)
	metricDb.SetConnMaxLifetime(time.Second * time.Duration(PG_CONN_RECYCLE_SECONDS))
	return nil
}

func IsPostgresDBType(dbType string) bool {
	if dbType == DBTYPE_BOUNCER || dbType == DBTYPE_PGPOOL {
		return false
	}
	return true
}

// every DB under monitoring should have exactly 1 sql.DB connection assigned, that will internally limit parallel access
func InitSqlConnPoolForMonitoredDBIfNil(md MonitoredDatabase) error {
	monitored_db_conn_cache_lock.Lock()
	defer monitored_db_conn_cache_lock.Unlock()

	conn, ok := monitored_db_conn_cache[md.DBUniqueName]
	if ok && conn != nil {
		return nil
	}

	if md.DBType == DBTYPE_BOUNCER {
		md.DBName = "pgbouncer"
	}

	conn, err := GetPostgresDBConnection(md.LibPQConnStr, md.Host, md.Port, md.DBName, md.User, md.Password,
		md.SslMode, md.SslRootCAPath, md.SslClientCertPath, md.SslClientKeyPath)
	if err != nil {
		return err
	}

	if useConnPooling {
		conn.SetMaxIdleConns(opts.MaxParallelConnectionsPerDb)
	} else {
		conn.SetMaxIdleConns(0)
	}
	conn.SetMaxOpenConns(opts.MaxParallelConnectionsPerDb)
	// recycling periodically makes sense as long sessions might bloat memory or maybe conn info (password) was changed
	conn.SetConnMaxLifetime(time.Second * time.Duration(PG_CONN_RECYCLE_SECONDS))

	monitored_db_conn_cache[md.DBUniqueName] = conn
	log.Debugf("[%s] Connection pool initialized with max %d parallel connections. Conn pooling: %v", md.DBUniqueName, opts.MaxParallelConnectionsPerDb, useConnPooling)

	return nil
}

func CloseOrLimitSqlConnPoolForMonitoredDBIfAny(dbUnique string) {
	monitored_db_conn_cache_lock.Lock()
	defer monitored_db_conn_cache_lock.Unlock()

	conn, ok := monitored_db_conn_cache[dbUnique]
	if !ok || conn == nil {
		return
	}

	if IsDBUndersized(dbUnique) || IsDBIgnoredBasedOnRecoveryState(dbUnique) {

		if useConnPooling {
			s := conn.Stats()
			if s.MaxOpenConnections > 1 {
				log.Debugf("[%s] Limiting SQL connection pool to max 1 connection due to dormant state ...", dbUnique)
				conn.SetMaxIdleConns(1)
				conn.SetMaxOpenConns(1)
			}
		}

	} else { // removed from config
		log.Debugf("[%s] Closing SQL connection pool ...", dbUnique)
		err := conn.Close()
		if err != nil {
			log.Error("[%s] Failed to close connection pool to %s nicely. Err: %v", dbUnique, err)
		}
		delete(monitored_db_conn_cache, dbUnique)
	}
}

func DBExecRead(conn *sqlx.DB, host_ident, sql string, args ...interface{}) ([](map[string]interface{}), error) {
	ret := make([]map[string]interface{}, 0)
	var rows *sqlx.Rows
	var err error

	if conn == nil {
		return nil, errors.New("nil connection")
	}

	rows, err = conn.Queryx(sql, args...)

	if err != nil {
		// connection problems or bad queries etc are quite common so caller should decide if to output something
		log.Debug("failed to query", host_ident, "sql:", sql, "err:", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		row := make(map[string]interface{})
		err = rows.MapScan(row)
		if err != nil {
			log.Error("failed to MapScan a result row", host_ident, err)
			return nil, err
		}
		ret = append(ret, row)
	}

	err = rows.Err()
	if err != nil {
		log.Error("failed to fully process resultset for", host_ident, "sql:", sql, "err:", err)
	}
	return ret, err
}

func DBExecInExplicitTX(conn *sqlx.DB, host_ident, query string, args ...interface{}) ([](map[string]interface{}), error) {
	ret := make([]map[string]interface{}, 0)
	var rows *sqlx.Rows
	var err error

	if conn == nil {
		return nil, errors.New("nil connection")
	}

	ctx := context.Background()
	txOpts := sql.TxOptions{}

	tx, err := conn.BeginTxx(ctx, &txOpts)
	if err != nil {
		return ret, err
	}
	defer tx.Commit()

	rows, err = tx.Queryx(query, args...)

	if err != nil {
		// connection problems or bad queries etc are quite common so caller should decide if to output something
		log.Debug("failed to query", host_ident, "sql:", query, "err:", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		row := make(map[string]interface{})
		err = rows.MapScan(row)
		if err != nil {
			log.Error("failed to MapScan a result row", host_ident, err)
			return nil, err
		}
		ret = append(ret, row)
	}

	err = rows.Err()
	if err != nil {
		log.Error("failed to fully process resultset for", host_ident, "sql:", query, "err:", err)
	}
	return ret, err
}

func DBExecReadByDbUniqueName(dbUnique, metricName string, stmtTimeoutOverride int64, sql string, args ...interface{}) ([](map[string]interface{}), error, time.Duration) {
	var conn *sqlx.DB
	var md MonitoredDatabase
	var data [](map[string]interface{})
	var err error
	var duration time.Duration
	var exists bool
	var sqlStmtTimeout string
	var sqlLockTimeout = "SET LOCAL lock_timeout TO '100ms';"

	if strings.TrimSpace(sql) == "" {
		return nil, errors.New("empty SQL"), duration
	}

	md, err = GetMonitoredDatabaseByUniqueName(dbUnique)
	if err != nil {
		return nil, err, duration
	}

	monitored_db_conn_cache_lock.RLock()
	// sqlx.DB itself is parallel safe
	conn, exists = monitored_db_conn_cache[dbUnique]
	monitored_db_conn_cache_lock.RUnlock()
	if !exists || conn == nil {
		log.Errorf("SQL connection for dbUnique %s not found or nil", dbUnique) // Should always be initialized in the main loop DB discovery code ...
		return nil, errors.New("SQL connection not found or nil"), duration
	}

	if !adHocMode && IsPostgresDBType(md.DBType) {
		stmtTimeout := md.StmtTimeout
		if stmtTimeoutOverride > 0 {
			stmtTimeout = stmtTimeoutOverride
		}
		if stmtTimeout > 0 { // 0 = don't change, use DB level settings
			if useConnPooling {
				sqlStmtTimeout = fmt.Sprintf("SET LOCAL statement_timeout TO '%ds';", stmtTimeout)
			} else {
				sqlStmtTimeout = fmt.Sprintf("SET statement_timeout TO '%ds';", stmtTimeout)
			}

		}
		if err != nil {
			atomic.AddUint64(&totalMetricFetchFailuresCounter, 1)
			return nil, err, duration
		}
	}

	if !useConnPooling {
		sqlLockTimeout = "SET lock_timeout TO '100ms';"
	}

	sqlToExec := sqlLockTimeout + sqlStmtTimeout + sql // bundle timeouts with actual SQL to reduce round-trip times
	//log.Debugf("Executing SQL: %s", sqlToExec)
	t1 := time.Now()
	if useConnPooling {
		data, err = DBExecInExplicitTX(conn, dbUnique, sqlToExec, args...)
	} else {
		data, err = DBExecRead(conn, dbUnique, sqlToExec, args...)
	}
	t2 := time.Now()
	if err != nil {
		atomic.AddUint64(&totalMetricFetchFailuresCounter, 1)
	}

	return data, err, t2.Sub(t1)
}

func GetAllActiveHostsFromConfigDB() ([](map[string]interface{}), error) {
	sql_latest := `
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
	sql_prev := `
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
	data, err := DBExecRead(configDb, CONFIGDB_IDENT, sql_latest)
	if err != nil {
		err1 := err
		log.Debugf("Failed to query the monitored DB-s config with latest SQL: %v ", err1)
		data, err = DBExecRead(configDb, CONFIGDB_IDENT, sql_prev)
		if err == nil {
			log.Warning("Fetching monitored DB-s config succeeded with SQL from previous schema version - gatherer update required!")
		} else {
			log.Errorf("Failed to query the monitored DB-s config: %v", err1) // show the original error
		}
	}
	return data, err
}

func OldPostgresMetricsDeleter(metricAgeDaysThreshold int64, schemaType string) {
	sqlDoesOldPartListingFuncExist := `SELECT count(*) FROM information_schema.routines WHERE routine_schema = 'admin' AND routine_name = 'get_old_time_partitions'`
	oldPartListingFuncExists := false // if func existing (>v1.8.1) then use it to drop old partitions in smaller batches
	// as for large setup (50+ DBs) one could reach the default "max_locks_per_transaction" otherwise

	ret, err := DBExecRead(metricDb, METRICDB_IDENT, sqlDoesOldPartListingFuncExist)
	if err == nil && len(ret) > 0 && ret[0]["count"].(int64) > 0 {
		oldPartListingFuncExists = true
	}

	time.Sleep(time.Hour * 1) // to reduce distracting log messages at startup

	for {
		// metric|metric-time|metric-dbname-time|custom
		if schemaType == "metric" {
			rows_deleted, err := DeleteOldPostgresMetrics(metricAgeDaysThreshold)
			if err != nil {
				log.Errorf("Failed to delete old (>%d days) metrics from Postgres: %v", metricAgeDaysThreshold, err)
				time.Sleep(time.Second * 300)
				continue
			}
			log.Infof("Deleted %d old metrics rows...", rows_deleted)
		} else if schemaType == "timescale" || (!oldPartListingFuncExists && (schemaType == "metric-time" || schemaType == "metric-dbname-time")) {
			parts_dropped, err := DropOldTimePartitions(metricAgeDaysThreshold)

			if err != nil {
				log.Errorf("Failed to drop old partitions (>%d days) from Postgres: %v", metricAgeDaysThreshold, err)
				time.Sleep(time.Second * 300)
				continue
			}
			log.Infof("Dropped %d old metric partitions...", parts_dropped)
		} else if oldPartListingFuncExists && (schemaType == "metric-time" || schemaType == "metric-dbname-time") {
			partsToDrop, err := GetOldTimePartitions(metricAgeDaysThreshold)
			if err != nil {
				log.Errorf("Failed to get a listing of old (>%d days) time partitions from Postgres metrics DB - check that the admin.get_old_time_partitions() function is rolled out: %v", metricAgeDaysThreshold, err)
				time.Sleep(time.Second * 300)
				continue
			}
			if len(partsToDrop) > 0 {
				log.Infof("Dropping %d old metric partitions one by one...", len(partsToDrop))
				for _, toDrop := range partsToDrop {
					sqlDropTable := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, toDrop)
					log.Debugf("Dropping old metric data partition: %s", toDrop)
					_, err := DBExecRead(metricDb, METRICDB_IDENT, sqlDropTable)
					if err != nil {
						log.Errorf("Failed to drop old partition %s from Postgres metrics DB: %v", toDrop, err)
						time.Sleep(time.Second * 300)
					} else {
						time.Sleep(time.Second * 5)
					}
				}
			} else {
				log.Infof("No old metric partitions found to drop...")
			}
		}
		time.Sleep(time.Hour * 12)
	}
}

func DeleteOldPostgresMetrics(metricAgeDaysThreshold int64) (int64, error) {
	// for 'metric' schema i.e. no time partitions
	var total_dropped int64
	get_top_lvl_tables_sql := `
	select 'public.' || quote_ident(c.relname) as table_full_name
	from pg_class c
	join pg_namespace n on n.oid = c.relnamespace
	where relkind in ('r', 'p') and nspname = 'public'
	and exists (select 1 from pg_attribute where attrelid = c.oid and attname = 'time')
	and pg_catalog.obj_description(c.oid, 'pg_class') = 'pgwatch3-generated-metric-lvl'
	order by 1
	`
	delete_sql := `
	with q_blocks_range as (
		select min(ctid), max(ctid) from (
		  select ctid from %s
			where time < (now() - '1day'::interval * %d)
			order by ctid
		  limit 5000
	    ) x
    ),
	q_deleted as (
	  delete from %s
	  where ctid between (select min from q_blocks_range) and (select max from q_blocks_range)
	  and time < (now() - '1day'::interval * %d)
	  returning *
	)
	select count(*) from q_deleted;
	`

	top_lvl_tables, err := DBExecRead(metricDb, METRICDB_IDENT, get_top_lvl_tables_sql)
	if err != nil {
		return total_dropped, err
	}

	for _, dr := range top_lvl_tables {

		log.Debugf("Dropping one chunk (max 5000 rows) of old data (if any found) from %v", dr["table_full_name"])
		sql := fmt.Sprintf(delete_sql, dr["table_full_name"].(string), metricAgeDaysThreshold, dr["table_full_name"].(string), metricAgeDaysThreshold)

		for {
			ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql)
			if err != nil {
				return total_dropped, err
			}
			if ret[0]["count"].(int64) == 0 {
				break
			}
			total_dropped += ret[0]["count"].(int64)
			log.Debugf("Dropped %d rows from %v, sleeping 100ms...", ret[0]["count"].(int64), dr["table_full_name"])
			time.Sleep(time.Millisecond * 500)
		}
	}
	return total_dropped, nil
}

func DropOldTimePartitions(metricAgeDaysThreshold int64) (int, error) {
	parts_dropped := 0
	var err error
	sql_old_part := `select admin.drop_old_time_partitions($1, $2)`

	ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql_old_part, metricAgeDaysThreshold, false)
	if err != nil {
		log.Error("Failed to drop old time partitions from Postgres metricDB:", err)
		return parts_dropped, err
	}
	parts_dropped = int(ret[0]["drop_old_time_partitions"].(int64))

	return parts_dropped, err
}

func GetOldTimePartitions(metricAgeDaysThreshold int64) ([]string, error) {
	partsToDrop := make([]string, 0)
	var err error
	sqlGetOldParts := `select admin.get_old_time_partitions($1)`

	ret, err := DBExecRead(metricDb, METRICDB_IDENT, sqlGetOldParts, metricAgeDaysThreshold)
	if err != nil {
		log.Error("Failed to get a listing of old time partitions from Postgres metricDB:", err)
		return partsToDrop, err
	}
	for _, row := range ret {
		partsToDrop = append(partsToDrop, row["get_old_time_partitions"].(string))
	}

	return partsToDrop, nil
}

func CheckIfPGSchemaInitializedOrFail() string {
	var partFuncSignature string
	var pgSchemaType string

	schema_type_sql := `select schema_type from admin.storage_schema_type`
	ret, err := DBExecRead(metricDb, METRICDB_IDENT, schema_type_sql)
	if err != nil {
		log.Fatal("have you initialized the metrics schema, including a row in 'storage_schema_type' table, from schema_base.sql?", err)
	}
	if err == nil && len(ret) == 0 {
		log.Fatal("no metric schema selected, no row in table 'storage_schema_type'. see the README from the 'pgwatch3/sql/metric_store' folder on choosing a schema")
	}
	pgSchemaType = ret[0]["schema_type"].(string)
	if !(pgSchemaType == "metric" || pgSchemaType == "metric-time" || pgSchemaType == "metric-dbname-time" || pgSchemaType == "custom" || pgSchemaType == "timescale") {
		log.Fatalf("Unknow Postgres schema type found from Metrics DB: %s", pgSchemaType)
	}

	if pgSchemaType == "custom" {
		sql := `
		SELECT has_table_privilege(session_user, 'public.metrics', 'INSERT') ok;
		`
		ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql)
		if err != nil || (err == nil && !ret[0]["ok"].(bool)) {
			log.Fatal("public.metrics table not existing or no INSERT privileges")
		}
	} else {
		sql := `
		SELECT has_table_privilege(session_user, 'admin.metrics_template', 'INSERT') ok;
		`
		ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql)
		if err != nil || (err == nil && !ret[0]["ok"].(bool)) {
			log.Fatal("admin.metrics_template table not existing or no INSERT privileges")
		}
	}

	if pgSchemaType == "metric" {
		partFuncSignature = "admin.ensure_partition_metric(text)"
	} else if pgSchemaType == "metric-time" {
		partFuncSignature = "admin.ensure_partition_metric_time(text,timestamp with time zone,integer)"
	} else if pgSchemaType == "metric-dbname-time" {
		partFuncSignature = "admin.ensure_partition_metric_dbname_time(text,text,timestamp with time zone,integer)"
	} else if pgSchemaType == "timescale" {
		partFuncSignature = "admin.ensure_partition_timescale(text)"
	}

	if partFuncSignature != "" {
		sql := `
			SELECT has_function_privilege(session_user,
				'%s',
				'execute') ok;
			`
		ret, err := DBExecRead(metricDb, METRICDB_IDENT, fmt.Sprintf(sql, partFuncSignature))
		if err != nil || (err == nil && !ret[0]["ok"].(bool)) {
			log.Fatalf("%s function not existing or no EXECUTE privileges. Have you rolled out the schema correctly from pgwatch3/sql/metric_store?", partFuncSignature)
		}
	}
	return pgSchemaType
}

func AddDBUniqueMetricToListingTable(db_unique, metric string) error {
	sql := `insert into admin.all_distinct_dbname_metrics
			select $1, $2
			where not exists (
				select * from admin.all_distinct_dbname_metrics where dbname = $1 and metric = $2
			)`
	_, err := DBExecRead(metricDb, METRICDB_IDENT, sql, db_unique, metric)
	return err
}

func UniqueDbnamesListingMaintainer(daemonMode bool) {
	// due to metrics deletion the listing can go out of sync (a trigger not really wanted)
	sql_top_level_metrics := `SELECT table_name FROM admin.get_top_level_metric_tables()`
	sql_distinct := `
	WITH RECURSIVE t(dbname) AS (
		SELECT MIN(dbname) AS dbname FROM %s
		UNION
		SELECT (SELECT MIN(dbname) FROM %s WHERE dbname > t.dbname) FROM t )
	SELECT dbname FROM t WHERE dbname NOTNULL ORDER BY 1
	`
	sql_delete := `DELETE FROM admin.all_distinct_dbname_metrics WHERE NOT dbname = ANY($1) and metric = $2 RETURNING *`
	sql_delete_all := `DELETE FROM admin.all_distinct_dbname_metrics WHERE metric = $1 RETURNING *`
	sql_add := `
		INSERT INTO admin.all_distinct_dbname_metrics SELECT u, $2 FROM (select unnest($1::text[]) as u) x
		WHERE NOT EXISTS (select * from admin.all_distinct_dbname_metrics where dbname = u and metric = $2)
		RETURNING *;
	`

	for {
		if daemonMode {
			time.Sleep(time.Hour * 24)
		}

		log.Infof("Refreshing admin.all_distinct_dbname_metrics listing table...")
		all_distinct_metric_tables, err := DBExecRead(metricDb, METRICDB_IDENT, sql_top_level_metrics)
		if err != nil {
			log.Error("Could not refresh Postgres dbnames listing table:", err)
		} else {
			for _, dr := range all_distinct_metric_tables {
				found_dbnames_map := make(map[string]bool)
				found_dbnames_arr := make([]string, 0)
				metric_name := strings.Replace(dr["table_name"].(string), "public.", "", 1)

				log.Debugf("Refreshing all_distinct_dbname_metrics listing for metric: %s", metric_name)
				ret, err := DBExecRead(metricDb, METRICDB_IDENT, fmt.Sprintf(sql_distinct, dr["table_name"], dr["table_name"]))
				if err != nil {
					log.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for '%s': %s", metric_name, err)
					break
				}
				for _, dr_dbname := range ret {
					found_dbnames_map[dr_dbname["dbname"].(string)] = true // "set" behaviour, don't want duplicates
				}

				// delete all that are not known and add all that are not there
				for k := range found_dbnames_map {
					found_dbnames_arr = append(found_dbnames_arr, k)
				}
				if len(found_dbnames_arr) == 0 { // delete all entries for given metric
					log.Debugf("Deleting Postgres all_distinct_dbname_metrics listing table entries for metric '%s':", metric_name)
					_, err = DBExecRead(metricDb, METRICDB_IDENT, sql_delete_all, metric_name)
					if err != nil {
						log.Errorf("Could not delete Postgres all_distinct_dbname_metrics listing table entries for metric '%s': %s", metric_name, err)
					}
					continue
				}
				ret, err = DBExecRead(metricDb, METRICDB_IDENT, sql_delete, pq.Array(found_dbnames_arr), metric_name)
				if err != nil {
					log.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metric_name, err)
				} else if len(ret) > 0 {
					log.Infof("Removed %d stale entries from all_distinct_dbname_metrics listing table for metric: %s", len(ret), metric_name)
				}
				ret, err = DBExecRead(metricDb, METRICDB_IDENT, sql_add, pq.Array(found_dbnames_arr), metric_name)
				if err != nil {
					log.Errorf("Could not refresh Postgres all_distinct_dbname_metrics listing table for metric '%s': %s", metric_name, err)
				} else if len(ret) > 0 {
					log.Infof("Added %d entry to the Postgres all_distinct_dbname_metrics listing table for metric: %s", len(ret), metric_name)
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
	if opts.Datastore != DATASTORE_POSTGRES {
		return
	}
	sql_ensure := `
	select admin.ensure_dummy_metrics_table($1) as created
	`
	PGDummyMetricTablesLock.Lock()
	defer PGDummyMetricTablesLock.Unlock()
	lastEnsureCall, ok := PGDummyMetricTables[metric]
	if ok && lastEnsureCall.After(time.Now().Add(-1*time.Hour)) {
		return
	} else {
		ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric)
		if err != nil {
			log.Errorf("Failed to create dummy partition of metric '%s': %v", metric, err)
		} else {
			if ret[0]["created"].(bool) {
				log.Infof("Created a dummy partition of metric '%s'", metric)
			}
			PGDummyMetricTables[metric] = time.Now()
		}
	}
}

func EnsureMetric(pg_part_bounds map[string]ExistingPartitionInfo, force bool) error {

	sql_ensure := `
	select * from admin.ensure_partition_metric($1)
	`
	for metric := range pg_part_bounds {

		_, ok := partitionMapMetric[metric] // sequential access currently so no lock needed
		if !ok || force {
			_, err := DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric)
			if err != nil {
				log.Errorf("Failed to create partition on metric '%s': %v", metric, err)
				return err
			}
			partitionMapMetric[metric] = ExistingPartitionInfo{}
		}
	}
	return nil
}

func EnsureMetricTimescale(pg_part_bounds map[string]ExistingPartitionInfo, force bool) error {
	var err error
	sql_ensure := `
	select * from admin.ensure_partition_timescale($1)
	`
	for metric := range pg_part_bounds {
		if strings.HasSuffix(metric, "_realtime") {
			continue
		}
		_, ok := partitionMapMetric[metric]
		if !ok {
			_, err = DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric)
			if err != nil {
				log.Errorf("Failed to create a TimescaleDB table for metric '%s': %v", metric, err)
				return err
			}
			partitionMapMetric[metric] = ExistingPartitionInfo{}
		}
	}

	err = EnsureMetricTime(pg_part_bounds, force, true)
	if err != nil {
		return err
	}
	return nil
}

func EnsureMetricTime(pg_part_bounds map[string]ExistingPartitionInfo, force bool, realtime_only bool) error {
	// TODO if less < 1d to part. end, precreate ?
	sql_ensure := `
	select * from admin.ensure_partition_metric_time($1, $2)
	`

	for metric, pb := range pg_part_bounds {
		if realtime_only && !strings.HasSuffix(metric, "_realtime") {
			continue
		}
		if pb.StartTime.IsZero() || pb.EndTime.IsZero() {
			return fmt.Errorf("zero StartTime/EndTime in partitioning request: [%s:%v]", metric, pb)
		}

		partInfo, ok := partitionMapMetric[metric]
		if !ok || (ok && (pb.StartTime.Before(partInfo.StartTime))) || force {
			ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric, pb.StartTime)
			if err != nil {
				log.Error("Failed to create partition on 'metrics':", err)
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
			ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric, pb.EndTime)
			if err != nil {
				log.Error("Failed to create partition on 'metrics':", err)
				return err
			}
			partInfo.EndTime = ret[0]["part_available_to"].(time.Time)
			partitionMapMetric[metric] = partInfo
		}
	}
	return nil
}

func EnsureMetricDbnameTime(metric_dbname_part_bounds map[string]map[string]ExistingPartitionInfo, force bool) error {
	// TODO if less < 1d to part. end, precreate ?
	sql_ensure := `
	select * from admin.ensure_partition_metric_dbname_time($1, $2, $3)
	`

	for metric, dbnameTimestampMap := range metric_dbname_part_bounds {
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
				ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric, dbname, pb.StartTime)
				if err != nil {
					log.Errorf("Failed to create partition for [%s:%s]: %v", metric, dbname, err)
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
				ret, err := DBExecRead(metricDb, METRICDB_IDENT, sql_ensure, metric, dbname, pb.EndTime)
				if err != nil {
					log.Errorf("Failed to create partition for [%s:%s]: %v", metric, dbname, err)
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
	sql_db_size := `select /* pgwatch3_generated */ pg_database_size(current_database());`
	var sizeMB int64

	lastDBSizeCheckLock.RLock()
	lastDBSizeCheckTime := lastDBSizeFetchTime[dbUnique]
	lastDBSize, ok := lastDBSizeMB[dbUnique]
	lastDBSizeCheckLock.RUnlock()

	if !ok || lastDBSizeCheckTime.Add(DB_SIZE_CACHING_INTERVAL).Before(time.Now()) {
		ver, err := DBGetPGVersion(dbUnique, DBTYPE_PG, false)
		if err != nil || (ver.ExecEnv != EXEC_ENV_AZURE_SINGLE) || (ver.ExecEnv == EXEC_ENV_AZURE_SINGLE && ver.ApproxDBSizeB < 1e12) {
			log.Debugf("[%s] determining DB size ...", dbUnique)

			data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 300, sql_db_size) // can take some time on ancient FS, use 300s stmt timeout
			if err != nil {
				log.Errorf("[%s] failed to determine DB size...cannot apply --min-db-size-mb flag. err: %v ...", dbUnique, err)
				return 0, err
			}
			sizeMB = data[0]["pg_database_size"].(int64) / 1048576
		} else {
			log.Debugf("[%s] Using approx DB size for the --min-db-size-mb filter ...", dbUnique)
			sizeMB = ver.ApproxDBSizeB / 1048576
		}

		log.Debugf("[%s] DB size = %d MB, caching for %v ...", dbUnique, sizeMB, DB_SIZE_CACHING_INTERVAL)

		lastDBSizeCheckLock.Lock()
		lastDBSizeFetchTime[dbUnique] = time.Now()
		lastDBSizeMB[dbUnique] = sizeMB
		lastDBSizeCheckLock.Unlock()

		return sizeMB, nil

	}
	log.Debugf("[%s] using cached DBsize %d MB for the --min-db-size-mb filter check", dbUnique, lastDBSize)
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
	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sqlPGExecEnv)
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
	where	/* NB! works only for v9.1+*/
		c.relpersistence != 't';
	`
	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sqlApproxDBSize)
	if err != nil {
		return 0, err
	}
	return data[0]["db_size_approx"].(int64), nil
}

func DBGetPGVersion(dbUnique string, dbType string, noCache bool) (DBVersionMapEntry, error) {
	var ver DBVersionMapEntry
	var verNew DBVersionMapEntry
	var ok bool
	sql := `
		select /* pgwatch3_generated */ (regexp_matches(
			regexp_replace(current_setting('server_version'), '(beta|devel).*', '', 'g'),
			E'\\d+\\.?\\d+?')
			)[1]::text as ver, pg_is_in_recovery(), current_database()::text;
	`
	sql_sysid := `select /* pgwatch3_generated */ system_identifier::text from pg_control_system();`
	sql_su := `select /* pgwatch3_generated */ rolsuper
			   from pg_roles r where rolname = session_user;`
	sql_extensions := `select /* pgwatch3_generated */ extname::text, (regexp_matches(extversion, $$\d+\.?\d+?$$))[1]::text as extversion from pg_extension order by 1;`
	pgpool_version := `SHOW POOL_VERSION` // supported from pgpool2 v3.0

	db_pg_version_map_lock.Lock()
	get_ver_lock, ok := db_get_pg_version_map_lock[dbUnique]
	if !ok {
		db_get_pg_version_map_lock[dbUnique] = sync.RWMutex{}
		get_ver_lock = db_get_pg_version_map_lock[dbUnique]
	}
	ver, ok = db_pg_version_map[dbUnique]
	db_pg_version_map_lock.Unlock()

	if !noCache && ok && ver.LastCheckedOn.After(time.Now().Add(time.Minute*-2)) { // use cached version for 2 min
		//log.Debugf("using cached postgres version %s for %s", ver.Version.String(), dbUnique)
		return ver, nil
	} else {
		get_ver_lock.Lock() // limit to 1 concurrent version info fetch per DB
		defer get_ver_lock.Unlock()
		log.Debugf("[%s][%s] determining DB version and recovery status...", dbUnique, dbType)

		if verNew.Extensions == nil {
			verNew.Extensions = make(map[string]decimal.Decimal)
		}

		if dbType == DBTYPE_BOUNCER {
			data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, "show version")
			if err != nil {
				return verNew, err
			}
			if len(data) == 0 {
				// surprisingly pgbouncer 'show version' outputs in pre v1.12 is emitted as 'NOTICE' which cannot be accessed from Go lib/pg
				verNew.Version, _ = decimal.NewFromString("0")
				verNew.VersionStr = "0"
			} else {
				matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(data[0]["version"].(string))
				if len(matches) != 1 {
					log.Errorf("[%s] Unexpected PgBouncer version input: %s", dbUnique, data[0]["version"].(string))
					return ver, fmt.Errorf("Unexpected PgBouncer version input: %s", data[0]["version"].(string))
				}
				verNew.VersionStr = matches[0]
				verNew.Version, _ = decimal.NewFromString(matches[0])
			}
		} else if dbType == DBTYPE_PGPOOL {
			data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, pgpool_version)
			if err != nil {
				return verNew, err
			}
			if len(data) == 0 {
				verNew.Version, _ = decimal.NewFromString("3.0")
				verNew.VersionStr = "3.0"
			} else {
				matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(string(data[0]["pool_version"].([]byte)))
				if len(matches) != 1 {
					log.Errorf("[%s] Unexpected PgPool version input: %s", dbUnique, data[0]["pool_version"].([]byte))
					return ver, fmt.Errorf("Unexpected PgPool version input: %s", data[0]["pool_version"].([]byte))
				}
				verNew.VersionStr = matches[0]
				verNew.Version, _ = decimal.NewFromString(matches[0])
			}
		} else {
			data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sql)
			if err != nil {
				if noCache {
					return ver, err
				} else {
					log.Infof("[%s] DBGetPGVersion failed, using old cached value. err: %v", dbUnique, err)
					return ver, nil
				}
			}
			verNew.Version, _ = decimal.NewFromString(data[0]["ver"].(string))
			verNew.VersionStr = data[0]["ver"].(string)
			verNew.IsInRecovery = data[0]["pg_is_in_recovery"].(bool)
			verNew.RealDbname = data[0]["current_database"].(string)

			if verNew.Version.GreaterThanOrEqual(decimal.NewFromFloat(10)) && addSystemIdentifier {
				log.Debugf("[%s] determining system identifier version (pg ver: %v)", dbUnique, verNew.VersionStr)
				data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sql_sysid)
				if err == nil && len(data) > 0 {
					verNew.SystemIdentifier = data[0]["system_identifier"].(string)
				}
			}

			if ver.ExecEnv != "" {
				verNew.ExecEnv = ver.ExecEnv // carry over as not likely to change ever
			} else {
				log.Debugf("[%s] determining the execution env...", dbUnique)
				execEnv := TryDiscoverExecutionEnv(dbUnique)
				if execEnv != "" {
					log.Debugf("[%s] running on execution env: %s", dbUnique, execEnv)
					verNew.ExecEnv = execEnv
				}
			}

			// to work around poor Azure Single Server FS functions performance for some metrics + the --min-db-size-mb filter
			if verNew.ExecEnv == EXEC_ENV_AZURE_SINGLE {
				approxSize, err := GetDBTotalApproxSize(dbUnique)
				if err == nil {
					verNew.ApproxDBSizeB = approxSize
				} else {
					verNew.ApproxDBSizeB = ver.ApproxDBSizeB
				}
			}

			log.Debugf("[%s] determining if monitoring user is a superuser...", dbUnique)
			data, err, _ = DBExecReadByDbUniqueName(dbUnique, "", 0, sql_su)
			if err == nil {
				verNew.IsSuperuser = data[0]["rolsuper"].(bool)
			}
			log.Debugf("[%s] superuser=%v", dbUnique, verNew.IsSuperuser)

			if verNew.Version.GreaterThanOrEqual(MinExtensionInfoAvailable) {
				//log.Debugf("[%s] determining installed extensions info...", dbUnique)
				data, err, _ = DBExecReadByDbUniqueName(dbUnique, "", 0, sql_extensions)
				if err != nil {
					log.Errorf("[%s] failed to determine installed extensions info: %v", dbUnique, err)
				} else {
					for _, dr := range data {
						extver, err := decimal.NewFromString(dr["extversion"].(string))
						if err != nil {
							log.Errorf("[%s] failed to determine extension version info for extension %s: %v", dbUnique, dr["extname"], err)
							continue
						}
						verNew.Extensions[dr["extname"].(string)] = extver
					}
					log.Debugf("[%s] installed extensions: %+v", dbUnique, verNew.Extensions)
				}
			}
		}

		verNew.LastCheckedOn = time.Now()
		db_pg_version_map_lock.Lock()
		db_pg_version_map[dbUnique] = verNew
		db_pg_version_map_lock.Unlock()
	}
	return verNew, nil
}

func DetectSprocChanges(dbUnique string, vme DBVersionMapEntry, storage_ch chan<- []MetricStoreMessage, host_state map[string]map[string]string) ChangeDetectionResults {
	detected_changes := make([](map[string]interface{}), 0)
	var first_run bool
	var change_counts ChangeDetectionResults

	log.Debugf("[%s][%s] checking for sproc changes...", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS)
	if _, ok := host_state["sproc_hashes"]; !ok {
		first_run = true
		host_state["sproc_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("sproc_hashes", vme, nil)
	if err != nil {
		log.Error("could not get sproc_hashes sql:", err)
		return change_counts
	}

	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "sproc_hashes", mvp.MetricAttrs.StatementTimeoutSeconds, mvp.Sql)
	if err != nil {
		log.Error("could not read sproc_hashes from monitored host: ", dbUnique, ", err:", err)
		return change_counts
	}

	for _, dr := range data {
		obj_ident := dr["tag_sproc"].(string) + DB_METRIC_JOIN_STR + dr["tag_oid"].(string)
		prev_hash, ok := host_state["sproc_hashes"][obj_ident]
		if ok { // we have existing state
			if prev_hash != dr["md5"].(string) {
				log.Info("detected change in sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "alter"
				detected_changes = append(detected_changes, dr)
				host_state["sproc_hashes"][obj_ident] = dr["md5"].(string)
				change_counts.Altered += 1
			}
		} else { // check for new / delete
			if !first_run {
				log.Info("detected new sproc:", dr["tag_sproc"], ", oid:", dr["tag_oid"])
				dr["event"] = "create"
				detected_changes = append(detected_changes, dr)
				change_counts.Created += 1
			}
			host_state["sproc_hashes"][obj_ident] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !first_run && len(host_state["sproc_hashes"]) != len(data) {
		deleted_sprocs := make([]string, 0)
		// turn resultset to map => [oid]=true for faster checks
		current_oid_map := make(map[string]bool)
		for _, dr := range data {
			current_oid_map[dr["tag_sproc"].(string)+DB_METRIC_JOIN_STR+dr["tag_oid"].(string)] = true
		}
		for sproc_ident := range host_state["sproc_hashes"] {
			_, ok := current_oid_map[sproc_ident]
			if !ok {
				splits := strings.Split(sproc_ident, DB_METRIC_JOIN_STR)
				log.Info("detected delete of sproc:", splits[0], ", oid:", splits[1])
				influx_entry := make(map[string]interface{})
				influx_entry["event"] = "drop"
				influx_entry["tag_sproc"] = splits[0]
				influx_entry["tag_oid"] = splits[1]
				if len(data) > 0 {
					influx_entry["epoch_ns"] = data[0]["epoch_ns"]
				} else {
					influx_entry["epoch_ns"] = time.Now().UnixNano()
				}
				detected_changes = append(detected_changes, influx_entry)
				deleted_sprocs = append(deleted_sprocs, sproc_ident)
				change_counts.Dropped += 1
			}
		}
		for _, deleted_sproc := range deleted_sprocs {
			delete(host_state["sproc_hashes"], deleted_sproc)
		}
	}
	log.Debugf("[%s][%s] detected %d sproc changes", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, len(detected_changes))
	if len(detected_changes) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storage_ch <- []MetricStoreMessage{MetricStoreMessage{DBUniqueName: dbUnique, MetricName: "sproc_changes", Data: detected_changes, CustomTags: md.CustomTags}}
	} else if opts.Datastore == DATASTORE_POSTGRES && first_run {
		EnsureMetricDummy("sproc_changes")
	}

	return change_counts
}

func DetectTableChanges(dbUnique string, vme DBVersionMapEntry, storage_ch chan<- []MetricStoreMessage, host_state map[string]map[string]string) ChangeDetectionResults {
	detected_changes := make([](map[string]interface{}), 0)
	var first_run bool
	var change_counts ChangeDetectionResults

	log.Debugf("[%s][%s] checking for table changes...", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS)
	if _, ok := host_state["table_hashes"]; !ok {
		first_run = true
		host_state["table_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("table_hashes", vme, nil)
	if err != nil {
		log.Error("could not get table_hashes sql:", err)
		return change_counts
	}

	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "table_hashes", mvp.MetricAttrs.StatementTimeoutSeconds, mvp.Sql)
	if err != nil {
		log.Error("could not read table_hashes from monitored host:", dbUnique, ", err:", err)
		return change_counts
	}

	for _, dr := range data {
		obj_ident := dr["tag_table"].(string)
		prev_hash, ok := host_state["table_hashes"][obj_ident]
		//log.Debug("inspecting table:", obj_ident, "hash:", prev_hash)
		if ok { // we have existing state
			if prev_hash != dr["md5"].(string) {
				log.Info("detected DDL change in table:", dr["tag_table"])
				dr["event"] = "alter"
				detected_changes = append(detected_changes, dr)
				host_state["table_hashes"][obj_ident] = dr["md5"].(string)
				change_counts.Altered += 1
			}
		} else { // check for new / delete
			if !first_run {
				log.Info("detected new table:", dr["tag_table"])
				dr["event"] = "create"
				detected_changes = append(detected_changes, dr)
				change_counts.Created += 1
			}
			host_state["table_hashes"][obj_ident] = dr["md5"].(string)
		}
	}
	// detect deletes
	if !first_run && len(host_state["table_hashes"]) != len(data) {
		deleted_tables := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		current_table_map := make(map[string]bool)
		for _, dr := range data {
			current_table_map[dr["tag_table"].(string)] = true
		}
		for table := range host_state["table_hashes"] {
			_, ok := current_table_map[table]
			if !ok {
				log.Info("detected drop of table:", table)
				influx_entry := make(map[string]interface{})
				influx_entry["event"] = "drop"
				influx_entry["tag_table"] = table
				if len(data) > 0 {
					influx_entry["epoch_ns"] = data[0]["epoch_ns"]
				} else {
					influx_entry["epoch_ns"] = time.Now().UnixNano()
				}
				detected_changes = append(detected_changes, influx_entry)
				deleted_tables = append(deleted_tables, table)
				change_counts.Dropped += 1
			}
		}
		for _, deleted_table := range deleted_tables {
			delete(host_state["table_hashes"], deleted_table)
		}
	}

	log.Debugf("[%s][%s] detected %d table changes", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, len(detected_changes))
	if len(detected_changes) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storage_ch <- []MetricStoreMessage{MetricStoreMessage{DBUniqueName: dbUnique, MetricName: "table_changes", Data: detected_changes, CustomTags: md.CustomTags}}
	} else if opts.Datastore == DATASTORE_POSTGRES && first_run {
		EnsureMetricDummy("table_changes")
	}

	return change_counts
}

func DetectIndexChanges(dbUnique string, vme DBVersionMapEntry, storage_ch chan<- []MetricStoreMessage, host_state map[string]map[string]string) ChangeDetectionResults {
	detected_changes := make([](map[string]interface{}), 0)
	var first_run bool
	var change_counts ChangeDetectionResults

	log.Debugf("[%s][%s] checking for index changes...", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS)
	if _, ok := host_state["index_hashes"]; !ok {
		first_run = true
		host_state["index_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("index_hashes", vme, nil)
	if err != nil {
		log.Error("could not get index_hashes sql:", err)
		return change_counts
	}

	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "index_hashes", mvp.MetricAttrs.StatementTimeoutSeconds, mvp.Sql)
	if err != nil {
		log.Error("could not read index_hashes from monitored host:", dbUnique, ", err:", err)
		return change_counts
	}

	for _, dr := range data {
		obj_ident := dr["tag_index"].(string)
		prev_hash, ok := host_state["index_hashes"][obj_ident]
		if ok { // we have existing state
			if prev_hash != (dr["md5"].(string) + dr["is_valid"].(string)) {
				log.Info("detected index change:", dr["tag_index"], ", table:", dr["table"])
				dr["event"] = "alter"
				detected_changes = append(detected_changes, dr)
				host_state["index_hashes"][obj_ident] = dr["md5"].(string) + dr["is_valid"].(string)
				change_counts.Altered += 1
			}
		} else { // check for new / delete
			if !first_run {
				log.Info("detected new index:", dr["tag_index"])
				dr["event"] = "create"
				detected_changes = append(detected_changes, dr)
				change_counts.Created += 1
			}
			host_state["index_hashes"][obj_ident] = dr["md5"].(string) + dr["is_valid"].(string)
		}
	}
	// detect deletes
	if !first_run && len(host_state["index_hashes"]) != len(data) {
		deleted_indexes := make([]string, 0)
		// turn resultset to map => [table]=true for faster checks
		current_index_map := make(map[string]bool)
		for _, dr := range data {
			current_index_map[dr["tag_index"].(string)] = true
		}
		for index_name := range host_state["index_hashes"] {
			_, ok := current_index_map[index_name]
			if !ok {
				log.Info("detected drop of index_name:", index_name)
				influx_entry := make(map[string]interface{})
				influx_entry["event"] = "drop"
				influx_entry["tag_index"] = index_name
				if len(data) > 0 {
					influx_entry["epoch_ns"] = data[0]["epoch_ns"]
				} else {
					influx_entry["epoch_ns"] = time.Now().UnixNano()
				}
				detected_changes = append(detected_changes, influx_entry)
				deleted_indexes = append(deleted_indexes, index_name)
				change_counts.Dropped += 1
			}
		}
		for _, deleted_index := range deleted_indexes {
			delete(host_state["index_hashes"], deleted_index)
		}
	}
	log.Debugf("[%s][%s] detected %d index changes", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, len(detected_changes))
	if len(detected_changes) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storage_ch <- []MetricStoreMessage{MetricStoreMessage{DBUniqueName: dbUnique, MetricName: "index_changes", Data: detected_changes, CustomTags: md.CustomTags}}
	} else if opts.Datastore == DATASTORE_POSTGRES && first_run {
		EnsureMetricDummy("index_changes")
	}

	return change_counts
}

func DetectPrivilegeChanges(dbUnique string, vme DBVersionMapEntry, storage_ch chan<- []MetricStoreMessage, host_state map[string]map[string]string) ChangeDetectionResults {
	detected_changes := make([](map[string]interface{}), 0)
	var first_run bool
	var change_counts ChangeDetectionResults

	log.Debugf("[%s][%s] checking object privilege changes...", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS)
	if _, ok := host_state["object_privileges"]; !ok {
		first_run = true
		host_state["object_privileges"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("privilege_changes", vme, nil)
	if err != nil || mvp.Sql == "" {
		log.Warningf("[%s][%s] could not get SQL for 'privilege_changes'. cannot detect privilege changes", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS)
		return change_counts
	}

	// returns rows of: object_type, tag_role, tag_object, privilege_type
	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "privilege_changes", mvp.MetricAttrs.StatementTimeoutSeconds, mvp.Sql)
	if err != nil {
		log.Errorf("[%s][%s] failed to fetch object privileges info: %v", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, err)
		return change_counts
	}

	current_state := make(map[string]bool)
	for _, dr := range data {
		obj_ident := fmt.Sprintf("%s#:#%s#:#%s#:#%s", dr["object_type"], dr["tag_role"], dr["tag_object"], dr["privilege_type"])
		if first_run {
			host_state["object_privileges"][obj_ident] = ""
		} else {
			_, ok := host_state["object_privileges"][obj_ident]
			if !ok {
				log.Infof("[%s][%s] detected new object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, dr["tag_role"], dr["object_type"], dr["tag_object"], dr["privilege_type"])
				dr["event"] = "GRANT"
				detected_changes = append(detected_changes, dr)
				change_counts.Created += 1
				host_state["object_privileges"][obj_ident] = ""
			}
			current_state[obj_ident] = true
		}
	}
	// check revokes - exists in old state only
	if !first_run && len(current_state) > 0 {
		for obj_prev_run := range host_state["object_privileges"] {
			if _, ok := current_state[obj_prev_run]; !ok {
				splits := strings.Split(obj_prev_run, "#:#")
				log.Infof("[%s][%s] detected removed object privileges: role=%s, object_type=%s, object=%s, privilege_type=%s",
					dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, splits[1], splits[0], splits[2], splits[3])
				revoke_entry := make(map[string]interface{})
				if epoch_ns, ok := data[0]["epoch_ns"]; ok {
					revoke_entry["epoch_ns"] = epoch_ns
				} else {
					revoke_entry["epoch_ns"] = time.Now().UnixNano()
				}
				revoke_entry["object_type"] = splits[0]
				revoke_entry["tag_role"] = splits[1]
				revoke_entry["tag_object"] = splits[2]
				revoke_entry["privilege_type"] = splits[3]
				revoke_entry["event"] = "REVOKE"
				detected_changes = append(detected_changes, revoke_entry)
				change_counts.Dropped += 1
				delete(host_state["object_privileges"], obj_prev_run)
			}
		}
	}

	if opts.Datastore == DATASTORE_POSTGRES && first_run {
		EnsureMetricDummy("privilege_changes")
	}
	log.Debugf("[%s][%s] detected %d object privilege changes...", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, len(detected_changes))
	if len(detected_changes) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storage_ch <- []MetricStoreMessage{MetricStoreMessage{DBUniqueName: dbUnique, MetricName: "privilege_changes", Data: detected_changes, CustomTags: md.CustomTags}}
	}

	return change_counts
}

func DetectConfigurationChanges(dbUnique string, vme DBVersionMapEntry, storage_ch chan<- []MetricStoreMessage, host_state map[string]map[string]string) ChangeDetectionResults {
	detected_changes := make([](map[string]interface{}), 0)
	var first_run bool
	var change_counts ChangeDetectionResults

	log.Debugf("[%s][%s] checking for configuration changes...", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS)
	if _, ok := host_state["configuration_hashes"]; !ok {
		first_run = true
		host_state["configuration_hashes"] = make(map[string]string)
	}

	mvp, err := GetMetricVersionProperties("configuration_hashes", vme, nil)
	if err != nil {
		log.Errorf("[%s][%s] could not get configuration_hashes sql: %v", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, err)
		return change_counts
	}

	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "configuration_hashes", mvp.MetricAttrs.StatementTimeoutSeconds, mvp.Sql)
	if err != nil {
		log.Errorf("[%s][%s] could not read configuration_hashes from monitored host: %v", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, err)
		return change_counts
	}

	for _, dr := range data {
		obj_ident := dr["tag_setting"].(string)
		obj_value := dr["value"].(string)
		prev_hash, ok := host_state["configuration_hashes"][obj_ident]
		if ok { // we have existing state
			if prev_hash != obj_value {
				if obj_ident == "connection_ID" {
					continue // ignore some weird Azure managed PG service setting
				}
				log.Warningf("[%s][%s] detected settings change: %s = %s (prev: %s)",
					dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, obj_ident, obj_value, prev_hash)
				dr["event"] = "alter"
				detected_changes = append(detected_changes, dr)
				host_state["configuration_hashes"][obj_ident] = obj_value
				change_counts.Altered += 1
			}
		} else { // check for new, delete not relevant here (pg_upgrade)
			if !first_run {
				log.Warningf("[%s][%s] detected new setting: %s", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, obj_ident)
				dr["event"] = "create"
				detected_changes = append(detected_changes, dr)
				change_counts.Created += 1
			}
			host_state["configuration_hashes"][obj_ident] = obj_value
		}
	}

	log.Debugf("[%s][%s] detected %d configuration changes", dbUnique, SPECIAL_METRIC_CHANGE_EVENTS, len(detected_changes))
	if len(detected_changes) > 0 {
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storage_ch <- []MetricStoreMessage{MetricStoreMessage{DBUniqueName: dbUnique, MetricName: "configuration_changes", Data: detected_changes, CustomTags: md.CustomTags}}
	} else if opts.Datastore == DATASTORE_POSTGRES {
		EnsureMetricDummy("configuration_changes")
	}

	return change_counts
}

func CheckForPGObjectChangesAndStore(dbUnique string, vme DBVersionMapEntry, storage_ch chan<- []MetricStoreMessage, host_state map[string]map[string]string) {
	sproc_counts := DetectSprocChanges(dbUnique, vme, storage_ch, host_state) // TODO some of Detect*() code could be unified...
	table_counts := DetectTableChanges(dbUnique, vme, storage_ch, host_state)
	index_counts := DetectIndexChanges(dbUnique, vme, storage_ch, host_state)
	conf_counts := DetectConfigurationChanges(dbUnique, vme, storage_ch, host_state)
	priv_change_counts := DetectPrivilegeChanges(dbUnique, vme, storage_ch, host_state)

	if opts.Datastore == DATASTORE_POSTGRES {
		EnsureMetricDummy("object_changes")
	}

	// need to send info on all object changes as one message as Grafana applies "last wins" for annotations with similar timestamp
	message := ""
	if sproc_counts.Altered > 0 || sproc_counts.Created > 0 || sproc_counts.Dropped > 0 {
		message += fmt.Sprintf(" sprocs %d/%d/%d", sproc_counts.Created, sproc_counts.Altered, sproc_counts.Dropped)
	}
	if table_counts.Altered > 0 || table_counts.Created > 0 || table_counts.Dropped > 0 {
		message += fmt.Sprintf(" tables/views %d/%d/%d", table_counts.Created, table_counts.Altered, table_counts.Dropped)
	}
	if index_counts.Altered > 0 || index_counts.Created > 0 || index_counts.Dropped > 0 {
		message += fmt.Sprintf(" indexes %d/%d/%d", index_counts.Created, index_counts.Altered, index_counts.Dropped)
	}
	if conf_counts.Altered > 0 || conf_counts.Created > 0 {
		message += fmt.Sprintf(" configuration %d/%d/%d", conf_counts.Created, conf_counts.Altered, conf_counts.Dropped)
	}
	if priv_change_counts.Dropped > 0 || priv_change_counts.Created > 0 {
		message += fmt.Sprintf(" privileges %d/%d/%d", priv_change_counts.Created, priv_change_counts.Altered, priv_change_counts.Dropped)
	}

	if message > "" {
		message = "Detected changes for \"" + dbUnique + "\" [Created/Altered/Dropped]:" + message
		log.Info(message)
		detected_changes_summary := make([](map[string]interface{}), 0)
		influx_entry := make(map[string]interface{})
		influx_entry["details"] = message
		influx_entry["epoch_ns"] = time.Now().UnixNano()
		detected_changes_summary = append(detected_changes_summary, influx_entry)
		md, _ := GetMonitoredDatabaseByUniqueName(dbUnique)
		storage_ch <- []MetricStoreMessage{MetricStoreMessage{DBUniqueName: dbUnique, DBType: md.DBType, MetricName: "object_changes", Data: detected_changes_summary, CustomTags: md.CustomTags}}
	}
}

// some extra work needed as pgpool SHOW commands don't specify the return data types for some reason
func FetchMetricsPgpool(msg MetricFetchMessage, vme DBVersionMapEntry, mvp MetricVersionProperties) ([]map[string]interface{}, error, time.Duration) {
	var ret_data = make([]map[string]interface{}, 0)
	var duration time.Duration
	epoch_ns := time.Now().UnixNano()

	sql_lines := strings.Split(strings.ToUpper(mvp.Sql), "\n")

	for _, sql := range sql_lines {
		if strings.HasPrefix(sql, "SHOW POOL_NODES") {
			data, err, dur := DBExecReadByDbUniqueName(msg.DBUniqueName, msg.MetricName, 0, sql)
			duration = duration + dur
			if err != nil {
				log.Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				return data, err, duration
			}

			for _, row := range data {
				ret_row := make(map[string]interface{})
				ret_row[EPOCH_COLUMN_NAME] = epoch_ns
				for k, v := range row {
					vs := string(v.([]byte))
					// need 1 tag so that Influx would not merge rows
					if k == "node_id" {
						ret_row["tag_node_id"] = vs
						continue
					}

					ret_row[k] = vs
					if k == "status" { // was changed from numeric to string at some pgpool version so leave the string
						// but also add "status_num" field
						if vs == "up" {
							ret_row["status_num"] = 1
						} else if vs == "down" {
							ret_row["status_num"] = 0
						} else {
							i, err := strconv.ParseInt(vs, 10, 64)
							if err == nil {
								ret_row["status_num"] = i
							}
						}
						continue
					}
					// everything is returned as text, so try to convert all numerics into ints / floats
					if k != "lb_weight" {
						i, err := strconv.ParseInt(vs, 10, 64)
						if err == nil {
							ret_row[k] = i
							continue
						}
					}
					f, err := strconv.ParseFloat(vs, 64)
					if err == nil {
						ret_row[k] = f
						continue
					}
				}
				ret_data = append(ret_data, ret_row)
			}
		} else if strings.HasPrefix(sql, "SHOW POOL_PROCESSES") {
			if len(ret_data) == 0 {
				log.Warningf("[%s][%s] SHOW POOL_NODES needs to be placed before SHOW POOL_PROCESSES. ignoring SHOW POOL_PROCESSES", msg.DBUniqueName, msg.MetricName)
				continue
			}

			data, err, dur := DBExecReadByDbUniqueName(msg.DBUniqueName, msg.MetricName, 0, sql)
			duration = duration + dur
			if err != nil {
				log.Errorf("[%s][%s] Could not fetch PgPool statistics: %v", msg.DBUniqueName, msg.MetricName, err)
				continue
			}

			// summarize processes_total / processes_active over all rows
			processes_total := 0
			processes_active := 0
			for _, row := range data {
				processes_total++
				v, ok := row["database"]
				if !ok {
					log.Infof("[%s][%s] column 'database' not found from data returned by SHOW POOL_PROCESSES, check pool version / SQL definition", msg.DBUniqueName, msg.MetricName)
					continue
				}
				if len(v.([]byte)) > 0 {
					processes_active++
				}
			}

			for _, ret_row := range ret_data {
				ret_row["processes_total"] = processes_total
				ret_row["processes_active"] = processes_active
			}
		}
	}

	//log.Fatalf("%+v", ret_data)
	return ret_data, nil, duration
}

func ReadMetricDefinitionMapFromPostgres(failOnError bool) (map[string]map[decimal.Decimal]MetricVersionProperties, error) {
	metric_def_map_new := make(map[string]map[decimal.Decimal]MetricVersionProperties)
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

	log.Info("updating metrics definitons from ConfigDB...")
	data, err := DBExecRead(configDb, CONFIGDB_IDENT, sql)
	if err != nil {
		if failOnError {
			log.Fatal(err)
		} else {
			log.Error(err)
			return metric_def_map, err
		}
	}
	if len(data) == 0 {
		log.Warning("no active metric definitions found from config DB")
		return metric_def_map_new, err
	}

	log.Debug(len(data), "active metrics found from config db (pgwatch3.metric)")
	for _, row := range data {
		_, ok := metric_def_map_new[row["m_name"].(string)]
		if !ok {
			metric_def_map_new[row["m_name"].(string)] = make(map[decimal.Decimal]MetricVersionProperties)
		}
		d, _ := decimal.NewFromString(row["m_pg_version_from"].(string))
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
		metric_def_map_new[row["m_name"].(string)][d] = MetricVersionProperties{
			Sql:                  row["m_sql"].(string),
			SqlSU:                row["m_sql_su"].(string),
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

	return metric_def_map_new, err
}

func DoesFunctionExists(dbUnique, functionName string) bool {
	log.Debug("Checking for function existence", dbUnique, functionName)
	sql := fmt.Sprintf("select /* pgwatch3_generated */ 1 from pg_proc join pg_namespace n on pronamespace = n.oid where proname = '%s' and n.nspname = 'public'", functionName)
	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sql)
	if err != nil {
		log.Error("Failed to check for function existence", dbUnique, functionName, err)
		return false
	}
	if len(data) > 0 {
		log.Debugf("Function %s exists on %s", functionName, dbUnique)
		return true
	}
	return false
}

// Called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
// NB! With newer Postgres version can even succeed if the user is not a real superuser due to some cloud-specific
// whitelisting or "trusted extensions" (a feature from v13). Ignores errors.
func TryCreateMissingExtensions(dbUnique string, extensionNames []string, existingExtensions map[string]decimal.Decimal) []string {
	sqlAvailable := `select name::text from pg_available_extensions`
	extsCreated := make([]string, 0)

	// For security reasons don't allow to execute random strings but check that it's an existing extension
	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sqlAvailable)
	if err != nil {
		log.Infof("[%s] Failed to get a list of available extensions: %v", dbUnique, err)
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
			log.Errorf("[%s] Requested extension %s not available on instance, cannot try to create...", dbUnique, extToCreate)
		} else {
			sqlCreateExt := `create extension ` + extToCreate
			_, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sqlCreateExt)
			if err != nil {
				log.Errorf("[%s] Failed to create extension %s (based on --try-create-listed-exts-if-missing input): %v", dbUnique, extToCreate, err)
			}
			extsCreated = append(extsCreated, extToCreate)
		}
	}

	return extsCreated
}

// Called once on daemon startup to try to create "metric fething helper" functions automatically
func TryCreateMetricsFetchingHelpers(dbUnique string) error {
	db_pg_version, err := DBGetPGVersion(dbUnique, DBTYPE_PG, false)
	if err != nil {
		log.Errorf("Failed to fetch pg version for \"%s\": %s", dbUnique, err)
		return err
	}

	if fileBasedMetrics {
		helpers, err := ReadMetricsFromFolder(path.Join(opts.MetricsFolder, FILE_BASED_METRIC_HELPERS_DIR), false)
		if err != nil {
			log.Errorf("Failed to fetch helpers from \"%s\": %s", path.Join(opts.MetricsFolder, FILE_BASED_METRIC_HELPERS_DIR), err)
			return err
		}
		log.Debug("%d helper definitions found from \"%s\"...", len(helpers), path.Join(opts.MetricsFolder, FILE_BASED_METRIC_HELPERS_DIR))

		for helperName := range helpers {
			if strings.Contains(helperName, "windows") {
				log.Infof("Skipping %s rollout. Windows helpers need to be rolled out manually", helperName)
				continue
			}
			if !DoesFunctionExists(dbUnique, helperName) {

				log.Debug("Trying to create metric fetching helpers for", dbUnique, helperName)
				mvp, err := GetMetricVersionProperties(helperName, db_pg_version, helpers)
				if err != nil {
					log.Warning("Could not find query text for", dbUnique, helperName)
					continue
				}
				_, err, _ = DBExecReadByDbUniqueName(dbUnique, "", 0, mvp.Sql)
				if err != nil {
					log.Warning("Failed to create a metric fetching helper for", dbUnique, helperName)
					log.Warning(err)
				} else {
					log.Info("Successfully created metric fetching helper for", dbUnique, helperName)
				}
			}
		}

	} else {
		sql_helpers := "select /* pgwatch3_generated */ distinct m_name from pgwatch3.metric where m_is_active and m_is_helper" // m_name is a helper function name
		data, err := DBExecRead(configDb, CONFIGDB_IDENT, sql_helpers)
		if err != nil {
			log.Error(err)
			return err
		}
		for _, row := range data {
			metric := row["m_name"].(string)

			if strings.Contains(metric, "windows") {
				log.Infof("Skipping %s rollout. Windows helpers need to be rolled out manually", metric)
				continue
			}
			if !DoesFunctionExists(dbUnique, metric) {

				log.Debug("Trying to create metric fetching helpers for", dbUnique, metric)
				mvp, err := GetMetricVersionProperties(metric, db_pg_version, nil)
				if err != nil {
					log.Warning("Could not find query text for", dbUnique, metric)
					continue
				}
				_, err, _ = DBExecReadByDbUniqueName(dbUnique, "", 0, mvp.Sql)
				if err != nil {
					log.Warning("Failed to create a metric fetching helper for", dbUnique, metric)
					log.Warning(err)
				} else {
					log.Warning("Successfully created metric fetching helper for", dbUnique, metric)
				}
			}
		}
	}
	return nil
}

// "resolving" reads all the DB names from the given host/port, additionally matching/not matching specified regex patterns
func ResolveDatabasesFromConfigEntry(ce MonitoredDatabase) ([]MonitoredDatabase, error) {
	var c *sqlx.DB
	var err error
	md := make([]MonitoredDatabase, 0)

	// some cloud providers limit access to template1 for some reason, so try with postgres and defaultdb (Aiven)
	templateDBsToTry := []string{"template1", "postgres", "defaultdb"}

	for _, templateDB := range templateDBsToTry {
		c, err = GetPostgresDBConnection(ce.LibPQConnStr, ce.Host, ce.Port, templateDB, ce.User, ce.Password,
			ce.SslMode, ce.SslRootCAPath, ce.SslClientCertPath, ce.SslClientKeyPath)
		if err != nil {
			return md, err
		}
		err = c.Ping()
		if err == nil {
			break
		} else {
			c.Close()
		}
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

	data, err := DBExecRead(c, ce.DBUniqueName, sql, ce.DBNameIncludePattern, ce.DBNameIncludePattern, ce.DBNameExcludePattern, ce.DBNameExcludePattern)
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
func GetGoPsutilDiskPG(dbUnique string) ([]map[string]interface{}, error) {
	sql := `select current_setting('data_directory') as dd, current_setting('log_directory') as ld, current_setting('server_version_num')::int as pgver`
	sqlTS := `select spcname::text as name, pg_catalog.pg_tablespace_location(oid) as location from pg_catalog.pg_tablespace where not spcname like any(array[E'pg\\_%'])`
	var ddDevice, ldDevice, walDevice uint64

	data, err, _ := DBExecReadByDbUniqueName(dbUnique, "", 0, sql)
	if err != nil || len(data) == 0 {
		log.Errorf("Failed to determine relevant PG disk paths via SQL: %v", err)
		return nil, err
	}

	dataDirPath := data[0]["dd"].(string)
	ddUsage, err := disk.Usage(dataDirPath)
	if err != nil {
		log.Errorf("Could not determine disk usage for path %v: %v", dataDirPath, err)
		return nil, err
	}

	retRows := make([]map[string]interface{}, 0)
	epoch_ns := time.Now().UnixNano()
	dd := make(map[string]interface{})
	dd["epoch_ns"] = epoch_ns
	dd["tag_dir_or_tablespace"] = "data_directory"
	dd["tag_path"] = dataDirPath
	dd["total"] = float64(ddUsage.Total)
	dd["used"] = float64(ddUsage.Used)
	dd["free"] = float64(ddUsage.Free)
	dd["percent"] = math.Round(100*ddUsage.UsedPercent) / 100
	retRows = append(retRows, dd)

	ddDevice, err = getPathUnderlyingDeviceId(dataDirPath)
	if err != nil {
		log.Errorf("Could not determine disk device ID of data_directory %v: %v", dataDirPath, err)
	}

	logDirPath := data[0]["ld"].(string)
	if !strings.HasPrefix(logDirPath, "/") {
		logDirPath = path.Join(dataDirPath, logDirPath)
	}
	if len(logDirPath) > 0 && CheckFolderExistsAndReadable(logDirPath) { // syslog etc considered out of scope
		ldDevice, err = getPathUnderlyingDeviceId(logDirPath)
		if err != nil {
			log.Infof("Could not determine disk device ID of log_directory %v: %v", logDirPath, err)
		}
		if err != nil || ldDevice != ddDevice { // no point to report same data in case of single folder configuration
			ld := make(map[string]interface{})
			ldUsage, err := disk.Usage(logDirPath)
			if err != nil {
				log.Infof("Could not determine disk usage for path %v: %v", logDirPath, err)
			} else {
				ld["epoch_ns"] = epoch_ns
				ld["tag_dir_or_tablespace"] = "log_directory"
				ld["tag_path"] = logDirPath
				ld["total"] = float64(ldUsage.Total)
				ld["used"] = float64(ldUsage.Used)
				ld["free"] = float64(ldUsage.Free)
				ld["percent"] = math.Round(100*ldUsage.UsedPercent) / 100
				retRows = append(retRows, ld)
			}
		}
	}

	var walDirPath string
	if CheckFolderExistsAndReadable(path.Join(dataDirPath, "pg_wal")) {
		walDirPath = path.Join(dataDirPath, "pg_wal")
	} else if CheckFolderExistsAndReadable(path.Join(dataDirPath, "pg_xlog")) {
		walDirPath = path.Join(dataDirPath, "pg_xlog") // < v10
	}

	if len(walDirPath) > 0 {
		walDevice, err = getPathUnderlyingDeviceId(walDirPath)
		if err != nil {
			log.Infof("Could not determine disk device ID of WAL directory %v: %v", walDirPath, err) // storing anyways
		}

		if err != nil || walDevice != ddDevice || walDevice != ldDevice { // no point to report same data in case of single folder configuration
			walUsage, err := disk.Usage(walDirPath)
			if err != nil {
				log.Errorf("Could not determine disk usage for WAL directory %v: %v", walDirPath, err)
			} else {
				wd := make(map[string]interface{})
				wd["epoch_ns"] = epoch_ns
				wd["tag_dir_or_tablespace"] = "pg_wal"
				wd["tag_path"] = walDirPath
				wd["total"] = float64(walUsage.Total)
				wd["used"] = float64(walUsage.Used)
				wd["free"] = float64(walUsage.Free)
				wd["percent"] = math.Round(100*walUsage.UsedPercent) / 100
				retRows = append(retRows, wd)
			}
		}
	}

	data, err, _ = DBExecReadByDbUniqueName(dbUnique, "", 0, sqlTS)
	if err != nil {
		log.Infof("Failed to determine relevant PG tablespace paths via SQL: %v", err)
	} else if len(data) > 0 {
		for _, row := range data {
			tsPath := row["location"].(string)
			tsName := row["name"].(string)

			tsDevice, err := getPathUnderlyingDeviceId(tsPath)
			if err != nil {
				log.Errorf("Could not determine disk device ID of tablespace %s (%s): %v", tsName, tsPath, err)
				continue
			}

			if tsDevice == ddDevice || tsDevice == ldDevice || tsDevice == walDevice {
				continue
			}
			tsUsage, err := disk.Usage(tsPath)
			if err != nil {
				log.Errorf("Could not determine disk usage for tablespace %s, directory %s: %v", row["name"].(string), row["location"].(string), err)
			}
			ts := make(map[string]interface{})
			ts["epoch_ns"] = epoch_ns
			ts["tag_dir_or_tablespace"] = tsName
			ts["tag_path"] = tsPath
			ts["total"] = float64(tsUsage.Total)
			ts["used"] = float64(tsUsage.Used)
			ts["free"] = float64(tsUsage.Free)
			ts["percent"] = math.Round(100*tsUsage.UsedPercent) / 100
			retRows = append(retRows, ts)
		}
	}

	return retRows, nil
}

func SendToPostgres(storeMessages []MetricStoreMessage) error {
	if len(storeMessages) == 0 {
		return nil
	}
	ts_warning_printed := false
	metricsToStorePerMetric := make(map[string][]MetricStoreMessagePostgres)
	rows_batched := 0
	total_rows := 0
	pg_part_bounds := make(map[string]ExistingPartitionInfo)                   // metric=min/max
	pg_part_bounds_dbname := make(map[string]map[string]ExistingPartitionInfo) // metric=[dbname=min/max]
	var err error

	if PGSchemaType == "custom" {
		metricsToStorePerMetric["metrics"] = make([]MetricStoreMessagePostgres, 0) // everything inserted into "metrics".
		// TODO  warn about collision if someone really names some new metric "metrics"
	}

	for _, msg := range storeMessages {
		if msg.Data == nil || len(msg.Data) == 0 {
			continue
		}
		log.Debug("SendToPG data[0] of ", len(msg.Data), ":", msg.Data[0])

		for _, dr := range msg.Data {
			var epoch_time time.Time
			var epoch_ns int64

			tags := make(map[string]interface{})
			fields := make(map[string]interface{})

			total_rows += 1

			if msg.CustomTags != nil {
				for k, v := range msg.CustomTags {
					tags[k] = fmt.Sprintf("%v", v)
				}
			}

			for k, v := range dr {
				if v == nil || v == "" {
					continue // not storing NULLs
				}
				if k == EPOCH_COLUMN_NAME {
					epoch_ns = v.(int64)
				} else if strings.HasPrefix(k, TAG_PREFIX) {
					tag := k[4:]
					tags[tag] = fmt.Sprintf("%v", v)
				} else {
					fields[k] = v
				}
			}

			if epoch_ns == 0 {
				if !ts_warning_printed && msg.MetricName != SPECIAL_METRIC_PGBOUNCER_STATS {
					log.Warning("No timestamp_ns found, server time will be used. measurement:", msg.MetricName)
					ts_warning_printed = true
				}
				epoch_time = time.Now()
			} else {
				epoch_time = time.Unix(0, epoch_ns)
			}

			var metricsArr []MetricStoreMessagePostgres
			var ok bool
			var metricNameTemp string

			if PGSchemaType == "custom" {
				metricNameTemp = "metrics"
			} else {
				metricNameTemp = msg.MetricName
			}

			metricsArr, ok = metricsToStorePerMetric[metricNameTemp]
			if !ok {
				metricsToStorePerMetric[metricNameTemp] = make([]MetricStoreMessagePostgres, 0)
			}
			metricsArr = append(metricsArr, MetricStoreMessagePostgres{Time: epoch_time, DBName: msg.DBUniqueName,
				Metric: msg.MetricName, Data: fields, TagData: tags})
			metricsToStorePerMetric[metricNameTemp] = metricsArr

			rows_batched += 1

			if PGSchemaType == "metric" || PGSchemaType == "metric-time" || PGSchemaType == "timescale" {
				// set min/max timestamps to check/create partitions
				bounds, ok := pg_part_bounds[msg.MetricName]
				if !ok || (ok && epoch_time.Before(bounds.StartTime)) {
					bounds.StartTime = epoch_time
					pg_part_bounds[msg.MetricName] = bounds
				}
				if !ok || (ok && epoch_time.After(bounds.EndTime)) {
					bounds.EndTime = epoch_time
					pg_part_bounds[msg.MetricName] = bounds
				}
			} else if PGSchemaType == "metric-dbname-time" {
				_, ok := pg_part_bounds_dbname[msg.MetricName]
				if !ok {
					pg_part_bounds_dbname[msg.MetricName] = make(map[string]ExistingPartitionInfo)
				}
				bounds, ok := pg_part_bounds_dbname[msg.MetricName][msg.DBUniqueName]
				if !ok || (ok && epoch_time.Before(bounds.StartTime)) {
					bounds.StartTime = epoch_time
					pg_part_bounds_dbname[msg.MetricName][msg.DBUniqueName] = bounds
				}
				if !ok || (ok && epoch_time.After(bounds.EndTime)) {
					bounds.EndTime = epoch_time
					pg_part_bounds_dbname[msg.MetricName][msg.DBUniqueName] = bounds
				}
			}
		}
	}

	if PGSchemaType == "metric" {
		err = EnsureMetric(pg_part_bounds, forceRecreatePGMetricPartitions)
	} else if PGSchemaType == "metric-time" {
		err = EnsureMetricTime(pg_part_bounds, forceRecreatePGMetricPartitions, false)
	} else if PGSchemaType == "metric-dbname-time" {
		err = EnsureMetricDbnameTime(pg_part_bounds_dbname, forceRecreatePGMetricPartitions)
	} else if PGSchemaType == "timescale" {
		err = EnsureMetricTimescale(pg_part_bounds, forceRecreatePGMetricPartitions)
	} else {
		log.Fatal("should never happen...")
	}
	if forceRecreatePGMetricPartitions {
		forceRecreatePGMetricPartitions = false
	}
	if err != nil {
		atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
		return err
	}

	// send data to PG, with a separate COPY for all metrics
	log.Debugf("COPY-ing %d metrics to Postgres metricsDB...", rows_batched)
	t1 := time.Now()

	txn, err := metricDb.Begin()
	if err != nil {
		log.Error("Could not start Postgres metricsDB transaction:", err)
		atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
		return err
	}
	defer func() {
		if err == nil {
			tx_err := txn.Commit()
			if tx_err != nil {
				log.Debug("COPY Commit to Postgres failed:", tx_err)
			}
		} else {
			tx_err := txn.Rollback()
			if tx_err != nil {
				log.Debug("COPY Rollback to Postgres failed:", tx_err)
			}
		}
	}()

	for metricName, metrics := range metricsToStorePerMetric {
		var stmt *sql.Stmt

		if PGSchemaType == "custom" {
			stmt, err = txn.Prepare(pq.CopyIn("metrics", "time", "dbname", "metric", "data", "tag_data"))
			if err != nil {
				log.Error("Could not prepare COPY to 'metrics' table:", err)
				atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
				return err
			}
		} else {
			log.Debugf("COPY-ing %d rows into '%s'...", len(metrics), metricName)
			stmt, err = txn.Prepare(pq.CopyIn(metricName, "time", "dbname", "data", "tag_data"))
			if err != nil {
				log.Errorf("Could not prepare COPY to '%s' table: %v", metricName, err)
				atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
				return err
			}
		}

		for _, m := range metrics {
			jsonBytes, err := mapToJson(m.Data)
			if err != nil {
				log.Errorf("Skipping 1 metric for [%s:%s] due to JSON conversion error: %s", m.DBName, m.Metric, err)
				atomic.AddUint64(&totalMetricsDroppedCounter, 1)
				continue
			}

			if len(m.TagData) > 0 {
				jsonBytesTags, err := mapToJson(m.TagData)
				if err != nil {
					log.Errorf("Skipping 1 metric for [%s:%s] due to JSON conversion error: %s", m.DBName, m.Metric, err)
					atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
					goto stmt_close
				}
				if PGSchemaType == "custom" {
					_, err = stmt.Exec(m.Time, m.DBName, m.Metric, string(jsonBytes), string(jsonBytesTags))
				} else {
					_, err = stmt.Exec(m.Time, m.DBName, string(jsonBytes), string(jsonBytesTags))
				}
				if err != nil {
					log.Errorf("Formatting metric %s data to COPY format failed for %s: %v ", m.Metric, m.DBName, err)
					atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
					goto stmt_close
				}
			} else {
				if PGSchemaType == "custom" {
					_, err = stmt.Exec(m.Time, m.DBName, m.Metric, string(jsonBytes), nil)
				} else {
					_, err = stmt.Exec(m.Time, m.DBName, string(jsonBytes), nil)
				}
				if err != nil {
					log.Errorf("Formatting metric %s data to COPY format failed for %s: %v ", m.Metric, m.DBName, err)
					atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
					goto stmt_close
				}
			}
		}

		_, err = stmt.Exec()
		if err != nil {
			log.Error("COPY to Postgres failed:", err)
			atomic.AddUint64(&datastoreWriteFailuresCounter, 1)
			if strings.Contains(err.Error(), "no partition") {
				log.Warning("Some metric partitions might have been removed, halting all metric storage. Trying to re-create all needed partitions on next run")
				forceRecreatePGMetricPartitions = true
			}
		}
	stmt_close:
		err = stmt.Close()
		if err != nil {
			log.Error("stmt.Close() failed:", err)
		}
	}

	t_diff := time.Since(t1)
	if err == nil {
		if len(storeMessages) == 1 {
			log.Infof("wrote %d/%d rows to Postgres for [%s:%s] in %.1f ms", rows_batched, total_rows,
				storeMessages[0].DBUniqueName, storeMessages[0].MetricName, float64(t_diff.Nanoseconds())/1000000)
		} else {
			log.Infof("wrote %d/%d rows from %d metric sets to Postgres in %.1f ms", rows_batched, total_rows,
				len(storeMessages), float64(t_diff.Nanoseconds())/1000000)
		}
		atomic.StoreInt64(&lastSuccessfulDatastoreWriteTimeEpoch, t1.Unix())
		atomic.AddUint64(&datastoreTotalWriteTimeMicroseconds, uint64(t_diff.Microseconds()))
		atomic.AddUint64(&datastoreWriteSuccessCounter, 1)
	}
	return err
}
