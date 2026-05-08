package sources

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
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

// SourceConn is the interface that all monitored source connection types must implement.
type SourceConn interface {
	Connect(ctx context.Context, opts CmdOpts) error
	Ping(ctx context.Context) error
	IsPostgresSource() bool
	GetSource() Source
	GetMetricInterval(name string) time.Duration
	SetMetricIntervals(main, standby metrics.MetricIntervals)
	Close()
}

// compile-time assertions
var _ SourceConn = (*DbConn)(nil)
var _ SourceConn = (*PromConn)(nil)

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

// DbConn represents a single connection to monitor. Unlike source, it contains a database connection.
// Continuous discovery sources (postgres-continuous-discovery, patroni-continuous-discovery, patroni-namespace-discovery)
// will produce multiple monitored databases structs based on the discovered databases.
type (
	DbConn struct {
		Source
		Conn       db.PgxPoolIface
		ConnConfig *pgxpool.Config
		RuntimeInfo
		sync.RWMutex
	}

	SourceConns []SourceConn
)

func NewDbConn(s Source) *DbConn {
	return &DbConn{
		Source: s,
		RuntimeInfo: RuntimeInfo{
			Extensions:  make(map[string]int),
			ChangeState: make(map[string]map[string]string),
		},
	}
}

// NewSourceConn is a factory dispatcher that returns a SourceConn interface.
func NewSourceConn(s Source) SourceConn {
	return NewDbConn(s)
}

// GetSource returns a copy of the embedded Source.
func (md *DbConn) GetSource() Source {
	return md.Source
}

// SetMetricIntervals atomically sets metric intervals; nil means "no change".
func (md *DbConn) SetMetricIntervals(main, standby metrics.MetricIntervals) {
	md.Lock()
	defer md.Unlock()
	if main != nil {
		md.Metrics = main
	}
	if standby != nil {
		md.MetricsStandby = standby
	}
}

// Close closes the connection if it is not nil.
func (md *DbConn) Close() {
	if md.Conn != nil {
		md.Conn.Close()
	}
}

// Ping will try to ping the server to ensure the connection is still alive
func (md *DbConn) Ping(ctx context.Context) (err error) {
	if md.Kind == SourcePgBouncer {
		// pgbouncer is very picky about the queries it accepts
		_, err = md.Conn.Exec(ctx, "SHOW VERSION")
		return
	}
	return md.Conn.Ping(ctx)
}

// Connect will establish a connection to the database if it's not already connected.
// If the connection is already established, it pings the server to ensure it's still alive.
func (md *DbConn) Connect(ctx context.Context, opts CmdOpts) (err error) {
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
func (md *DbConn) ParseConfig() (err error) {
	if md.ConnConfig == nil {
		md.ConnConfig, err = pgxpool.ParseConfig(md.ConnStr)
		return
	}
	return
}

// GetClusterIdentifier returns a unique identifier for the host assuming SysId is the same for
// primary and all replicas but connection information is different
func (md *DbConn) GetClusterIdentifier() string {
	if err := md.ParseConfig(); err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%s:%d", md.SystemIdentifier, md.ConnConfig.ConnConfig.Host, md.ConnConfig.ConnConfig.Port)
}

// GetDatabaseName returns the database name from the connection string
func (md *DbConn) GetDatabaseName() string {
	if err := md.ParseConfig(); err != nil {
		return ""
	}
	return md.ConnConfig.ConnConfig.Database
}

// GetMetricInterval returns the metric interval for the connection
func (md *DbConn) GetMetricInterval(name string) time.Duration {
	md.RLock()
	defer md.RUnlock()
	if md.IsInRecovery && len(md.MetricsStandby) > 0 {
		return time.Duration(md.MetricsStandby[name]) * time.Second
	}
	return time.Duration(md.Metrics[name]) * time.Second
}

// IsClientOnSameHost checks if the pgwatch client is running on the same host as the PostgreSQL server
func (md *DbConn) IsClientOnSameHost() bool {
	ok, err := db.IsClientOnSameHost(md.Conn)
	return ok && err == nil
}

// SetDatabaseName sets the database name in the connection config for resolved databases
func (md *DbConn) SetDatabaseName(name string) {
	if err := md.ParseConfig(); err != nil {
		return
	}
	md.ConnStr = "" // unset the connection string to force conn config usage
	md.ConnConfig.ConnConfig.Database = name
}

func (md *DbConn) IsPostgresSource() bool {
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

func (md *DbConn) FetchRuntimeInfo(ctx context.Context, forceRefetch bool) (err error) {
	md.Lock()
	defer md.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !forceRefetch && md.LastCheckedOn.After(time.Now().Add(time.Minute*-5)) { // use cached version for 5 min
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

func (md *DbConn) FetchVersion(ctx context.Context, sql string) (version string, ver int, err error) {
	if err = md.Conn.QueryRow(ctx, sql, pgx.QueryExecModeSimpleProtocol).Scan(&version); err != nil {
		return
	}
	ver = VersionToInt(version)
	return
}

// DiscoverPlatform tries to discover the platform based on the database version string and some special settings
// that are only available on certain platforms. Returns the platform name or "UNKNOWN" if not sure.
func (md *DbConn) DiscoverPlatform(ctx context.Context) (platform string) {
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
func (md *DbConn) FetchApproxSize(ctx context.Context) (size int64) {
	sqlApproxDBSize := `select /* pgwatch_generated */ current_setting('block_size')::int8 * sum(relpages) from pg_class c where c.relpersistence != 't'`
	_ = md.Conn.QueryRow(ctx, sqlApproxDBSize).Scan(&size)
	return
}

// FunctionExists checks if a function exists in the database
func (md *DbConn) FunctionExists(ctx context.Context, functionName string) (exists bool) {
	sql := `select /* pgwatch_generated */ true 
from 
	pg_proc join pg_namespace n on pronamespace = n.oid 
where 
	proname = $1 and n.nspname = 'public'`
	_ = md.Conn.QueryRow(ctx, sql, functionName).Scan(&exists)
	return
}

// TryCreateMissingExtensions should be called once on daemon startup if some commonly wanted extension (most notably pg_stat_statements) is missing.
func (md *DbConn) TryCreateMissingExtensions(ctx context.Context, extensions []string) (string, error) {
	md.RLock()
	defer md.RUnlock()

	sqlAvailableExts := `select name::text from pg_available_extensions order by 1`
	CreatedExts := make([]string, 0)

	data, err := md.Conn.Query(ctx, sqlAvailableExts)
	if err != nil {
		return "", err
	}
	availableExts, err := pgx.CollectRows(data, pgx.RowTo[string])
	if err != nil {
		return "", err
	}

	for _, extToCreate := range extensions {
		if _, ok := md.Extensions[extToCreate]; ok {
			continue
		}
		if _, ok := slices.BinarySearch(availableExts, extToCreate); !ok {
			err = errors.Join(err, fmt.Errorf("requested extension %s is not available on instance", extToCreate))
			continue
		}
		if _, e := md.Conn.Exec(ctx, fmt.Sprintf(`create extension if not exists "%s"`, extToCreate)); e != nil {
			err = errors.Join(err, fmt.Errorf("failed to create extension %s: %w", extToCreate, e))
		} else {
			CreatedExts = append(CreatedExts, extToCreate)
		}
	}
	return strings.Join(CreatedExts, ","), err
}

// TryCreateMetricsHelpers should be called once on daemon startup to try to create "metric fetching helper" functions automatically
func (md *DbConn) TryCreateMetricsHelpers(ctx context.Context, getSQLFn func(string) string) (err error) {
	md.RLock()
	defer md.RUnlock()
	var sql string
	metricsMap := maps.Clone(md.Metrics)
	maps.Insert(metricsMap, maps.All(md.MetricsStandby))
	for metricName := range metricsMap {
		if sql = getSQLFn(metricName); sql == "" {
			continue
		}
		if _, e := md.Conn.Exec(ctx, sql); e != nil {
			err = errors.Join(err, fmt.Errorf("failed to create helper for metric %s: %w", metricName, e))
		}
	}
	return
}

func (mds SourceConns) GetMonitoredDatabase(DBUniqueName string) SourceConn {
	for _, md := range mds {
		if md.GetSource().Name == DBUniqueName {
			return md
		}
	}
	return nil
}

// PromConn represents a Prometheus source connection (stub for future use).
type PromConn struct {
	Source
	HTTPClient *http.Client
	sync.RWMutex
}

func NewPromConn(s Source) *PromConn {
	return &PromConn{
		Source: s,
	}
}

func (pc *PromConn) Connect(_ context.Context, _ CmdOpts) error       { return nil }
func (pc *PromConn) Ping(_ context.Context) error                     { return nil }
func (pc *PromConn) IsPostgresSource() bool                           { return false }
func (pc *PromConn) GetSource() Source                                { return pc.Source }
func (pc *PromConn) FetchRuntimeInfo(_ context.Context, _ bool) error { return nil }
func (pc *PromConn) Close()                                           {}

func (pc *PromConn) GetMetricInterval(name string) time.Duration {
	pc.RLock()
	defer pc.RUnlock()
	return time.Duration(pc.Metrics[name]) * time.Second
}

func (pc *PromConn) SetMetricIntervals(main, standby metrics.MetricIntervals) {
	pc.Lock()
	defer pc.Unlock()
	if main != nil {
		pc.Metrics = main
	}
}
