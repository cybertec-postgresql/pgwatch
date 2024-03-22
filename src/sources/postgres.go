package sources

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	pgx "github.com/jackc/pgx/v5"
)

func NewPostgresSourcesReader(ctx context.Context, conn db.PgxPoolIface, opts *config.Options) (Reader, error) {
	return &dbSourcesReader{
		ctx:      ctx,
		configDb: conn,
		opts:     opts,
	}, conn.Ping(ctx)

}

type dbSourcesReader struct {
	ctx      context.Context
	configDb db.Querier
	opts     *config.Options
}

func (r *dbSourcesReader) GetMonitoredDatabases() (dbs MonitoredDatabases, err error) {
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
	encryption, coalesce(host_config, '{}') as host_config, 
	only_if_master
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

// "resolving" reads all the DB names from the given host/port, additionally matching/not matching specified regex patterns
func (ce MonitoredDatabase) ResolveDatabasesFromConfigEntry() (resolvedDbs MonitoredDatabases, err error) {
	var (
		c      db.PgxPoolIface
		dbname string
		rows   pgx.Rows
	)
	c, err = db.New(context.TODO(), ce.ConnStr)
	if err != nil {
		return
	}
	defer c.Close()

	sql := `select /* pgwatch3_generated */
		quote_ident(datname)::text as datname_escaped
		from pg_database
		where not datistemplate
		and datallowconn
		and has_database_privilege (datname, 'CONNECT')
		and case when length(trim($1)) > 0 then datname ~ $1 else true end
		and case when length(trim($2)) > 0 then not datname ~ $2 else true end`

	if rows, err = c.Query(context.TODO(), sql, ce.IncludePattern, ce.ExcludePattern); err != nil {
		return nil, err
	}
	for rows.Next() {
		if err = rows.Scan(&dbname); err != nil {
			return nil, err
		}
		rdb := ce
		rdb.DBUniqueName += "_" + dbname
		resolvedDbs = append(resolvedDbs, rdb)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return
}
