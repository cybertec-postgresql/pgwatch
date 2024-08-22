package sources

// This file contains the implementation of the ReaderWriter interface for the PostgreSQL database.
// Monitored sources are stored in the `pgwatch.source` table in the configuration database.

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch/db"
	pgx "github.com/jackc/pgx/v5"
)

func NewPostgresSourcesReaderWriter(ctx context.Context, connstr string) (ReaderWriter, error) {
	conn, err := db.New(ctx, connstr)
	if err != nil {
		return nil, err
	}
	return NewPostgresSourcesReaderWriterConn(ctx, conn)
}

func NewPostgresSourcesReaderWriterConn(ctx context.Context, conn db.PgxPoolIface) (ReaderWriter, error) {
	return &dbSourcesReaderWriter{
		ctx:      ctx,
		configDb: conn,
	}, conn.Ping(ctx)

}

type dbSourcesReaderWriter struct {
	ctx      context.Context
	configDb db.PgxIface
}

func (r *dbSourcesReaderWriter) WriteSources(dbs Sources) error {
	tx, err := r.configDb.Begin(context.Background())
	if err != nil {
		return err
	}
	if _, err = tx.Exec(context.Background(), `truncate pgwatch.source`); err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	for _, md := range dbs {
		if err = r.updateDatabase(tx, md); err != nil {
			return err
		}
	}
	return tx.Commit(context.Background())
}

func (r *dbSourcesReaderWriter) updateDatabase(conn db.PgxIface, md Source) (err error) {
	m := db.MarshallParam
	sql := `insert into pgwatch.source(
	name, 
	"group", 
	dbtype, 
	connstr, 
	config, 
	config_standby, 
	preset_config, 
	preset_config_standby, 
	is_superuser, 
	include_pattern, 
	exclude_pattern, 
	custom_tags, 
	host_config, 
	only_if_master) 
values 
	($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12, $13, $14) 
on conflict (name) do update set
	"group" = $2, 
	dbtype = $3, 
	connstr = $4, 
	config = $5, 
	config_standby = $6, 
	preset_config = NULLIF($7, ''),
	preset_config_standby = NULLIF($8, ''), 
	is_superuser = $9, 
	include_pattern = $10, 
	exclude_pattern = $11, 
	custom_tags = $12,
	host_config = $13, 
	only_if_master = $14`
	_, err = conn.Exec(context.Background(), sql,
		md.Name, md.Group, md.Kind,
		md.ConnStr, m(md.Metrics), m(md.MetricsStandby), md.PresetMetrics, md.PresetMetricsStandby,
		md.IsSuperuser, md.IncludePattern, md.ExcludePattern, m(md.CustomTags),
		m(md.HostConfig), md.OnlyIfMaster)
	return err
}

func (r *dbSourcesReaderWriter) UpdateSource(md Source) error {
	return r.updateDatabase(r.configDb, md)
}

func (r *dbSourcesReaderWriter) DeleteSource(name string) error {
	_, err := r.configDb.Exec(context.Background(), `delete from pgwatch.source where name = $1`, name)
	return err
}

func (r *dbSourcesReaderWriter) GetSources() (dbs Sources, err error) {
	sqlLatest := `select /* pgwatch_generated */
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
	pgwatch.source`
	rows, err := r.configDb.Query(context.Background(), sqlLatest)
	if err != nil {
		return nil, err
	}
	dbs, err = pgx.CollectRows[Source](rows, pgx.RowToStructByNameLax)
	return
}
