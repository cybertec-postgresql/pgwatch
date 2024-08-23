package config

import (
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/log"
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
	c := &Options{Logging: log.LoggingCmdOpts{LogLevel: "debug"}}
	assert.True(t, c.Verbose())
	c = &Options{Logging: log.LoggingCmdOpts{LogLevel: "info"}}
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

	os.Args = []string{0: "config_test"} // clientname arg is missing, but set PW3_CONFIG
	t.Setenv("PW3_SOURCES", "postgresql://foo:baz@bar/test")
	_, err = New(nil)
	assert.NoError(t, err)
}
