package metrics

import (
	"context"
	"errors"
)

func GetDefaultBuiltInMetrics() []string {
	return []string{"sproc_changes", "table_changes", "index_changes", "privilege_changes", "object_changes", "configuration_changes"}
}

// NewDefaultMetricReader creates a new default metric reader with an empty path.
func NewDefaultMetricReader(context.Context) (ReaderWriter, error) {
	return &defaultMetricReader{}, nil
}

func GetDefaultMetrics() (metrics *Metrics) {
	defMetricReader := &fileMetricReader{}
	metrics, _ = defMetricReader.GetMetrics()
	return
}

type defaultMetricReader struct{}

func (dmrw *defaultMetricReader) WriteMetrics(*Metrics) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReader) DeleteMetric(string) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReader) UpdateMetric(string, Metric) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReader) DeletePreset(string) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReader) UpdatePreset(string, Preset) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReader) GetMetrics() (*Metrics, error) {
	return GetDefaultMetrics(), nil
}
