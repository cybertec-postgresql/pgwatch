package metrics

import (
	"context"
	_ "embed"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

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
}

func (fmr *fileMetricReader) WriteMetrics(metricDefs *Metrics) error {
	yamlData, _ := yaml.Marshal(metricDefs)
	return os.WriteFile(fmr.path, yamlData, 0644)
}

//go:embed metrics.yaml
var defaultMetricsYAML []byte

func (fmr *fileMetricReader) GetMetrics() (metrics *Metrics, err error) {
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
			if m, err = fmr.getMetrics(path); err == nil {
				maps.Copy(metrics.PresetDefs, m.PresetDefs)
				maps.Copy(metrics.MetricDefs, m.MetricDefs)
			}
			return err
		})
	case mode.IsRegular():
		metrics, err = fmr.getMetrics(fmr.path)
	}
	return
}

func (fmr *fileMetricReader) getMetrics(metricsFilePath string) (metrics *Metrics, err error) {
	var yamlFile []byte
	if yamlFile, err = os.ReadFile(metricsFilePath); err != nil {
		return
	}
	metrics = &Metrics{MetricDefs{}, PresetDefs{}}
	err = yaml.Unmarshal(yamlFile, &metrics)
	return
}

func (fmr *fileMetricReader) DeleteMetric(metricName string) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	delete(metrics.MetricDefs, metricName)
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) UpdateMetric(metricName string, metric Metric) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	metrics.MetricDefs[metricName] = metric
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) CreateMetric(metricName string, metric Metric) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	// Check if metric already exists
	if _, exists := metrics.MetricDefs[metricName]; exists {
		return ErrMetricExists
	}
	metrics.MetricDefs[metricName] = metric
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) DeletePreset(presetName string) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	delete(metrics.PresetDefs, presetName)
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) UpdatePreset(presetName string, preset Preset) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	metrics.PresetDefs[presetName] = preset
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) CreatePreset(presetName string, preset Preset) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	// Check if preset already exists
	if _, exists := metrics.PresetDefs[presetName]; exists {
		return ErrPresetExists
	}
	metrics.PresetDefs[presetName] = preset
	return fmr.WriteMetrics(metrics)
}
