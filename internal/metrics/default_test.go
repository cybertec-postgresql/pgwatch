package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
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

// T047: preset postgres-exporter-basic is resolvable and contains the required metric families.
func TestDefaultMetrics_PostgresExporterBasicPreset(t *testing.T) {
	m := metrics.GetDefaultMetrics()
	if !assert.NotNil(t, m, "GetDefaultMetrics must not return nil") {
		return
	}

	preset, ok := m.PresetDefs["postgres-exporter-basic"]
	if !assert.True(t, ok, "preset 'postgres-exporter-basic' must exist in default metrics") {
		return
	}

	requiredFamilies := []string{
		"pg_stat_activity_count",
		"pg_stat_bgwriter_checkpoints_timed",
		"pg_stat_replication_pg_wal_lsn_diff",
	}
	for _, family := range requiredFamilies {
		assert.Contains(t, preset.Metrics, family,
			"preset 'postgres-exporter-basic' must include metric family %q", family)
	}
}
