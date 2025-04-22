package sources_test

import (
	"context"
	"testing"

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
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	md := &sources.SourceConn{Conn: mock}

	mock.ExpectQuery("select").WillReturnRows(pgxmock.NewRows([]string{"exec_env"}).AddRow("AZURE_SINGLE"))

	assert.Equal(t, "AZURE_SINGLE", md.DiscoverPlatform(ctx))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSourceConn_GetApproxSize(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	md := &sources.SourceConn{Conn: mock}

	mock.ExpectQuery("select").WillReturnRows(pgxmock.NewRows([]string{"size"}).AddRow(42))

	size, err := md.GetApproxSize(ctx)
	assert.EqualValues(t, 42, size)
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
