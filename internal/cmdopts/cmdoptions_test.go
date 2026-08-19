package cmdopts

import (
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/log"
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

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name              string
		sources           string
		metrics           string
		wantErr           bool
		wantErrSubstring  string
		wantSourcesAfter  string
		wantMetricsAfter  string
	}{
		{
			name:     "both empty returns error",
			sources:  "",
			metrics:  "",
			wantErr:  true,
			wantErrSubstring: "both --sources and --metrics are empty",
		},
		{
			name:             "only metrics PG inherits sources",
			sources:          "",
			metrics:          "postgres://u@h/config",
			wantErr:          false,
			wantSourcesAfter: "postgres://u@h/config",
			wantMetricsAfter: "postgres://u@h/config",
		},
		{
			name:             "only sources PG inherits metrics",
			sources:          "postgres://u@h/config",
			metrics:          "",
			wantErr:          false,
			wantSourcesAfter: "postgres://u@h/config",
			wantMetricsAfter: "postgres://u@h/config",
		},
		{
			name:    "identical PG connstrs no error",
			sources: "postgres://u@h/config",
			metrics: "postgres://u@h/config",
			wantErr: false,
		},
		{
			name:             "two different PG connstrs rejected",
			sources:          "postgres://u@h/config1",
			metrics:          "postgres://u@h/config2",
			wantErr:          true,
			wantErrSubstring: "--sources and --metrics must use the same configuration database",
		},
		{
			name:    "PG sources with YAML metrics allowed",
			sources: "postgres://u@h/config",
			metrics: "metrics.yaml",
			wantErr: false,
		},
		{
			name:    "YAML sources with PG metrics allowed",
			sources: "sources.yaml",
			metrics: "postgres://u@h/config",
			wantErr: false,
		},
		{
			name:             "YAML sources with empty metrics keeps metrics empty",
			sources:          "sources.yaml",
			metrics:          "",
			wantErr:          false,
			wantSourcesAfter: "sources.yaml",
			wantMetricsAfter: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			c := NewCmdOptions()
			c.Sources.Sources = tt.sources
			c.Metrics.Metrics = tt.metrics
			err := c.ValidateConfig()
			if tt.wantErr {
				a.Error(err)
				if tt.wantErrSubstring != "" {
					a.Contains(err.Error(), tt.wantErrSubstring)
				}
				return
			}
			a.NoError(err)
			if tt.wantSourcesAfter != "" {
				a.Equal(tt.wantSourcesAfter, c.Sources.Sources)
			}
			if tt.wantMetricsAfter != "" {
				a.Equal(tt.wantMetricsAfter, c.Metrics.Metrics)
			}
		})
	}
}

func TestConfig(t *testing.T) {
	os.Args = []string{0: "config_test", "--sources=sample.config.yaml"}
	_, err := New(nil)
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--unknown"}
	_, err = New(nil)
	assert.Error(t, err)

	os.Args = []string{0: "config_test"} // sources arg is missing, but set PW_CONFIG
	t.Setenv("PW_SOURCES", "postgresql://foo:baz@bar/test")
	_, err = New(nil)
	assert.NoError(t, err)
}
