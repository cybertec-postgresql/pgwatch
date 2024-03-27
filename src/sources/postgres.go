package sources

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch3/db"
	pgx "github.com/jackc/pgx/v5"
)

func NewPostgresSourcesReaderWriter(ctx context.Context, conn db.PgxPoolIface) (ReaderWriter, error) {
	return &dbSourcesReaderWriter{
		ctx:      ctx,
		configDb: conn,
	}, conn.Ping(ctx)

}

type dbSourcesReaderWriter struct {
	ctx      context.Context
	configDb db.PgxIface
}

func (r *dbSourcesReaderWriter) WriteMonitoredDatabases(dbs MonitoredDatabases) error {
	tx, err := r.configDb.Begin(context.Background())
	if err != nil {
		return err
	}
	if _, err = tx.Exec(context.Background(), `truncate pgwatch3.source`); err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	for _, md := range dbs {
		if err = updateDatabase(tx, md); err != nil {
			return err
		}
	}
	return tx.Commit(context.Background())
}

func updateDatabase(conn db.PgxIface, md MonitoredDatabase) (err error) {
	sql := `insert into pgwatch3.source(
name, "group", dbtype, connstr, config, config_standby, preset_config, 
preset_config_standby, is_superuser, include_pattern, exclude_pattern, custom_tags, host_config, only_if_master) 
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) on conflict (name) do update set
"group" = $2, dbtype = $3, connstr = $4, config = $5, config_standby = $6, preset_config = $7,
preset_config_standby = $8, is_superuser = $9, include_pattern = $10, exclude_pattern = $11, custom_tags = $12,
host_config = $13, only_if_master = $14`
	_, err = conn.Exec(context.Background(), sql,
		md.DBUniqueName, md.Group, md.Kind,
		md.ConnStr, md.Metrics, md.MetricsStandby, md.PresetMetrics, md.PresetMetricsStandby,
		md.IsSuperuser, md.IncludePattern, md.ExcludePattern, md.CustomTags,
		md.HostConfig, md.OnlyIfMaster)
	return err
}

func (r *dbSourcesReaderWriter) UpdateDatabase(md MonitoredDatabase) error {
	return updateDatabase(r.configDb, md)
}

func (r *dbSourcesReaderWriter) DeleteDatabase(name string) error {
	_, err := r.configDb.Exec(context.Background(), `delete from pgwatch3.source where name = $1`, name)
	return err
}

func (r *dbSourcesReaderWriter) GetMonitoredDatabases() (dbs MonitoredDatabases, err error) {
	sqlLatest := `select /* pgwatch3_generated */
	name, 
	"group", 
	dbtype, 
	connstr,
	coalesce(config, '{}'::jsonb) as config, 
	coalesce(config_standby, '{}'::jsonb) as config_standby,
	coalesce(preset_config, '') as preset_config,
	coalesce(preset_config_standby, '') as preset_config_standby,
	is_superuser,
	coalesce(include_pattern, '') as include_pattern, 
	coalesce(exclude_pattern, '') as exclude_pattern,
	coalesce(custom_tags, '{}'::jsonb) as custom_tags, 
	coalesce(host_config, '{}') as host_config, 
	only_if_master,
	is_enabled
from
	pgwatch3.source
`
	rows, err := r.configDb.Query(context.Background(), sqlLatest)
	if err != nil {
		return nil, err
	}
	dbs, err = pgx.CollectRows[MonitoredDatabase](rows, pgx.RowToStructByNameLax)
	return
}
