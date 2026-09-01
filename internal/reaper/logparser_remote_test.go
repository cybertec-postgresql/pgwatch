package reaper

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pglogwatch/pgremote"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The remote path, through pgremote.
//
// "Remote" means the log files are on the database server and pgwatch cannot
// open them, so it reads them through pg_read_file over the same connection it
// collects metrics on. pgxmock stands in for that connection, which makes the
// SQL pgwatch issues part of the test rather than something only a live server
// would reveal.
//
// It also covers jsonlog remotely, which is two new things at once: before
// this migration the remote path read csvlog through a regex, and jsonlog was
// rejected at construction.

const remoteLogDir = "/var/lib/postgresql/data/log"

func TestRemotePathCountsThroughPgRemote(t *testing.T) {
	logName := "postgresql.json"
	remotePath := path.Join(remoteLogDir, logName)

	content := strings.Join([]string{
		jsonRecord(1, "testdb", "ERROR"),
		jsonRecord(2, "testdb", "WARNING"),
		jsonRecord(3, "otherdb", "ERROR"),
		jsonRecord(4, "testdb", "LOG"),
	}, "\n") + "\n"

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(expectedSettingsQuery).
		WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
			AddRow(true, false, true, false, remoteLogDir, "en", defaultLinePrefix))

	// false: the client is NOT on the same host, which is what sends
	// pgwatch down the remote path.
	mock.ExpectQuery(`SELECT COALESCE`).
		WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false))

	// The privilege check: pgwatch must be able to list the directory and
	// read a file before it commits to this path.
	mock.ExpectQuery(`select name from pg_ls_logdir\(\) limit 1`).
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow(logName))
	mock.ExpectQuery(`select pg_read_file\(\$1, 0, 0\)`).
		WithArgs(filepath.Join(remoteLogDir, logName)).
		WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(""))

	// Offset seeding. An empty result means pgwatch has not seen this file
	// before and cannot seed it to an end, so it reads from byte zero --
	// a log file created after pgwatch started, which is when the remote
	// path has anything to count.
	mock.ExpectQuery(`select name, size from pg_ls_logdir\(\)`).
		WillReturnRows(pgxmock.NewRows([]string{"name", "size"}))

	// pgremote lists the directory itself, then reads the file in chunks.
	mock.ExpectQuery(`SELECT name, size FROM pg_ls_logdir\(\) ORDER BY name`).
		WillReturnRows(pgxmock.NewRows([]string{"name", "size"}).
			AddRow(logName, int64(len(content))))
	mock.ExpectQuery(`SELECT pg_read_file\(\$1, \$2, \$3\)`).
		WithArgs(remotePath, int64(0), int64(maxChunkSize)).
		WillReturnRows(pgxmock.NewRows([]string{"pg_read_file"}).AddRow(content))

	src := &sources.DbConn{
		Source: sources.Source{
			Name:    "remote-source",
			Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 1},
		},
		Conn: mock,
	}
	src.RealDbname = "testdb"

	ctx, cancel := context.WithTimeout(testutil.TestContext, 20*time.Second)
	defer cancel()

	storeCh := make(chan metrics.MeasurementEnvelope, 32)
	lp, err := NewLogParser(ctx, src, storeCh)
	require.NoError(t, err)

	go func() { _ = lp.ParseLogs() }()

	got := awaitCounts(t, ctx, storeCh, func(sum map[string]int64) bool {
		return sum["error_total"] >= 2 && sum["log_total"] >= 1
	})

	assert.Equal(t, int64(1), got["error"], "one ERROR in testdb")
	assert.Equal(t, int64(1), got["warning"])
	assert.Equal(t, int64(1), got["log"])
	assert.Equal(t, int64(2), got["error_total"], "both databases' ERRORs")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The glob is what stops a server writing two destinations being counted twice.
//
// pg_ls_logdir lists everything in the directory, so a server set to
// "stderr,jsonlog" has two complete copies of its log sitting there. Reading
// both would double every count -- and it would look like a busy server, not
// like a bug.
func TestRemoteGlobSelectsOneDestination(t *testing.T) {
	for _, tc := range []struct {
		name       string
		csv, jsonl bool
		wantGlob   string
		wantKept   []string
	}{
		{"csvlog", true, false, "*.csv", []string{"postgresql.csv"}},
		{"jsonlog", false, true, "*.json", []string{"postgresql.json"}},
		{"stderr", false, false, "*.log", []string{"postgresql.log"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lp := &LogParser{LogConfig: &LogConfig{
				CSVDestination:  tc.csv,
				JSONDestination: tc.jsonl,
			}}
			assert.Equal(t, tc.wantGlob, lp.remoteGlob())

			// Every destination's files are present, as they would be
			// on a server logging to more than one.
			var kept []string
			for _, name := range []string{"postgresql.csv", "postgresql.json", "postgresql.log"} {
				if ok, err := path.Match(lp.remoteGlob(), name); err == nil && ok {
					kept = append(kept, name)
				}
			}
			assert.Equal(t, tc.wantKept, kept)
		})
	}
}

// A directory that cannot be listed is an error, not an empty log.
//
// Reporting zero events for a permissions problem is the failure that hides
// itself: the dashboard shows a quiet server rather than a broken collector.
func TestRemoteListFailureIsReported(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT name, size FROM pg_ls_logdir\(\) ORDER BY name`).
		WillReturnError(assert.AnError)

	_, err = pgremote.Open(testutil.TestContext, mock, pgremote.Config{Dir: remoteLogDir})
	require.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
