package metrics_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestDeaultMetrics(t *testing.T) {
	fmr, err := metrics.NewYAMLMetricReaderWriter(ctx, "") // empty path is reserved for default metrics
	assert.NoError(t, err)

	// Test GetMetrics
	metricsDefs, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Test WriteMetrics
	err = fmr.WriteMetrics(metricsDefs)
	assert.Error(t, err)

	// Test DeleteMetric
	err = fmr.DeleteMetric("test")
	assert.Error(t, err)

	// Test UpdateMetric
	err = fmr.UpdateMetric("test", metrics.Metric{})
	assert.Error(t, err)

	// Test DeletePreset
	err = fmr.DeletePreset("test")
	assert.Error(t, err)

	// Test UpdatePreset
	err = fmr.UpdatePreset("test", metrics.Preset{})
	assert.Error(t, err)
}

func TestWriteMetricsToFile(t *testing.T) {
	// Define test data
	metricDefs := metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"test_metric": metrics.Metric{
				SQLs: map[int]string{
					1: "SELECT 1",
				},
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

	fmr, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Call the function being tested
	err = fmr.WriteMetrics(&metricDefs)
	assert.NoError(t, err)

	// Read the contents of the file
	metrics, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the file contains the expected data
	assert.Equal(t, metricDefs, *metrics)
}
func TestMetricsToFile(t *testing.T) {
	// Define test data
	metricDefs := metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"existing_metric": metrics.Metric{
				SQLs: map[int]string{
					1: "SELECT 1",
				},
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

	fmr, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Write initial metrics to the file
	err = fmr.WriteMetrics(&metricDefs)
	assert.NoError(t, err)

	// Call the function being tested
	newMetric := metrics.Metric{
		SQLs: map[int]string{
			1: "SELECT 2",
		},
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

	// Call the function being tested
	err = fmr.DeleteMetric("new_metric")
	assert.NoError(t, err)

	// Read the updated metrics from the file
	updatedMetrics, err = fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the metric was deleted correctly
	assert.Zero(t, updatedMetrics.MetricDefs["new_metric"])
}

func TestPresetsToFile(t *testing.T) {
	// Define test data
	presetDefs := metrics.PresetDefs{
		"existing_preset": metrics.Preset{
			Description: "Existing preset",
			Metrics: map[string]float64{
				"existing_metric": 1.0,
			},
		},
	}

	// Create a temporary file for testing
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	fmr, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Write initial presets to the file
	err = fmr.WriteMetrics(&metrics.Metrics{
		PresetDefs: presetDefs,
	})
	assert.NoError(t, err)

	// Call the function being tested
	newPreset := metrics.Preset{
		Description: "New preset",
		Metrics: map[string]float64{
			"new_metric": 1.0,
		},
	}
	err = fmr.UpdatePreset("new_preset", newPreset)
	assert.NoError(t, err)

	// Read the updated presets from the file
	updatedMetrics, err := fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the preset was updated correctly
	expectedPresets := presetDefs
	expectedPresets["new_preset"] = newPreset
	assert.Equal(t, expectedPresets, updatedMetrics.PresetDefs)

	// check the delete preset function
	err = fmr.DeletePreset("new_preset")
	assert.NoError(t, err)

	// Read the updated presets from the file
	updatedMetrics, err = fmr.GetMetrics()
	assert.NoError(t, err)

	// Assert that the preset was deleted correctly
	assert.Zero(t, updatedMetrics.PresetDefs["new_preset"])
}

func TestErrorHandlingToFile(t *testing.T) {
	fmr, err := metrics.NewYAMLMetricReaderWriter(ctx, "/") // empty path is reserved for default metrics
	assert.NoError(t, err)

	// Test WriteMetrics
	err = fmr.WriteMetrics(&metrics.Metrics{})
	assert.Error(t, err)

	// Test GetMetrics
	_, err = fmr.GetMetrics()
	assert.Error(t, err)

	// Test DeleteMetric
	err = fmr.DeleteMetric("test")
	assert.Error(t, err)

	// Test UpdateMetric
	err = fmr.UpdateMetric("test", metrics.Metric{})
	assert.Error(t, err)

	// Test DeletePreset
	err = fmr.DeletePreset("test")
	assert.Error(t, err)

	// Test UpdatePreset
	err = fmr.UpdatePreset("test", metrics.Preset{})
	assert.Error(t, err)

	// Test invalid YAML
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")
	file, err := os.Create(tempFile)
	assert.NoError(t, err)
	defer file.Close()
	_, err = file.WriteString("invalid yaml")
	assert.NoError(t, err)

	fmr, err = metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	_, err = fmr.GetMetrics()
	assert.Error(t, err)
}

func TestCreateMetricAndPreset(t *testing.T) {

	a := assert.New(t)

	t.Run("YAML_CreateMetric_Success", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_metrics.yaml")
		defer os.Remove(tmpFile)

		// Create YAML reader/writer
		yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tmpFile)
		a.NoError(err)

		// Initialize empty metrics file first
		emptyMetrics := &metrics.Metrics{
			MetricDefs: make(map[string]metrics.Metric),
			PresetDefs: make(map[string]metrics.Preset),
		}
		err = yamlrw.WriteMetrics(emptyMetrics)
		a.NoError(err)

		// Create a new metric
		testMetric := metrics.Metric{
			Description: "Test metric for creation",
		}
		err = yamlrw.CreateMetric("test_metric", testMetric)
		a.NoError(err)

		// Verify it was created
		m, err := yamlrw.GetMetrics()
		a.NoError(err)
		a.Contains(m.MetricDefs, "test_metric")
		a.Equal("Test metric for creation", m.MetricDefs["test_metric"].Description)

		// Try to create the same metric again - should fail
		err = yamlrw.CreateMetric("test_metric", testMetric)
		a.Error(err)
		a.Equal(metrics.ErrMetricExists, err)
	})

	t.Run("YAML_CreatePreset_Success", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test_presets.yaml")
		defer os.Remove(tmpFile)

		yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tmpFile)
		a.NoError(err)

		// Initialize empty metrics file first
		emptyMetrics := &metrics.Metrics{
			MetricDefs: make(map[string]metrics.Metric),
			PresetDefs: make(map[string]metrics.Preset),
		}
		err = yamlrw.WriteMetrics(emptyMetrics)
		a.NoError(err)

		// Create a new preset
		testPreset := metrics.Preset{
			Description: "Test preset for creation",
			Metrics:     map[string]float64{"db_stats": 60},
		}
		err = yamlrw.CreatePreset("test_preset", testPreset)
		a.NoError(err)

		// Verify it was created
		m, err := yamlrw.GetMetrics()
		a.NoError(err)
		a.Contains(m.PresetDefs, "test_preset")
		a.Equal("Test preset for creation", m.PresetDefs["test_preset"].Description)

		// Try to create the same preset again - should fail
		err = yamlrw.CreatePreset("test_preset", testPreset)
		a.Error(err)
		a.Equal(metrics.ErrPresetExists, err)
	})
}

func TestMetricsDir(t *testing.T) {
	a := assert.New(t)

	// first metrics file data
	metrics1 := metrics.Metrics{
		MetricDefs: map[string]metrics.Metric{
			"metric1": {
				Description: "metric1 description",
			},
		},
		PresetDefs: map[string]metrics.Preset{
			"preset1": {
				Description: "preset1 description",
				Metrics: map[string]float64{
					"metric1": 10,
				},
			},
		},
	}

	// second metrics file data
	metrics2 := metrics.Metrics{
		MetricDefs: map[string]metrics.Metric{
			"metric2": {
				Description: "metric2 description",
			},
		},
		PresetDefs: map[string]metrics.Preset{
			"preset2": {
				Description: "preset2 description",
				Metrics: map[string]float64{
					"metric2": 10,
				},
			},
		},
	}

	metrics1File, err := yaml.Marshal(metrics1)
	a.NoError(err)
	metrics2File, err := yaml.Marshal(metrics2)
	a.NoError(err)

	// write data to different files in a folder
	tempDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tempDir, "metrics1.yaml"), metrics1File, 0644)
	a.NoError(err)
	err = os.WriteFile(filepath.Join(tempDir, "metrics2.yaml"), metrics2File, 0644)
	a.NoError(err)

	// use folder of yaml files for metrics configs
	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempDir)
	a.NoError(err)

	// load metrics configs from folder
	ms, err := yamlrw.GetMetrics()
	a.NoError(err)
	a.Equal("metric1 description", ms.MetricDefs["metric1"].Description)
	a.Equal("preset1 description", ms.PresetDefs["preset1"].Description)
	a.Equal("metric2 description", ms.MetricDefs["metric2"].Description)
	a.Equal("preset2 description", ms.PresetDefs["preset2"].Description)
}