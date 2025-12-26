package logparse

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	ctx, cancel := context.WithTimeout(testCtx, 2*time.Second)
	defer cancel()

	// Create a channel to receive measurement envelopes
	storeCh := make(chan metrics.MeasurementEnvelope, 10)

	lp, err := NewLogParser(ctx, sourceConn, "testdb", 0, storeCh)
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
	case <-time.After(2*time.Second):
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
