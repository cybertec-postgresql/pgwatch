package sources

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"gopkg.in/yaml.v3"
)

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
