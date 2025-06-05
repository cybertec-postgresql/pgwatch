package db

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"

	jsoniter "github.com/json-iterator/go"

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
	if b, err := jsoniter.ConfigFastest.Marshal(v); err == nil {
		return string(b)
	}
	return nil
}

// Function to determine if the client is connected to the same host as the PostgreSQL server
func IsClientOnSameHost(conn PgxIface) (bool, error) {
	ctx := context.Background()

	// Step 1: Check connection type using SQL
	var isUnixSocket bool
	err := conn.QueryRow(ctx, "SELECT COALESCE(inet_client_addr(), inet_server_addr()) IS NULL").Scan(&isUnixSocket)
	if err != nil || isUnixSocket {
		return isUnixSocket, err
	}

	// Step 2: Retrieve unique cluster identifier
	var dataDirectory string
	if err := conn.QueryRow(ctx, "SHOW data_directory").Scan(&dataDirectory); err != nil {
		return false, err
	}

	var systemIdentifier uint64
	if err := conn.QueryRow(ctx, "SELECT system_identifier FROM pg_control_system()").Scan(&systemIdentifier); err != nil {
		return false, err
	}

	// Step 3: Compare system identifier from file system
	pgControlFile := filepath.Join(dataDirectory, "global", "pg_control")
	file, err := os.Open(pgControlFile)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var fileSystemIdentifier uint64
	if err := binary.Read(file, binary.LittleEndian, &fileSystemIdentifier); err != nil {
		return false, err
	}

	return fileSystemIdentifier == systemIdentifier, nil
}
