package sources

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	pgx "github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v2"
)

type querier interface {
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
}

type Reader interface {
	GetMonitoredDatabases() (MonitoredDatabases, error)
}

func NewYAMLConfigReader(ctx context.Context, opts *config.Options) (Reader, error) {
	return &fileConfigReader{
		ctx:  ctx,
		opts: opts,
	}, nil
}

type fileConfigReader struct {
	ctx  context.Context
	opts *config.Options
}

func (fcr *fileConfigReader) GetMonitoredDatabases() (dbs MonitoredDatabases, err error) {
	var fi fs.FileInfo
	if fi, err = os.Stat(fcr.opts.Source.Config); err != nil {
		return
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		err = filepath.WalkDir(fcr.opts.Source.Config, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := strings.ToLower(d.Name())
			if d.IsDir() || !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
				return nil
			}
			var mdbs MonitoredDatabases
			if mdbs, err = fcr.getMonitoredDatabases(path); err == nil {
				dbs = append(dbs, mdbs...)
			}
			return err
		})
	case mode.IsRegular():
		dbs, err = fcr.getMonitoredDatabases(fcr.opts.Source.Config)
	}
	if err != nil {
		return nil, err
	}
	return dbs.Expand()
}

func (fcr *fileConfigReader) getMonitoredDatabases(configFilePath string) (dbs MonitoredDatabases, err error) {
	var yamlFile []byte
	if yamlFile, err = os.ReadFile(configFilePath); err != nil {
		return
	}
	c := make([]MonitoredDatabase, 0) // there can be multiple configs in a single file
	if err = yaml.Unmarshal(yamlFile, &c); err != nil {
		return
	}
	for _, v := range c {
		if v.Kind == "" {
			v.Kind = SourcePostgres
		}
		if v.IsEnabled && (len(fcr.opts.Source.Group) == 0 || slices.Contains(fcr.opts.Source.Group, v.Group)) {
			dbs = append(dbs, fcr.expandEnvVars(v))
		}
	}
	return
}

func (fcr *fileConfigReader) expandEnvVars(md MonitoredDatabase) MonitoredDatabase {
	if strings.HasPrefix(md.Encryption, "$") {
		md.Encryption = os.ExpandEnv(md.Encryption)
	}
	if strings.HasPrefix(string(md.Kind), "$") {
		md.Kind = Kind(os.ExpandEnv(string(md.Kind)))
	}
	if strings.HasPrefix(md.DBUniqueName, "$") {
		md.DBUniqueName = os.ExpandEnv(md.DBUniqueName)
	}
	if strings.HasPrefix(md.IncludePattern, "$") {
		md.IncludePattern = os.ExpandEnv(md.IncludePattern)
	}
	if strings.HasPrefix(md.ExcludePattern, "$") {
		md.ExcludePattern = os.ExpandEnv(md.ExcludePattern)
	}
	if strings.HasPrefix(md.PresetMetrics, "$") {
		md.PresetMetrics = os.ExpandEnv(md.PresetMetrics)
	}
	if strings.HasPrefix(md.PresetMetricsStandby, "$") {
		md.PresetMetricsStandby = os.ExpandEnv(md.PresetMetricsStandby)
	}
	return md
}

func NewPostgresConfigReader(ctx context.Context, opts *config.Options) (Reader, error) {
	configDb, err := db.InitAndTestConfigStoreConnection(ctx, opts.Source.Config)
	return &dbConfigReader{
		ctx:      ctx,
		configDb: configDb,
		opts:     opts,
	}, err
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
	if len(r.opts.Source.Group) > 0 {
		sqlLatest += " and md_group in (" + strings.Join(r.opts.Source.Group, ",") + ")"
	}
	rows, err := r.configDb.Query(context.Background(), sqlLatest)
	if err != nil {
		return nil, err
	}
	dbs, err = pgx.CollectRows[MonitoredDatabase](rows, pgx.RowToStructByNameLax)
	for _, md := range dbs {
		if md.Encryption == "aes-gcm-256" && r.opts.Source.AesGcmKeyphrase != "" {
			md.ConnStr = r.opts.Decrypt(md.ConnStr)
		}
	}
	return
}
