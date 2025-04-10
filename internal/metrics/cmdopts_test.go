package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheAge(t *testing.T) {
	tests := []struct {
		name     string
		opts     CmdOpts
		expected time.Duration
	}{
		{
			name:     "Cache enabled with positive value",
			opts:     CmdOpts{InstanceLevelCacheMaxSeconds: 30},
			expected: 30 * time.Second,
		},
		{
			name:     "Cache disabled with zero value",
			opts:     CmdOpts{InstanceLevelCacheMaxSeconds: 0},
			expected: 0,
		},
		{
			name:     "Cache disable with incorrect value",
			opts:     CmdOpts{InstanceLevelCacheMaxSeconds: -30},
			expected: 3600 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.opts.CacheAge())
		})
	}
}
