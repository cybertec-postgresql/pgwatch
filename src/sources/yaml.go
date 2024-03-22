package sources

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

func NewYAMLSourcesReaderWriter(ctx context.Context, path string) (ReaderWriter, error) {
	return &fileSourcesReaderWriter{
		ctx:  ctx,
		path: path,
	}, nil
}

type fileSourcesReaderWriter struct {
	ctx  context.Context
	path string
}

func (fcr *fileSourcesReaderWriter) WriteMonitoredDatabases(mds MonitoredDatabases) error {
	yamlData, err := yaml.Marshal(mds)
	if err != nil {
		return err
	}
	return os.WriteFile(fcr.path, yamlData, 0644)
}

func (fcr *fileSourcesReaderWriter) UpdateDatabase(md MonitoredDatabase) error {
	dbs, err := fcr.GetMonitoredDatabases()
	if err != nil {
		return err
	}
	for i, db := range dbs {
		if db.DBUniqueName == md.DBUniqueName {
			dbs[i] = md
			return fcr.WriteMonitoredDatabases(dbs)
		}
	}
	dbs = append(dbs, md)
	return fcr.WriteMonitoredDatabases(dbs)
}

func (fcr *fileSourcesReaderWriter) DeleteDatabase(name string) error {
	dbs, err := fcr.GetMonitoredDatabases()
	if err != nil {
		return err
	}
	dbs = slices.DeleteFunc(dbs, func(md MonitoredDatabase) bool { return md.DBUniqueName == name })
	return fcr.WriteMonitoredDatabases(dbs)
}

func (fcr *fileSourcesReaderWriter) GetMonitoredDatabases() (dbs MonitoredDatabases, err error) {
	var fi fs.FileInfo
	if fi, err = os.Stat(fcr.path); err != nil {
		return
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		err = filepath.WalkDir(fcr.path, func(path string, d fs.DirEntry, err error) error {
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
		dbs, err = fcr.getMonitoredDatabases(fcr.path)
	}
	if err != nil {
		return nil, err
	}
	return
}

func (fcr *fileSourcesReaderWriter) getMonitoredDatabases(configFilePath string) (dbs MonitoredDatabases, err error) {
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

func (fcr *fileSourcesReaderWriter) expandEnvVars(md MonitoredDatabase) MonitoredDatabase {
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
