package db

import (
	"context"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	retry "github.com/sethvargo/go-retry"
)

const (
	pgConnRecycleSeconds = 1800      // applies for monitored nodes
	applicationName      = "pgwatch" // will be set on all opened PG connections for informative purposes
)

func Ping(ctx context.Context, connStr string) error {
	c, err := pgx.Connect(ctx, connStr)
	if c != nil {
		_ = c.Close(ctx)
	}
	return err
}

type ConnConfigCallback = func(*pgxpool.Config) error

// New create a new pool
func New(ctx context.Context, connStr string, callbacks ...ConnConfigCallback) (PgxPoolIface, error) {
	connConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	return NewWithConfig(ctx, connConfig, callbacks...)
}

// NewWithConfig creates a new pool with a given config
func NewWithConfig(ctx context.Context, connConfig *pgxpool.Config, callbacks ...ConnConfigCallback) (PgxPoolIface, error) {
	logger := log.GetLogger(ctx)
	if connConfig.ConnConfig.ConnectTimeout == 0 {
		connConfig.ConnConfig.ConnectTimeout = time.Second * 5
	}
	connConfig.MaxConnIdleTime = 15 * time.Second
	connConfig.MaxConnLifetime = pgConnRecycleSeconds * time.Second
	connConfig.ConnConfig.RuntimeParams["application_name"] = applicationName
	connConfig.ConnConfig.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
		logger.WithField("severity", n.Severity).WithField("notice", n.Message).Info("Notice received")
	}
	tracelogger := &tracelog.TraceLog{
		Logger:   log.NewPgxLogger(logger),
		LogLevel: tracelog.LogLevelDebug,
	}
	connConfig.ConnConfig.Tracer = tracelogger
	for _, f := range callbacks {
		if err := f(connConfig); err != nil {
			return nil, err
		}
	}
	return pgxpool.NewWithConfig(ctx, connConfig)
}

type ConnInitCallback = func(context.Context, PgxIface) error

// Init checks if connection is establised. If not, retries connection 3 times with delay 1s
func Init(ctx context.Context, db PgxPoolIface, init ConnInitCallback) error {
	var backoff = retry.WithMaxRetries(3, retry.NewConstant(1*time.Second))
	logger := log.GetLogger(ctx)
	if err := retry.Do(ctx, backoff, func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			logger.WithError(err).Error("connection failed")
			logger.Info("sleeping before reconnecting...")
			return retry.RetryableError(err)
		}
		return nil
	}); err != nil {
		return err
	}
	return init(ctx, db)
}

// DoesSchemaExist checks if schema exists
func DoesSchemaExist(ctx context.Context, conn PgxIface, schema string) (bool, error) {
	var exists bool
	sqlSchemaExists := "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)"
	err := conn.QueryRow(ctx, sqlSchemaExists, schema).Scan(&exists)
	return exists, err
}
