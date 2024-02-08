package sources

import (
	"context"

	"github.com/cybertec-postgresql/pgwatch3/db"
	pgx "github.com/jackc/pgx/v5"
)

// "resolving" reads all the DB names from the given host/port, additionally matching/not matching specified regex patterns
func ResolveDatabasesFromConfigEntry(ce MonitoredDatabase) (resolvedDbs MonitoredDatabases, err error) {
	var (
		c      db.PgxPoolIface
		dbname string
		rows   pgx.Rows
	)
	c, err = db.GetPostgresDBConnection(context.TODO(), ce.ConnStr)
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
