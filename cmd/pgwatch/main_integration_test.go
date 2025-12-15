package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const ImageName = "docker.io/postgres:17-alpine"

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

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
    cpu_load:
        sqls:
            11:  select 'metric fetched via PostgreSQL not OS'
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

	t.Run("non-direct os stats", func(t *testing.T) {
		os.Remove(jsonSink) // truncate output file
		os.Args = []string{
			"pgwatch",
			"--sources", sourcesYaml,
			"--metrics", metricsYaml,
			"--sink", "jsonfile://" + jsonSink,
			"--web-disable",
		}

		go main()
		<-time.After(5 * time.Second) // Wait for main to fetch metric
		cancel()
		<-mainCtx.Done() // Wait for main to finish

		datab, err := os.ReadFile(jsonSink)
		assert.NoError(t, err)
		assert.Contains(t, string(datab), "metric fetched via PostgreSQL not OS")
	})
}

// TestServerLogEventCounts_Integration tests the server_log_event_counts metric
// which parses PostgreSQL CSV logs to count error and warning events.
func TestServerLogEventCounts_Integration(t *testing.T) {
	tempDir := t.TempDir()

	// Create PostgreSQL config file with CSV logging enabled
	pgConfigPath := filepath.Join(tempDir, "postgresql.conf")
	pgConfig := `
listen_addresses = '*'
log_destination = 'csvlog'
logging_collector = on
log_directory = 'pg_log'
log_filename = 'pgwatch-%M%S'
log_truncate_on_rotation = on
log_min_messages = notice
log_rotation_size = 10
`
	require.NoError(t, os.WriteFile(pgConfigPath, []byte(pgConfig), 0644))

	// Start PostgreSQL container with CSV logging
	pg, tearDown, err := testutil.SetupPostgresContainerWithConfig(pgConfigPath)
	require.NoError(t, err)
	defer tearDown()

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	t.Log("Connection string:", connStr)

	// Prepare output files
	jsonSink := filepath.Join(tempDir, "out.json")
	metricsYaml := filepath.Join(tempDir, "metrics.yaml")
	sourcesYaml := filepath.Join(tempDir, "sources.yaml")

	// The server_log_event_counts is a special metric handled by ParseLogs
	// We need to define it even though it's special - the SQL won't be used
	require.NoError(t, os.WriteFile(metricsYaml, []byte(`
metrics:
    server_log_event_counts:
        sqls:
            11: select 1 # not used, handled specially by ParseLogs
`), 0644))

	// Configure source with server_log_event_counts metric
	// The metric interval is set to 1 second for faster testing
	require.NoError(t, os.WriteFile(sourcesYaml, []byte(`
- name: logtest
  conn_str: `+connStr+`
  kind: postgres
  is_enabled: true
  custom_metrics:
    server_log_event_counts: 1
`), 0644))

	// Mock Exit to capture exit code
	var gotExit int32
	Exit = func(code int) { gotExit = int32(code) }
	defer func() { Exit = os.Exit }()

	os.Args = []string{
		"pgwatch",
		"--metrics", metricsYaml,
		"--sources", sourcesYaml,
		"--sink", "jsonfile://" + jsonSink,
		"--web-disable",
	}

	// Start pgwatch in background
	go main()

	// Wait a moment for pgwatch to start and connect
	time.Sleep(2 * time.Second)

	// Connect to the database and generate warnings and errors
	conn, err := pgx.Connect(ctx, connStr)
	require.NoError(t, err)
	defer conn.Close(ctx)

	// Generate WARNING messages using RAISE
	_, err = conn.Exec(ctx, `
		DO $$
		BEGIN
			RAISE WARNING 'Test warning message 1';
			RAISE WARNING 'Test warning message 2';
			RAISE WARNING 'Test warning message 3';
		END $$;
	`)
	require.NoError(t, err)

	// Generate ERROR messages (these will be logged but won't fail the connection)
	// We use a function that raises errors we can catch
	for range 2 {
		_, _ = conn.Exec(ctx, `
			DO $$
			BEGIN
				RAISE EXCEPTION 'Test error message 1';
			END $$;
		`)
	}

	// Also generate an actual ERROR by trying invalid SQL (this will be caught)
	_, _ = conn.Exec(ctx, `SELECT * FROM nonexistent_table_12345`)

	// Wait for metrics to be collected (log parsing interval + some processing time)
	time.Sleep(3 * time.Second)

	// Read and verify output
	data, err := os.ReadFile(jsonSink)
	require.NoError(t, err, "output json file should exist")

	// Check that we have server_log_event_counts metric data
	dataStr := string(data)
	assert.Contains(t, dataStr, `"metric":"server_log_event_counts"`,
		"should contain server_log_event_counts metric")
	assert.Contains(t, dataStr, `"dbname":"logtest"`,
		"should contain the source name")

	// Verify that warnings were captured (we generated 3 warnings)
	// The log parsing should have captured at least some of them
	warningCaptured := 
		strings.Contains(dataStr, `"warning":1`) ||
		strings.Contains(dataStr, `"warning":2`) ||
		strings.Contains(dataStr, `"warning":3`) &&
		strings.Contains(dataStr, `"warning_total":1`) ||
		strings.Contains(dataStr, `"warning_total":2`) ||
		strings.Contains(dataStr, `"warning_total":3`)
	assert.True(t, warningCaptured, "should have captured at least one warning, got: %s", dataStr)

	// Verify that errors were captured (we generated 3 ERRORs)
	errorCaptured := 
		strings.Contains(dataStr, `"error":1`) ||
		strings.Contains(dataStr, `"error":2`) ||
		strings.Contains(dataStr, `"error":3`) &&
		strings.Contains(dataStr, `"error_total":1`) ||
		strings.Contains(dataStr, `"error_total":2`) ||
		strings.Contains(dataStr, `"error_total":3`)
	assert.True(t, errorCaptured, "should have captured at least one error, got: %s", dataStr)


	var logFile string
	err = conn.QueryRow(ctx, `SELECT pg_current_logfile();`).Scan(&logFile)
	assert.NoError(t, err)

	// Generate enough warnings to get the file rotated
	_, err = conn.Exec(ctx, `
		DO $$
		BEGIN
			FOR cnt in 1..80 LOOP
				RAISE WARNING 'Test warning message';
			END LOOP;
		END $$;
	`)
	assert.NoError(t, err)

	var newLogFile string
	err = conn.QueryRow(ctx, `SELECT pg_current_logfile();`).Scan(&newLogFile)
	assert.NoError(t, err)
	assert.NotEqual(t, logFile, newLogFile)

	time.Sleep(1 * time.Second)
	_, err = conn.Exec(ctx, `
		DO $$
		BEGIN
			RAISE NOTICE 'This notice will appear in the new log file';
		END $$;
	`)
	assert.NoError(t, err)

	time.Sleep(3 * time.Second)

	data, err = os.ReadFile(jsonSink)
	dataStr = string(data)
	assert.NoError(t, err)
	assert.Contains(t, dataStr, `"notice":1`, "should contain notice mesage")
	assert.Contains(t, dataStr, `"notice_total":1`, "should contain notice message")

	// Stop pgwatch
	cancel()
	<-mainCtx.Done()
	assert.Equal(t, cmdopts.ExitCodeOK, gotExit, "expected exit code 0")
}
