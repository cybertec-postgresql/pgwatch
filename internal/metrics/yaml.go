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

func (fmr *fileMetricReader) GetMetrics() (*Metrics, error) {
	if fmr.path == "" {
		metrics := new(Metrics)
		metricsYaml := defaultMetricsYAML
		if err := yaml.Unmarshal(metricsYaml, metrics); err != nil {
			return nil, err
		}
		return metrics, nil
	} 
	return fmr.loadMetricsFromYaml()	
}

// Loads a Metrics struct from a file or a folder of YAML files
// located at the fileMetricReader path.
func (fmr *fileMetricReader) loadMetricsFromYaml() (*Metrics, error) {
	fi, err := os.Stat(fmr.path)
	if err != nil {
		return nil, err
	}

	var yamlFiles [][]byte
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

			var singleFileData []byte
			if singleFileData, err = os.ReadFile(path); err != nil {
				return err
			}
			yamlFiles = append(yamlFiles, singleFileData)

			return nil
		})

		if err != nil {
			return nil, err
		}
	case mode.IsRegular():
		var singleFileData []byte
		if singleFileData, err = os.ReadFile(fmr.path); err != nil {
			return nil, err
		}
		yamlFiles = append(yamlFiles, singleFileData)
	}


	metrics := &Metrics {
		MetricDefs: make(MetricDefs),
		PresetDefs: make(PresetDefs),
	}

	for _, singleFileData := range yamlFiles {
		var fileMetrics Metrics
		if err = yaml.Unmarshal(singleFileData, &fileMetrics); err != nil {
			return nil, err
		}
		maps.Copy(metrics.PresetDefs, fileMetrics.PresetDefs)
		maps.Copy(metrics.MetricDefs, fileMetrics.MetricDefs)
	}
	return metrics, nil
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
