package cmdopts

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigInitCommand_Execute(t *testing.T) {
	a := assert.New(t)
	t.Run("subcommand is missing", func(*testing.T) {
		os.Args = []string{0: "config_test", "config"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("subcommand is invalid", func(*testing.T) {
		os.Args = []string{0: "config_test", "config", "invalid"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("sources and metrics are empty", func(*testing.T) {
		os.Args = []string{0: "config_test", "config", "init"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("metrics is a proper file name", func(*testing.T) {
		fname := t.TempDir() + "/metrics.yaml"
		os.Args = []string{0: "config_test", "--metrics=" + fname, "config", "init"}
		_, err := New(io.Discard)
		a.NoError(err)
		a.FileExists(fname)
		fi, err := os.Stat(fname)
		require.NoError(t, err)
		a.True(fi.Size() > 0)
	})

	t.Run("sources is a proper file name", func(*testing.T) {
		fname := t.TempDir() + "/sources.yaml"
		os.Args = []string{0: "config_test", "--sources=" + fname, "config", "init"}
		_, err := New(io.Discard)
		a.NoError(err)
		a.FileExists(fname)
		fi, err := os.Stat(fname)
		require.NoError(t, err)
		a.True(fi.Size() > 0)
	})

	t.Run("metrics is an invalid file name", func(*testing.T) {
		os.Args = []string{0: "config_test", "--metrics=/", "config", "init"}
		opts, err := New(io.Discard)
		a.Error(err)
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

	t.Run("metrics is proper postgres connectin string", func(*testing.T) {
		os.Args = []string{0: "config_test", "--metrics=postgresql://foo@bar/baz", "config", "init"}
		opts, err := New(io.Discard)
		a.Error(err)
		a.Equal(ExitCodeConfigError, opts.ExitCode)
	})

}

func TestConfigUpgradeCommand_Execute(t *testing.T) {
	a := assert.New(t)

	t.Run("sources and metrics are empty", func(*testing.T) {
		os.Args = []string{0: "config_test", "config", "upgrade"}
		_, err := New(io.Discard)
		a.Error(err)
	})

	t.Run("metrics is a proper file name but files are not upgradable", func(*testing.T) {
		fname := t.TempDir() + "/metrics.yaml"
		os.Args = []string{0: "config_test", "--metrics=" + fname, "config", "upgrade"}
		c, err := New(io.Discard)
		a.Error(err)
		a.True(c.CommandCompleted)
		a.Equal(ExitCodeConfigError, c.ExitCode)
	})

}
