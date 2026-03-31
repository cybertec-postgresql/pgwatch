package sources_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

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
		{kind: "invalid", expected: false},
	}

	for _, tt := range tests {
		got := tt.kind.IsValid()
		assert.True(t, got == tt.expected, "IsValid(%v) = %v, want %v", tt.kind, got, tt.expected)
	}
}

func TestSource_Equal(t *testing.T) {
	var correctInterval float64 = 60
	var incorrectInterval float64 = 10

	s1 := sources.Source{
		Metrics:        map[string]float64{"wal": correctInterval},
		MetricsStandby: map[string]float64{"wal": correctInterval},
	}

	srcs := sources.Sources{
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
		},
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			Metrics:              map[string]float64{"wal": correctInterval},
			MetricsStandby:       map[string]float64{"wal": correctInterval},
		},
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			Metrics:              map[string]float64{"wal": incorrectInterval},
			MetricsStandby:       map[string]float64{"wal": incorrectInterval},
		},
		{
			Metrics:        map[string]float64{"wal": incorrectInterval},
			MetricsStandby: map[string]float64{"wal": incorrectInterval},
		},
		{
			Metrics:        map[string]float64{"wal": correctInterval, "db_size": correctInterval},
			MetricsStandby: map[string]float64{"wal": correctInterval, "db_size": correctInterval},
		},
		{
			Metrics:        map[string]float64{"wal": correctInterval},
			MetricsStandby: map[string]float64{"wal": correctInterval},
		},
	}

	assert.False(t, s1.Equal(srcs[0]))
	assert.False(t, s1.Equal(srcs[1]))
	assert.False(t, s1.Equal(srcs[2]))
	assert.False(t, s1.Equal(srcs[3]))
	assert.False(t, s1.Equal(srcs[4]))
	assert.True(t, s1.Equal(srcs[5]))

	s1 = sources.Source{
		PresetMetrics:        "basic",
		PresetMetricsStandby: "basic",
		Metrics:              map[string]float64{"wal": correctInterval},
		MetricsStandby:       map[string]float64{"wal": correctInterval},
	}

	srcs = sources.Sources{
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
		},
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			Metrics:              map[string]float64{"wal": incorrectInterval},
			MetricsStandby:       map[string]float64{"wal": incorrectInterval},
		},
		{
			PresetMetrics:        "azure",
			PresetMetricsStandby: "azure",
		},
		{
			PresetMetrics:        "azure",
			PresetMetricsStandby: "azure",
			Metrics:              map[string]float64{"wal": correctInterval},
			MetricsStandby:       map[string]float64{"wal": correctInterval},
		},
		{
			Metrics:        map[string]float64{"wal": correctInterval},
			MetricsStandby: map[string]float64{"wal": correctInterval},
		},
	}

	assert.True(t, s1.Equal(srcs[0]))
	assert.True(t, s1.Equal(srcs[1]))
	assert.False(t, s1.Equal(srcs[2]))
	assert.False(t, s1.Equal(srcs[3]))
	assert.False(t, s1.Equal(srcs[4]))
}

func TestSource_IsSameConnection(t *testing.T) {
	s1 := sources.Source{
		Name:                 "test_db",
		Group:                "default",
		ConnStr:              "postgres://user:pass@localhost:5432/db",
		Kind:                 sources.SourcePostgres,
		IncludePattern:       "prod_*",
		ExcludePattern:       "test_*",
		IsEnabled:            true,
		OnlyIfMaster:         false,
		Metrics:              map[string]float64{"wal": 60},
		MetricsStandby:       map[string]float64{"wal": 60},
		PresetMetrics:        "basic",
		PresetMetricsStandby: "basic",
		CustomTags:           map[string]string{"env": "prod"},
	}

	srcs := sources.Sources{
		{
			// Same connection details and different metrics/presets/tags
			Name:                 "test_db",
			Group:                "default",
			ConnStr:              "postgres://user:pass@localhost:5432/db",
			Kind:                 sources.SourcePostgres,
			IncludePattern:       "prod_*",
			ExcludePattern:       "test_*",
			IsEnabled:            true,
			OnlyIfMaster:         false,
			Metrics:              map[string]float64{"db_size": 300},
			MetricsStandby:       map[string]float64{"db_size": 300},
			PresetMetrics:        "exhaustive",
			PresetMetricsStandby: "exhaustive",
			CustomTags:           map[string]string{"env": "staging"},
		},
		{
			// Different name "different connection"
			Name:           "different_name",
			Group:          "default",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "prod_*",
			ExcludePattern: "test_*",
			IsEnabled:      true,
			OnlyIfMaster:   false,
		},
		{
			// Different group "different connection"
			Name:           "test_db",
			Group:          "different_group",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "prod_*",
			ExcludePattern: "test_*",
			IsEnabled:      true,
			OnlyIfMaster:   false,
		},
		{
			// different connection string "different connection"
			Name:           "test_db",
			Group:          "default",
			ConnStr:        "postgres://user:pass@remote:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "prod_*",
			ExcludePattern: "test_*",
			IsEnabled:      true,
			OnlyIfMaster:   false,
		},
		{
			// different kind "different connection"
			Name:           "test_db",
			Group:          "default",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePgBouncer,
			IncludePattern: "prod_*",
			ExcludePattern: "test_*",
			IsEnabled:      true,
			OnlyIfMaster:   false,
		},
		{
			// different include pattern "different connection"
			Name:           "test_db",
			Group:          "default",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "dev_*",
			ExcludePattern: "test_*",
			IsEnabled:      true,
			OnlyIfMaster:   false,
		},
		{
			// different exclude pattern "different connection"
			Name:           "test_db",
			Group:          "default",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "prod_*",
			ExcludePattern: "ignore_*",
			IsEnabled:      true,
			OnlyIfMaster:   false,
		},
		{
			// different enabled state "different connection"
			Name:           "test_db",
			Group:          "default",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "prod_*",
			ExcludePattern: "test_*",
			IsEnabled:      false,
			OnlyIfMaster:   false,
		},
		{
			// different master only state "different connection"
			Name:           "test_db",
			Group:          "default",
			ConnStr:        "postgres://user:pass@localhost:5432/db",
			Kind:           sources.SourcePostgres,
			IncludePattern: "prod_*",
			ExcludePattern: "test_*",
			IsEnabled:      true,
			OnlyIfMaster:   true,
		},
		{
			// identical to s1 "same connection"
			Name:                 "test_db",
			Group:                "default",
			ConnStr:              "postgres://user:pass@localhost:5432/db",
			Kind:                 sources.SourcePostgres,
			IncludePattern:       "prod_*",
			ExcludePattern:       "test_*",
			IsEnabled:            true,
			OnlyIfMaster:         false,
			Metrics:              map[string]float64{"wal": 60},
			MetricsStandby:       map[string]float64{"wal": 60},
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			CustomTags:           map[string]string{"env": "prod"},
		},
	}

	assert.True(t, s1.IsSameConnection(srcs[0]))
	assert.False(t, s1.IsSameConnection(srcs[1]))
	assert.False(t, s1.IsSameConnection(srcs[2]))
	assert.False(t, s1.IsSameConnection(srcs[3]))
	assert.False(t, s1.IsSameConnection(srcs[4]))
	assert.False(t, s1.IsSameConnection(srcs[5]))
	assert.False(t, s1.IsSameConnection(srcs[6]))
	assert.False(t, s1.IsSameConnection(srcs[7]))
	assert.False(t, s1.IsSameConnection(srcs[8]))
	assert.True(t, s1.IsSameConnection(srcs[9]))
}
