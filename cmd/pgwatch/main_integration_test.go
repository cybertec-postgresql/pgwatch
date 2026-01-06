package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	t.Run("special source names", func(t *testing.T) {
		// Test various special source names with dots, capitals, underscores, hyphens, and numbers
		specialNames := []string{
			"source.new",
			"Source.With.Dots",
			"UPPERCASE_SOURCE",
			"MixedCase_Source-123",
			"source-with-hyphens",
			"source_with_underscores",
			"Source123.Test_Name-456",
		}

		specialSourcesYaml := filepath.Join(tempDir, "special_sources.yaml")

		// Build YAML with all special source names
		yamlContent := ""
		for _, name := range specialNames {
			yamlContent += `
- name: ` + name + `
  conn_str: ` + connStr + `
  kind: postgres
  is_enabled: true
  custom_metrics:
    test_metric: 60
`
		}
		require.NoError(t, os.WriteFile(specialSourcesYaml, []byte(yamlContent), 0644))

		os.Args = []string{
			"pgwatch",
			"--metrics", metricsYaml,
			"--sources", specialSourcesYaml,
			"--sink", connStr,
			"--web-disable",
		}

		go main()
		<-time.After(5 * time.Second) // Wait for main to fetch metrics and write to sink
		cancel()
		<-mainCtx.Done() // Wait for main to finish

		assert.Equal(t, cmdopts.ExitCodeOK, gotExit, "expected exit code 0 with special source names")

		// Connect to the sink database and verify metrics were collected for each special source name
		sinkConn, err := pgx.Connect(context.Background(), connStr)
		require.NoError(t, err)
		defer sinkConn.Close(context.Background())

		// Query the admin.all_distinct_dbname_metrics table to verify all sources are present
		rows, err := sinkConn.Query(context.Background(),
			"SELECT dbname FROM admin.all_distinct_dbname_metrics WHERE metric = 'test_metric'")
		require.NoError(t, err)

		foundSources := make(map[string]bool)
		for rows.Next() {
			var dbname string
			require.NoError(t, rows.Scan(&dbname))
			foundSources[dbname] = true
		}
		require.NoError(t, rows.Err())

		// Verify each special source name has metrics in the sink
		for _, name := range specialNames {
			assert.True(t, foundSources[name],
				"expected source name %q to have metrics in the sink database", name)
		}

		// Also verify actual metrics data exists in the test_metric table
		var count int
		err = sinkConn.QueryRow(context.Background(),
			"SELECT count(*) FROM public.test_metric").Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, len(specialNames), "expected metrics to be stored in test_metric table")
	})
}
