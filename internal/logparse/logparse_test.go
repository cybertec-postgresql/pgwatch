package logparse

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCtx = context.Background()

func TestNewLogParser(t *testing.T) {
	tempDir := t.TempDir()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	sourceConn := &sources.SourceConn{
		Source: sources.Source{
			Name: "test-source",
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

		lp, err := NewLogParser(testCtx, sourceConn, "testdb", 60.0, storeCh)
		assert.NoError(t, err)
		assert.NotNil(t, lp)
		assert.Equal(t, tempDir, lp.LogFolder)
		assert.Equal(t, "en", lp.ServerMessagesLang)
		assert.Equal(t, "testdb", lp.RealDbname)
		assert.Equal(t, 60.0, lp.Interval)
		assert.NotNil(t, lp.LogsMatchRegex)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("tryDetermineLogFolder error", func(t *testing.T) {
		// Mock log folder detection to fail
		mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
			WillReturnError(assert.AnError)

		lp, err := NewLogParser(testCtx, sourceConn, "testdb", 60.0, storeCh)
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

		lp, err := NewLogParser(testCtx, sourceConn, "testdb", 60.0, storeCh)
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

		lp, err := NewLogParser(testCtx, sourceConn, "testdb", 60.0, storeCh)
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

		lp, err := NewLogParser(testCtx, sourceConn, "testdb", 60.0, storeCh)
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

		logPath, err := tryDetermineLogFolder(testCtx, mock)
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

		logPath, err := tryDetermineLogFolder(testCtx, mock)
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

func TestCheckHasPrivileges(t *testing.T) {
	tempDir := t.TempDir()

	names := [2]string{"pg_ls_logdir() fails", "pg_read_file() permission denied"}
	for _, name := range names {
		t.Run("checkHasPrivileges fails - "+name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			// Mock the log folder detection query
			mock.ExpectQuery(`select current_setting\('data_directory'\) as dd, current_setting\('log_directory'\) as ld`).
				WillReturnRows(pgxmock.NewRows([]string{"dd", "ld"}).AddRow("", tempDir))

			// Mock the language detection query
			mock.ExpectQuery(`select current_setting\('lc_messages'\)::varchar\(2\) as lc_messages;`).
				WillReturnRows(pgxmock.NewRows([]string{"lc_messages"}).AddRow("en"))

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

			lp, err := NewLogParser(testCtx, sourceConn, "testdb", 0, storeCh)
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
