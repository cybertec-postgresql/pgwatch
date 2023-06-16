package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
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

func InitAndTestConfigStoreConnection(ctx context.Context, host, port, dbname, user, password string, requireSSL, failOnErr bool) (configDb PgxPoolIface, err error) {
	logger := log.GetLogger(ctx)
	SSLMode := map[bool]string{false: "disable", true: "require"}[requireSSL]
	var retries = 3 // ~15s
	defer func() {
		if err != nil && failOnErr {
			logger.Fatal("could not ping configDb! exit.", err)
		}
	}()
	for i := 0; i <= retries; i++ {
		// configDb is used by the main thread only. no verify-ca/verify-full support currently
		if configDb, err = GetPostgresDBConnection(ctx, "", host, port, dbname, user, password, SSLMode, "", "", ""); err != nil {
			return
		}
		if err = configDb.Ping(ctx); err == nil {
			logger.Info("connect to configDb OK!")
			return
		}
		if i < retries {
			logger.Errorf("could not ping configDb! retrying in 5s. %d retries left. err: %v", retries-i, err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second * 5):
				continue
			}
		}
	}
	return
}

func InitAndTestMetricStoreConnection(ctx context.Context, connStr string, failOnErr bool) (metricDb PgxPoolIface, err error) {
	logger := log.GetLogger(ctx)
	var retries = 3 // ~15s
	for i := 0; i <= retries; i++ {
		metricDb, err = GetPostgresDBConnection(ctx, connStr, "", "", "", "", "", "", "", "", "")
		if err != nil {
			if i < retries {
				logger.Errorf("could not open metricDb connection. retrying in 5s. %d retries left. err: %v", retries-i, err)
				time.Sleep(time.Second * 5)
				continue
			}
			if failOnErr {
				logger.Fatal("could not open metricDb connection! exit. err:", err)
			} else {
				logger.Error("could not open metricDb connection:", err)
				return
			}
		}

		err = metricDb.Ping(ctx)

		if err != nil {
			if i < retries {
				logger.Errorf("could not ping metricDb! retrying in 5s. %d retries left. err: %v", retries-i, err)
				time.Sleep(time.Second * 5)
				continue
			}
			if failOnErr {
				logger.Fatal("could not ping metricDb! exit.", err)
			} else {
				return
			}
		} else {
			logger.Info("connect to metricDb OK!")
			break
		}
	}
	return
}

var (
	sqlInit          = "create schema ..."
	MetricSchemaSQLs = map[string]string{"Schema Init": sqlInit}
)

// ExecuteSchemaScripts executes initial schema scripts
func ExecuteSchemaScripts(ctx context.Context, conn PgxIface, sqls map[string]string) (err error) {
	logger := log.GetLogger(ctx)
	for sqlName, sql := range sqls {
		logger.Info("Executing script: ", sqlName)
		if _, err = conn.Exec(ctx, sql); err != nil {
			logger.WithError(err).Error("Script execution failed")
			logger.Warn("Dropping \"timetable\" schema...")
			_, err = conn.Exec(ctx, "DROP SCHEMA IF EXISTS timetable CASCADE")
			if err != nil {
				logger.WithError(err).Error("Schema dropping failed")
			}
			return err
		}
		logger.Info("Schema file executed: " + sqlName)
	}
	logger.Info("Configuration schema created...")
	return nil
}
