package db

import (
	"context"
	"encoding/json"
	"reflect"

	pgx "github.com/jackc/pgx/v5"
	pgconn "github.com/jackc/pgx/v5/pgconn"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
)

type Querier interface {
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
}

// PgxIface is common interface for every pgx class
type PgxIface interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

// PgxConnIface is interface representing pgx connection
type PgxConnIface interface {
	PgxIface
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Close(ctx context.Context) error
	Ping(ctx context.Context) error
}

// PgxPoolIface is interface representing pgx pool
type PgxPoolIface interface {
	PgxIface
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Close()
	Config() *pgxpool.Config
	Ping(ctx context.Context) error
	Stat() *pgxpool.Stat
}

func MarshallParamToJSONB(v any) any {
	if v == nil {
		return nil
	}
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Map, reflect.Slice:
		if val.Len() == 0 {
			return nil
		}
	case reflect.Struct:
		if reflect.DeepEqual(v, reflect.Zero(val.Type()).Interface()) {
			return nil
		}
	}
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return nil
}
