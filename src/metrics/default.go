package metrics

import (
	"context"
	"errors"
)

// NewDefaultMetricReaderWriter creates a new default metric reader with an empty path.
func NewDefaultMetricReaderWriter(context.Context) (ReaderWriter, error) {
	return &defaultMetricReaderWriter{}, nil
}

func GetDefaultMetrics() (metrics *Metrics) {
	defMetricReader := &fileMetricReader{}
	metrics, _ = defMetricReader.GetMetrics()
	return
}

type defaultMetricReaderWriter struct{}

func (dmrw *defaultMetricReaderWriter) WriteMetrics(*Metrics) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReaderWriter) DeleteMetric(string) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReaderWriter) UpdateMetric(string, Metric) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReaderWriter) DeletePreset(string) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReaderWriter) UpdatePreset(string, Preset) error {
	return errors.ErrUnsupported
}

func (dmrw *defaultMetricReaderWriter) GetMetrics() (*Metrics, error) {
	return GetDefaultMetrics(), nil
}
