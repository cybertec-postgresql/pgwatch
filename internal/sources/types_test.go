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
