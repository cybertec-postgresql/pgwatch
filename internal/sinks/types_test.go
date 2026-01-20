package sinks_test

import (
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sinks"
	"github.com/stretchr/testify/assert"
)

func TestSyncOp_String(t *testing.T) {
	tests := []struct {
		name     string
		op       sinks.SyncOp
		expected string
	}{
		{
			name:     "InvalidOp returns InvalidOp",
			op:       sinks.InvalidOp,
			expected: "InvalidOp",
		},
		{
			name:     "AddOp returns AddOp",
			op:       sinks.AddOp,
			expected: "AddOp",
		},
		{
			name:     "DeleteOp returns DeleteOp",
			op:       sinks.DeleteOp,
			expected: "DeleteOp",
		},
		{
			name:     "DefineOp returns DefineOp",
			op:       sinks.DefineOp,
			expected: "DefineOp",
		},
		{
			name:     "undefined value returns Unknown",
			op:       sinks.SyncOp(100),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.op.String())
		})
	}
}
