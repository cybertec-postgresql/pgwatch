package reaper

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pglogwatch"
	"github.com/cybertec-postgresql/pglogwatch/pgremote"
	"github.com/cybertec-postgresql/pgwatch/v7/internal/testutil"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Byte-offset resumption.

func TestOffsetsStartAtEndOfAnExistingFile(t *testing.T) {
	sizes := map[string]int64{"/logs/postgresql.csv": 4096}
	o := newEndSeededOffsets(func(p string) (int64, bool) {
		s, ok := sizes[p]
		return s, ok
	})

	// A file with content this process did not write starts at its end.
	// Counting it would report months of retained logs as one interval.
	off, ok := o.Get("/logs/postgresql.csv")
	assert.True(t, ok)
	assert.Equal(t, int64(4096), off)

	// The seeded offset sticks even if the file grows, so the second look
	// does not skip what arrived in between.
	sizes["/logs/postgresql.csv"] = 9000
	off, ok = o.Get("/logs/postgresql.csv")
	assert.True(t, ok)
	assert.Equal(t, int64(4096), off, "the seed is taken once, not re-taken")
}

func TestOffsetsStartAtZeroForAFileThatDoesNotExistYet(t *testing.T) {
	o := newEndSeededOffsets(func(string) (int64, bool) { return 0, false })

	// A file created after pgwatch started is read from its first byte:
	// everything in it happened on this process's watch.
	off, ok := o.Get("/logs/created-later.csv")
	assert.False(t, ok)
	assert.Zero(t, off)
}

func TestOffsetsRoundTripAByteOffset(t *testing.T) {
	o := newEndSeededOffsets(func(string) (int64, bool) { return 0, false })
	o.Set("/logs/a.csv", 1234)

	off, ok := o.Get("/logs/a.csv")
	require.True(t, ok)
	assert.Equal(t, int64(1234), off)
}

// The bound must not evict the file currently being read.
//
// The pre-migration code bounded its map by clearing ALL of it at the limit,
// which discarded the active file's offset too. That file would then be
// re-seeded to its current end and every record written since would be
// skipped -- a silent gap in the counts, triggered only on a server with
// thousands of rotated logs.
func TestOffsetBoundEvictsTheOldestNotTheActiveFile(t *testing.T) {
	o := newEndSeededOffsets(func(string) (int64, bool) { return 0, false })

	active := "/logs/active.csv"
	o.Set(active, 100)

	// Push the map well past its bound with files that are never read
	// again, touching the active file as we go, as a real run would.
	for i := range maxTrackedFiles * 2 {
		o.Set(fmt.Sprintf("/logs/rotated-%d.csv", i), int64(i))
		o.Set(active, int64(200+i))
	}

	o.mu.Lock()
	size := len(o.seen)
	o.mu.Unlock()
	assert.LessOrEqual(t, size, maxTrackedFiles, "the map stays bounded")

	off, ok := o.Get(active)
	assert.True(t, ok, "the active file must survive eviction")
	assert.Equal(t, int64(200+maxTrackedFiles*2-1), off)

	// The oldest rotated file is the one that went.
	_, ok = o.Get("/logs/rotated-0.csv")
	assert.False(t, ok, "the least recently used entry is evicted")
}

// Restart resumption.
//
// The property is not "resumption works" but the two failures it can have:
// counting a record twice, and skipping one. Both are silent -- a duplicated
// ERROR and a dropped ERROR look like a busy server and a quiet one -- so the
// test asserts exact counts across a simulated restart rather than that the
// second pass found "some" records.
//
// jsonlog is used because a jsonlog record is complete at its newline. A
// stderr record is not: it ends where the next one begins, so the last record
// written is always pending and every assertion needs a sentinel
// (see TestStderrDestinationProducesCounts). That is a property of the format,
// not of resumption, and it does not belong in this test.

// jsonRecord renders one NDJSON log line.
func jsonRecord(i int, db, severity string) string {
	return fmt.Sprintf(
		`{"timestamp":"2023-12-01 10:%02d:00.000 UTC","dbname":%q,"error_severity":%q,"message":"record %d"}`,
		i, db, severity, i)
}

// newResumeParser builds a LogParser wired to a directory, without a database.
//
// Everything NewLogParser gets from the server is supplied directly here: the
// point is to restart the parser over one unchanged directory, and a mocked
// settings query per restart would add nothing but noise.
func newResumeParser(ctx context.Context, t *testing.T, dir string, offsets *endSeededOffsets) (*LogParser, chan metrics.MeasurementEnvelope) {
	t.Helper()
	ch := make(chan metrics.MeasurementEnvelope, 32)
	return &LogParser{
		ctx: ctx,
		LogConfig: &LogConfig{
			CollectorEnabled: true,
			JSONDestination:  true,
			Directory:        dir,
		},
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "resume-test"}},
		realDbname:       "testdb",
		Interval:         time.Second,
		StoreCh:          ch,
		eventCounts:      make(map[string]int64),
		eventCountsTotal: make(map[string]int64),
		offsets:          offsets,
	}, ch
}

// readAvailable consumes everything currently in the directory and returns the
// per-instance counts, without sending anything.
//
// Follow is off: this is one pass over what exists, which is what a restart
// does before it catches up.
func readAvailable(t *testing.T, lp *LogParser) map[string]int64 {
	t.Helper()
	fs := &pglogwatch.FileSet{
		Dir:     lp.Directory,
		Format:  lp.parserFormat(),
		Follow:  false,
		Offsets: lp.offsets,
	}
	rc, err := fs.Open(lp.ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, rc.Close()) }()

	require.NoError(t, lp.consume(rc))

	lp.countsMu.Lock()
	defer lp.countsMu.Unlock()
	out := make(map[string]int64, len(lp.eventCountsTotal))
	for k, v := range lp.eventCountsTotal {
		out[k] = v
	}
	return out
}

func TestRestartCountsNothingTwiceAndSkipsNothing(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "postgresql.json")

	offsets := newEndSeededOffsets(localFileSize)

	require.NoError(t, os.WriteFile(logFile, nil, 0o600))
	// A stored offset of zero, which is what a pgwatch that was already
	// running when this file was created would hold. Without it the store
	// seeds to end-of-file on first sight -- correct in production, and it
	// would make this test measure the seeding rather than the resumption.
	offsets.Set(logFile, 0)
	appendLines(t, logFile,
		jsonRecord(1, "testdb", "ERROR"),
		jsonRecord(2, "testdb", "WARNING"),
		jsonRecord(3, "otherdb", "ERROR"),
	)

	ctx, cancel := context.WithTimeout(testutil.TestContext, 30*time.Second)
	defer cancel()

	lp1, _ := newResumeParser(ctx, t, dir, offsets)
	first := readAvailable(t, lp1)
	assert.Equal(t, int64(2), first["ERROR"])
	assert.Equal(t, int64(1), first["WARNING"])

	resumeFrom, ok := offsets.Get(logFile)
	require.True(t, ok, "the first pass must have recorded an offset")

	// Restart: a new LogParser with fresh counters, over the same
	// directory and the same offsets. Nothing was appended in between, so
	// a parser that re-reads from the start would count all three again.
	lp2, _ := newResumeParser(ctx, t, dir, offsets)
	second := readAvailable(t, lp2)
	assert.Empty(t, second, "a restart with no new records must count nothing")

	// Now append, and restart again. Exactly the new records, and only
	// once each: a parser that resumed too early would recount record 3,
	// one that resumed too late would miss record 4.
	appendLines(t, logFile,
		jsonRecord(4, "testdb", "FATAL"),
		jsonRecord(5, "testdb", "ERROR"),
	)

	lp3, _ := newResumeParser(ctx, t, dir, offsets)
	third := readAvailable(t, lp3)
	assert.Equal(t, int64(1), third["FATAL"], "record 4 must be counted exactly once")
	assert.Equal(t, int64(1), third["ERROR"], "record 5, and not record 1 or 3 again")
	assert.Zero(t, third["WARNING"], "record 2 must not be counted a second time")

	// The whole file was read exactly once across the three passes.
	final, ok := offsets.Get(logFile)
	require.True(t, ok)
	size, ok := localFileSize(logFile)
	require.True(t, ok)
	assert.Equal(t, size, final, "the offset ends at end-of-file: nothing skipped")
	assert.Greater(t, final, resumeFrom, "and it advanced over the appended records")
}

// Resumption must SEEK rather than re-read and discard.
//
// The pre-migration parser resumed by counting lines and skipping them one
// ReadString at a time, so resuming N lines into a file meant reading N lines
// to get there -- every restart paying again for everything already parsed.
// The count assertions above cannot tell the difference between seeking and
// re-reading-then-discarding; the bytes handed to the parser can.
func TestResumeReadsOnlyTheNewBytes(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "postgresql.json")
	offsets := newEndSeededOffsets(localFileSize)

	require.NoError(t, os.WriteFile(logFile, nil, 0o600))
	offsets.Set(logFile, 0) // as above: read from the start, not from the end
	var bulk []string
	for i := range 200 {
		bulk = append(bulk, jsonRecord(i%60, "testdb", "LOG"))
	}
	appendLines(t, logFile, bulk...)

	ctx, cancel := context.WithTimeout(testutil.TestContext, 30*time.Second)
	defer cancel()

	lp1, _ := newResumeParser(ctx, t, dir, offsets)
	require.Equal(t, int64(200), readAvailable(t, lp1)["LOG"])

	bulkSize, ok := localFileSize(logFile)
	require.True(t, ok)

	appendLines(t, logFile, jsonRecord(59, "testdb", "PANIC"))
	newSize, ok := localFileSize(logFile)
	require.True(t, ok)
	appended := newSize - bulkSize

	// Count the bytes the parser is actually given on the second pass.
	lp2, _ := newResumeParser(ctx, t, dir, offsets)
	fs := &pglogwatch.FileSet{Dir: dir, Format: pglogwatch.FormatJSON, Offsets: offsets}
	rc, err := fs.Open(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, rc.Close()) }()

	counted := &countingReader{r: rc}
	require.NoError(t, lp2.consume(counted))

	assert.Equal(t, int64(1), lp2.eventCountsTotal["PANIC"])
	assert.Equal(t, appended, counted.n,
		"the resumed pass reads the appended bytes and not the 200 records before them")
}

// countingReader records how many bytes were read through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

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

	got := awaitCounts(ctx, t, storeCh, func(sum map[string]int64) bool {
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
	got := awaitCounts(ctx, t, storeCh, func(sum map[string]int64) bool {
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
func awaitCounts(ctx context.Context, t *testing.T, ch <-chan metrics.MeasurementEnvelope, want func(map[string]int64) bool) map[string]int64 {
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
