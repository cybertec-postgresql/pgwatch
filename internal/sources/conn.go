package sources

import (
	"context"
	"reflect"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SourceConn represents a single connection to monitor. Unlike source, it contains a database connection.
// Continuous discovery sources (postgres-continuous-discovery, patroni-continuous-discovery, patroni-namespace-discovery)
// will produce multiple monitored databases structs based on the discovered databases.
type (
	SourceConn struct {
		Source
		Conn       db.PgxPoolIface
		ConnConfig *pgxpool.Config
	}

	SourceConns []*SourceConn
)

// Ping will try to ping the server to ensure the connection is still alive
func (md *SourceConn) Ping(ctx context.Context) (err error) {
	if md.Kind == SourcePgBouncer {
		// pgbouncer is very picky about the queries it accepts
		_, err = md.Conn.Exec(ctx, "SHOW VERSION")
		return
	}
	return md.Conn.Ping(ctx)
}

// NewWithConfig is a function that creates a new connection pool with the given config.
var NewWithConfig = db.NewWithConfig

// Connect will establish a connection to the database if it's not already connected.
// If the connection is already established, it pings the server to ensure it's still alive.
func (md *SourceConn) Connect(ctx context.Context, opts CmdOpts) (err error) {
	if md.Conn == nil {
		if err = md.ParseConfig(); err != nil {
			return err
		}
		if md.Kind == SourcePgBouncer {
			md.ConnConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		}
		if opts.MaxParallelConnectionsPerDb > 0 {
			md.ConnConfig.MaxConns = int32(opts.MaxParallelConnectionsPerDb)
		}
		md.Conn, err = NewWithConfig(ctx, md.ConnConfig)
		if err != nil {
			return err
		}
	}
	return md.Ping(ctx)
}

// ParseConfig will parse the connection string and store the result in the connection config
func (md *SourceConn) ParseConfig() (err error) {
	if md.ConnConfig == nil {
		md.ConnConfig, err = pgxpool.ParseConfig(md.ConnStr)
		return
	}
	return
}

// GetDatabaseName returns the database name from the connection string
func (md *SourceConn) GetDatabaseName() string {
	if err := md.ParseConfig(); err != nil {
		return ""
	}
	return md.ConnConfig.ConnConfig.Database
}

// SetDatabaseName sets the database name in the connection config for resolved databases
func (md *SourceConn) SetDatabaseName(name string) {
	if err := md.ParseConfig(); err != nil {
		return
	}
	md.ConnStr = "" // unset the connection string to force conn config usage
	md.ConnConfig.ConnConfig.Database = name
}

func (md *SourceConn) IsPostgresSource() bool {
	return md.Kind != SourcePgBouncer && md.Kind != SourcePgPool
}

// TryDiscoverPlatform tries to discover the platform based on the database version string and some special settings
// that are only available on certain platforms. Returns the platform name or "UNKNOWN" if not sure.
func (md *SourceConn) DiscoverPlatform(ctx context.Context) (platform string) {
	sql := `select /* pgwatch_generated */
	case
	  when exists (select * from pg_settings where name = 'pg_qs.host_database' and setting = 'azure_sys') and version() ~* 'compiled by Visual C' then 'AZURE_SINGLE'
	  when exists (select * from pg_settings where name = 'pg_qs.host_database' and setting = 'azure_sys') and version() ~* 'compiled by gcc' then 'AZURE_FLEXIBLE'
	  when exists (select * from pg_settings where name = 'cloudsql.supported_extensions') then 'GOOGLE'
	else
	  'UNKNOWN'
	end as exec_env`
	_ = md.Conn.QueryRow(ctx, sql).Scan(&platform)
	return
}

// GetApproxSize returns the approximate size of the database in bytes
func (md *SourceConn) GetApproxSize(ctx context.Context) (size int64, err error) {
	sqlApproxDBSize := `select /* pgwatch_generated */ 
	current_setting('block_size')::int8 * sum(relpages)
from 
	pg_class c
where
	c.relpersistence != 't'`
	err = md.Conn.QueryRow(ctx, sqlApproxDBSize).Scan(&size)
	return
}

// FunctionExists checks if a function exists in the database
func (md *SourceConn) FunctionExists(ctx context.Context, functionName string) (exists bool) {
	sql := `select /* pgwatch_generated */ true 
from 
	pg_proc join pg_namespace n on pronamespace = n.oid 
where 
	proname = $1 and n.nspname = 'public'`
	_ = md.Conn.QueryRow(ctx, sql, functionName).Scan(&exists)
	return
}

func (mds SourceConns) GetMonitoredDatabase(DBUniqueName string) *SourceConn {
	for _, md := range mds {
		if md.Name == DBUniqueName {
			return md
		}
	}
	return nil
}

// SyncFromReader will update the monitored databases with the latest configuration from the reader.
// Any resolution errors will be returned, e.g. etcd unavailability.
// It's up to the caller to proceed with the databases available or stop the execution due to errors.
func (mds SourceConns) SyncFromReader(r Reader) (newmds SourceConns, err error) {
	srcs, err := r.GetSources()
	if err != nil {
		return nil, err
	}
	newmds, err = srcs.ResolveDatabases()
	for _, newMD := range newmds {
		md := mds.GetMonitoredDatabase(newMD.Name)
		if md == nil {
			continue
		}
		if reflect.DeepEqual(md.Source, newMD.Source) {
			// keep the existing connection if the source is the same
			newMD.Conn = md.Conn
			newMD.ConnConfig = md.ConnConfig
			continue
		}
		if md.Conn != nil {
			md.Conn.Close()
		}
	}
	return newmds, err
}
