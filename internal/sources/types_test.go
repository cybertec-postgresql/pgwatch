package sources_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

var ctx = context.Background()

func TestKind_IsValid(t *testing.T) {
	tests := []struct {
		kind     sources.Kind
		expected bool
	}{
		{kind: sources.SourcePostgres, expected: true},
		{kind: sources.SourcePostgresContinuous, expected: true},
		{kind: sources.SourcePgBouncer, expected: true},
		{kind: sources.SourcePgPool, expected: true},
		{kind: sources.SourcePatroni, expected: true},
		{kind: sources.SourcePatroniContinuous, expected: true},
		{kind: sources.SourcePatroniNamespace, expected: true},
		{kind: "invalid", expected: false},
	}

	for _, tt := range tests {
		got := tt.kind.IsValid()
		assert.True(t, got == tt.expected, "IsValid(%v) = %v, want %v", tt.kind, got, tt.expected)
	}
}

func TestSource_IsDefaultGroup(t *testing.T) {
	sources := sources.Sources{
		{
			Name:  "test_source",
			Group: "default",
		},
		{
			Name:  "test_source3",
			Group: "",
		},
		{
			Name:  "test_source2",
			Group: "custom_group",
		},
	}
	assert.True(t, sources[0].IsDefaultGroup())
	assert.True(t, sources[1].IsDefaultGroup())
	assert.False(t, sources[2].IsDefaultGroup())
}


func TestSource_Equal(t *testing.T) {
	var correctInterval float64 = 60
	var incorrectInterval float64 = 10

	s1 := sources.Source{
		Metrics: map[string]float64{"wal" : correctInterval},
		MetricsStandby: map[string]float64{"wal" : correctInterval},
	}

	srcs := sources.Sources{
		{
			PresetMetrics: "basic",
			PresetMetricsStandby: "basic",
		},
		{
			PresetMetrics: "basic",
			PresetMetricsStandby: "basic",
			Metrics: map[string]float64{"wal" : correctInterval},
			MetricsStandby: map[string]float64{"wal" : correctInterval},
		},
		{
			PresetMetrics: "basic",
			PresetMetricsStandby: "basic",
			Metrics: map[string]float64{"wal" : incorrectInterval},
			MetricsStandby: map[string]float64{"wal" : incorrectInterval},
		},
		{
			Metrics: map[string]float64{"wal" : incorrectInterval},
			MetricsStandby: map[string]float64{"wal" : incorrectInterval},
		},
		{
			Metrics: map[string]float64{"wal" : correctInterval, "db_size" : correctInterval},
			MetricsStandby: map[string]float64{"wal" : correctInterval, "db_size" : correctInterval},
		},
		{
			Metrics: map[string]float64{"wal" : correctInterval},
			MetricsStandby: map[string]float64{"wal" : correctInterval},
		},
	}

	assert.False(t, s1.Equal(srcs[0]))
	assert.False(t, s1.Equal(srcs[1]))
	assert.False(t, s1.Equal(srcs[2])) 
	assert.False(t, s1.Equal(srcs[3])) 
	assert.False(t, s1.Equal(srcs[4])) 
	assert.True(t, s1.Equal(srcs[5])) 

	s1 = sources.Source{
		PresetMetrics: "basic",
		PresetMetricsStandby : "basic",
		Metrics: map[string]float64{"wal" : correctInterval},
		MetricsStandby: map[string]float64{"wal" : correctInterval},
	}

	srcs = sources.Sources{
		{
			PresetMetrics: "basic",
			PresetMetricsStandby: "basic",
		},
		{
			PresetMetrics: "basic",
			PresetMetricsStandby: "basic",
			Metrics: map[string]float64{"wal" : incorrectInterval},
			MetricsStandby: map[string]float64{"wal" : incorrectInterval},
		},
		{
			PresetMetrics: "azure",
			PresetMetricsStandby: "azure",
		},
		{
			PresetMetrics: "azure",
			PresetMetricsStandby: "azure",
			Metrics: map[string]float64{"wal" : correctInterval},
			MetricsStandby: map[string]float64{"wal" : correctInterval},
		},
		{
			Metrics: map[string]float64{"wal" : correctInterval},
			MetricsStandby: map[string]float64{"wal" : correctInterval},
		},
	}

	assert.True(t, s1.Equal(srcs[0]))
	assert.True(t, s1.Equal(srcs[1]))
	assert.False(t, s1.Equal(srcs[2]))
	assert.False(t, s1.Equal(srcs[3]))
	assert.False(t, s1.Equal(srcs[4]))
}