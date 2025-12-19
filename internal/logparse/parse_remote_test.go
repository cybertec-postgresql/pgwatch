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

func TestLogParseRemote(t *testing.T) {
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
		pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

	mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("log.csv"))
	mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
		WithArgs(filepath.Join(tempDir, "log.csv")).
		WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow("dummy data"))

	mock.ExpectQuery(`select name, size, modification from pg_ls_logdir\(\) where name like '%csv' order by modification desc limit 1;`).
		WillReturnRows(pgxmock.NewRows([]string{"name", "size", "modification"}).AddRow("test.csv", 0, time.Now()))
	mock.ExpectQuery(`select size, modification from pg_ls_logdir\(\) where name = \$1;`).
		WithArgs("test.csv").
		WillReturnRows(pgxmock.NewRows([]string{"size", "modification"}).AddRow(len(logContent), time.Now()))
	mock.ExpectQuery(`select pg_read_file\(\$1, \$2, \$3\)`).
		WithArgs(logFile, int32(0), int32(len(logContent))).
		WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(logContent))

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
	//
	// Also ensures that the mode detection logic works correctly,
	// because different modes has different query expectations.
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

	// remote mode being entirely controlled by pgxmock
	// gave us flexbility to test more details
	assert.Equal(t, int64(1), data["error"])
	assert.Equal(t, int64(1), data["error_total"])
	measurement = <-storeCh
	assert.Equal(t, int64(1), measurement.Data[0]["warning"])
	assert.Equal(t, int64(1), measurement.Data[0]["warning_total"])
	measurement = <-storeCh
	assert.Equal(t, int64(1), measurement.Data[0]["error_total"])
}
