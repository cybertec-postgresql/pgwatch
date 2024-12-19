package sources

// This file contains the implementation of the ReaderWriter interface for the YAML file.

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

func (fcr *fileSourcesReaderWriter) WriteSources(mds Sources) error {
	yamlData, _ := yaml.Marshal(mds)
	return os.WriteFile(fcr.path, yamlData, 0644)
}

func (fcr *fileSourcesReaderWriter) UpdateSource(md Source) error {
	dbs, err := fcr.GetSources()
	if err != nil {
		return err
	}
	for i, db := range dbs {
		if db.Name == md.Name {
			dbs[i] = md
			return fcr.WriteSources(dbs)
		}
	}
	dbs = append(dbs, md)
	return fcr.WriteSources(dbs)
}

func (fcr *fileSourcesReaderWriter) DeleteSource(name string) error {
	dbs, err := fcr.GetSources()
	if err != nil {
		return err
	}
	dbs = slices.DeleteFunc(dbs, func(md Source) bool { return md.Name == name })
	return fcr.WriteSources(dbs)
}

func (fcr *fileSourcesReaderWriter) GetSources() (dbs Sources, err error) {
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
			var mdbs Sources
			if mdbs, err = fcr.getSources(path); err == nil {
				dbs = append(dbs, mdbs...)
			}
			return err
		})
	case mode.IsRegular():
		dbs, err = fcr.getSources(fcr.path)
	}
	if err != nil {
		return nil, err
	}
	return
}

func (fcr *fileSourcesReaderWriter) getSources(configFilePath string) (dbs Sources, err error) {
	var yamlFile []byte
	if yamlFile, err = os.ReadFile(configFilePath); err != nil {
		return
	}
	c := make(Sources, 0) // there can be multiple configs in a single file
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

func (fcr *fileSourcesReaderWriter) expandEnvVars(md Source) Source {
	if strings.HasPrefix(string(md.Kind), "$") {
		md.Kind = Kind(os.ExpandEnv(string(md.Kind)))
	}
	if strings.HasPrefix(md.Name, "$") {
		md.Name = os.ExpandEnv(md.Name)
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
