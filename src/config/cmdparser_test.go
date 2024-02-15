package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFail(t *testing.T) {
	tests := [][]string{
		{0: "go-test", "--unknown-option"},
		{0: "go-test", "-c", "client01", "-f", "foo"},
	}
	for _, d := range tests {
		os.Args = d
		_, err := NewConfig(nil)
		assert.Error(t, err)
	}
}

func TestParseSuccess(t *testing.T) {
	tests := [][]string{
		{0: "go-test", "--version"},
		{0: "go-test", "--aes-gcm-password-to-encrypt", "foobaz"},
	}
	for _, d := range tests {
		os.Args = d
		_, err := NewConfig(nil)
		assert.NoError(t, err)
	}
}

func TestLogLevel(t *testing.T) {
	c := &Options{Logging: LoggingOpts{LogLevel: "debug"}}
	assert.True(t, c.Verbose())
	c = &Options{Logging: LoggingOpts{LogLevel: "info"}}
	assert.False(t, c.Verbose())
}

func TestNewCmdOptions(t *testing.T) {
	c := NewCmdOptions("-c", "config_unit_test", "--password=somestrong")
	assert.NotNil(t, c)
}
