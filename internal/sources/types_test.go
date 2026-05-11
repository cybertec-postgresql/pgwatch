package sources_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func TestKind_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		kind     sources.Kind
		expected bool
	}{
		{name: "postgres", kind: sources.SourcePostgres, expected: true},
		{name: "postgres continuous", kind: sources.SourcePostgresContinuous, expected: true},
		{name: "pgbouncer", kind: sources.SourcePgBouncer, expected: true},
		{name: "pgpool", kind: sources.SourcePgPool, expected: true},
		{name: "patroni", kind: sources.SourcePatroni, expected: true},
		{name: "prometheus", kind: sources.SourcePrometheus, expected: true},
		{name: "invalid", kind: "invalid", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.kind.IsValid()
			assert.Equal(t, tt.expected, got, "IsValid(%v)", tt.kind)
		})
	}
}

func TestValidate_PrometheusKindAcceptsEmptyOptionalFields(t *testing.T) {
	srcs := sources.Sources{
		{
			Name:    "prom-test",
			ConnStr: "http://localhost:9187/metrics",
			Kind:    sources.SourcePrometheus,
		},
	}

	validated, err := srcs.Validate()

	assert.NoError(t, err)
	assert.Len(t, validated, 1)
	assert.Equal(t, sources.SourcePrometheus, validated[0].Kind)
}

func TestSource_Equal(t *testing.T) {
	var correctInterval = 60
	var incorrectInterval = 10

	s1 := sources.Source{
		Metrics:        metrics.MetricIntervals{"wal": correctInterval},
		MetricsStandby: metrics.MetricIntervals{"wal": correctInterval},
	}

	srcs := sources.Sources{
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
		},
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			Metrics:              metrics.MetricIntervals{"wal": correctInterval},
			MetricsStandby:       metrics.MetricIntervals{"wal": correctInterval},
		},
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			Metrics:              metrics.MetricIntervals{"wal": incorrectInterval},
			MetricsStandby:       metrics.MetricIntervals{"wal": incorrectInterval},
		},
		{
			Metrics:        metrics.MetricIntervals{"wal": incorrectInterval},
			MetricsStandby: metrics.MetricIntervals{"wal": incorrectInterval},
		},
		{
			Metrics:        metrics.MetricIntervals{"wal": correctInterval, "db_size": correctInterval},
			MetricsStandby: metrics.MetricIntervals{"wal": correctInterval, "db_size": correctInterval},
		},
		{
			Metrics:        metrics.MetricIntervals{"wal": correctInterval},
			MetricsStandby: metrics.MetricIntervals{"wal": correctInterval},
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
		Metrics:              metrics.MetricIntervals{"wal": correctInterval},
		MetricsStandby:       metrics.MetricIntervals{"wal": correctInterval},
	}

	srcs = sources.Sources{
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
		},
		{
			PresetMetrics:        "basic",
			PresetMetricsStandby: "basic",
			Metrics:              metrics.MetricIntervals{"wal": incorrectInterval},
			MetricsStandby:       metrics.MetricIntervals{"wal": incorrectInterval},
		},
		{
			PresetMetrics:        "azure",
			PresetMetricsStandby: "azure",
		},
		{
			PresetMetrics:        "azure",
			PresetMetricsStandby: "azure",
			Metrics:              metrics.MetricIntervals{"wal": correctInterval},
			MetricsStandby:       metrics.MetricIntervals{"wal": correctInterval},
		},
		{
			Metrics:        metrics.MetricIntervals{"wal": correctInterval},
			MetricsStandby: metrics.MetricIntervals{"wal": correctInterval},
		},
	}

	assert.True(t, s1.Equal(srcs[0]))
	assert.True(t, s1.Equal(srcs[1]))
	assert.False(t, s1.Equal(srcs[2]))
	assert.False(t, s1.Equal(srcs[3]))
	assert.False(t, s1.Equal(srcs[4]))
}
