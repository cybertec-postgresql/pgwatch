package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch3/db"
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

func TestExecuteSchemaScripts(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	conn.ExpectPing()
	conn.ExpectQuery("SELECT EXISTS").
		WithArgs("admin").
		WillReturnError(errors.New("expected"))
	err = db.InitMeasurementDb(ctx, conn)
	assert.Error(t, err)

	conn.ExpectPing()
	conn.ExpectQuery("SELECT EXISTS").
		WithArgs("pgwatch3").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	err = db.InitConfigDb(ctx, conn)
	assert.NoError(t, err)
}
