package sources

import (
	"context"
	"strings"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	pgx "github.com/jackc/pgx/v5"
)

type querier interface {
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
}

func NewPostgresConfigReader(ctx context.Context, opts *config.Options) (Reader, db.PgxPoolIface, error) {
	configDb, err := db.New(ctx, opts.Sources.Config)
	if err != nil {
		return nil, nil, err
	}
	return &dbConfigReader{
		ctx:      ctx,
		configDb: configDb,
		opts:     opts,
	}, configDb, db.InitConfigDb(ctx, configDb)
}

type dbConfigReader struct {
	ctx      context.Context
	configDb querier
	opts     *config.Options
}

func (r *dbConfigReader) GetMonitoredDatabases() (dbs MonitoredDatabases, err error) {
	sqlLatest := `select /* pgwatch3_generated */
	md_name, 
	md_group, 
	md_dbtype, 
	md_connstr,
	coalesce(p.pc_config, md_config) as md_config, 
	coalesce(s.pc_config, md_config_standby, '{}'::jsonb) as md_config_standby,
	md_is_superuser,
	coalesce(md_include_pattern, '') as md_include_pattern, 
	coalesce(md_exclude_pattern, '') as md_exclude_pattern,
	coalesce(md_custom_tags, '{}'::jsonb) as md_custom_tags, 
	md_encryption, coalesce(md_host_config, '{}') as md_host_config, 
	md_only_if_master
from
	pgwatch3.monitored_db 
	left join pgwatch3.preset_config p on p.pc_name = md_preset_config_name
	left join pgwatch3.preset_config s on s.pc_name = md_preset_config_name_standby
where
	md_is_enabled`
	if len(r.opts.Sources.Groups) > 0 {
		sqlLatest += " and md_group in (" + strings.Join(r.opts.Sources.Groups, ",") + ")"
	}
	rows, err := r.configDb.Query(context.Background(), sqlLatest)
	if err != nil {
		return nil, err
	}
	dbs, err = pgx.CollectRows[MonitoredDatabase](rows, pgx.RowToStructByNameLax)
	for _, md := range dbs {
		if md.Encryption == "aes-gcm-256" && r.opts.Sources.AesGcmKeyphrase != "" {
			md.ConnStr = r.opts.Decrypt(md.ConnStr)
		}
	}
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
