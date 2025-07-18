package metrics

import (
	"testing"

	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
)

func TestMigrate(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	conn.ExpectExec(`CREATE TABLE IF NOT EXISTS pgwatch\.migration`).WillReturnResult(pgxmock.NewResult("CREATE", 1))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	conn.ExpectBegin()
	conn.ExpectExec(`INSERT INTO`).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	conn.ExpectBegin()
	conn.ExpectExec(`UPDATE pgwatch\.metric`).WillReturnResult(pgxmock.NewResult("UPDATE", 0)) // combined migration SQL without parameters
	conn.ExpectExec(`INSERT INTO`).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	dmrw := &dbMetricReaderWriter{ctx, conn}
	err = dmrw.Migrate()
	a.NoError(err)
}

func TestNeedsMigration(t *testing.T) {
	a := assert.New(t)
	conn, err := pgxmock.NewPool()
	a.NoError(err)

	conn.ExpectQuery(`SELECT to_regclass`).
		WithArgs("pgwatch.migration").
		WillReturnRows(pgxmock.NewRows([]string{"to_regclass"}).AddRow(true))
	conn.ExpectQuery(`SELECT count`).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

	dmrw := &dbMetricReaderWriter{ctx, conn}
	needs, err := dmrw.NeedsMigration()
	a.NoError(err)
	a.True(needs)
}

func TestMigrateFail(t *testing.T) {
	oldInitMigrator := initMigrator
	t.Cleanup(func() {
		initMigrator = oldInitMigrator
	})
	a := assert.New(t)
	dmrw := &dbMetricReaderWriter{}
	initMigrator = func(*dbMetricReaderWriter) (*migrator.Migrator, error) {
		return nil, assert.AnError
	}
	err := dmrw.Migrate()
	a.Error(err)
}

func TestNeedsMigrationFail(t *testing.T) {
	oldInitMigrator := initMigrator
	t.Cleanup(func() {
		initMigrator = oldInitMigrator
	})
	a := assert.New(t)
	dmrw := &dbMetricReaderWriter{}
	initMigrator = func(*dbMetricReaderWriter) (*migrator.Migrator, error) {
		return nil, assert.AnError
	}
	_, err := dmrw.NeedsMigration()
	a.Error(err)
}
