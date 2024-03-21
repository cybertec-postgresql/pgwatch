package metrics_test

import (
	"path/filepath"
	"testing"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/stretchr/testify/assert"
)

func TestWriteMetricsToFile(t *testing.T) {
	// Define test data
	metricDefs := metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"test_metric": metrics.Metric{
				SQLs: map[int]string{
					1: "SELECT 1",
				},
				Enabled:         true,
				InitSQL:         "SELECT 1",
				NodeStatus:      "primary",
				Gauges:          []string{"gauge1", "gauge2"},
				IsInstanceLevel: true,
				StorageName:     "storage1",
				Description:     "Test metric",
			},
		},
		PresetDefs: metrics.PresetDefs{
			"test_preset": metrics.Preset{
				Description: "Test preset",
				Metrics: map[string]float64{
					"test_metric": 1.0,
				},
			},
		},
	}

	// Create a temporary file for testing
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	// Call the function being tested
	err := metrics.WriteMetricsToFile(metricDefs, tempFile)
	assert.NoError(t, err)

	// Read the contents of the file
	fmr, err := metrics.NewYAMLMetricReader(nil, tempFile)
	assert.NoError(t, err)
	metrics, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the file contains the expected data
	assert.Equal(t, metricDefs, *metrics)
}
