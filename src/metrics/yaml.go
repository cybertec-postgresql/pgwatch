package metrics

import (
	"context"
	_ "embed"
	"os"

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
	metrics = new(Metrics)
	var s []byte
	if fmr.path == "" {
		s = defaultMetricsYAML
	} else {
		if s, err = os.ReadFile(fmr.path); err != nil {
			return nil, err
		}
	}
	if err = yaml.Unmarshal(s, metrics); err != nil {
		return nil, err
	}
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
