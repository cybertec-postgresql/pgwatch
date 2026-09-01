package reaper

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pglogwatch"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
func newResumeParser(t *testing.T, ctx context.Context, dir string, offsets *endSeededOffsets) (*LogParser, chan metrics.MeasurementEnvelope) {
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

	lp1, _ := newResumeParser(t, ctx, dir, offsets)
	first := readAvailable(t, lp1)
	assert.Equal(t, int64(2), first["ERROR"])
	assert.Equal(t, int64(1), first["WARNING"])

	resumeFrom, ok := offsets.Get(logFile)
	require.True(t, ok, "the first pass must have recorded an offset")

	// Restart: a new LogParser with fresh counters, over the same
	// directory and the same offsets. Nothing was appended in between, so
	// a parser that re-reads from the start would count all three again.
	lp2, _ := newResumeParser(t, ctx, dir, offsets)
	second := readAvailable(t, lp2)
	assert.Empty(t, second, "a restart with no new records must count nothing")

	// Now append, and restart again. Exactly the new records, and only
	// once each: a parser that resumed too early would recount record 3,
	// one that resumed too late would miss record 4.
	appendLines(t, logFile,
		jsonRecord(4, "testdb", "FATAL"),
		jsonRecord(5, "testdb", "ERROR"),
	)

	lp3, _ := newResumeParser(t, ctx, dir, offsets)
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

	lp1, _ := newResumeParser(t, ctx, dir, offsets)
	require.Equal(t, int64(200), readAvailable(t, lp1)["LOG"])

	bulkSize, ok := localFileSize(logFile)
	require.True(t, ok)

	appendLines(t, logFile, jsonRecord(59, "testdb", "PANIC"))
	newSize, ok := localFileSize(logFile)
	require.True(t, ok)
	appended := newSize - bulkSize

	// Count the bytes the parser is actually given on the second pass.
	lp2, _ := newResumeParser(t, ctx, dir, offsets)
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
