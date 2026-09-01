package reaper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stderr end to end.
//
// This is the case that used to fail at construction:
//
//	log_destination must contain 'csvlog' for log parsing to work
//
// stderr is PostgreSQL's default, so before this migration pgwatch's log metric
// did not work on an unconfigured server at all. The test is end to end rather
// than a check of parserFormat because that error came from NewLogParser and
// the counting is what actually has to work.

const stderrLinePrefix = `%m [%p] %u@%d `

func TestStderrDestinationProducesCounts(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "postgresql.log")

	// Created empty. Offsets seed to end-of-file on first sight, so content
	// written BEFORE the parser starts is deliberately not counted; writing
	// after it starts is what a real server does anyway.
	require.NoError(t, os.WriteFile(logFile, nil, 0o600))

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(expectedSettingsQuery).
		WillReturnRows(pgxmock.NewRows([]string{"is_enabled", "csvlog_dest", "jsonlog_dest", "log_trunc", "log_dir", "lc_messages", "line_prefix"}).
			AddRow(true, false, false, false, dir, "en", stderrLinePrefix))
	mock.ExpectQuery(`SELECT COALESCE`).
		WillReturnRows(pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true))

	src := &sources.DbConn{
		Source: sources.Source{
			Name:    "test-source",
			Metrics: metrics.MetricIntervals{specialMetricServerLogEventCounts: 1},
		},
		Conn: mock,
	}
	src.RealDbname = "testdb"

	ctx, cancel := context.WithTimeout(testutil.TestContext, 20*time.Second)
	defer cancel()

	storeCh := make(chan metrics.MeasurementEnvelope, 32)
	lp, err := NewLogParser(ctx, src, storeCh)
	require.NoError(t, err, "stderr must no longer be rejected at construction")

	go func() { _ = lp.ParseLogs() }()
	time.Sleep(300 * time.Millisecond) // let the follower reach the file

	// Three records for testdb and one for otherdb, then a sentinel.
	//
	// The sentinel is not decoration. A stderr record ends where the NEXT
	// record begins -- DETAIL, HINT and STATEMENT lines belong to the
	// record above them, so the parser cannot know a record is complete
	// until it sees the line after it. The last record written is
	// therefore always pending, and on a live server that is invisible
	// because more log arrives. In a test it is the difference between an
	// assertion and a flake, so a PANIC is written last and never asserted
	// on: it flushes the LOG and stays pending itself.
	appendLines(t, logFile,
		`2023-12-01 10:30:45.123 UTC [12345] postgres@testdb ERROR:  duplicate key value violates unique constraint`,
		`2023-12-01 10:30:46.124 UTC [12345] postgres@testdb WARNING:  this is a warning message`,
		`2023-12-01 10:30:47.125 UTC [12346] postgres@otherdb ERROR:  another error message`,
		`2023-12-01 10:30:48.126 UTC [12347] postgres@testdb LOG:  checkpoint starting`,
		`2023-12-01 10:30:49.127 UTC [12348] postgres@otherdb PANIC:  sentinel, never counted`,
	)

	// Counts are zeroed on every send, so an interval boundary can fall in
	// the middle of the batch. Accumulating across envelopes is both what
	// the sinks do and what makes the assertion independent of timing.
	got := awaitCounts(t, ctx, storeCh, func(sum map[string]int64) bool {
		return sum["error_total"] >= 2 && sum["log_total"] >= 1
	})

	assert.Equal(t, int64(1), got["error"], "one ERROR in testdb")
	assert.Equal(t, int64(1), got["warning"], "one WARNING in testdb")
	assert.Equal(t, int64(1), got["log"], "one LOG in testdb")
	assert.Equal(t, int64(2), got["error_total"], "two ERRORs across the instance")
	assert.Equal(t, int64(1), got["warning_total"])
	assert.Equal(t, int64(0), got["panic_total"], "the sentinel is still pending")
}

// appendLines writes the way a server does: opened for append, newline
// terminated.
func appendLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test fixture
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	for _, l := range lines {
		_, err = f.WriteString(l + "\n")
		require.NoError(t, err)
	}
}

// awaitCounts sums envelopes until the running total satisfies want.
func awaitCounts(t *testing.T, ctx context.Context, ch <-chan metrics.MeasurementEnvelope, want func(map[string]int64) bool) map[string]int64 {
	t.Helper()
	sum := make(map[string]int64)
	deadline := time.After(15 * time.Second)
	for {
		select {
		case env := <-ch:
			require.Len(t, env.Data, 1)
			for k, v := range env.Data[0] {
				if n, ok := v.(int64); ok && k != metrics.EpochColumnName {
					sum[k] += n
				}
			}
			if want(sum) {
				return sum
			}
		case <-ctx.Done():
			t.Fatalf("context ended before the expected counts arrived; got %v", sum)
			return nil
		case <-deadline:
			t.Fatalf("timed out waiting for the expected counts; got %v", sum)
			return nil
		}
	}
}
