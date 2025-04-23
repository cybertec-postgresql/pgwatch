package sources

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewConn and NewConnWithConfig are wrappers to allow testing
var (
	NewConn           = db.New
	NewConnWithConfig = db.NewWithConfig
)

const (
	EnvUnknown       = "UNKNOWN"
	EnvAzureSingle   = "AZURE_SINGLE"
	EnvAzureFlexible = "AZURE_FLEXIBLE"
	EnvGoogle        = "GOOGLE"
)

type RuntimeInfo struct {
	LastCheckedOn    time.Time
	IsInRecovery     bool
	VersionStr       string
	Version          int
	RealDbname       string
	SystemIdentifier string
	IsSuperuser      bool
	Extensions       map[string]int
	ExecEnv          string
	ApproxDBSizeB    int64
}

// SourceConn represents a single connection to monitor. Unlike source, it contains a database connection.
// Continuous discovery sources (postgres-continuous-discovery, patroni-continuous-discovery, patroni-namespace-discovery)
// will produce multiple monitored databases structs based on the discovered databases.
type (
	SourceConn struct {
		Source
		Conn       db.PgxPoolIface
		ConnConfig *pgxpool.Config
		RuntimeInfo
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
		md.Conn, err = NewConnWithConfig(ctx, md.ConnConfig)
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

// GetMetricInterval returns the metric interval for the connection
func (md *SourceConn) GetMetricInterval(name string) float64 {
	if md.IsInRecovery && len(md.MetricsStandby) > 0 {
		return md.MetricsStandby[name]
	}
	return md.Metrics[name]
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

func (md *SourceConn) GetMonitoredDatabaseSettings(ctx context.Context, forceRefetch bool) (_ RuntimeInfo, err error) {
	if ctx.Err() != nil {
		return md.RuntimeInfo, ctx.Err()
	}

	var dbNewSettings RuntimeInfo

	if !forceRefetch && md.LastCheckedOn.After(time.Now().Add(time.Minute*-2)) { // use cached version for 2 min
		return md.RuntimeInfo, nil
	}

	if dbNewSettings.Extensions == nil {
		dbNewSettings.Extensions = make(map[string]int)
	}

	switch md.Kind {
	case SourcePgBouncer:
		if err = md.Conn.QueryRow(ctx, "SHOW VERSION").Scan(&dbNewSettings.VersionStr); err != nil {
			return dbNewSettings, err
		}
		matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(dbNewSettings.VersionStr)
		if len(matches) != 1 {
			return md.RuntimeInfo, fmt.Errorf("unexpected PgBouncer version input: %s", dbNewSettings.VersionStr)
		}
		dbNewSettings.Version = VersionToInt(matches[0])
	case SourcePgPool:
		if err = md.Conn.QueryRow(ctx, "SHOW POOL_VERSION").Scan(&dbNewSettings.VersionStr); err != nil {
			return dbNewSettings, err
		}

		matches := rBouncerAndPgpoolVerMatch.FindStringSubmatch(dbNewSettings.VersionStr)
		if len(matches) != 1 {
			return md.RuntimeInfo, fmt.Errorf("unexpected PgPool version input: %s", dbNewSettings.VersionStr)
		}
		dbNewSettings.Version = VersionToInt(matches[0])
	default:
		sql := `select /* pgwatch_generated */ 
	div(current_setting('server_version_num')::int, 10000) as ver, 
	version(), 
	pg_is_in_recovery(), 
	current_database()::TEXT,
	system_identifier,
	current_setting('is_superuser')::bool
FROM
	pg_control_system()`

		err = md.Conn.QueryRow(ctx, sql).
			Scan(&dbNewSettings.Version, &dbNewSettings.VersionStr,
				&dbNewSettings.IsInRecovery, &dbNewSettings.RealDbname,
				&dbNewSettings.SystemIdentifier, &dbNewSettings.IsSuperuser)
		if err != nil {
			return md.RuntimeInfo, err
		}

		if md.ExecEnv != "" {
			dbNewSettings.ExecEnv = md.ExecEnv // carry over as not likely to change ever
		} else {
			dbNewSettings.ExecEnv = md.DiscoverPlatform(ctx)
		}

		// to work around poor Azure Single Server FS functions performance for some metrics + the --min-db-size-mb filter
		if dbNewSettings.ExecEnv == EnvAzureSingle {
			if approxSize, err := md.GetApproxSize(ctx); err == nil {
				dbNewSettings.ApproxDBSizeB = approxSize
			} else {
				dbNewSettings.ApproxDBSizeB = md.ApproxDBSizeB
			}
		}

		sqlExtensions := `select /* pgwatch_generated */ extname::text, (regexp_matches(extversion, $$\d+\.?\d+?$$))[1]::text as extversion from pg_extension order by 1;`
		var res pgx.Rows
		res, err = md.Conn.Query(ctx, sqlExtensions)
		if err == nil {
			var ext string
			var ver string
			_, err = pgx.ForEachRow(res, []any{&ext, &ver}, func() error {
				extver := VersionToInt(ver)
				if extver == 0 {
					return fmt.Errorf("unexpected extension %s version input: %s", ext, ver)
				}
				dbNewSettings.Extensions[ext] = extver
				return nil
			})
		}

	}

	dbNewSettings.LastCheckedOn = time.Now()
	md.RuntimeInfo = dbNewSettings // store the new settings in the struct
	return md.RuntimeInfo, err
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
