package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/cmdopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const ImageName = "docker.io/postgres:17-alpine"

var ctx = context.Background()

func initTestContainer() (*postgres.PostgresContainer, error) {
	dbName := "pgwatch"
	dbUser := "pgwatch"
	dbPassword := "pgwatchadmin"

	return postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
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
`), 0644))

	pg, err := initTestContainer()
	require.NoError(t, err)
	defer func() { assert.NoError(t, pg.Terminate(ctx)) }()
	connStr, err := pg.ConnectionString(ctx)
	t.Log(connStr)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(sourcesYaml, []byte(`
- name: test1
  conn_str: `+connStr+`
  kind: postgres
  is_enabled: true
  custom_metrics:
    test_metric: 60
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
		go main()
		<-time.After(5 * time.Second) // Wait for main to fetch some metrics
		cancel()
		<-mainCtx.Done() // Wait for main to finish
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
		go main()
		<-time.After(1 * time.Second) // Wait for main to fetch some metrics
		cancel()
		<-mainCtx.Done() // Wait for main to finish
		assert.Equal(t, cmdopts.ExitCodeWebUIError, gotExit, "port should be busy and fail to bind")
	})
}
