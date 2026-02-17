package sources

// This file contains the implementation of the ReaderWriter interface for the YAML file.

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
	sync.Mutex
}

// WriteSources writes sources to file with locking
func (fcr *fileSourcesReaderWriter) WriteSources(mds Sources) error {
	fcr.Lock()
	defer fcr.Unlock()
	return fcr.writeSources(mds)
}

// writeSources writes sources to file without locking (internal use only)
func (fcr *fileSourcesReaderWriter) writeSources(mds Sources) error {
	yamlData, _ := yaml.Marshal(mds)
	return os.WriteFile(fcr.path, yamlData, 0644)
}

// UpdateSource updates an existing source or creates it if it doesn't exist, then writes the updated sources back to file
func (fcr *fileSourcesReaderWriter) UpdateSource(md Source) error {
	fcr.Lock()
	defer fcr.Unlock()
	dbs, err := fcr.getSources()
	if err != nil {
		return err
	}
	for i, db := range dbs {
		if db.Name == md.Name {
			dbs[i] = md
			return fcr.writeSources(dbs)
		}
	}
	dbs = append(dbs, md)
	return fcr.writeSources(dbs)
}

// CreateSource creates a new source if it doesn't already exist, then writes the updated sources back to file
func (fcr *fileSourcesReaderWriter) CreateSource(md Source) error {
	fcr.Lock()
	defer fcr.Unlock()
	dbs, err := fcr.getSources()
	if err != nil {
		return err
	}
	// Check if source already exists
	for _, db := range dbs {
		if db.Name == md.Name {
			return ErrSourceExists
		}
	}
	dbs = append(dbs, md)
	return fcr.writeSources(dbs)
}

// DeleteSource deletes a source by name and writes the updated sources back to file
func (fcr *fileSourcesReaderWriter) DeleteSource(name string) error {
	fcr.Lock()
	defer fcr.Unlock()
	dbs, err := fcr.getSources()
	if err != nil {
		return err
	}
	dbs = slices.DeleteFunc(dbs, func(md Source) bool { return md.Name == name })
	return fcr.writeSources(dbs)
}

// GetSources reads sources from file with locking
func (fcr *fileSourcesReaderWriter) GetSources() (dbs Sources, err error) {
	fcr.Lock()
	defer fcr.Unlock()
	return fcr.getSources()
}

// getSources reads sources from file without locking (internal use only)
func (fcr *fileSourcesReaderWriter) getSources() (dbs Sources, err error) {
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
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if d.IsDir() || ext != ".yaml" && ext != ".yml" {
				return nil
			}
			var mdbs Sources
			if mdbs, err = fcr.loadSourcesFromFile(path); err == nil {
				dbs = append(dbs, mdbs...)
			}
			return err
		})
	case mode.IsRegular():
		dbs, err = fcr.loadSourcesFromFile(fcr.path)
	}
	if err != nil {
		return nil, err
	}
	return dbs.Validate()
}

// loadSourcesFromFile reads sources from a single YAML file, expands environment variables, and returns them
func (fcr *fileSourcesReaderWriter) loadSourcesFromFile(configFilePath string) (dbs Sources, err error) {
	var yamlFile []byte
	if yamlFile, err = os.ReadFile(configFilePath); err != nil {
		return
	}
	c := make(Sources, 0) // there can be multiple configs in a single file
	if err = yaml.Unmarshal(yamlFile, &c); err != nil {
		return
	}
	for _, v := range c {
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
