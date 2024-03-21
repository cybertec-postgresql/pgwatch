package metrics_test

import (
	"context"
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

	fmr, err := metrics.NewYAMLMetricReaderWriter(context.Background(), tempFile)
	assert.NoError(t, err)

	// Call the function being tested
	err = fmr.WriteMetrics(&metricDefs)
	assert.NoError(t, err)

	// Read the contents of the file
	assert.NoError(t, err)
	metrics, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the file contains the expected data
	assert.Equal(t, metricDefs, *metrics)
}
func TestUpdateMetric(t *testing.T) {
	// Define test data
	metricDefs := metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"existing_metric": metrics.Metric{
				SQLs: map[int]string{
					1: "SELECT 1",
				},
				Enabled:         true,
				InitSQL:         "SELECT 1",
				NodeStatus:      "primary",
				Gauges:          []string{"gauge1", "gauge2"},
				IsInstanceLevel: true,
				StorageName:     "storage1",
				Description:     "Existing metric",
			},
		},
		PresetDefs: metrics.PresetDefs{
			"test_preset": metrics.Preset{
				Description: "Test preset",
				Metrics: map[string]float64{
					"existing_metric": 1.0,
				},
			},
		},
	}

	// Create a temporary file for testing
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	fmr, err := metrics.NewYAMLMetricReaderWriter(context.Background(), tempFile)
	assert.NoError(t, err)

	// Write initial metrics to the file
	err = fmr.WriteMetrics(&metricDefs)
	assert.NoError(t, err)

	// Call the function being tested
	newMetric := metrics.Metric{
		SQLs: map[int]string{
			1: "SELECT 2",
		},
		Enabled:         true,
		InitSQL:         "SELECT 2",
		NodeStatus:      "primary",
		Gauges:          []string{"gauge3", "gauge4"},
		IsInstanceLevel: true,
		StorageName:     "storage2",
		Description:     "New metric",
	}
	err = fmr.UpdateMetric("new_metric", newMetric)
	assert.NoError(t, err)

	// Read the updated metrics from the file
	updatedMetrics, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the metric was updated correctly
	expectedMetrics := metricDefs
	expectedMetrics.MetricDefs["new_metric"] = newMetric
	assert.Equal(t, expectedMetrics, *updatedMetrics)
}
func TestDeleteMetric(t *testing.T) {
	// Define test data
	metricDefs := metrics.MetricDefs{
		"existing_metric": metrics.Metric{
			SQLs: map[int]string{
				1: "SELECT 1",
			},
			Enabled:         true,
			InitSQL:         "SELECT 1",
			NodeStatus:      "primary",
			Gauges:          []string{"gauge1", "gauge2"},
			IsInstanceLevel: true,
			StorageName:     "storage1",
			Description:     "Existing metric",
		},
		"metric_to_delete": metrics.Metric{
			SQLs: map[int]string{
				1: "SELECT 2",
			},
			Enabled:         true,
			InitSQL:         "SELECT 2",
			NodeStatus:      "primary",
			Gauges:          []string{"gauge3", "gauge4"},
			IsInstanceLevel: true,
			StorageName:     "storage2",
			Description:     "Metric to delete",
		},
	}

	// Create a temporary file for testing
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	fmr, err := metrics.NewYAMLMetricReaderWriter(context.Background(), tempFile)
	assert.NoError(t, err)

	// Write initial metrics to the file
	err = fmr.WriteMetrics(&metrics.Metrics{
		MetricDefs: metricDefs,
	})
	assert.NoError(t, err)

	// Call the function being tested
	err = fmr.DeleteMetric("metric_to_delete")
	assert.NoError(t, err)

	// Read the updated metrics from the file
	updatedMetrics, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the metric was deleted correctly
	expectedMetrics := metricDefs
	delete(expectedMetrics, "metric_to_delete")
	assert.Equal(t, expectedMetrics, updatedMetrics.MetricDefs)
}
