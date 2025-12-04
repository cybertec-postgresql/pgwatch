package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
)

var ctx = context.Background()

func TestPing(t *testing.T) {
	connStr := "foo_boo"
	assert.Error(t, db.Ping(ctx, connStr))

	pg, teardown, err := testutil.SetupPostgresContainer()
	defer teardown()
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

func TestNew(t *testing.T) {
	pg, teardown, err := testutil.SetupPostgresContainer()
	defer teardown()
	require.NoError(t, err)
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
