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

	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	isDir := false
	if fi.Mode().IsDir() {
		isDir = true
		// create `new_metrics.yaml` to hold any 
		// metrics/presets created via the web api
		_, err = os.Create(filepath.Join(path, "new_metrics.yaml"))
		if err != nil {
			return nil, err
		}
	}

	return &fileMetricReader{
		ctx:  ctx,
		path: path,
		isDir: isDir,
	}, nil
}

type fileMetricReader struct {
	ctx  context.Context
	path string
	isDir bool
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
	var yamlFiles [][]byte
	var err error
	if fmr.isDir {
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
	} else {
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

	if fmr.isDir {
		return fmr.deleteMetricFromMetricsDir(metricName)
	}	
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) UpdateMetric(metricName string, metric Metric) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	metrics.MetricDefs[metricName] = metric

	if fmr.isDir {
		return fmr.updateMetricInMetricsDir(metricName, metric)
	}
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

	if fmr.isDir {
		filePath := filepath.Join(fmr.path, "new_metrics.yaml")

		var fileData []byte
		var err error
		if fileData, err = os.ReadFile(filePath); err != nil {
			return err
		}

		fileMetrics := Metrics{
			MetricDefs: make(MetricDefs),
			PresetDefs: make(PresetDefs),
		}
		if err = yaml.Unmarshal(fileData, &fileMetrics); err != nil {
			return err
		}
		fileMetrics.MetricDefs[metricName] = metric

		yamlData, _ := yaml.Marshal(fileMetrics)
		return os.WriteFile(filePath, yamlData, 0644)
	}

	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) DeletePreset(presetName string) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	delete(metrics.PresetDefs, presetName)

	if fmr.isDir {
		return fmr.deletePresetFromMetricsDir(presetName)
	}	
	return fmr.WriteMetrics(metrics)
}

func (fmr *fileMetricReader) UpdatePreset(presetName string, preset Preset) error {
	metrics, err := fmr.GetMetrics()
	if err != nil {
		return err
	}
	metrics.PresetDefs[presetName] = preset

	if fmr.isDir {
		return fmr.updatePresetInMetricsDir(presetName, preset)
	}
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

	if fmr.isDir {
		filePath := filepath.Join(fmr.path, "new_metrics.yaml")

		var fileData []byte
		var err error
		if fileData, err = os.ReadFile(filePath); err != nil {
			return err
		}

		fileMetrics := Metrics{
			MetricDefs: make(MetricDefs),
			PresetDefs: make(PresetDefs),
		}
		if err = yaml.Unmarshal(fileData, &fileMetrics); err != nil {
			return err
		}
		fileMetrics.PresetDefs[presetName] = preset

		yamlData, _ := yaml.Marshal(fileMetrics)
		return os.WriteFile(filePath, yamlData, 0644)
	}

	return fmr.WriteMetrics(metrics)
}

// Traverses all YAML files in the metrics directory,
// passing a pointer to each file's metrics to `processMetricsFile()`
// and rewrites the file if it returns true.
func (fmr *fileMetricReader) WalkMetricsDir(processMetricsFile func(metrics *Metrics) bool) error {
	return filepath.WalkDir(fmr.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if d.IsDir() || ext != ".yaml" && ext != ".yml" {
			return nil
		}

		var FileData []byte 
		if FileData, err = os.ReadFile(path); err != nil {
			return err
		}

		metricsFile := Metrics{
			MetricDefs: make(MetricDefs),
			PresetDefs: make(PresetDefs),
		}

		if err = yaml.Unmarshal(FileData, &metricsFile); err != nil {
			return err
		}

		if rewriteFile := processMetricsFile(&metricsFile); !rewriteFile {
			return nil
		}

		yamlData, _ := yaml.Marshal(metricsFile)
		return os.WriteFile(path, yamlData, 0644)
	})
}

func (fmr *fileMetricReader) deleteMetricFromMetricsDir(name string) error {
	processMetricsFile := func(fileMetrics *Metrics) bool {
		_, exists := fileMetrics.MetricDefs[name]
		if exists {
			delete(fileMetrics.MetricDefs, name)
			return true // rewrite metrics file
		}
		return false
	}
	return fmr.WalkMetricsDir(processMetricsFile)
}

func (fmr *fileMetricReader) deletePresetFromMetricsDir(name string) error {
	processMetricsFile := func(fileMetrics *Metrics) bool {
		_, exists := fileMetrics.PresetDefs[name]
		if exists {
			delete(fileMetrics.PresetDefs, name)
			return true // rewrite metrics file
		}
		return false
	}
	return fmr.WalkMetricsDir(processMetricsFile)
}

func (fmr *fileMetricReader) updateMetricInMetricsDir(name string, metric Metric) error {
	processMetricsFile := func(fileMetrics *Metrics) bool {
		_, exists := fileMetrics.MetricDefs[name]
		if exists {
			fileMetrics.MetricDefs[name] = metric
			return true // rewrite metrics file
		}
		return false
	}
	return fmr.WalkMetricsDir(processMetricsFile)
}

func (fmr *fileMetricReader) updatePresetInMetricsDir(name string, preset Preset) error {
	processMetricsFile := func(fileMetrics *Metrics) bool {
		_, exists := fileMetrics.PresetDefs[name]
		if exists {
			fileMetrics.PresetDefs[name] = preset
			return true // rewrite metrics file
		}
		return false
	}
	return fmr.WalkMetricsDir(processMetricsFile)
}