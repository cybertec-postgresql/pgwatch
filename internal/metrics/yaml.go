package metrics

import (
	"context"
	_ "embed"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

func NewYAMLMetricReaderWriter(ctx context.Context, path string) (ReaderWriter, error) {
	if path == "" {
		return NewDefaultMetricReader(ctx)
	}
	return &fileMetricReader{
		ctx:  ctx,
		path: path,
	}, nil
}

type fileMetricReader struct {
	ctx  context.Context
	path string
	sync.Mutex
}

// WriteMetrics writes metrics to file with locking
func (fmr *fileMetricReader) WriteMetrics(metricDefs *Metrics) error {
	fmr.Lock()
	defer fmr.Unlock()
	return fmr.writeMetrics(metricDefs)
}

// writeMetrics writes metrics to file without locking (internal use only)
func (fmr *fileMetricReader) writeMetrics(metricDefs *Metrics) error {
	yamlData, _ := yaml.Marshal(metricDefs)
	return os.WriteFile(fmr.path, yamlData, 0644)
}

//go:embed metrics.yaml
var defaultMetricsYAML []byte

// GetMetrics reads metrics from file or returns default metrics if path is empty with locking
func (fmr *fileMetricReader) GetMetrics() (metrics *Metrics, err error) {
	fmr.Lock()
	defer fmr.Unlock()
	return fmr.getMetrics()
}

// getMetrics reads metrics from file or returns default metrics if path is empty without locking (internal use only)
func (fmr *fileMetricReader) getMetrics() (metrics *Metrics, err error) {
	metrics = &Metrics{MetricDefs{}, PresetDefs{}}
	if fmr.path == "" {
		err = yaml.Unmarshal(defaultMetricsYAML, metrics)
		return
	}

	fi, err := os.Stat(fmr.path)
	if err != nil {
		return nil, err
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		err = filepath.WalkDir(fmr.path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if d.IsDir() || ext != ".yaml" && ext != ".yml" {
				return nil
			}
			var m *Metrics
			if m, err = fmr.loadMetricsFromFile(path); err == nil {
				maps.Copy(metrics.PresetDefs, m.PresetDefs)
				maps.Copy(metrics.MetricDefs, m.MetricDefs)
			}
			return err
		})
	case mode.IsRegular():
		metrics, err = fmr.loadMetricsFromFile(fmr.path)
	}
	return
}

// loadMetricsFromFile reads metrics from a single YAML file
func (fmr *fileMetricReader) loadMetricsFromFile(metricsFilePath string) (metrics *Metrics, err error) {
	var yamlFile []byte
	if yamlFile, err = os.ReadFile(metricsFilePath); err != nil {
		return
	}
	metrics = &Metrics{MetricDefs{}, PresetDefs{}}
	err = yaml.Unmarshal(yamlFile, &metrics)
	return
}

// DeleteMetric deletes a metric by name and writes the updated metrics back to file
func (fmr *fileMetricReader) DeleteMetric(metricName string) error {
	fmr.Lock()
	defer fmr.Unlock()
	metrics, err := fmr.getMetrics()
	if err != nil {
		return err
	}
	delete(metrics.MetricDefs, metricName)
	return fmr.writeMetrics(metrics)
}

// UpdateMetric updates an existing metric or creates it if it doesn't exist, then writes the updated metrics back to file
func (fmr *fileMetricReader) UpdateMetric(metricName string, metric Metric) error {
	fmr.Lock()
	defer fmr.Unlock()
	metrics, err := fmr.getMetrics()
	if err != nil {
		return err
	}
	metrics.MetricDefs[metricName] = metric
	return fmr.writeMetrics(metrics)
}

// CreateMetric creates a new metric if it doesn't already exist, then writes the updated metrics back to file
func (fmr *fileMetricReader) CreateMetric(metricName string, metric Metric) error {
	fmr.Lock()
	defer fmr.Unlock()
	metrics, err := fmr.getMetrics()
	if err != nil {
		return err
	}
	// Check if metric already exists
	if _, exists := metrics.MetricDefs[metricName]; exists {
		return ErrMetricExists
	}
	metrics.MetricDefs[metricName] = metric
	return fmr.writeMetrics(metrics)
}

// DeletePreset deletes a preset by name and writes the updated metrics back to file
func (fmr *fileMetricReader) DeletePreset(presetName string) error {
	fmr.Lock()
	defer fmr.Unlock()
	metrics, err := fmr.getMetrics()
	if err != nil {
		return err
	}
	delete(metrics.PresetDefs, presetName)
	return fmr.writeMetrics(metrics)
}

// UpdatePreset updates an existing preset or creates it if it doesn't exist, then writes the updated metrics back to file
func (fmr *fileMetricReader) UpdatePreset(presetName string, preset Preset) error {
	fmr.Lock()
	defer fmr.Unlock()
	metrics, err := fmr.getMetrics()
	if err != nil {
		return err
	}
	metrics.PresetDefs[presetName] = preset
	return fmr.writeMetrics(metrics)
}

// CreatePreset creates a new preset if it doesn't already exist, then writes the updated metrics back to file
func (fmr *fileMetricReader) CreatePreset(presetName string, preset Preset) error {
	fmr.Lock()
	defer fmr.Unlock()
	metrics, err := fmr.getMetrics()
	if err != nil {
		return err
	}
	// Check if preset already exists
	if _, exists := metrics.PresetDefs[presetName]; exists {
		return ErrPresetExists
	}
	metrics.PresetDefs[presetName] = preset
	return fmr.writeMetrics(metrics)
}
