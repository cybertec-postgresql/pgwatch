package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/stretchr/testify/assert"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func TestGetSQL(t *testing.T) {
	m := Metric{}
	m.SQLs = SQLs{
		1: "one",
		3: "three",
		5: "five",
		6: "six",
	}
	tests := map[int]string{
		2:  "one",
		3:  "three",
		4:  "three",
		5:  "five",
		6:  "six",
		10: "six",
		0:  "",
	}

	for i, tt := range tests {
		if got := m.GetSQL(i); got != tt {
			t.Errorf("VersionToInt() = %v, want %v", got, tt)
		}
	}
}
func TestPrimaryOnly(t *testing.T) {
	m := Metric{NodeStatus: "primary"}
	assert.True(t, m.PrimaryOnly())
	assert.False(t, m.StandbyOnly())
	m.NodeStatus = "standby"
	assert.False(t, m.PrimaryOnly())
	assert.True(t, m.StandbyOnly())
}

func TestMeasurement(t *testing.T) {
	m := NewMeasurement(1234567890)
	assert.Equal(t, int64(1234567890), m.GetEpoch(), "epoch should be equal")
	m[EpochColumnName] = "wrong type"
	assert.True(t, time.Now().UnixNano()-m.GetEpoch() < int64(time.Second), "epoch should be close to now")
}

func TestMeasurements(t *testing.T) {
	m := Measurements{}
	assert.False(t, m.IsEpochSet(), "epoch should not be set")
	assert.True(t, time.Now().UnixNano()-m.GetEpoch() < 100, "epoch should be close to now")
	m = append(m, NewMeasurement(1234567890))
	assert.True(t, m.IsEpochSet(), "epoch should be set")
	assert.Equal(t, int64(1234567890), m.GetEpoch(), "epoch should be equal")
	m1 := m.DeepCopy()
	assert.Equal(t, m, m1, "deep copy should be equal")
	m1.Touch()
	assert.NotEqual(t, m, m1, "deep copy should be different")
	assert.True(t, time.Now().UnixNano()-m1.GetEpoch() < int64(time.Second), "epoch should be close to now")
}

func TestFilterByNames(t *testing.T) {
	// Setup test data
	metrics := &Metrics{
		MetricDefs: MetricDefs{
			"cpu_load": Metric{
				Description: "CPU load metric",
				InitSQL:     "CREATE FUNCTION cpu_load()",
			},
			"db_size": Metric{
				Description: "Database size metric",
			},
			"db_stats": Metric{
				Description: "Database stats metric",
			},
			"replication": Metric{
				Description: "Replication metric",
			},
		},
		PresetDefs: PresetDefs{
			"minimal": Preset{
				Description: "Minimal preset",
				Metrics: map[string]float64{
					"cpu_load": 60,
					"db_size":  300,
				},
			},
			"standard": Preset{
				Description: "Standard preset",
				Metrics: map[string]float64{
					"cpu_load":    60,
					"db_size":     300,
					"db_stats":    60,
					"replication": 120,
				},
			},
		},
	}

	tests := []struct {
		name          string
		names         []string
		wantMetrics   []string
		wantPresets   []string
		wantErr       bool
		errContains   string
	}{
		{
			name:        "empty names returns all",
			names:       []string{},
			wantMetrics: []string{"cpu_load", "db_size", "db_stats", "replication"},
			wantPresets: []string{"minimal", "standard"},
			wantErr:     false,
		},
		{
			name:        "single metric",
			names:       []string{"cpu_load"},
			wantMetrics: []string{"cpu_load"},
			wantPresets: []string{},
			wantErr:     false,
		},
		{
			name:        "multiple metrics",
			names:       []string{"cpu_load", "db_size"},
			wantMetrics: []string{"cpu_load", "db_size"},
			wantPresets: []string{},
			wantErr:     false,
		},
		{
			name:        "single preset includes all its metrics",
			names:       []string{"minimal"},
			wantMetrics: []string{"cpu_load", "db_size"},
			wantPresets: []string{"minimal"},
			wantErr:     false,
		},
		{
			name:        "multiple presets",
			names:       []string{"minimal", "standard"},
			wantMetrics: []string{"cpu_load", "db_size", "db_stats", "replication"},
			wantPresets: []string{"minimal", "standard"},
			wantErr:     false,
		},
		{
			name:        "mix of metrics and presets",
			names:       []string{"minimal", "replication"},
			wantMetrics: []string{"cpu_load", "db_size", "replication"},
			wantPresets: []string{"minimal"},
			wantErr:     false,
		},
		{
			name:        "non-existent metric",
			names:       []string{"nonexistent"},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "mix of existing and non-existing",
			names:       []string{"cpu_load", "nonexistent"},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "non-existent preset",
			names:       []string{"nonexistent_preset"},
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := metrics.FilterByNames(tt.names)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Check metrics
			assert.Equal(t, len(tt.wantMetrics), len(result.MetricDefs), "metric count mismatch")
			for _, metricName := range tt.wantMetrics {
				assert.Contains(t, result.MetricDefs, metricName, "expected metric not found: "+metricName)
			}

			// Check presets
			assert.Equal(t, len(tt.wantPresets), len(result.PresetDefs), "preset count mismatch")
			for _, presetName := range tt.wantPresets {
				assert.Contains(t, result.PresetDefs, presetName, "expected preset not found: "+presetName)
			}
		})
	}
}
