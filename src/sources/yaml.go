package sources

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"gopkg.in/yaml.v3"
)

func NewYAMLSourcesReader(ctx context.Context, opts *config.Options) (Reader, error) {
	return &fileSourcesReader{
		ctx:  ctx,
		opts: opts,
	}, nil
}

type fileSourcesReader struct {
	ctx  context.Context
	opts *config.Options
}

func (fcr *fileSourcesReader) GetMonitoredDatabases() (dbs MonitoredDatabases, err error) {
	var fi fs.FileInfo
	if fi, err = os.Stat(fcr.opts.Sources.Config); err != nil {
		return
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		err = filepath.WalkDir(fcr.opts.Sources.Config, func(path string, d fs.DirEntry, err error) error {
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
		dbs, err = fcr.getMonitoredDatabases(fcr.opts.Sources.Config)
	}
	if err != nil {
		return nil, err
	}
	return
}

func (fcr *fileSourcesReader) getMonitoredDatabases(configFilePath string) (dbs MonitoredDatabases, err error) {
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
		dbs = append(dbs, fcr.expandEnvVars(v))
	}
	return
}

func (fcr *fileSourcesReader) expandEnvVars(md MonitoredDatabase) MonitoredDatabase {
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
