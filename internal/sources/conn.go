package sources

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"sync"
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
	EnvAzureSingle   = "AZURE_SINGLE" //discontinued
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
	ApproxDbSize     int64
	ChangeState      map[string]map[string]string // ["category"][object_identifier] = state
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
		sync.RWMutex
	}

	SourceConns []*SourceConn
)

func NewSourceConn(s Source) *SourceConn {
	return &SourceConn{
		Source: s,
		RuntimeInfo: RuntimeInfo{
			Extensions:  make(map[string]int),
			ChangeState: make(map[string]map[string]string),
		},
	}
}

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
		// Set lock_timeout at connection level for PostgreSQL sources to avoid
		// wrapping every query in a transaction with SET LOCAL lock_timeout
		if md.IsPostgresSource() && opts.ConnLockTimeout != "" && opts.ConnLockTimeout != "0" {
			md.ConnConfig.ConnConfig.RuntimeParams["lock_timeout"] = opts.ConnLockTimeout
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

// GetUniqueIdentifier returns a unique identifier for the host assuming SysId is the same for
// primary and all replicas but connection information is different
func (md *SourceConn) GetClusterIdentifier() string {
	if err := md.ParseConfig(); err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d", md.SystemIdentifier, md.ConnConfig.ConnConfig.Host, md.ConnConfig.ConnConfig.Port)
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
	md.RLock()
	defer md.RUnlock()
	if md.IsInRecovery && len(md.MetricsStandby) > 0 {
		return md.MetricsStandby[name]
	}
	return md.Metrics[name]
}

// IsClientOnSameHost checks if the pgwatch client is running on the same host as the PostgreSQL server
func (md *SourceConn) IsClientOnSameHost() bool {
	ok, err := db.IsClientOnSameHost(md.Conn)
	return ok && err == nil
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

func (md *SourceConn) FetchRuntimeInfo(ctx context.Context, forceRefetch bool) (err error) {
	md.Lock()
	defer md.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !forceRefetch && md.LastCheckedOn.After(time.Now().Add(time.Minute*-2)) { // use cached version for 2 min
		return nil
	}
	switch md.Kind {
	case SourcePgBouncer, SourcePgPool:
		if md.VersionStr, md.Version, err = md.FetchVersion(ctx, func() string {
			if md.Kind == SourcePgBouncer {
				return "SHOW VERSION"
			}
			return "SHOW POOL_VERSION"
		}()); err != nil {
			return
		}
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
			Scan(&md.Version, &md.VersionStr,
				&md.IsInRecovery, &md.RealDbname,
				&md.SystemIdentifier, &md.IsSuperuser)
		if err != nil {
			return err
		}

		md.ExecEnv = md.DiscoverPlatform(ctx)
		md.ApproxDbSize = md.FetchApproxSize(ctx)

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
				md.Extensions[ext] = extver
				return nil
			})
		}

	}
	md.LastCheckedOn = time.Now()
	return err
}

func (md *SourceConn) FetchVersion(ctx context.Context, sql string) (version string, ver int, err error) {
	if err = md.Conn.QueryRow(ctx, sql, pgx.QueryExecModeSimpleProtocol).Scan(&version); err != nil {
		return
	}
	ver = VersionToInt(version)
	return
}

// TryDiscoverPlatform tries to discover the platform based on the database version string and some special settings
// that are only available on certain platforms. Returns the platform name or "UNKNOWN" if not sure.
func (md *SourceConn) DiscoverPlatform(ctx context.Context) (platform string) {
	if md.ExecEnv != "" {
		return md.ExecEnv // carry over as not likely to change ever
	}
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

// FetchApproxSize returns the approximate size of the database in bytes
func (md *SourceConn) FetchApproxSize(ctx context.Context) (size int64) {
	sqlApproxDBSize := `select /* pgwatch_generated */ current_setting('block_size')::int8 * sum(relpages) from pg_class c where c.relpersistence != 't'`
	_ = md.Conn.QueryRow(ctx, sqlApproxDBSize).Scan(&size)
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
