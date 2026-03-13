package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startMain starts main() in a goroutine and returns a channel that is closed
// when main() fully returns (including its deferred Exit() call). This must be
// used instead of `go main()` + `<-mainCtx.Done()` because mainCtx.Done() closes
// as soon as cancel() is called, before main()'s deferred Exit() has executed,
// causing a race with the test's `defer func() { Exit = os.Exit }()`.
func startMain() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()
	return done
}

func TestMain_Integration(t *testing.T) {
	// Prepare temp dir for output
	tempDir := t.TempDir()
	jsonSink := filepath.Join(tempDir, "out.json")

	// Prepare minimal metrics and sources YAML files
	metricsYaml := filepath.Join(tempDir, "metrics.yaml")
	sourcesYaml := filepath.Join(tempDir, "sources.yaml")
	require.NoError(t, os.WriteFile(metricsYaml, []byte(`
metrics:
    test_metric:
        sqls:
            11:  select (extract(epoch from now()) * 1e9)::int8 as epoch_ns, 42 as test_metric
    cpu_load:
        sqls:
            11:  select 'metric fetched via PostgreSQL not OS'
`), 0644))

	pg, tearDown, err := testutil.SetupPostgresContainer()
	require.NoError(t, err)
	defer tearDown()
	connStr, err := pg.ConnectionString(testutil.TestContext, "sslmode=disable")
	t.Log(connStr)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(sourcesYaml, []byte(`
- name: test1
  conn_str: `+connStr+`
  kind: postgres
  is_enabled: true
  custom_metrics:
    test_metric: 60
    cpu_load: 60
`), 0644))

	// Mock Exit to capture exit code
	var gotExit int32
	Exit = func(code int) { gotExit = int32(code) }
	defer func() { Exit = os.Exit }()

	t.Run("full run", func(t *testing.T) {
		os.Args = []string{
			"pgwatch",
			"--metrics", metricsYaml,
			"--sources", sourcesYaml,
			"--sink", "jsonfile://" + jsonSink,
			"--web-disable",
		}
		mainDone := startMain()
		<-time.After(5 * time.Second) // Wait for main to fetch some metrics
		cancel()
		<-mainDone // Wait for main to fully exit
		assert.Equal(t, cmdopts.ExitCodeOK, gotExit, "expected exit code 0")
		data, err := os.ReadFile(jsonSink)
		t.Log(string(data))
		assert.NoError(t, err, "output json file should exist")
		assert.NotEmpty(t, data, "output json file should not be empty")
		assert.Contains(t, string(data), `"test_metric":42`)
		assert.Contains(t, string(data), `"dbname":"test1"`)
		assert.Contains(t, string(data), `"metric":"test_metric"`)
	})

	t.Run("source ping", func(t *testing.T) {
		os.Args = []string{"pgwatch", "--sources", sourcesYaml, "source", "ping"}
		main()
		assert.Equal(t, cmdopts.ExitCodeOK, gotExit, "expected exit code 0")
	})

	t.Run("failed option", func(t *testing.T) {
		os.Args = []string{"pgwatch", "--uknnown-option"}
		main()
		assert.Equal(t, cmdopts.ExitCodeConfigError, gotExit)
	})

	t.Run("failed config reader", func(t *testing.T) {
		os.Args = []string{"pgwatch", "--sources", "fooboo"}
		main()
		assert.Equal(t, cmdopts.ExitCodeConfigError, gotExit)
	})

	t.Run("failed sink writer", func(t *testing.T) {
		os.Args = []string{"pgwatch", "--sources", sourcesYaml, "--sink", "fooboo"}
		main()
		assert.Equal(t, cmdopts.ExitCodeConfigError, gotExit)
	})

	t.Run("failed web init", func(t *testing.T) {
		os.Args = []string{"pgwatch", "--sources", sourcesYaml, "--sink", "jsonfile://" + jsonSink, "--web-addr", "localhost:-42"}
		mainDone := startMain()
		<-time.After(1 * time.Second) // Wait for main to fetch some metrics
		cancel()
		<-mainDone // Wait for main to fully exit
		assert.Equal(t, cmdopts.ExitCodeWebUIError, gotExit, "port should be busy and fail to bind")
	})
}
