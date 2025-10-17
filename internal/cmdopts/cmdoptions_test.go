package cmdopts

import (
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

// NewCmdOptions returns a new instance of CmdOptions with default values
func NewCmdOptions(args ...string) *Options {
	cmdOpts := new(Options)
	_, _ = flags.NewParser(cmdOpts, flags.PrintErrors).ParseArgs(args)
	return cmdOpts
}

func TestParseFail(t *testing.T) {
	tests := [][]string{
		{0: "go-test", "--unknown-option"},
		{0: "go-test", "-c", "client01", "-f", "foo"},
	}
	for _, d := range tests {
		os.Args = d
		_, err := New(nil)
		assert.Error(t, err)
	}
}

func TestParseSuccess(t *testing.T) {
	tests := [][]string{
		{0: "go-test", "--help"},
	}
	for _, d := range tests {
		os.Args = d
		c, err := New(nil)
		assert.True(t, c.Help)
		assert.Error(t, err)
	}
}

func TestLogLevel(t *testing.T) {
	c := &Options{Logging: log.CmdOpts{LogLevel: "debug"}}
	assert.True(t, c.Verbose())
	c = &Options{Logging: log.CmdOpts{LogLevel: "info"}}
	assert.False(t, c.Verbose())
}

func TestNewCmdOptions(t *testing.T) {
	c := NewCmdOptions("-c", "config_unit_test", "--password=somestrong")
	assert.NotNil(t, c)
}

func TestConfig(t *testing.T) {
	os.Args = []string{0: "config_test", "--sources=sample.config.yaml"}
	_, err := New(nil)
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--unknown"}
	_, err = New(nil)
	assert.Error(t, err)

	os.Args = []string{0: "config_test"} // sources arg is missing, but set PW3_CONFIG
	t.Setenv("PW_SOURCES", "postgresql://foo:baz@bar/test")
	_, err = New(nil)
	assert.NoError(t, err)
}

func TestPartitionInterval(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid standard interval",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=24h"},
			expectError: false,
		},
		{
			name:        "valid custom interval",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=2h"},
			expectError: false,
		},
		{
			name:        "valid custom interval with days",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=72h"},
			expectError: false,
		},
		{
			name:        "invalid interval format",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=invalid"},
			expectError: true,
			errorMsg:    "invalid duration",
		},
		{
			name:        "prohibited year interval",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=8760h"},
			expectError: true,
			errorMsg:    "cannot use 1 year intervals",
		},
		{
			name:        "prohibited interval longer than year",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=9000h"},
			expectError: true,
			errorMsg:    "cannot use intervals longer than 1 year",
		},
		{
			name:        "prohibited minute interval",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=30m"},
			expectError: true,
			errorMsg:    "cannot use minute or second-based intervals",
		},
		{
			name:        "prohibited second interval",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=3600s"},
			expectError: true,
			errorMsg:    "cannot use minute or second-based intervals",
		},
		{
			name:        "standard partition interval still works",
			args:        []string{0: "test", "--sources=postgresql://test", "--partition-interval=168h"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			_, err := New(nil)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
