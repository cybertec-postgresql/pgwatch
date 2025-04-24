package sources_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

const ImageName = "docker.io/postgres:17-alpine"

func TestSourceConn_Connect(t *testing.T) {

	t.Run("failed config parsing", func(t *testing.T) {
		md := &sources.SourceConn{}
		md.ConnStr = "invalid connection string"
		err := md.Connect(ctx, sources.CmdOpts{})
		assert.Error(t, err)
	})

	t.Run("failed connection", func(t *testing.T) {
		md := &sources.SourceConn{}
		sources.NewConnWithConfig = func(_ context.Context, _ *pgxpool.Config, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
			return nil, assert.AnError
		}
		err := md.Connect(ctx, sources.CmdOpts{})
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("successful connection to pgbouncer", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		sources.NewConnWithConfig = func(_ context.Context, _ *pgxpool.Config, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
			return mock, nil
		}

		md := &sources.SourceConn{}
		md.Kind = sources.SourcePgBouncer

		opts := sources.CmdOpts{}
		opts.MaxParallelConnectionsPerDb = 3

		mock.ExpectExec("SHOW VERSION").WillReturnResult(pgconn.NewCommandTag("SELECT 1"))

		err = md.Connect(ctx, opts)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSourceConn_ParseConfig(t *testing.T) {
	md := &sources.SourceConn{}
	assert.NoError(t, md.ParseConfig())
	//cached config
	assert.NoError(t, md.ParseConfig())
}

func TestSourceConn_GetDatabaseName(t *testing.T) {
	md := &sources.SourceConn{}
	md.ConnStr = "postgres://user:password@localhost:5432/mydatabase"
	expected := "mydatabase"
	// check pgx.ConnConfig related code
	got := md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
	// check ConnStr parsing
	got = md.Source.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)

	md = &sources.SourceConn{}
	md.ConnStr = "foo boo"
	expected = ""
	got = md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
}

func TestSourceConn_SetDatabaseName(t *testing.T) {
	md := &sources.SourceConn{}
	md.ConnStr = "postgres://user:password@localhost:5432/mydatabase"
	expected := "mydatabase"
	// check ConnStr parsing
	md.SetDatabaseName(expected)
	got := md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
	// check pgx.ConnConfig related code
	expected = "newdatabase"
	md.SetDatabaseName(expected)
	got = md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)

	md = &sources.SourceConn{}
	md.ConnStr = "foo boo"
	expected = ""
	md.SetDatabaseName("ingored due to invalid ConnStr")
	got = md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
}

func TestSourceConn_DiscoverPlatform(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	md := &sources.SourceConn{Conn: mock}

	mock.ExpectQuery("select").WillReturnRows(pgxmock.NewRows([]string{"exec_env"}).AddRow("AZURE_SINGLE"))
	md.ExecEnv = md.DiscoverPlatform(ctx)
	assert.Equal(t, "AZURE_SINGLE", md.ExecEnv)
	assert.Equal(t, "AZURE_SINGLE", md.DiscoverPlatform(ctx)) // cached
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSourceConn_GetApproxSize(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	md := &sources.SourceConn{Conn: mock}

	mock.ExpectQuery("select").WillReturnRows(pgxmock.NewRows([]string{"size"}).AddRow(42))

	assert.EqualValues(t, 42, md.FetchApproxSize(ctx))
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSourceConn_FunctionExists(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	md := &sources.SourceConn{Conn: mock}

	mock.ExpectQuery("select").WithArgs("get_foo").WillReturnRows(pgxmock.NewRows([]string{"exists"}))

	assert.False(t, md.FunctionExists(ctx, "get_foo"))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSourceConn_IsPostgresSource(t *testing.T) {
	md := &sources.SourceConn{}
	md.Kind = sources.SourcePostgres
	assert.True(t, md.IsPostgresSource(), "IsPostgresSource() = false, want true")

	md.Kind = sources.SourcePgBouncer
	assert.False(t, md.IsPostgresSource(), "IsPostgresSource() = true, want false")

	md.Kind = sources.SourcePgPool
	assert.False(t, md.IsPostgresSource(), "IsPostgresSource() = true, want false")

	md.Kind = sources.SourcePatroni
	assert.True(t, md.IsPostgresSource(), "IsPostgresSource() = false, want true")
}

func TestSourceConn_Ping(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	md := &sources.SourceConn{Conn: db}

	db.ExpectPing()
	md.Kind = sources.SourcePostgres
	assert.NoError(t, md.Ping(ctx), "Ping() = error, want nil")

	db.ExpectExec("SHOW VERSION").WillReturnResult(pgconn.NewCommandTag("SELECT 1"))
	md.Conn = db
	md.Kind = sources.SourcePgBouncer
	assert.NoError(t, md.Ping(ctx), "Ping() = error, want nil")
}

func TestSourceConn_GetMetricInterval(t *testing.T) {
	md := &sources.SourceConn{
		Source: sources.Source{
			Metrics:        map[string]float64{"foo": 1.5, "bar": 2.5},
			MetricsStandby: map[string]float64{"foo": 3.5},
		},
	}

	t.Run("primary uses Metrics", func(t *testing.T) {
		md.IsInRecovery = false
		assert.Equal(t, 1.5, md.GetMetricInterval("foo"))
		assert.Equal(t, 2.5, md.GetMetricInterval("bar"))
	})

	t.Run("standby uses MetricsStandby if present", func(t *testing.T) {
		md.IsInRecovery = true
		assert.Equal(t, 3.5, md.GetMetricInterval("foo"))
		assert.Equal(t, 0.0, md.GetMetricInterval("bar"))
	})

	t.Run("standby with empty MetricsStandby falls back to Metrics", func(t *testing.T) {
		md.IsInRecovery = true
		md.MetricsStandby = map[string]float64{}
		assert.Equal(t, 1.5, md.GetMetricInterval("foo"))
	})
}

type testSourceReader struct {
	sources.Sources
	error
}

func (r testSourceReader) GetSources() (sources.Sources, error) {
	return r.Sources, r.error
}

func TestMonitoredDatabases_SyncFromReader_error(t *testing.T) {
	reader := testSourceReader{error: assert.AnError}
	mds := sources.SourceConns{}
	_, err := mds.SyncFromReader(reader)
	assert.Error(t, err)
}

func TestMonitoredDatabases_SyncFromReader(t *testing.T) {
	db, _ := pgxmock.NewPool()
	src := sources.Source{
		Name:      "test",
		Kind:      sources.SourcePostgres,
		IsEnabled: true,
		ConnStr:   "postgres://user:password@localhost:5432/mydatabase",
	}
	reader := testSourceReader{Sources: sources.Sources{src}}
	// first read the sources
	mds, _ := reader.GetSources()
	assert.NotNil(t, mds, "GetSources() = nil, want not nil")
	// then resolve the databases
	mdbs, _ := mds.ResolveDatabases()
	assert.NotNil(t, mdbs, "ResolveDatabases() = nil, want not nil")
	// pretend that we have a connection
	mdbs[0].Conn = db
	db.ExpectClose()
	// sync the databases and make sure they are the same
	newmdbs, _ := mdbs.SyncFromReader(reader)
	assert.NotNil(t, newmdbs)
	assert.Equal(t, mdbs[0].ConnStr, newmdbs[0].ConnStr)
	assert.Equal(t, db, newmdbs[0].Conn)
	// change the connection string and check if databases are updated
	reader.Sources[0].ConnStr = "postgres://user:password@localhost:5432/anotherdatabase"
	newmdbs, _ = mdbs.SyncFromReader(reader)
	assert.NotNil(t, newmdbs)
	assert.NotEqual(t, mdbs[0].ConnStr, newmdbs[0].ConnStr)
	assert.Nil(t, newmdbs[0].Conn)
	assert.NoError(t, db.ExpectationsWereMet())
	// change the unique name of the source and check if it's updated
	reader.Sources[0].Name = "another"
	newmdbs, _ = mdbs.SyncFromReader(reader)
	assert.NotNil(t, newmdbs)
	assert.NotEqual(t, mdbs[0].Name, newmdbs[0].Name)
	assert.Nil(t, newmdbs[0].Conn)
}

func TestVersionToInt(t *testing.T) {
	tests := []struct {
		arg  string
		want int
	}{
		{"", 0},
		{"foo", 0},
		{"13", 13_00_00},
		{"3.0", 3_00_00},
		{"9.6.3", 9_06_03},
		{"v9.6-beta2", 9_06_00},
	}
	for _, tt := range tests {
		if got := sources.VersionToInt(tt.arg); got != tt.want {
			t.Errorf("VersionToInt() = %v, want %v", got, tt.want)
		}
	}
}

func TestSourceConn_FetchRuntimeInfo(t *testing.T) {
	ctx := context.Background()

	t.Run("cancelled context", func(t *testing.T) {
		ctxNew, cancel := context.WithCancel(ctx)
		cancel()
		err := (&sources.SourceConn{}).FetchRuntimeInfo(ctxNew, true)
		assert.Error(t, err)
	})

	t.Run("cached version", func(t *testing.T) {
		md := &sources.SourceConn{
			RuntimeInfo: sources.RuntimeInfo{
				LastCheckedOn: time.Now().Add(-time.Minute),
				Version:       42,
			},
		}
		err := md.FetchRuntimeInfo(ctx, false)
		assert.NoError(t, err)
		assert.Equal(t, 42, md.Version)
	})

	t.Run("pgbouncer version fetch", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{
			Conn:   mock,
			Source: sources.Source{Kind: sources.SourcePgBouncer},
		}
		mock.ExpectQuery("SHOW VERSION").WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow("PgBouncer 1.12.0"))
		err = md.FetchRuntimeInfo(ctx, true)
		assert.NoError(t, err)
		assert.Contains(t, md.VersionStr, "PgBouncer")
		assert.True(t, md.Version > 0)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("pgpool version fetch", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{
			Conn:   mock,
			Source: sources.Source{Kind: sources.SourcePgPool},
		}
		mock.ExpectQuery("SHOW POOL_VERSION").WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow("4.1.2"))
		err = md.FetchRuntimeInfo(ctx, true)
		assert.NoError(t, err)
		assert.Contains(t, md.VersionStr, "4.1.2")
		assert.True(t, md.Version > 0)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres version and extensions", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{
			Conn:   mock,
			Source: sources.Source{Kind: sources.SourcePostgres},
		}
		mock.ExpectQuery("select").WillReturnRows(
			pgxmock.NewRows([]string{"ver", "version", "pg_is_in_recovery", "current_database", "system_identifier", "is_superuser"}).
				AddRow(13, "PostgreSQL 13.3", false, "testdb", "42424242", true),
		)
		mock.ExpectQuery("select").WillReturnRows(
			pgxmock.NewRows([]string{"exec_env"}).AddRow("UNKNOWN"),
		)
		mock.ExpectQuery("select").WillReturnRows(
			pgxmock.NewRows([]string{"approx_size"}).AddRow(42),
		)

		mock.ExpectQuery("select").WillReturnRows(
			pgxmock.NewRows([]string{"extname", "extversion"}).AddRow("pg_stat_statements", "1.8"),
		)
		err = md.FetchRuntimeInfo(ctx, true)
		assert.NoError(t, err)
		assert.Equal(t, 13, md.Version)
		assert.Equal(t, "testdb", md.RealDbname)
		assert.Contains(t, md.Extensions, "pg_stat_statements")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{
			Conn:   mock,
			Source: sources.Source{Kind: sources.SourcePgBouncer},
		}
		mock.ExpectQuery("SHOW VERSION").WillReturnError(fmt.Errorf("db error"))
		err = md.FetchRuntimeInfo(ctx, true)
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSourceConn_FetchVersion(t *testing.T) {
	ctx := context.Background()

	t.Run("valid version string", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{Conn: mock}
		mock.ExpectQuery("SHOW VERSION").WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow("FooBar 1.12.0"))
		verStr, verInt, err := md.FetchVersion(ctx, "SHOW VERSION")
		assert.NoError(t, err)
		assert.Equal(t, "FooBar 1.12.0", verStr)
		assert.Equal(t, 1_12_00, verInt)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid version string", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{Conn: mock}
		mock.ExpectQuery("SHOW VERSION").WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow("invalid version"))
		_, verInt, err := md.FetchVersion(ctx, "SHOW VERSION")
		assert.Equal(t, 0, verInt)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		md := &sources.SourceConn{Conn: mock}
		mock.ExpectQuery("SHOW VERSION").WillReturnError(assert.AnError)
		_, _, err = md.FetchVersion(ctx, "SHOW VERSION")
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
