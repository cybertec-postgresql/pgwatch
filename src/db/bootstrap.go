package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	retry "github.com/sethvargo/go-retry"
)

const (
	pgConnRecycleSeconds = 1800       // applies for monitored nodes
	applicationName      = "pgwatch3" // will be set on all opened PG connections for informative purposes
)

func GetPostgresDBConnection(ctx context.Context, libPqConnString, host, port, dbname, user, password, sslmode, sslrootcert, sslcert, sslkey string) (PgxPoolIface, error) {
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
			host, port, dbname, sslmode, user, applicationName, sslrootcert, sslcert, sslkey)
		if password != "" { // having empty string as password effectively disables .pgpass so include only if password given
			connStr += fmt.Sprintf(" password='%s'", password)
		}
	}

	connConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	connConfig.MaxConnIdleTime = 15 * time.Second
	connConfig.MaxConnLifetime = pgConnRecycleSeconds * time.Second
	tracelogger := &tracelog.TraceLog{
		Logger:   log.NewPgxLogger(log.GetLogger(ctx)),
		LogLevel: tracelog.LogLevelDebug, //map[bool]tracelog.LogLevel{false: tracelog.LogLevelWarn, true: tracelog.LogLevelDebug}[true],
	}
	connConfig.ConnConfig.Tracer = tracelogger
	return pgxpool.NewWithConfig(ctx, connConfig)
}

var backoff = retry.WithMaxRetries(3, retry.NewConstant(1*time.Second))

func InitAndTestConfigStoreConnection(ctx context.Context, host, port, dbname, user, password string, requireSSL bool) (configDb PgxPoolIface, err error) {
	logger := log.GetLogger(ctx)
	SSLMode := map[bool]string{false: "disable", true: "require"}[requireSSL]
	if err = retry.Do(ctx, backoff, func(ctx context.Context) error {
		if configDb, err = GetPostgresDBConnection(ctx, "", host, port, dbname, user, password, SSLMode, "", "", ""); err == nil {
			err = configDb.Ping(ctx)
		}
		if err != nil {
			logger.WithError(err).Error("Connection failed")
			logger.Info("Sleeping before reconnecting...")
			return retry.RetryableError(err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	err = ExecuteConfigSchemaScripts(ctx, configDb)
	return
}

func InitAndTestMetricStoreConnection(ctx context.Context, connStr string) (metricDb PgxPoolIface, err error) {
	logger := log.GetLogger(ctx)
	if err = retry.Do(ctx, backoff, func(ctx context.Context) error {
		if metricDb, err = GetPostgresDBConnection(ctx, connStr, "", "", "", "", "", "", "", "", ""); err == nil {
			err = metricDb.Ping(ctx)
		}
		if err != nil {
			logger.WithError(err).Error("Connection failed")
			logger.Info("Sleeping before reconnecting...")
			return retry.RetryableError(err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	err = ExecuteMetricSchemaScripts(ctx, metricDb)
	return
}

var (
	configSchemaSQLs = []string{
		sqlConfigSchema,
		sqlConfigDefinitions,
	}
	metricSchemaSQLs = []string{
		sqlMetricAdminSchema,
		sqlMetricAdminFunctions,
		sqlMetricEnsurePartitionPostgres,
		sqlMetricEnsurePartitionTimescale,
		sqlMetricChangeChunkIntervalTimescale,
		sqlMetricChangeCompressionIntervalTimescale,
	}
)

func ExecuteConfigSchemaScripts(ctx context.Context, conn PgxIface) error {
	log.GetLogger(ctx).Info("Executing configuration schema scripts: ", len(configSchemaSQLs))
	return executeSchemaScripts(ctx, conn, "pgwatch3", configSchemaSQLs)
}

func ExecuteMetricSchemaScripts(ctx context.Context, conn PgxIface) error {
	log.GetLogger(ctx).Info("Executing metric storage schema scripts: ", len(metricSchemaSQLs))
	return executeSchemaScripts(ctx, conn, "admin", metricSchemaSQLs)
}

// executeSchemaScripts executes initial schema scripts
func executeSchemaScripts(ctx context.Context, conn PgxIface, schema string, sqls []string) (err error) {
	var exists bool
	err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)", schema).Scan(&exists)
	if err != nil || exists {
		return
	}
	for _, sql := range sqls {
		if _, err = conn.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

type MetricSchemaType int

const (
	MetricSchemaPostgres MetricSchemaType = iota
	MetricSchemaTimescale
)

func GetMetricSchemaType(ctx context.Context, conn PgxIface) (metricSchema MetricSchemaType, err error) {
	// return 1 (MetricSchemaTimescale) if the extension present
	sqlSchemaType := `select count(*) from pg_catalog.pg_extension where extname = 'timescaledb'`
	err = conn.QueryRow(ctx, sqlSchemaType).Scan(&metricSchema)
	return
}
