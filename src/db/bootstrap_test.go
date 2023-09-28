package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestGetMetricSchemaType(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	conn.ExpectQuery("SELECT schema_type").
		WillReturnError(errors.New("expected"))
	_, err = db.GetMetricSchemaType(ctx, conn)
	assert.Error(t, err)

	conn.ExpectQuery("SELECT schema_type").
		WillReturnRows(pgxmock.NewRows([]string{"schema_type"}).AddRow(true))
	schemaType, err := db.GetMetricSchemaType(context.Background(), conn)
	assert.NoError(t, err)
	assert.Equal(t, db.MetricSchemaTimescale, schemaType)
}

func TestExecuteSchemaScripts(t *testing.T) {
	conn, err := pgxmock.NewPool()
	assert.NoError(t, err)

	conn.ExpectQuery("SELECT EXISTS").
		WithArgs("admin").
		WillReturnError(errors.New("expected"))
	err = db.ExecuteMetricSchemaScripts(ctx, conn)
	assert.Error(t, err)

	conn.ExpectQuery("SELECT EXISTS").
		WithArgs("pgwatch3").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	err = db.ExecuteConfigSchemaScripts(ctx, conn)
	assert.NoError(t, err)

	conn.ExpectQuery("SELECT EXISTS").
		WithArgs("pgwatch3").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	conn.
		ExpectExec("create schema if not exists pgwatch3").
		WillReturnResult(pgconn.NewCommandTag("OK"))
	errMetricCreation := errors.New("metric creation failed")
	conn.
		ExpectExec("-- metric definitions").
		WillReturnError(errMetricCreation)
	err = db.ExecuteConfigSchemaScripts(ctx, conn)
	assert.ErrorIs(t, err, errMetricCreation)
}
