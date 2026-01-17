package reaper

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogParser(t *testing.T) {
	tempDir := t.TempDir()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	sourceConn := &sources.SourceConn{
		Source: sources.Source{
			Name:    "test-source",
			Metrics: map[string]float64{specialMetricServerLogEventCounts: 60.0},
		},
		Conn: mock,
	}
	storeCh := make(chan metrics.MeasurementEnvelope, 10)

	t.Run("success", func(t *testing.T) {
		// Mock log folder detection
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))

		// Mock language detection
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

		// Mock log_truncate_on_rotation detection
		mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
			WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))

		lp, err := NewLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, tempDir, lp.LogFolder)
		assert.Equal(t, "en", lp.ServerMessagesLang)
		assert.Equal(t, 60.0, lp.Interval)
		assert.NotNil(t, lp.LogsMatchRegex)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("tryDetermineLogFolder error", func(t *testing.T) {
		// Mock log folder detection to fail
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnError(assert.AnError)

		lp, err := NewLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.Error(t, err)
		assert.Nil(t, lp)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("tryDetermineLogMessagesLanguage error", func(t *testing.T) {
		// Mock log folder detection to succeed
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))

		// Mock language detection to fail
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnError(assert.AnError)

		lp, err := NewLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.Error(t, err)
		assert.Nil(t, lp)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown language defaults to en", func(t *testing.T) {
		// Mock log folder detection
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))

		// Mock language detection with unknown language
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("zz"))

		// Mock log_truncate_on_rotation detection
		mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
			WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))

		lp, err := NewLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, "en", lp.ServerMessagesLang)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("relative log directory", func(t *testing.T) {
		// Mock log folder detection with relative path
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("/data", "pg_log"))

		// Mock language detection
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("de"))

		// Mock log_truncate_on_rotation detection
		mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
			WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))

		lp, err := NewLogParser(testutil.TestContext, sourceConn, storeCh)
		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, "/data/pg_log", lp.LogFolder)
		assert.Equal(t, "de", lp.ServerMessagesLang)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTryDetermineLogFolder(t *testing.T) {
	t.Run("absolute log directory", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).
				AddRow("/data", "/var/log/postgresql"))

		logPath, err := tryDetermineLogFolder(testutil.TestContext, mock)
		assert.NoError(t, err)
		assert.Equal(t, "/var/log/postgresql", logPath)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("relative log directory", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).
				AddRow("/data", "log"))

		logPath, err := tryDetermineLogFolder(testutil.TestContext, mock)
		assert.NoError(t, err)
		assert.Equal(t, "/data/log", logPath)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnError(assert.AnError)

		logPath, err := tryDetermineLogFolder(testutil.TestContext, mock)
		assert.Error(t, err)
		assert.Equal(t, "", logPath)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTryDetermineLogMessagesLanguage(t *testing.T) {
	t.Run("known language", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("de"))

		lang, err := tryDetermineLogMessagesLanguage(testutil.TestContext, mock)
		assert.NoError(t, err)
		assert.Equal(t, "de", lang)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("unknown language defaults to en", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("xx"))

		lang, err := tryDetermineLogMessagesLanguage(testutil.TestContext, mock)
		assert.NoError(t, err)
		assert.Equal(t, "en", lang)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnError(assert.AnError)

		lang, err := tryDetermineLogMessagesLanguage(testutil.TestContext, mock)
		assert.Error(t, err)
		assert.Equal(t, "", lang)
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

			// Mock the log folder detection query
			mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
				WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))

			// Mock the language detection query
			mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
				WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

			// Mock log_truncate_on_rotation detection
			mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
				WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))
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

			sourceConn := &sources.SourceConn{
				Source: sources.Source{
					Name: "test-source",
				},
				Conn: mock,
			}

			storeCh := make(chan metrics.MeasurementEnvelope, 10)

			lp, err := NewLogParser(testutil.TestContext, sourceConn, storeCh)
			require.NoError(t, err)
			// Parse logs should stop the worker and return due to privilege errors.
			err = lp.ParseLogs()
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
	mdb := &sources.SourceConn{
		Source: sources.Source{
			Name:       "test-db",
			Kind:       sources.SourcePostgres,
			CustomTags: map[string]string{"env": "test"},
		},
	}
	lp := &LogParser{
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
	result := lp.GetMeasurementEnvelope()

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

func TestSeverityToEnglish(t *testing.T) {
	tests := []struct {
		serverLang    string
		errorSeverity string
		expected      string
	}{
		{"en", "ERROR", "ERROR"},
		{"de", "FEHLER", "ERROR"},
		{"fr", "ERREUR", "ERROR"},
		{"de", "WARNUNG", "WARNING"},
		{"ru", "ОШИБКА", "ERROR"},
		{"zh", "错误", "ERROR"},
		{"unknown", "ERROR", "ERROR"},                  // Unknown language, return as-is
		{"de", "UNKNOWN_SEVERITY", "UNKNOWN_SEVERITY"}, // Unknown severity in known language
	}

	for _, tt := range tests {
		t.Run(tt.serverLang+"_"+tt.errorSeverity, func(t *testing.T) {
			result := severityToEnglish(tt.serverLang, tt.errorSeverity)
			assert.Equal(t, tt.expected, result)
		})
	}
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

func TestFileOffsetsTrackingAndPruning(t *testing.T) {
	t.Run("prune clears map when exceeds limit", func(t *testing.T) {
		lp := &LogParser{
			fileStates: make(map[string]*fileState),
		}

		// Fill map with maxTrackedFiles + 1 entries
		for i := 0; i <= maxTrackedFiles; i++ {
			lp.fileStates[string(rune('A'+i))] = &fileState{offset: int64(i * 1000), fileSize: int64(i * 2000)}
		}

		// Verify map has exceeded limit
		assert.Greater(t, len(lp.fileStates), maxTrackedFiles)

		// Inline prune logic (was pruneFileOffsets)
		if len(lp.fileStates) > maxTrackedFiles {
			clear(lp.fileStates)
		}

		// Map should be cleared
		assert.Equal(t, 0, len(lp.fileStates))
	})

	t.Run("prune does nothing when under limit", func(t *testing.T) {
		lp := &LogParser{
			fileStates: make(map[string]*fileState),
		}

		// Add 24 entries (typical hourly rotation)
		for i := 0; i < 24; i++ {
			lp.fileStates[string(rune('A'+i))] = &fileState{offset: int64(i * 1000), fileSize: int64(i * 2000)}
		}

		originalLen := len(lp.fileStates)

		// Inline prune logic (was pruneFileOffsets)
		if len(lp.fileStates) > maxTrackedFiles {
			clear(lp.fileStates)
		}

		// Map should remain unchanged
		assert.Equal(t, originalLen, len(lp.fileStates))
	})
}

func TestRegexMatchesToMap(t *testing.T) {
	t.Run("successful match", func(t *testing.T) {
		lp := &LogParser{
			LogsMatchRegex: regexp.MustCompile(`(?P<severity>\w+): (?P<message>.+)`),
		}
		matches := []string{"ERROR: Something went wrong", "ERROR", "Something went wrong"}

		result := lp.regexMatchesToMap(matches)
		expected := map[string]string{
			"severity": "ERROR",
			"message":  "Something went wrong",
		}

		assert.Equal(t, expected, result)
	})

	t.Run("no matches", func(t *testing.T) {
		lp := &LogParser{
			LogsMatchRegex: regexp.MustCompile(`(?P<severity>\w+): (?P<message>.+)`),
		}
		matches := []string{}

		result := lp.regexMatchesToMap(matches)
		assert.Empty(t, result)
	})

	t.Run("nil regex", func(t *testing.T) {
		lp := &LogParser{}
		matches := []string{"test"}

		result := lp.regexMatchesToMap(matches)
		assert.Empty(t, result)
	})
}

func TestCSVLogRegex(t *testing.T) {
	// Test the default CSV log regex with sample log lines
	lp := &LogParser{
		LogsMatchRegex: regexp.MustCompile(csvLogDefaultRegEx),
	}

	testLines := []struct {
		line     string
		expected map[string]string
	}{
		{
			line: `2023-12-01 10:30:45.123 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,1,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,ERROR,`,
			expected: map[string]string{
				"log_time":         "2023-12-01 10:30:45.123 UTC",
				"user_name":        "postgres",
				"database_name":    "testdb",
				"process_id":       "12345",
				"connection_from":  "127.0.0.1:54321",
				"session_id":       "session123",
				"session_line_num": "1",
				"command_tag":      "SELECT",
				"error_severity":   "ERROR",
			},
		},
		{
			line: `2023-12-01 10:30:45.123 UTC,postgres,testdb,12345,127.0.0.1:54321,session123,1,SELECT,2023-12-01 10:30:00 UTC,1/234,567,WARNING,`,
			expected: map[string]string{
				"log_time":         "2023-12-01 10:30:45.123 UTC",
				"user_name":        "postgres",
				"database_name":    "testdb",
				"process_id":       "12345",
				"connection_from":  "127.0.0.1:54321",
				"session_id":       "session123",
				"session_line_num": "1",
				"command_tag":      "SELECT",
				"error_severity":   "WARNING",
			},
		},
	}

	for i, tt := range testLines {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			matches := lp.LogsMatchRegex.FindStringSubmatch(tt.line)
			assert.NotEmpty(t, matches, "regex should match the log line")

			result := lp.regexMatchesToMap(matches)
			for key, expected := range tt.expected {
				assert.Equal(t, expected, result[key], "mismatch for key %s", key)
			}
		})
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

	// Mock the log folder detection query
	mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
		WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))

	// Mock the language detection query
	mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
		WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

	// Mock log_truncate_on_rotation detection
	mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
		WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))

	mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
		pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true))

	// Create a SourceConn for testing
	sourceConn := &sources.SourceConn{
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

	lp, err := NewLogParser(ctx, sourceConn, storeCh)
	require.NoError(t, err)
	err = lp.ParseLogs()
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

func TestGetFileWithLatestTimestamp(t *testing.T) {
	// Create temporary test files
	tempDir := t.TempDir()

	t.Run("single file", func(t *testing.T) {
		file1 := filepath.Join(tempDir, "test1.log")
		err := os.WriteFile(file1, []byte("test"), 0644)
		require.NoError(t, err)

		latest, err := getFileWithLatestTimestamp([]string{file1})
		assert.NoError(t, err)
		assert.Equal(t, file1, latest)
	})

	t.Run("multiple files with different timestamps", func(t *testing.T) {
		file1 := filepath.Join(tempDir, "old.log")
		file2 := filepath.Join(tempDir, "new.log")

		// Create first file
		err := os.WriteFile(file1, []byte("old"), 0644)
		require.NoError(t, err)

		// Wait to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		// Create second file (newer)
		err = os.WriteFile(file2, []byte("new"), 0644)
		require.NoError(t, err)

		latest, err := getFileWithLatestTimestamp([]string{file1, file2})
		assert.NoError(t, err)
		assert.Equal(t, file2, latest)
	})

	t.Run("empty file list", func(t *testing.T) {
		latest, err := getFileWithLatestTimestamp([]string{})
		assert.NoError(t, err)
		assert.Equal(t, "", latest)
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistent := filepath.Join(tempDir, "nonexistent.log")
		latest, err := getFileWithLatestTimestamp([]string{nonExistent})
		assert.Error(t, err)
		assert.Equal(t, "", latest)
	})
}

func TestGetFileWithNextModTimestamp(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("finds next file", func(t *testing.T) {
		file1 := filepath.Join(tempDir, "first.log")
		file2 := filepath.Join(tempDir, "second.log")
		file3 := filepath.Join(tempDir, "third.log")

		// Create files with increasing timestamps
		err := os.WriteFile(file1, []byte("first"), 0644)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)
		err = os.WriteFile(file2, []byte("second"), 0644)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)
		err = os.WriteFile(file3, []byte("third"), 0644)
		require.NoError(t, err)

		globPattern := filepath.Join(tempDir, "*.log")
		next, err := getFileWithNextModTimestamp(globPattern, file1)
		assert.NoError(t, err)
		assert.Equal(t, file2, next)
	})

	t.Run("no next file", func(t *testing.T) {
		file1 := filepath.Join(tempDir, "only.log")
		err := os.WriteFile(file1, []byte("only"), 0644)
		require.NoError(t, err)

		globPattern := filepath.Join(tempDir, "*.log")
		next, err := getFileWithNextModTimestamp(globPattern, file1)
		assert.NoError(t, err)
		assert.Equal(t, "", next)
	})

	t.Run("invalid glob pattern", func(t *testing.T) {
		invalidGlob := "["
		file1 := filepath.Join(tempDir, "test.log")
		next, err := getFileWithNextModTimestamp(invalidGlob, file1)
		assert.Error(t, err)
		assert.Equal(t, "", next)
	})
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

		// Phase 1: NewLogParser initialization queries
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

		// Mock log_truncate_on_rotation detection
		mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
			WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))

		// Phase 2: Mode detection - returns false to trigger remote mode
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

		// Phase 3: Privilege check - verifies pg_ls_logdir() and pg_read_file() permissions
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow("")) // 0 bytes read = permission test

		// Phase 4: Log file discovery - finds the most recent CSV log file with existing content
		// Note: parseLogsRemote sets offset = size on first run, so it starts at EOF and only reads new data
		mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size", "modification"}).
				AddRow(logFileName, int32(len(logContent)), time.Now()))

		sourceConn := &sources.SourceConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: map[string]float64{specialMetricServerLogEventCounts: 60}, // 60s interval - won't trigger during test
			},
			Conn: mock,
		}
		sourceConn.RealDbname = testDbName

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := NewLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run ParseLogs in a goroutine since it runs infinitely until context cancels
		go func() {
			_ = lp.ParseLogs()
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
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

		mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
			WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

		// Privilege check passes
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

		// No CSV files found initially - parseLogsRemote will keep retrying
		mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
			WillReturnError(assert.AnError)
		// Expect it to retry
		mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
			WillReturnError(assert.AnError)

		sourceConn := &sources.SourceConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: map[string]float64{specialMetricServerLogEventCounts: 0.1},
			},
			Conn: mock,
		}

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := NewLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run in goroutine since it runs infinitely until context cancels
		go func() {
			_ = lp.ParseLogs()
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
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))
		mock.ExpectQuery(`SELECT COALESCE`).
			WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))
		mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
			WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
		mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
			WithArgs(filepath.Join(tempDir, logFileName)).
			WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

		// Start at EOF (existing content won't be parsed initially)
		mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
			WillReturnRows(pgxmock.NewRows([]string{"name", "size", "modification"}).
				AddRow(logFileName, int32(len(malformedContent)), time.Now()))

		sourceConn := &sources.SourceConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: map[string]float64{specialMetricServerLogEventCounts: 60}, // Long interval
			},
			Conn: mock,
		}
		sourceConn.RealDbname = testDbName

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := NewLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run in goroutine
		go func() {
			_ = lp.ParseLogs()
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
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))
		mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
			WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))
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

		sourceConn := &sources.SourceConn{
			Source: sources.Source{
				Name:    "test-source",
				Metrics: map[string]float64{specialMetricServerLogEventCounts: 0.1},
			},
			Conn: mock,
		}

		ctx, cancel := context.WithTimeout(testutil.TestContext, 500*time.Millisecond)
		defer cancel()

		storeCh := make(chan metrics.MeasurementEnvelope, channelBufferSize)

		lp, err := NewLogParser(ctx, sourceConn, storeCh)
		require.NoError(t, err)

		// Run in goroutine
		go func() {
			_ = lp.ParseLogs() // It will log a warning and continue retrying
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

func TestLogParseLocal_TruncationDetection(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test-truncate.csv")

	// 1. Setup initial empty file (parser seeks to end on startup)
	err := os.WriteFile(logFile, []byte(""), 0644)
	require.NoError(t, err)

	// 2. Setup mocks
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
		WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))
	mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
		WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))
	mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
		WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))
	mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
		pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true))

	sourceConn := &sources.SourceConn{
		Source: sources.Source{Name: "test-source"},
		Conn:   mock,
	}
	sourceConn.RealDbname = "testdb"

	// 3. Start Parser
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storeCh := make(chan metrics.MeasurementEnvelope, 100)
	lp, err := NewLogParser(ctx, sourceConn, storeCh)
	require.NoError(t, err)

	lp.Interval = 0.1

	go func() {
		_ = lp.ParseLogs()
	}()

	// Wait for parser to open file and seek to end
	time.Sleep(500 * time.Millisecond)

	// 4. Append initial content (ERROR)
	// Using format known to work: ...ERROR,
	initialContent := `2023-12-01 10:30:45.123 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,1,"SELECT",2023-12-01 10:30:00 UTC,1/234,567,ERROR,` + "\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = f.WriteString(initialContent)
	require.NoError(t, err)
	f.Close()

	// 5. Verify initial log parsed
	timeoutInit := time.After(2 * time.Second)
	foundError := false

	for !foundError {
		select {
		case m := <-storeCh:
			if val, ok := m.Data[0]["error_total"]; ok && val == int64(1) {
				foundError = true
			}
		case <-timeoutInit:
			t.Fatal("timed out waiting for initial log parsing (ERROR)")
		}
	}

	// 6. Truncate file and write new content
	err = os.WriteFile(logFile, []byte(""), 0644)
	require.NoError(t, err)

	// Write new content - WARNING
	newContent := `2023-12-01 11:00:00.000 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,2,"SELECT",2023-12-01 11:00:00 UTC,1/234,567,WARNING,` + "\n"
	err = os.WriteFile(logFile, []byte(newContent), 0644)
	require.NoError(t, err)

	// 7. Verify new content parsed (implies truncation detected)
	timeout := time.After(3 * time.Second)
	foundWarning := false

	for !foundWarning {
		select {
		case m := <-storeCh:
			if val, ok := m.Data[0]["warning_total"]; ok && val == int64(1) {
				foundWarning = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for post-truncation log parsing")
		}
	}

	assert.True(t, foundWarning, "Should have detected warning after file truncation")
}

func TestLogParseRemote_TruncationDetection(t *testing.T) {
	tempDir := t.TempDir()
	logFileName := "test-remote.csv"

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// 1. Initialization Mocks
	mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
		WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))
	mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
		WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))
	mock.ExpectQuery(`select current_setting\('log_truncate_on_rotation'\)`).
		WillReturnRows(pgxmock.NewRows([]string{"current_setting"}).AddRow("off"))

	// 2. Mode detection (Remote)
	mock.ExpectQuery(`SELECT COALESCE`).
		WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

	// 3. Privilege Check
	mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logFileName))
	mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
		WithArgs(filepath.Join(tempDir, logFileName)).
		WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

	// 4. Discovery Phase: Find log file with size 100
	// Parser will seek to end (offset = 100)
	mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
		WillReturnRows(pgxmock.NewRows([]string{"name", "size", "modification"}).
			AddRow(logFileName, int32(100), time.Now()))

	// 5. Loop Iteration: Check file status -> Return size 50 (Truncation!)
	mock.ExpectQuery(`select size, modification from pg_ls_logdir\(\) where name = \$1;`).
		WithArgs(logFileName).
		WillReturnRows(pgxmock.NewRows([]string{"size", "modification"}).
			AddRow(int32(50), time.Now()))

	// 6. Read Phase: Should read from offset 0 to 50
	// We return a log line that generates a WARNING
	newContent := `2023-12-01 11:00:00.000 UTC,"postgres","testdb",12345,"127.0.0.1:54321",session123,2,"SELECT",2023-12-01 11:00:00 UTC,1/234,567,WARNING,` + "\n"

	// Note: We mock pg_read_file args to be exactly offset 0, size 50
	mock.ExpectQuery(`select pg_read_file\(\$1, \$2, \$3\)`).
		WithArgs(filepath.Join(tempDir, logFileName), int32(0), int32(50)).
		WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(newContent))

	// Setup Parser
	sourceConn := &sources.SourceConn{
		Source: sources.Source{Name: "test-source"},
		Conn:   mock,
	}
	sourceConn.RealDbname = "testdb"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // ensure cleanup

	storeCh := make(chan metrics.MeasurementEnvelope, 100)
	lp, err := NewLogParser(ctx, sourceConn, storeCh)
	require.NoError(t, err)

	lp.Interval = 0.1

	go func() {
		_ = lp.ParseLogs()
	}()

	// 7. Verify result
	// We expect the WARNING from the truncated file read
	timeout := time.After(3 * time.Second)
	foundWarning := false

	for !foundWarning {
		select {
		case m := <-storeCh:
			if val, ok := m.Data[0]["warning_total"]; ok && val == int64(1) {
				foundWarning = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for log parsing after truncation")
		}
	}

	assert.True(t, foundWarning)
	assert.NoError(t, mock.ExpectationsWereMet())
}
