package metrics

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCtx = context.Background()

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

func TestEventCountsToMetricStoreMessages(t *testing.T) {
	mdb := &sources.SourceConn{
		Source: sources.Source{
			Name:       "test-db",
			Kind:       sources.SourcePostgres,
			CustomTags: map[string]string{"env": "test"},
		},
	}

	eventCounts := map[string]int64{
		"ERROR":   5,
		"WARNING": 10,
	}

	eventCountsTotal := map[string]int64{
		"ERROR":   15,
		"WARNING": 25,
		"INFO":    50,
	}

	result := eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal, mdb)

	assert.Equal(t, "test-db", result.DBName)
	assert.Equal(t, string(sources.SourcePostgres), result.SourceType)
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

	// Check that all PgSeverities are zeroed
	for _, severity := range PgSeverities {
		assert.Equal(t, int64(0), eventCounts[severity])
	}
}

func TestTryDetermineLogFolder(t *testing.T) {
	t.Run("absolute log directory", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).
				AddRow("/data", "/var/log/postgresql"))

		logPath, err := tryDetermineLogFolder(testCtx, mock)
		assert.NoError(t, err)
		assert.Equal(t, "/var/log/postgresql/*.csv", logPath)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("relative log directory", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).
				AddRow("/data", "log"))

		logPath, err := tryDetermineLogFolder(testCtx, mock)
		assert.NoError(t, err)
		assert.Equal(t, "/data/log/*.csv", logPath)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnError(assert.AnError)

		logPath, err := tryDetermineLogFolder(testCtx, mock)
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

		lang, err := tryDetermineLogMessagesLanguage(testCtx, mock)
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

		lang, err := tryDetermineLogMessagesLanguage(testCtx, mock)
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

		lang, err := tryDetermineLogMessagesLanguage(testCtx, mock)
		assert.Error(t, err)
		assert.Equal(t, "", lang)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRegexMatchesToMap(t *testing.T) {
	t.Run("successful match", func(t *testing.T) {
		regex := regexp.MustCompile(`(?P<severity>\w+): (?P<message>.+)`)
		matches := []string{"ERROR: Something went wrong", "ERROR", "Something went wrong"}

		result := regexMatchesToMap(regex, matches)
		expected := map[string]string{
			"severity": "ERROR",
			"message":  "Something went wrong",
		}

		assert.Equal(t, expected, result)
	})

	t.Run("no matches", func(t *testing.T) {
		regex := regexp.MustCompile(`(?P<severity>\w+): (?P<message>.+)`)
		matches := []string{}

		result := regexMatchesToMap(regex, matches)
		assert.Empty(t, result)
	})

	t.Run("nil regex", func(t *testing.T) {
		matches := []string{"test"}

		result := regexMatchesToMap(nil, matches)
		assert.Empty(t, result)
	})
}

func TestCSVLogRegex(t *testing.T) {
	// Test the default CSV log regex with sample log lines
	regex, err := regexp.Compile(CSVLogDefaultRegEx)
	require.NoError(t, err)

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
			matches := regex.FindStringSubmatch(tt.line)
			assert.NotEmpty(t, matches, "regex should match the log line")

			result := regexMatchesToMap(regex, matches)
			for key, expected := range tt.expected {
				assert.Equal(t, expected, result[key], "mismatch for key %s", key)
			}
		})
	}
}

// Integration test that creates actual log files and tests the ParseLogs function
func TestLogParse(t *testing.T) {
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

	// pretend we're connected via UNIX socket
	mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
		pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true))
	// Mock the language detection query
	mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
		WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

	// Create a SourceConn for testing
	sourceConn := &sources.SourceConn{
		Source: sources.Source{
			Name: "test-source",
			HostConfig: sources.HostConfigAttrs{
				LogsGlobPath: filepath.Join(tempDir, "*.csv"),
				// Use default regex by leaving LogsMatchRegex empty
			},
		},
		Conn: mock,
	}

	// Create a context with timeout to prevent test from hanging
	ctx, cancel := context.WithTimeout(testCtx, 3*time.Second)
	defer cancel()

	// Create a channel to receive measurement envelopes
	storeCh := make(chan MeasurementEnvelope, 10)

	// Use a short interval for testing (0.5 seconds)
	ParseLogs(ctx, sourceConn, "testdb", 0.5, storeCh)

	// Wait for measurements to be sent or timeout
	var measurement MeasurementEnvelope
	select {
	case measurement = <-storeCh:
		assert.NotEmpty(t, measurement.Data, "Measurement data should not be empty")
	case <-time.After(2 * time.Second):
		break
	}

	assert.Equal(t, "test-source", measurement.DBName)
	assert.Equal(t, specialMetricServerLogEventCounts, measurement.MetricName)

	// Verify the data contains expected fields for both local and total counts
	data := measurement.Data[0]
	// Check that severity counts are present
	_, hasError := data["error"]
	_, hasWarning := data["warning"]
	assert.True(t, hasError && hasWarning, "Should have at least error and warning")

	// Ensure mock expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}
