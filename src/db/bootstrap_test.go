package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cybertec-postgresql/pgwatch3/db"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var ctx = context.Background()

func TestGetTableColumns(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	conn.ExpectQuery("SELECT attname").
		WithArgs("foo").
		WillReturnError(errors.New("expected"))
	_, err = db.GetTableColumns(ctx, conn, "foo")
	assert.Error(t, err)

	conn.ExpectQuery("SELECT attname").
		WithArgs("foo").
		WillReturnRows(pgxmock.NewRows([]string{"attname"}).AddRow("col1").AddRow("col2"))
	cols, err := db.GetTableColumns(ctx, conn, "foo")
	assert.NoError(t, err)
	assert.Equal(t, []string{"col1", "col2"}, cols)
}

func TestPing(t *testing.T) {
	connStr := "foo_boo"
	assert.Error(t, db.Ping(ctx, connStr))

	pg, err := initTestContainer()
	require.NoError(t, err)
	connStr, err = pg.ConnectionString(ctx)
	assert.NoError(t, err)
	assert.NoError(t, db.Ping(ctx, connStr))
	assert.NoError(t, pg.Terminate(ctx))
}

func TestDoesSchemaExist(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)
	conn.ExpectQuery("SELECT EXISTS").
		WithArgs("public").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	exists, err := db.DoesSchemaExist(ctx, conn, "public")
	assert.NoError(t, err)
	assert.True(t, exists)
}
func TestInit(t *testing.T) {

	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)
	initCalled := false
	initFunc := func(context.Context, db.PgxIface) error {
		initCalled = true
		return nil
	}

	// Test successful initialization
	conn.ExpectPing()
	err = db.Init(ctx, conn, initFunc)
	assert.NoError(t, err)
	assert.True(t, initCalled)

	// Test failed initialization with 3 retries
	conn.ExpectPing().Times(1 + 3).WillReturnError(errors.New("connection failed"))
	initCalled = false
	err = db.Init(ctx, conn, initFunc)
	assert.Error(t, err)
	assert.False(t, initCalled)

	assert.NoError(t, conn.ExpectationsWereMet())
}

func initTestContainer() (*postgres.PostgresContainer, error) {
	dbName := "pgwatch3"
	dbUser := "pgwatch3"
	dbPassword := "pgwatch3admin"

	return postgres.Run(ctx,
		"docker.io/postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
}
func TestNew(t *testing.T) {
	pg, err := initTestContainer()
	assert.NoError(t, err)
	defer func() { assert.NoError(t, pg.Terminate(ctx)) }()
	connStr, err := pg.ConnectionString(ctx)
	t.Log(connStr)
	assert.NoError(t, err)

	initCalled := false
	initFunc := func(*pgxpool.Config) error {
		initCalled = true
		return nil
	}
	// Test successful initialization
	pool, err := db.New(context.Background(), connStr, initFunc)
	assert.NoError(t, err)
	assert.NotNil(t, pool)
	assert.True(t, initCalled)
	_, err = pool.Exec(ctx, `DO $$
BEGIN
   RAISE NOTICE 'This is a notice';
END $$;`)
	assert.NoError(t, err)
	pool.Close()

	// Test failed initialization
	initCalled = false
	pool, err = db.New(context.Background(), "foo", initFunc)
	assert.Error(t, err)
	assert.Nil(t, pool)
	assert.False(t, initCalled)

	// Test failed initialization with callback
	initFunc = func(*pgxpool.Config) error {
		return errors.New("callback failed")
	}
	initCalled = false
	pool, err = db.New(context.Background(), connStr, initFunc)
	assert.Error(t, err)
	assert.Nil(t, pool)
	assert.False(t, initCalled)
}
