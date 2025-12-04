package sources

// SourceOpts specifies the sources related command-line options
type CmdOpts struct {
	Sources                      string   `short:"s" long:"sources" mapstructure:"config" description:"Postgres URI, file or folder of YAML files containing info on which DBs to monitor" env:"PW_SOURCES"`
	Refresh                      int      `long:"refresh" mapstructure:"refresh" description:"How frequently to resync sources and metrics" env:"PW_REFRESH" default:"120"`
	Groups                       []string `short:"g" long:"group" mapstructure:"group" description:"Groups for filtering which databases to monitor. By default all are monitored" env:"PW_GROUP"`
	MinDbSizeMB                  int64    `long:"min-db-size-mb" mapstructure:"min-db-size-mb" description:"Smaller size DBs will be ignored and not monitored until they reach the threshold." env:"PW_MIN_DB_SIZE_MB" default:"0"`
	MaxParallelConnectionsPerDb  int      `long:"max-parallel-connections-per-db" mapstructure:"max-parallel-connections-per-db" description:"Max parallel metric fetches per DB. Note the multiplication effect on multi-DB instances" env:"PW_MAX_PARALLEL_CONNECTIONS_PER_DB" default:"4"`
	TryCreateListedExtsIfMissing string   `long:"try-create-listed-exts-if-missing" mapstructure:"try-create-listed-exts-if-missing" description:"Try creating the listed extensions (comma sep.) on first connect for all monitored DBs when missing. Main usage - pg_stat_statements" env:"PW_TRY_CREATE_LISTED_EXTS_IF_MISSING" default:""`
	CreateHelpers                bool     `long:"create-helpers" mapstructure:"create-helpers" description:"Create helper database objects from metric definitions" env:"PW_CREATE_HELPERS"`
	ConnLockTimeout              string   `long:"conn-lock-timeout" mapstructure:"conn-lock-timeout" description:"PostgreSQL lock_timeout for metric query connections. Set to 0 to disable" env:"PW_CONN_LOCK_TIMEOUT" default:"100ms"`
}
