package sources_test

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var ctx = context.Background()

func TestKind_IsValid(t *testing.T) {
	tests := []struct {
		kind     sources.Kind
		expected bool
	}{
		{kind: sources.SourcePostgres, expected: true},
		{kind: sources.SourcePostgresContinuous, expected: true},
		{kind: sources.SourcePgBouncer, expected: true},
		{kind: sources.SourcePgPool, expected: true},
		{kind: sources.SourcePatroni, expected: true},
		{kind: sources.SourcePatroniContinuous, expected: true},
		{kind: sources.SourcePatroniNamespace, expected: true},
		{kind: "invalid", expected: false},
	}

	for _, tt := range tests {
		got := tt.kind.IsValid()
		assert.True(t, got == tt.expected, "IsValid(%v) = %v, want %v", tt.kind, got, tt.expected)
	}
}

func TestMonitoredDatabase_Connect(t *testing.T) {
	pgContainer, err := postgres.Run(ctx,
		"docker.io/postgres:16-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	assert.NoError(t, err)
	defer func() { assert.NoError(t, pgContainer.Terminate(ctx)) }()

	// Create a new MonitoredDatabase instance
	md := &sources.MonitoredDatabase{}
	md.ConnStr, err = pgContainer.ConnectionString(ctx)
	assert.NoError(t, err)

	// Call the Connect method
	err = md.Connect(ctx, sources.CmdOpts{})
	assert.NoError(t, err)

	// Check cached connection
	err = md.Connect(ctx, sources.CmdOpts{})
	assert.NoError(t, err)
}
func TestMonitoredDatabase_GetDatabaseName(t *testing.T) {
	md := &sources.MonitoredDatabase{}
	md.ConnStr = "postgres://user:password@localhost:5432/mydatabase"
	expected := "mydatabase"
	// check pgx.ConnConfig related code
	got := md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
	// check ConnStr parsing
	got = md.Source.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)

	md = &sources.MonitoredDatabase{}
	md.ConnStr = "foo boo"
	expected = ""
	got = md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
}

func TestMonitoredDatabase_SetDatabaseName(t *testing.T) {
	md := &sources.MonitoredDatabase{}
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

	md = &sources.MonitoredDatabase{}
	md.ConnStr = "foo boo"
	expected = ""
	md.SetDatabaseName("ingored due to invalid ConnStr")
	got = md.GetDatabaseName()
	assert.Equal(t, expected, got, "GetDatabaseName() = %v, want %v", got, expected)
}

func TestMonitoredDatabase_IsPostgresSource(t *testing.T) {
	md := &sources.MonitoredDatabase{}
	md.Kind = sources.SourcePostgres
	assert.True(t, md.IsPostgresSource(), "IsPostgresSource() = false, want true")

	md.Kind = sources.SourcePgBouncer
	assert.False(t, md.IsPostgresSource(), "IsPostgresSource() = true, want false")

	md.Kind = sources.SourcePgPool
	assert.False(t, md.IsPostgresSource(), "IsPostgresSource() = true, want false")

	md.Kind = sources.SourcePatroni
	assert.True(t, md.IsPostgresSource(), "IsPostgresSource() = false, want true")
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
	mds := sources.MonitoredDatabases{}
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
