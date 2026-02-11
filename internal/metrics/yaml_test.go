package metrics_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

// TestConcurrentUpdates tests for race conditions during concurrent metric updates
func TestConcurrentUpdates(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Create initial empty metrics file
	initialMetrics := &metrics.Metrics{
		MetricDefs: make(map[string]metrics.Metric),
		PresetDefs: make(map[string]metrics.Preset),
	}
	err = yamlrw.WriteMetrics(initialMetrics)
	assert.NoError(t, err)

	// Number of concurrent updates
	numGoroutines := 10
	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines)

	// Each goroutine will add a unique metric
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			metricName := fmt.Sprintf("metric_%d", id)
			testMetric := metrics.Metric{
				Description: fmt.Sprintf("Test metric %d", id),
				SQLs: map[int]string{
					1: fmt.Sprintf("SELECT %d", id),
				},
			}

			// Simulate small random delay
			time.Sleep(time.Millisecond * time.Duration(id%3))

			err := yamlrw.UpdateMetric(metricName, testMetric)
			if err != nil {
				errorChan <- fmt.Errorf("goroutine %d: %w", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors during updates
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
		t.Logf("Error during concurrent update: %v", err)
	}

	// ensure all metrics were saved
	finalMetrics, err := yamlrw.GetMetrics()
	assert.NoError(t, err)

	assert.Equal(t, numGoroutines, len(finalMetrics.MetricDefs),
		"Expected %d metrics, but got %d. Some updates were lost due to race condition!",
		numGoroutines, len(finalMetrics.MetricDefs))

	// Print which metrics are missing if test fails
	if len(finalMetrics.MetricDefs) != numGoroutines {
		for i := 0; i < numGoroutines; i++ {
			metricName := fmt.Sprintf("metric_%d", i)
			if _, exists := finalMetrics.MetricDefs[metricName]; !exists {
				t.Logf("LOST UPDATE: %s was not saved", metricName)
			}
		}
	}
}

// TestConcurrentCreates tests concurrent CreateMetric calls
func TestConcurrentCreates(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Initialize empty metrics file
	err = yamlrw.WriteMetrics(&metrics.Metrics{
		MetricDefs: make(map[string]metrics.Metric),
		PresetDefs: make(map[string]metrics.Preset),
	})
	assert.NoError(t, err)

	numGoroutines := 15
	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			metricName := fmt.Sprintf("new_metric_%d", id)
			testMetric := metrics.Metric{
				Description: fmt.Sprintf("New metric %d", id),
				SQLs: map[int]string{
					1: fmt.Sprintf("SELECT %d", id),
				},
			}

			err := yamlrw.CreateMetric(metricName, testMetric)
			if err != nil {
				errorChan <- fmt.Errorf("create metric %s: %w", metricName, err)
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Logf("Error during concurrent create: %v", err)
	}

	// Verify all metrics were created
	finalMetrics, err := yamlrw.GetMetrics()
	assert.NoError(t, err)

	assert.Equal(t, numGoroutines, len(finalMetrics.MetricDefs),
		"Expected %d metrics, got %d", numGoroutines, len(finalMetrics.MetricDefs))
}

// TestMixedConcurrentOperations tests create, update, and delete happening concurrently
func TestMixedConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Setup initial metrics
	initialMetrics := &metrics.Metrics{
		MetricDefs: map[string]metrics.Metric{
			"existing_1": {
				Description: "Existing 1",
				SQLs:        map[int]string{1: "SELECT 1"},
			},
			"existing_2": {
				Description: "Existing 2",
				SQLs:        map[int]string{1: "SELECT 2"},
			},
			"to_delete": {
				Description: "Will be deleted",
				SQLs:        map[int]string{1: "SELECT 99"},
			},
		},
		PresetDefs: make(map[string]metrics.Preset),
	}
	err = yamlrw.WriteMetrics(initialMetrics)
	assert.NoError(t, err)

	var wg sync.WaitGroup
	errorChan := make(chan error, 20)

	// Concurrent creates
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := yamlrw.CreateMetric(fmt.Sprintf("new_%d", id), metrics.Metric{
				Description: fmt.Sprintf("New %d", id),
				SQLs:        map[int]string{1: "SELECT 1"},
			})
			if err != nil {
				errorChan <- fmt.Errorf("create new_%d: %w", id, err)
			}
		}(i)
	}

	// Concurrent updates to the same metric
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := yamlrw.UpdateMetric("existing_1", metrics.Metric{
				Description: fmt.Sprintf("Updated by goroutine %d", id),
				SQLs:        map[int]string{1: "SELECT UPDATED"},
			})
			if err != nil {
				errorChan <- fmt.Errorf("update existing_1: %w", err)
			}
		}(i)
	}

	// Concurrent updates to different metrics
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := yamlrw.UpdateMetric("existing_2", metrics.Metric{
				Description: fmt.Sprintf("Updated existing_2 by %d", id),
				SQLs:        map[int]string{1: "SELECT 2 UPDATED"},
			})
			if err != nil {
				errorChan <- fmt.Errorf("update existing_2: %w", err)
			}
		}(i)
	}

	// Concurrent delete
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := yamlrw.DeleteMetric("to_delete")
		if err != nil {
			errorChan <- fmt.Errorf("delete to_delete: %w", err)
		}
	}()

	wg.Wait()
	close(errorChan)

	// Log any errors
	for err := range errorChan {
		t.Logf("Error during mixed operations: %v", err)
	}

	// Verify final state
	finalMetrics, err := yamlrw.GetMetrics()
	assert.NoError(t, err)

	assert.Equal(t, 7, len(finalMetrics.MetricDefs),
		"Expected 7 metrics (5 new + 2 existing), got %d", len(finalMetrics.MetricDefs))

	assert.Contains(t, finalMetrics.MetricDefs, "existing_1", "existing_1 should still exist")
	assert.Contains(t, finalMetrics.MetricDefs, "existing_2", "existing_2 should still exist")
	assert.NotContains(t, finalMetrics.MetricDefs, "to_delete", "to_delete should be deleted")

	for i := 0; i < 5; i++ {
		metricName := fmt.Sprintf("new_%d", i)
		assert.Contains(t, finalMetrics.MetricDefs, metricName, "new_%d should exist", i)
	}
}

// TestConcurrentPresetOperations tests concurrent preset operations
func TestConcurrentPresetOperations(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Initialize
	err = yamlrw.WriteMetrics(&metrics.Metrics{
		MetricDefs: make(map[string]metrics.Metric),
		PresetDefs: map[string]metrics.Preset{
			"existing_preset": {
				Description: "Existing preset",
				Metrics:     map[string]float64{"metric1": 10},
			},
		},
	})
	assert.NoError(t, err)

	var wg sync.WaitGroup
	numPresets := 10

	// Concurrent preset creates
	for i := 0; i < numPresets; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			yamlrw.CreatePreset(fmt.Sprintf("preset_%d", id), metrics.Preset{
				Description: fmt.Sprintf("Preset %d", id),
				Metrics:     map[string]float64{"metric1": float64(id)},
			})
		}(i)
	}

	// Concurrent updates to existing preset
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			yamlrw.UpdatePreset("existing_preset", metrics.Preset{
				Description: fmt.Sprintf("Updated by %d", id),
				Metrics:     map[string]float64{"metric1": float64(id * 10)},
			})
		}(i)
	}

	wg.Wait()

	// Verify
	finalMetrics, err := yamlrw.GetMetrics()
	assert.NoError(t, err)

	// Should have numPresets + 1 (existing_preset)
	assert.Equal(t, numPresets+1, len(finalMetrics.PresetDefs),
		"Expected %d presets, got %d", numPresets+1, len(finalMetrics.PresetDefs))
}

// TestConcurrentReadsDuringWrites tests
func TestConcurrentReadsDuringWrites(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Initialize
	err = yamlrw.WriteMetrics(&metrics.Metrics{
		MetricDefs: make(map[string]metrics.Metric),
		PresetDefs: make(map[string]metrics.Preset),
	})
	assert.NoError(t, err)

	var wg sync.WaitGroup
	stopReading := make(chan struct{})

	// Start multiple readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopReading:
					return
				default:
					_, err := yamlrw.GetMetrics()
					if err != nil {
						t.Logf("Reader %d error: %v", readerID, err)
					}
					time.Sleep(time.Millisecond)
				}
			}
		}(i)
	}

	// Perform writes while reads are happening
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			yamlrw.UpdateMetric(fmt.Sprintf("metric_%d", id), metrics.Metric{
				Description: fmt.Sprintf("Metric %d", id),
				SQLs:        map[int]string{1: "SELECT 1"},
			})
		}(i)
	}

	// Wait for all writes to complete
	time.Sleep(100 * time.Millisecond)
	close(stopReading)
	wg.Wait()

	// Verify final state
	finalMetrics, err := yamlrw.GetMetrics()
	assert.NoError(t, err)
	assert.Equal(t, 20, len(finalMetrics.MetricDefs), "Expected 20 metrics")
}

// TestDuplicateCreateAttempts tests that CreateMetric properly handles concurrent duplicate attempts
func TestDuplicateCreateAttempts(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "metrics.yaml")

	yamlrw, err := metrics.NewYAMLMetricReaderWriter(ctx, tempFile)
	assert.NoError(t, err)

	// Initialize
	err = yamlrw.WriteMetrics(&metrics.Metrics{
		MetricDefs: make(map[string]metrics.Metric),
		PresetDefs: make(map[string]metrics.Preset),
	})
	assert.NoError(t, err)

	var wg sync.WaitGroup
	numAttempts := 10
	errorCount := 0
	successCount := 0
	var mu sync.Mutex

	// Multiple goroutines try to create the same metric
	for i := 0; i < numAttempts; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := yamlrw.CreateMetric("duplicate_metric", metrics.Metric{
				Description: fmt.Sprintf("Attempt %d", id),
				SQLs:        map[int]string{1: "SELECT 1"},
			})

			mu.Lock()
			if err != nil {
				if err == metrics.ErrMetricExists {
					errorCount++
				} else {
					t.Logf("Unexpected error in goroutine %d: %v", id, err)
				}
			} else {
				successCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	t.Logf("Success: %d, ErrMetricExists: %d", successCount, errorCount)

	// Exactly one should succeed, rest should get ErrMetricExists
	assert.Equal(t, 1, successCount, "Exactly one create should succeed")
	assert.Equal(t, numAttempts-1, errorCount, "All others should get ErrMetricExists")

	// Verify only one metric exists
	finalMetrics, err := yamlrw.GetMetrics()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(finalMetrics.MetricDefs), "Only one metric should exist")
}
