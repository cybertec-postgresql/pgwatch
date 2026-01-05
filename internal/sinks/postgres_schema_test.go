package sinks

import (
	"testing"

	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
)

func TestPostgresWriterMigrate(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	// Expect migration table creation and migration execution
	conn.ExpectExec(`CREATE TABLE IF NOT EXISTS admin\.migration`).WillReturnResult(pgxmock.NewResult("CREATE", 1))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	conn.ExpectBegin()
	conn.ExpectExec(`INSERT INTO`).WillReturnResult(pgxmock.NewResult("INSERT", 1))

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
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))

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
