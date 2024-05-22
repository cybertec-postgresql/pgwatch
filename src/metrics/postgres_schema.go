package metrics

import (
	"context"
	"fmt"

	"github.com/cybertec-postgresql/pgwatch3/log"
	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/jackc/pgx/v5"
)

var pgxMigrator *migrator.Migrator

func (dmrw *dbMetricReaderWriter) initMigrator() (*migrator.Migrator, error) {
	if pgxMigrator != nil {
		return pgxMigrator, nil
	}
	m, err := migrator.New(
		migrator.TableName("pgwatch3.migration"),
		migrator.SetNotice(func(s string) {
			log.GetLogger(dmrw.ctx).Info(s)
		}),
		migrations(),
	)
	return m, fmt.Errorf("cannot initialize migration: %w", err)
}

// MigrateDb upgrades database with all migrations
func (dmrw *dbMetricReaderWriter) Migrate() error {
	m, err := dmrw.initMigrator()
	if err != nil {
		return err
	}
	return m.Migrate(dmrw.ctx, dmrw.configDb)
}

// NeedsMigration checks if database needs migration
func (dmrw *dbMetricReaderWriter) NeedsMigration() (bool, error) {
	m, err := dmrw.initMigrator()
	if err != nil {
		return false, err
	}
	return m.NeedUpgrade(dmrw.ctx, dmrw.configDb)
}

// Migrations holds function returning all updgrade migrations needed
var migrations func() migrator.Option = func() migrator.Option {
	return migrator.Migrations(
		&migrator.Migration{
			Name: "00179 Apply metrics migrations for v3",
			Func: func(context.Context, pgx.Tx) error {
				// "migrations" table will be created automatically
				return nil
			},
		},

		// adding new migration here, update "pgwatch3"."migration" in "postgres_schema.sql"
		// and "dbapi" variable in main.go!

		// &migrator.Migration{
		// 	Name: "000XX Short description of a migration",
		// 	Func: func(ctx context.Context, tx pgx.Tx) error {
		// 		return executeMigrationScript(ctx, tx, "000XX.sql")
		// 	},
		// },
	)
}
