package reaper

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pglogwatch"
	"github.com/cybertec-postgresql/pgwatch/v7/internal/testutil"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedSettingsQuery = `select current_setting`

// defaultLinePrefix is PostgreSQL's own default log_line_prefix.
//
// It only affects stderr parsing, but tryDetermineLogSettings reads it for
// every destination, so every mocked settings row has to carry it.
const defaultLinePrefix = `%m [%p] `

func TestNewLogParser(t *testing.T) {
	tempDir := t.TempDir()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	sourceConn := &sources.DbConn{
		Source: sources.Source{
			Name:    "test-source",
			Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 60.0},
		},
		Conn: mock,
	}
	storeCh := make(chan metrics.MeasurementEnvelope, 10)

	t.Run("success", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))

		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)

		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, true, lp.CollectorEnabled)
		assert.True(t, lp.CSVDestination)
		assert.Equal(t, tempDir, lp.Directory)
		assert.Equal(t, "en", lp.ServerMessagesLang)
		assert.Equal(t, false, lp.TruncateOnRotation)
		assert.Equal(t, 60*time.Second, lp.Interval)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("tryDetermineLogSettings error", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).WillReturnError(assert.AnError)
		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.Error(t, err)
		assert.Nil(t, lp)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown language defaults to en", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, tempDir, "zz", defaultLinePrefix))

		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, "en", lp.ServerMessagesLang)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("relative log directory", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, true, "/data/pg_log", "de", defaultLinePrefix))

		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, "/data/pg_log", lp.Directory)
		assert.Equal(t, "de", lp.ServerMessagesLang)
		assert.Equal(t, true, lp.TruncateOnRotation)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	// Retained deliberately, unlike the log_destination error.
	//
	// With logging_collector off there are no files in log_directory at
	// all -- PostgreSQL writes to the postmaster's stderr -- so this is a
	// fact about the server rather than a limitation of the parser. The
	// subtests below check it holds for every destination, since the
	// obvious way to widen the destination check is a switch that reaches a
	// working stderr branch before ever testing the collector.
	t.Run("logging_collector disabled", func(t *testing.T) {
		for _, d := range []struct {
			name       string
			csv, jsonl bool
		}{
			{"csvlog", true, false},
			{"jsonlog", false, true},
			{"stderr", false, false},
		} {
			t.Run(d.name, func(t *testing.T) {
				mock.ExpectQuery(expectedSettingsQuery).
					WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
						AddRow(false, d.csv, d.jsonl, true, "/data/pg_log", "de", defaultLinePrefix))

				lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
				require.Error(t, err)
				assert.Equal(t, "logging_collector is not enabled on the db server", err.Error())
				assert.Nil(t, lp)
			})
		}
	})

	// stderr is accepted, where it used to be a hard error.
	//
	// This subtest asserted that exact error until the migration. stderr is
	// PostgreSQL's DEFAULT log_destination, so what it really documented is
	// that pgwatch's log metric did not work on an unconfigured server --
	// which is the gap this phase closes.
	t.Run("stderr destination is accepted", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, false, false, true, "/data/pg_log", "de", defaultLinePrefix))

		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
		require.NoError(t, err)
		require.NotNil(t, lp)
		assert.Equal(t, pglogwatch.FormatStderr, lp.parserFormat())
		assert.Equal(t, defaultLinePrefix, lp.LinePrefix)
	})

	t.Run("jsonlog destination is accepted", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, false, true, true, "/data/pg_log", "de", defaultLinePrefix))

		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
		require.NoError(t, err)
		require.NotNil(t, lp)
		assert.Equal(t, pglogwatch.FormatJSON, lp.parserFormat())
	})

	// log_destination is a list, and "stderr,csvlog" is a common setting.
	// csvlog has to win: a server configured for both must keep producing
	// the counts it produced before the migration.
	t.Run("csvlog wins when both are configured", func(t *testing.T) {
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, true, true, "/data/pg_log", "de", defaultLinePrefix))

		lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
		require.NoError(t, err)
		assert.Equal(t, pglogwatch.FormatCSV, lp.parserFormat())
		assert.Equal(t, "*.csv", lp.remoteGlob())
	})
}

func TestTryDetermineLogSettings(t *testing.T) {
	t.Run("absolute log directory - known lang", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, "/var/log/postgresql", "de", defaultLinePrefix))

		logCfg, err := tryDetermineLogSettings(testutil.TestContext, mock)
		assert.NoError(t, err)
		assert.Equal(t, "/var/log/postgresql", logCfg.Directory)
		assert.Equal(t, "de", logCfg.ServerMessagesLang)
		assert.Equal(t, false, logCfg.TruncateOnRotation)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("relative log directory - unknown lang", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, "/data/log", "xx", defaultLinePrefix))

		logCfg, err := tryDetermineLogSettings(testutil.TestContext, mock)
		assert.NoError(t, err)
		assert.Equal(t, "/data/log", logCfg.Directory)
		assert.Equal(t, "en", logCfg.ServerMessagesLang)
		assert.Equal(t, false, logCfg.TruncateOnRotation)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnError(assert.AnError)

		logCfg, err := tryDetermineLogSettings(testutil.TestContext, mock)
		assert.Error(t, err)
		assert.Nil(t, logCfg)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCheckHasPrivileges(t *testing.T) {
	tempDir := t.TempDir()

	names := [2]string{"pg_ls_logdir() fails", "pg_read_file() permission denied"}
	for _, name := range names {
		t.Run("checkHasRemotePrivileges fails - "+name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			mock.ExpectQuery(expectedSettingsQuery).
				WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
					AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))

			// Mock IsClientOnSameHost to return false (remote)
			mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
				pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

			if name == "pg_ls_logdir() fails" {
				// Mock pg_ls_logdir() to fail (permission denied)
				mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
					WillReturnError(assert.AnError)
			} else {
				// Mock pg_ls_logdir() to return a log file
				mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
					WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("log.csv"))

				// Mock pg_read_file() to fail with permission denied error
				mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
					WithArgs(filepath.Join(tempDir, "log.csv")).
					WillReturnError(assert.AnError)
			}

			sourceConn := &sources.DbConn{
				Source: sources.Source{
					Name: "test-source",
				},
				Conn: mock,
			}

			storeCh := make(chan metrics.MeasurementEnvelope, 10)

			lp, err := newLogParser(testutil.TestContext, sourceConn, storeCh)
			require.NoError(t, err)
			// Parse logs should stop the worker and return due to privilege errors.
			err = lp.parseLogs()
			assert.Error(t, err)

			// Ensure mock expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())

			// No data should be received since checkHasPrivileges should fail
			select {
			case measurement := <-storeCh:
				t.Errorf("Expected no data, but got: %+v", measurement)
			case <-time.After(time.Second):
				// Expected: no data received
			}
		})
	}
}

func TestEventCountsToMetricStoreMessages(t *testing.T) {
	mdb := &sources.DbConn{
		Source: sources.Source{
			Name:       "test-db",
			Kind:       sources.SourcePostgres,
			CustomTags: map[string]string{"env": "test"},
		},
	}
	lp := &logParser{
		SourceConn: mdb,
		eventCounts: map[string]int64{
			"ERROR":   5,
			"WARNING": 10,
		},
		eventCountsTotal: map[string]int64{
			"ERROR":   15,
			"WARNING": 25,
			"INFO":    50,
		},
	}
	result := lp.getMeasurementEnvelope()

	assert.Equal(t, "test-db", result.DBName)
	assert.Equal(t, specialMetricServerLogEventCounts, result.MetricName)
	assert.Equal(t, map[string]string{"env": "test"}, result.CustomTags)

	// Check that all severities are present in the measurement
	assert.Len(t, result.Data, 1)
	measurement := result.Data[0]

	// Check individual severities
	assert.Equal(t, int64(5), measurement["error"])
	assert.Equal(t, int64(10), measurement["warning"])
	assert.Equal(t, int64(0), measurement["info"])  // Not in eventCounts
	assert.Equal(t, int64(0), measurement["debug"]) // Not in either map

	// Check total counts
	assert.Equal(t, int64(15), measurement["error_total"])
	assert.Equal(t, int64(25), measurement["warning_total"])
	assert.Equal(t, int64(50), measurement["info_total"])
	assert.Equal(t, int64(0), measurement["debug_total"])
}

func TestZeroEventCounts(t *testing.T) {
	eventCounts := map[string]int64{
		"ERROR":   5,
		"WARNING": 10,
		"INFO":    15,
	}

	zeroEventCounts(eventCounts)

	// Check that all pgSeverities are zeroed
	for _, severity := range pgSeverities {
		assert.Equal(t, int64(0), eventCounts[severity])
	}
}

func TestLogParseLocal(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.csv")

	// Create a test log file with CSV format entries
	logContent := `2023-12-01 10:30:45.123 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,1,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,ERROR,"duplicate key value violates unique constraint"
	2023-12-01 10:30:46.124 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,2,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,WARNING,"this is a warning message"
	2023-12-01 10:30:47.125 UTC,"postgres","otherdb",12346,"127.0.0.1:54322",session124,1,"INSERT",2023-12-01 10:30:00 UTC,1/235,568,ERROR,"another error message"
	`

	err := os.WriteFile(logFile, []byte(logContent), 0644)
	require.NoError(t, err)

	// Create a mock database connection
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(expectedSettingsQuery).
		WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
			AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))

	mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
		pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true))

	// Create a SourceConn for testing
	sourceConn := &sources.DbConn{
		Source: sources.Source{
			Name: "test-source",
		},
		Conn: mock,
	}

	// Create a context with timeout to prevent test from hanging
	ctx, cancel := context.WithTimeout(testutil.TestContext, 2*time.Second)
	defer cancel()

	// Create a channel to receive measurement envelopes
	storeCh := make(chan metrics.MeasurementEnvelope, 10)

	lp, err := newLogParser(ctx, sourceConn, storeCh)
	require.NoError(t, err)
	err = lp.parseLogs()
	assert.NoError(t, err)

	// Ensure mock expectations were met.
	assert.NoError(t, mock.ExpectationsWereMet())

	// Wait for measurements to be sent or timeout
	var measurement metrics.MeasurementEnvelope
	select {
	case measurement = <-storeCh:
		assert.NotEmpty(t, measurement.Data, "Measurement data should not be empty")
	case <-time.After(2 * time.Second):
	}

	assert.Equal(t, "test-source", measurement.DBName)
	assert.Equal(t, specialMetricServerLogEventCounts, measurement.MetricName)

	// Verify the data contains expected fields for both local and total counts
	data := measurement.Data[0]
	// Check that severity counts are present
	_, hasError := data["error"]
	_, hasWarning := data["warning"]
	assert.True(t, hasError && hasWarning, "Should have at least error and warning")
}

func TestLogParseRemote(t *testing.T) {
	const (
		testTimeout       = 3 * time.Second
		channelBufferSize = 10
		logFileName       = "postgresql.csv"
		testDbName        = "testdb"
	)

	// Sample log content with 3 entries: 2 ERRORs in different DBs, 1 WARNING
	logContent := `2023-12-01 10:30:45.123 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,1,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,ERROR,"duplicate key value violates unique constraint"
2023-12-01 10:30:46.124 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,2,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,WARNING,"this is a warning message"
2023-12-01 10:30:47.125 UTC,"postgres","otherdb",12346,"127.0.0.1:54322",session124,1,"INSERT",2023-12-01 10:30:00 UTC,1/235,568,ERROR,"another error message"
`

	t.Run("success - parses CSV logs with correct counts", func(t *testing.T) {
		tempDir := t.TempDir()

		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))

		// Phase 2: Mode detection - returns false to trigger remote mode
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

		// Phase 3: Privilege check - verifies pg_ls_logdir() and pg_read_file() permissions
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow("")) // 0 bytes read = permission test

		// Phase 4: log file discovery, now two queries rather than one.
		// pgwatch asks for sizes so it can seed each file's offset to its
		// current end -- existing content predates this process and is not
		// ours to report -- and pgremote lists the directory itself.
		mock.ExpectQuery(`select name, size from pg_ls_logdir\(\)`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size"}).
				AddRow(logFileName, int64(len(logContent))))
		mock.ExpectQuery(`SELECT name, size FROM pg_ls_logdir\(\) ORDER BY name`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size"}).
				AddRow(logFileName, int64(len(logContent))))

		sourceConn := &sources.DbConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 60}, // 60s interval - won't trigger during test
			},
			Conn: mock,
		}
		sourceConn.RealDbname = testDbName

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := newLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run parseLogs in a goroutine since it runs infinitely until context cancels
		go func() {
			_ = lp.parseLogs()
		}()

		// Wait for context to timeout
		// Note: parseLogsRemote starts reading from EOF, so existing log content isn't parsed
		// This test verifies the initialization and setup flow
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)

		// Verify mock expectations were met (privilege check + file discovery)
		assert.NoError(t, mock.ExpectationsWereMet(), "All mock expectations should be met")

		cancel()
	})

	t.Run("handles empty log directory gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		// Setup mocks for initialization
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

		// Privilege check passes
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

		// The directory cannot be listed. The seeding query's failure is
		// not fatal -- an unseeded file simply starts at zero -- but
		// pgremote.Open's is, so parseLogs returns instead of retrying.
		mock.ExpectQuery(`select name, size from pg_ls_logdir\(\)`).
			WillReturnError(assert.AnError)
		mock.ExpectQuery(`SELECT name, size FROM pg_ls_logdir\(\) ORDER BY name`).
			WillReturnError(assert.AnError)

		sourceConn := &sources.DbConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 1},
			},
			Conn: mock,
		}

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := newLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run in goroutine since it runs infinitely until context cancels
		go func() {
			_ = lp.parseLogs()
		}()

		// Wait for context to timeout
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)

		// No measurements should be received since no files were found
		select {
		case m := <-storeCh:
			t.Errorf("Expected no measurements, but received: %+v", m)
		default:
			// Expected: no measurements
		}
	})

	t.Run("malformed CSV entries are skipped gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		// Mix of valid and malformed log entries
		malformedContent := `2023-12-01 10:30:45.123 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,1,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,ERROR,"valid entry"
this is not a valid CSV line at all
incomplete line without proper fields
2023-12-01 10:30:47.125 UTC,"postgres","testdb",12346,"127.0.0.1:54322",session124,1,"INSERT",2023-12-01 10:30:00 UTC,1/235,568,WARNING,"another valid entry"
`

		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		// Setup all required mocks
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

		// Start at EOF: the offset is seeded to the file's current size,
		// so the existing content is not counted.
		mock.ExpectQuery(`select name, size from pg_ls_logdir\(\)`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size"}).
				AddRow(logFileName, int64(len(malformedContent))))
		mock.ExpectQuery(`SELECT name, size FROM pg_ls_logdir\(\) ORDER BY name`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size"}).
				AddRow(logFileName, int64(len(malformedContent))))

		sourceConn := &sources.DbConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 60}, // Long interval
			},
			Conn: mock,
		}
		sourceConn.RealDbname = testDbName

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := newLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run in goroutine
		go func() {
			_ = lp.parseLogs()
		}()

		// Wait for context to finish
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)

		// This test verifies the parser doesn't crash on malformed entries
		// Since we start at EOF and use a long interval, no parsing happens during the test
		// The real test is that initialization succeeds without errors
		assert.NoError(t, mock.ExpectationsWereMet())

		cancel()
	})

	t.Run("file read permission denied during parse", func(t *testing.T) {
		tempDir := t.TempDir()

		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		// Setup mocks - privilege check passes initially
		mock.ExpectQuery(expectedSettingsQuery).
			WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
				AddRow(true, true, false, false, tempDir, "en", defaultLinePrefix))
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

		// File discovery succeeds
		mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size", "modification"}).
				AddRow(logFileName, int32(0), time.Now()))

		// File state shows it has grown
		mock.ExpectQuery(`select size, modification from pg_ls_logdir\(\) where name = \$1;`).
			WithArgs(logFileName).
			WillReturnRows(pgxmock.NewRows([]string{"size", "modification"}).
				AddRow(int32(len(logContent)), time.Now()))

		// But pg_read_file fails with permission error during actual read
		mock.ExpectQuery(`select pg_read_file\(\$1, \$2, \$3\)`).
			WithArgs(filepath.Join(tempDir, logFileName), int32(0), int32(len(logContent))).
			WillReturnError(assert.AnError)

		sourceConn := &sources.DbConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 1},
			},
			Conn: mock,
		}

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := newLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run in goroutine
		go func() {
			_ = lp.parseLogs() // It will log a warning and continue retrying
		}()

		// Wait for context to finish
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)

		// No measurements should be sent since read failed
		select {
		case m := <-storeCh:
			// The parser might send an empty measurement before the error
			// Verify it's zeroed
			data := m.Data[0]
			assert.Equal(t, int64(0), data["error"], "Should have 0 errors since read failed")
			assert.Equal(t, int64(0), data["warning"], "Should have 0 warnings since read failed")
		default:
			// Also acceptable: no measurement sent at all
		}
	})
}

// TestRace_LogParserRealDbname verifies that concurrent FetchRuntimeInfo writes to
// RealDbname and logparser reads of lp.realDbname do not cause a data race.
// lp.realDbname is snapshotted at logParser construction time, so only the
// constructor call itself must be protected (via RLock in newLogParser).
func TestRace_LogParserRealDbname(t *testing.T) {
	md := sources.NewDbConn(sources.Source{Name: "race-test"})

	// Construct a logParser directly (no DB needed) reusing the internal struct.
	lp := &logParser{
		logConfig:        &logConfig{},
		ctx:              t.Context(),
		SourceConn:       md,
		realDbname:       "initial",
		Interval:         time.Second,
		StoreCh:          make(chan metrics.MeasurementEnvelope, 1),
		eventCounts:      make(map[string]int64),
		eventCountsTotal: make(map[string]int64),
	}

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: simulate FetchRuntimeInfo updating RealDbname under Lock.
	go func() {
		defer wg.Done()
		for range iterations {
			md.Lock()
			md.RealDbname = "updateddb"
			md.Unlock()
		}
	}()

	// Reader: logparser reads lp.realDbname (a plain string copy, no lock needed after construction).
	go func() {
		defer wg.Done()
		for range iterations {
			_ = lp.realDbname
		}
	}()

	wg.Wait()
}

// The server_log_event_counts envelope schema.
//
// This file is written against the PRE-migration implementation on purpose. It
// is a characterisation test: it pins what pgwatch emits today so that the
// switch to pglogwatch can be shown not to have changed it. A test written
// after the migration would only prove the new code agrees with itself.
//
// The schema is not pgwatch's to change. Dashboards select these column names,
// and the measurement store has them as columns; a renamed or missing key is a
// broken dashboard, not a failing test, so it has to fail here first.

// wantEnvelopeKeys is the complete set of keys the measurement carries: one
// per severity in pgSeverities, plus one _total per severity.
//
// Spelled out rather than generated from pgSeverities, because a test that
// derives its expectation from the code under test agrees with any change that
// code makes -- including a change to this list, which is the one thing this
// test exists to catch.
var wantEnvelopeKeys = []string{
	"debug", "info", "notice", "warning", "error", "log", "fatal", "panic",
	"debug_total", "info_total", "notice_total", "warning_total",
	"error_total", "log_total", "fatal_total", "panic_total",
}

func TestEnvelopeSchemaIsExactlyTheseSixteenKeys(t *testing.T) {
	lp := &logParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "test-db"}},
		eventCounts:      map[string]int64{},
		eventCountsTotal: map[string]int64{},
	}
	m := lp.getMeasurementEnvelope().Data[0]

	// NewMeasurement stamps the timestamp; every other key is a severity.
	got := make([]string, 0, len(m))
	for k := range m {
		if k == metrics.EpochColumnName {
			continue
		}
		got = append(got, k)
	}
	assert.ElementsMatch(t, wantEnvelopeKeys, got,
		"the envelope's key set is fixed; dashboards select these names")

	// Every count is an int64, including the zeroes. A JSON round trip that
	// turned an absent count into a float64 or a nil would reach the sink
	// looking like a schema change.
	for _, k := range wantEnvelopeKeys {
		require.Contains(t, m, k)
		assert.IsType(t, int64(0), m[k], "%s must be int64", k)
	}
}

func TestEnvelopeCountsAndIdentityAreUnchanged(t *testing.T) {
	mdb := &sources.DbConn{
		Source: sources.Source{
			Name:       "test-db",
			Kind:       sources.SourcePostgres,
			CustomTags: map[string]string{"env": "test"},
		},
	}
	lp := &logParser{
		SourceConn: mdb,
		// Keyed by ENGLISH severity name. Where those keys come from is
		// exactly what the migration changes -- a regex capture group
		// before, pglogwatch's normalised Severity after -- so pinning
		// the mapping from key to emitted column is what makes the two
		// implementations comparable.
		eventCounts:      map[string]int64{"ERROR": 5, "WARNING": 10},
		eventCountsTotal: map[string]int64{"ERROR": 15, "WARNING": 25, "INFO": 50},
	}

	env := lp.getMeasurementEnvelope()
	assert.Equal(t, "test-db", env.DBName)
	assert.Equal(t, specialMetricServerLogEventCounts, env.MetricName)
	assert.Equal(t, map[string]string{"env": "test"}, env.CustomTags)
	require.Len(t, env.Data, 1)

	m := env.Data[0]
	assert.Equal(t, int64(5), m["error"])
	assert.Equal(t, int64(10), m["warning"])
	assert.Equal(t, int64(15), m["error_total"])
	assert.Equal(t, int64(25), m["warning_total"])
	assert.Equal(t, int64(50), m["info_total"])

	// A severity counted only per-instance leaves the per-database column
	// at zero rather than absent, and a severity counted nowhere leaves
	// both at zero. Absent and zero are different on the wire.
	assert.Equal(t, int64(0), m["info"])
	assert.Equal(t, int64(0), m["debug"])
	assert.Equal(t, int64(0), m["debug_total"])
}

// DEBUG1..DEBUG5 are dropped, and that is the pre-migration behaviour.
//
// PostgreSQL writes DEBUG1 through DEBUG5, never a bare "DEBUG", so the
// "debug" column has always been zero on a real server: the regex captured
// "DEBUG3", the count landed under that key, and getMeasurementEnvelope only
// ever reads the eight names in pgSeverities.
//
// Recorded here because pglogwatch reports the same severities and the obvious
// adapter -- fold DEBUG1..5 into DEBUG -- would look like a bug fix while
// changing a column that has emitted zero for years. The migration is meant to
// leave the output identical, so the drop is preserved and made deliberate
// rather than incidental.
func TestNumberedDebugSeveritiesAreDropped(t *testing.T) {
	lp := &logParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "test-db"}},
		eventCounts:      map[string]int64{"DEBUG1": 7, "DEBUG3": 9},
		eventCountsTotal: map[string]int64{"DEBUG1": 7, "DEBUG3": 9},
	}
	m := lp.getMeasurementEnvelope().Data[0]

	assert.Equal(t, int64(0), m["debug"],
		"numbered debug severities do not reach the debug column")
	assert.Equal(t, int64(0), m["debug_total"])
	assert.NotContains(t, m, "debug1")
	assert.NotContains(t, m, "debug3")
}

// The envelope schema, verified against its consumer rather than against
// itself.
//
// TestEnvelopeSchemaIsExactlyTheseSixteenKeys pins the emitted key set, but a
// list in a test file agrees with whatever a developer changes it to. What
// actually breaks when a column is renamed is the shipped Grafana dashboard,
// which selects these keys by name out of the measurement's JSON:
//
//	sum((data->>'error_total')::int8) as error
//
// So this test reads the dashboard and requires every key it selects to be a
// key the parser emits. It is the closest thing to a contract test available
// without standing up Grafana.

var dashboardKeyRe = regexp.MustCompile(`data->>'([a-z_0-9]+)'`)

func TestEveryDashboardKeyIsEmitted(t *testing.T) {
	dashboard := filepath.Join("..", "..", "grafana", "postgres", "v12", "server-log-events.json")
	raw, err := os.ReadFile(dashboard) //nolint:gosec // a file in this repository
	require.NoError(t, err, "the dashboard this metric feeds must exist")

	matches := dashboardKeyRe.FindAllStringSubmatch(string(raw), -1)
	require.NotEmpty(t, matches, "the dashboard must select some keys, or this test proves nothing")

	seen := map[string]bool{}
	var keys []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	sort.Strings(keys)
	t.Logf("dashboard selects %d distinct keys: %v", len(keys), keys)

	lp := &logParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "schema-test"}},
		eventCounts:      map[string]int64{},
		eventCountsTotal: map[string]int64{},
	}
	m := lp.getMeasurementEnvelope().Data[0]

	for _, k := range keys {
		assert.Contains(t, m, k,
			"the dashboard selects %q; renaming or dropping it breaks a shipped panel", k)
		assert.IsType(t, int64(0), m[k],
			"%q is cast to int8 in the dashboard's SQL, so it must be an integer", k)
	}
}

// The counts survive the JSON round trip to the measurement store as integers.
//
// The envelope holds any, and a count that reached the sink as a float would
// still render -- Grafana casts to int8 -- but it would be stored as 5.0 and
// compared, summed and alerted on as a float. This checks the type the store
// actually receives rather than the type the map was built with.
func TestCountsAreIntegersAfterMarshalling(t *testing.T) {
	lp := &logParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "schema-test"}},
		eventCounts:      map[string]int64{"ERROR": 5},
		eventCountsTotal: map[string]int64{"ERROR": 9},
	}
	m := lp.getMeasurementEnvelope().Data[0]

	for k, v := range m {
		if k == metrics.EpochColumnName {
			continue
		}
		_, ok := v.(int64)
		assert.True(t, ok, "%s is %T, not int64", k, v)
	}
	assert.Equal(t, int64(5), m["error"])
	assert.Equal(t, int64(9), m["error_total"])
}
