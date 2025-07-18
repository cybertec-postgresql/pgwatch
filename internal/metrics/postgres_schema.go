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

		&migrator.Migration{
			Name: "00824 Refactor recommendations",
			Func: func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `
					-- 1. Update all reco_ metrics to use metric_storage_name = 'recommendations'
					UPDATE pgwatch.metric 
					SET storage_name = 'recommendations' 
					WHERE name LIKE 'reco_%' AND COALESCE(storage_name, '') = '';

					-- 2. Remove the placeholder 'recommendations' metric if it exists
					DELETE FROM pgwatch.metric WHERE name = 'recommendations';

					-- 3. Update 'exhaustive' and 'full' presets to replace 'recommendations' with individual reco_ metrics
					UPDATE pgwatch.preset 
					SET metrics = metrics - 'recommendations' || $reco_metrics${
						"reco_add_index": 43200, 
						"reco_default_public_schema": 50400, 
						"reco_disabled_triggers": 57600, 
						"reco_drop_index": 64800, 
						"reco_nested_views": 72000, 
						"reco_partial_index_candidates": 79200, 
						"reco_sprocs_wo_search_path": 86400, 
						"reco_superusers": 93600
					}$reco_metrics$::jsonb
					WHERE name IN ('exhaustive', 'full') AND metrics ? 'recommendations';

					-- 4. Insert new 'recommendations' preset if it doesn't exist
					INSERT INTO pgwatch.preset (name, description, metrics) 
					VALUES ('recommendations', 'performance and security recommendations', 
						$reco_metrics${
							"reco_add_index": 43200, 
							"reco_default_public_schema": 50400, 
							"reco_disabled_triggers": 57600, 
							"reco_drop_index": 64800, 
							"reco_nested_views": 72000, 
							"reco_partial_index_candidates": 79200, 
							"reco_sprocs_wo_search_path": 86400, 
							"reco_superusers": 93600
						}$reco_metrics$::jsonb)
					ON CONFLICT (name) DO NOTHING;

					-- 5. Update source configs to replace 'recommendations' with individual reco_ metrics
					UPDATE pgwatch.source 
					SET config = config - 'recommendations' || 
						$reco_metrics${
							"reco_add_index": 43200, 
							"reco_default_public_schema": 50400, 
							"reco_disabled_triggers": 57600, 
							"reco_drop_index": 64800, 
							"reco_nested_views": 72000, 
							"reco_partial_index_candidates": 79200, 
							"reco_sprocs_wo_search_path": 86400, 
							"reco_superusers": 93600
						}$reco_metrics$::jsonb
					WHERE config ? 'recommendations';

					-- 6. Update source standby configs to replace 'recommendations' with individual reco_ metrics
					UPDATE pgwatch.source 
					SET config_standby = config_standby - 'recommendations' || 
						$reco_metrics${
							"reco_add_index": 43200, 
							"reco_default_public_schema": 50400, 
							"reco_disabled_triggers": 57600, 
							"reco_drop_index": 64800, 
							"reco_nested_views": 72000, 
							"reco_partial_index_candidates": 79200, 
							"reco_sprocs_wo_search_path": 86400, 
							"reco_superusers": 93600
						}$reco_metrics$::jsonb
					WHERE config_standby ? 'recommendations';
				`)
				return err
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
