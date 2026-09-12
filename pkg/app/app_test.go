package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/log"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sinks"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/webserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newOptions returns options that start a collector without touching any
// database: the built-in metrics, an empty source list and a file sink.
func newOptions(t *testing.T) *cmdopts.Options {
	t.Helper()
	dir := t.TempDir()
	sourcesYaml := filepath.Join(dir, "sources.yaml")
	require.NoError(t, os.WriteFile(sourcesYaml, []byte("[]\n"), 0644))
	return &cmdopts.Options{
		Sources: sources.CmdOpts{Sources: sourcesYaml, Refresh: 120},
		Sinks:   sinks.CmdOpts{Sinks: []string{"jsonfile://" + filepath.Join(dir, "out.json")}},
		Logging: log.CmdOpts{LogLevel: "error"},
		WebUI:   webserver.CmdOpts{WebDisable: webserver.WebDisableAll},
	}
}

func TestNew(t *testing.T) {
	t.Run("nil options", func(t *testing.T) {
		a, err := New(context.Background(), nil)
		assert.Error(t, err)
		assert.Nil(t, a)
	})

	t.Run("accessors", func(t *testing.T) {
		opts := newOptions(t)
		a, err := New(context.Background(), opts)
		require.NoError(t, err)
		assert.Same(t, opts, a.Options())
		assert.NotNil(t, a.Logger())
		assert.False(t, a.Ready(), "not ready before Run")
	})
}

func TestRun_ExitCodes(t *testing.T) {
	t.Run("ExitCodeConfigError on config readers", func(t *testing.T) {
		opts := newOptions(t)
		opts.Sources.Sources = filepath.Join(t.TempDir(), "no-such-file.yaml")
		a, err := New(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, int(cmdopts.ExitCodeConfigError), a.Run(context.Background()))
	})

	t.Run("ExitCodeConfigError on sink writer", func(t *testing.T) {
		opts := newOptions(t)
		opts.Sinks.Sinks = []string{"foboo"}
		a, err := New(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, int(cmdopts.ExitCodeConfigError), a.Run(context.Background()))
	})

	t.Run("ExitCodeUpgradeError on pending migration", func(t *testing.T) {
		a, err := New(context.Background(), newOptions(t))
		require.NoError(t, err)
		a.needsUpgrade = func() (bool, error) { return true, nil }
		assert.Equal(t, int(cmdopts.ExitCodeUpgradeError), a.Run(context.Background()))
	})

	t.Run("ExitCodeUpgradeError on failed schema check", func(t *testing.T) {
		a, err := New(context.Background(), newOptions(t))
		require.NoError(t, err)
		a.needsUpgrade = func() (bool, error) { return false, errors.New("schema check failed") }
		assert.Equal(t, int(cmdopts.ExitCodeUpgradeError), a.Run(context.Background()))
	})

	t.Run("ExitCodeWebUIError on unusable address", func(t *testing.T) {
		opts := newOptions(t)
		opts.WebUI = webserver.CmdOpts{WebDisable: webserver.WebDisableUI, WebAddr: "localhost:-42"}
		a, err := New(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, int(cmdopts.ExitCodeWebUIError), a.Run(context.Background()))
	})

	t.Run("ExitCodeWebUIError without a UI provider", func(t *testing.T) {
		opts := newOptions(t)
		opts.WebUI = webserver.CmdOpts{WebAddr: "localhost:0"}
		a, err := New(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, int(cmdopts.ExitCodeWebUIError), a.Run(context.Background()))
	})

	t.Run("ExitCodeFatalError on panic", func(t *testing.T) {
		a, err := New(context.Background(), newOptions(t))
		require.NoError(t, err)
		a.needsUpgrade = func() (bool, error) { panic("boom") }
		assert.Equal(t, int(cmdopts.ExitCodeFatalError), a.Run(context.Background()))
	})

	t.Run("ExitCodeOK on cancelled context", func(t *testing.T) {
		a, err := New(context.Background(), newOptions(t))
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		code := make(chan int, 1)
		go func() { code <- a.Run(ctx) }()
		waitReady(t, a)
		cancel()
		assert.Equal(t, int(cmdopts.ExitCodeOK), waitCode(t, code))
	})

	t.Run("ExitCodeUserCancel on interrupt", func(t *testing.T) {
		a, err := New(context.Background(), newOptions(t))
		require.NoError(t, err)
		code := make(chan int, 1)
		go func() { code <- a.Run(context.Background()) }()
		waitReady(t, a) // the signal handler is in place well before this
		require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))
		assert.Equal(t, int(cmdopts.ExitCodeUserCancel), waitCode(t, code))
	})
}

func waitReady(t *testing.T, a *App) {
	t.Helper()
	require.Eventually(t, a.Ready, 10*time.Second, 10*time.Millisecond, "collector should start")
}

func waitCode(t *testing.T, code <-chan int) int {
	t.Helper()
	select {
	case c := <-code:
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
		return -1
	}
}
