package metrics

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	migrator "github.com/cybertec-postgresql/pgx-migrator"
	"github.com/jackc/pgx/v5"
)

//go:embed postgres_schema.sql
var sqlConfigSchema string

var initSchema = func(ctx context.Context, conn db.PgxIface) (err error) {
	var exists bool
	if exists, err = db.DoesSchemaExist(ctx, conn, "pgwatch"); err != nil || exists {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, sqlConfigSchema); err != nil {
		return err
	}
	if err := writeMetricsToPostgres(ctx, tx, GetDefaultMetrics()); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

var initMigrator = func(dmrw *dbMetricReaderWriter) (*migrator.Migrator, error) {
	return migrator.New(
		migrator.TableName("pgwatch.migration"),
		migrator.SetNotice(func(s string) {
			log.GetLogger(dmrw.ctx).Info(s)
		}),
		migrations(),
	)
}

// MigrateDb upgrades database with all migrations
func (dmrw *dbMetricReaderWriter) Migrate() error {
	m, err := initMigrator(dmrw)
	if err != nil {
		return fmt.Errorf("cannot initialize migration: %w", err)
	}
	return m.Migrate(dmrw.ctx, dmrw.configDb)
}

// NeedsMigration checks if database needs migration
func (dmrw *dbMetricReaderWriter) NeedsMigration() (bool, error) {
	m, err := initMigrator(dmrw)
	if err != nil {
		return false, err
	}
	return m.NeedUpgrade(dmrw.ctx, dmrw.configDb)
}

// migrations holds function returning all updgrade migrations needed
var migrations func() migrator.Option = func() migrator.Option {
	return migrator.Migrations(
		&migrator.Migration{
			Name: "00179 Apply metrics migrations for v3",
			Func: func(context.Context, pgx.Tx) error {
				// "migrations" table will be created automatically
				return nil
			},
		},

		// adding new migration here, update "pgwatch"."migration" in "postgres_schema.sql"
		// and "dbapi" variable in main.go!

		// &migrator.Migration{
		// 	Name: "000XX Short description of a migration",
		// 	Func: func(ctx context.Context, tx pgx.Tx) error {
		// 		return executeMigrationScript(ctx, tx, "000XX.sql")
		// 	},
		// },
	)
}
