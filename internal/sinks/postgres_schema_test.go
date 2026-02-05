package sinks

import (
	"context"
	"testing"

	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
)

func TestPostgresWriterMigrate(t *testing.T) {
	oldInitMigrator := initMigrator
	t.Cleanup(func() {
		initMigrator = oldInitMigrator
	})

	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	// Mock the migrator to use simple migrations for testing
	initMigrator = func(_ *PostgresWriter) (*migrator.Migrator, error) {
		return migrator.New(
			migrator.TableName("admin.migration"),
			migrator.Migrations(
				&migrator.Migration{
					Name: "Test migration 1",
					Func: func(ctx context.Context, tx pgx.Tx) error {
						_, err := tx.Query(ctx, "SELECT 1 AS col1")
						return err
					},
				},
				&migrator.Migration{
					Name: "Test migration 2",
					Func: func(ctx context.Context, tx pgx.Tx) error {
						_, err := tx.Query(ctx, "SELECT 2 AS col2")
						return err
					},
				},
			),
		)
	}

	conn.ExpectExec(`CREATE TABLE IF NOT EXISTS admin\.migration`).WillReturnResult(pgxmock.NewResult("CREATE", 1))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	// Expect transaction for migration 1 execution
	conn.ExpectBegin()
	conn.ExpectQuery(`SELECT 1 AS col1`).WillReturnRows(pgxmock.NewRows([]string{"col1"}).AddRow(1))
	conn.ExpectExec(`INSERT INTO`).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	conn.ExpectCommit()

	// Expect transaction for migration 2 execution
	conn.ExpectBegin()
	conn.ExpectQuery(`SELECT 2 AS col2`).WillReturnRows(pgxmock.NewRows([]string{"col2"}).AddRow(2))
	conn.ExpectExec(`INSERT INTO`).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	conn.ExpectCommit()

	pgw := &PostgresWriter{ctx: ctx, sinkDb: conn}
	err = pgw.Migrate()
	a.NoError(err)
	a.NoError(conn.ExpectationsWereMet())
}

func TestPostgresWriterNeedsMigration(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	// Expect checks for migration table existence and pending migrations
	conn.ExpectQuery(`SELECT to_regclass`).
		WithArgs("admin.migration").
		WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	pgw := &PostgresWriter{ctx: ctx, sinkDb: conn}
	needs, err := pgw.NeedsMigration()
	a.NoError(err)
	a.True(needs)
	a.NoError(conn.ExpectationsWereMet())
}

func TestPostgresWriterNeedsMigrationNoMigrationNeeded(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	// Expect checks for migration table existence and no pending migrations
	conn.ExpectQuery(`SELECT to_regclass`).
		WithArgs("admin.migration").
		WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))

	pgw := &PostgresWriter{ctx: ctx, sinkDb: conn}
	needs, err := pgw.NeedsMigration()
	a.NoError(err)
	a.False(needs)
	a.NoError(conn.ExpectationsWereMet())
}

func TestPostgresWriterMigrateFail(t *testing.T) {
	oldInitMigrator := initMigrator
	t.Cleanup(func() {
		initMigrator = oldInitMigrator
	})
	a := assert.New(t)
	pgw := &PostgresWriter{ctx: ctx}
	initMigrator = func(*PostgresWriter) (*migrator.Migrator, error) {
		return nil, assert.AnError
	}
	err := pgw.Migrate()
	a.Error(err)
	a.Contains(err.Error(), "cannot initialize migration")
}

func TestPostgresWriterNeedsMigrationFail(t *testing.T) {
	oldInitMigrator := initMigrator
	t.Cleanup(func() {
		initMigrator = oldInitMigrator
	})
	a := assert.New(t)
	pgw := &PostgresWriter{ctx: ctx}
	initMigrator = func(*PostgresWriter) (*migrator.Migrator, error) {
		return nil, assert.AnError
	}
	_, err := pgw.NeedsMigration()
	a.Error(err)
}
