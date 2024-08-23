package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/metrics"
	"github.com/stretchr/testify/assert"
)

func TestGetDefaultBuiltInMetrics(t *testing.T) {
	expectedMetrics := []string{"sproc_changes", "table_changes", "index_changes", "privilege_changes", "object_changes", "configuration_changes"}
	metrics := metrics.GetDefaultBuiltInMetrics()
	assert.Equal(t, expectedMetrics, metrics, "The default built-in metrics should match the expected list")
}

func TestNewDefaultMetricReader(t *testing.T) {
	reader, err := metrics.NewDefaultMetricReader(context.Background())
	assert.NoError(t, err, "Creating a new default metric reader should not produce an error")
	assert.NotNil(t, reader, "The metric reader should not be nil")
}

func TestDefaultMetricReaderUnsupportedOperations(t *testing.T) {
	reader, _ := metrics.NewDefaultMetricReader(context.Background())
	err := reader.WriteMetrics(nil)
	assert.Equal(t, errors.ErrUnsupported, err, "WriteMetrics should return ErrUnsupported")

	err = reader.DeleteMetric("")
	assert.Equal(t, errors.ErrUnsupported, err, "DeleteMetric should return ErrUnsupported")

	err = reader.UpdateMetric("", metrics.Metric{})
	assert.Equal(t, errors.ErrUnsupported, err, "UpdateMetric should return ErrUnsupported")

	err = reader.DeletePreset("")
	assert.Equal(t, errors.ErrUnsupported, err, "DeletePreset should return ErrUnsupported")

	err = reader.UpdatePreset("", metrics.Preset{})
	assert.Equal(t, errors.ErrUnsupported, err, "UpdatePreset should return ErrUnsupported")

	metrics, err := reader.GetMetrics()
	assert.NotNil(t, metrics, "The metrics object should not be nil")
	assert.NoError(t, err, "GetMetrics should return default metrics")
}
