package metrics

import (
	"context"
	_ "embed"
	"os"

	"gopkg.in/yaml.v3"
)

// NewDefaultMetricReader creates a new default metric reader with an empty path.
func NewDefaultMetricReader(ctx context.Context) (Reader, error) {
	return &fileMetricReader{
		ctx: ctx,
	}, nil
}

func GetDefaultMetrics() (metrics *Metrics) {
	defMetricReader := &fileMetricReader{}
	metrics, _ = defMetricReader.GetMetrics()
	return
}

func NewYAMLMetricReader(ctx context.Context, path string) (Reader, error) {
	return &fileMetricReader{
		ctx:  ctx,
		path: path,
	}, nil
}

type fileMetricReader struct {
	ctx  context.Context
	path string
}

func WriteMetricsToFile(metricDefs Metrics, filename string) error {
	yamlData, err := yaml.Marshal(metricDefs)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, yamlData, 0644)
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
