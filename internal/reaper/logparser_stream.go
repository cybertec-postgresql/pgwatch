package reaper

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pglogwatch"
	"github.com/cybertec-postgresql/pglogwatch/pgremote"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/log"
)

// The pglogwatch-backed parsing engine.
//
// pgwatch used to carry two log parsers -- one for local files, one for
// pg_read_file -- each with its own line splitting, rotation handling and
// resumption logic, both driven by a regex over csvlog. Both are replaced by
// one loop over pglogwatch.Parser; the only thing that differs between local
// and remote is which io.Reader it is given.
//
// What pgwatch keeps is what is actually pgwatch's: resolving the GUCs,
// deciding local versus remote, counting per database and per instance, and
// the envelope shape the sinks and dashboards expect.

// counting is what this whole file exists to do, and it is worth stating
// plainly because the rest is plumbing:
//
//	per-database counts go to eventCounts   -- only records whose Database
//	                                           matches the source's dbname
//	per-instance counts go to eventCountsTotal -- every record
//
// Both are keyed by the ENGLISH severity name. pglogwatch normalises localised
// severities itself, from Config.MessagesLang, which is why lc_messages is
// still resolved and passed down but severityToEnglish is no longer called on
// the hot path.

// parseStream drives the parser over one reader until the reader ends or the
// context is cancelled.
//
// The send is on a ticker in THIS goroutine while the parse runs in another,
// rather than checked inside the parse loop, because a following reader blocks
// when the server is quiet. Checking the interval between records would mean a
// quiet server stops reporting -- and "no errors in this interval" is a
// measurement pgwatch is supposed to emit, not a gap.
func (lp *LogParser) parseStream(rc io.ReadCloser) error {
	defer func() { _ = rc.Close() }()

	parsed := make(chan error, 1)
	go func() { parsed <- lp.consume(rc) }()

	tick := time.NewTicker(lp.tickInterval())
	defer tick.Stop()

	for {
		select {
		case <-lp.ctx.Done():
			return nil
		case err := <-parsed:
			// Send what the last partial interval accumulated before
			// returning, so a shutdown or a rotation does not discard
			// counts that were already parsed.
			lp.sendCounts()
			return err
		case <-tick.C:
			if lp.HasSendIntervalElapsed() {
				lp.sendCounts()
			}
		}
	}
}

// minSendTickInterval keeps a zero or negative Interval from reaching
// time.NewTicker, which panics on one.
//
// The pre-migration loops slept on time.After(0), which returns immediately,
// so a zero interval meant "send as fast as possible" rather than "never" --
// HasSendIntervalElapsed is unconditionally true at zero. That behaviour is
// preserved, at a rate that is a loop rather than a spin. In production the
// interval comes from GetMetricInterval and is always positive; zero shows up
// in tests, where a panic is a poor way to learn it.
const minSendTickInterval = 100 * time.Millisecond

func (lp *LogParser) tickInterval() time.Duration {
	if lp.Interval < minSendTickInterval {
		return minSendTickInterval
	}
	return lp.Interval
}

// consume is the parse loop. It runs in its own goroutine; everything it
// touches on lp is guarded by lp.countsMu.
func (lp *LogParser) consume(r io.Reader) error {
	logger := log.GetLogger(lp.ctx)

	p := pglogwatch.New(r, pglogwatch.Config{
		Format:       lp.parserFormat(),
		MessagesLang: lp.ServerMessagesLang,

		// pgwatch counts severities and nothing else, so a message it
		// cannot read is not worth a log line per occurrence -- a
		// malformed line is normal at the start of a rotated file and
		// after a resume. The parser counts them; the count is reported
		// once, when the stream ends.
		OnMalformed: nil,
	})

	for p.Next() {
		lp.count(p.Record())
	}
	if s := p.Stats(); s.Malformed > 0 || s.Truncated > 0 {
		logger.Debugf("pglogwatch: %d records, %d malformed, %d over-long",
			s.Records, s.Malformed, s.Truncated)
	}
	return p.Err()
}

// count attributes one record to the per-database and per-instance tallies.
func (lp *LogParser) count(rec *pglogwatch.Record) {
	severity := rec.Severity.String()
	if severity == "" {
		// An unrecognised severity. The regex this replaces required
		// \w+ and would not have matched the line at all, so not
		// counting it is the pre-migration behaviour.
		return
	}

	lp.countsMu.Lock()
	defer lp.countsMu.Unlock()

	// Comparing a string against string(someByteSlice) does not allocate:
	// the compiler recognises the pattern and compares the bytes in place.
	// Writing string(rec.Database) into a variable first WOULD allocate,
	// on every record, which is most of what this migration is here to
	// avoid.
	if lp.realDbname == string(rec.Database) {
		lp.eventCounts[severity]++
	}
	lp.eventCountsTotal[severity]++
}

// sendCounts emits an envelope and zeroes the tallies, or returns without
// blocking if the context is done.
func (lp *LogParser) sendCounts() {
	lp.countsMu.Lock()
	envelope := lp.getMeasurementEnvelopeLocked()
	lp.countsMu.Unlock()

	select {
	case <-lp.ctx.Done():
		return
	case lp.StoreCh <- envelope:
	}

	lp.countsMu.Lock()
	zeroEventCounts(lp.eventCounts)
	zeroEventCounts(lp.eventCountsTotal)
	lp.lastSendTime = time.Now()
	lp.countsMu.Unlock()
}

// parserFormat maps the resolved log_destination to a parser format.
func (lp *LogParser) parserFormat() pglogwatch.Format {
	return pglogwatch.FormatCSV
}

// openLocal presents the log directory as one stream.
func (lp *LogParser) openLocal() (io.ReadCloser, error) {
	fs := &pglogwatch.FileSet{
		Dir:                lp.Directory,
		Format:             lp.parserFormat(),
		Follow:             true,
		TruncateOnRotation: lp.TruncateOnRotation,
		PollInterval:       lp.Interval,
		Offsets:            lp.offsets,
	}
	return fs.Open(lp.ctx)
}

// openRemote presents pg_read_file as one stream.
func (lp *LogParser) openRemote() (io.ReadCloser, error) {
	return pgremote.Open(lp.ctx, lp.SourceConn.Conn, pgremote.Config{
		Dir:       lp.Directory,
		Glob:      lp.remoteGlob(),
		ChunkSize: int64(maxChunkSize),
		Offsets:   lp.offsets,
	})
}

func (lp *LogParser) remoteGlob() string {
	return csvLogDefaultGlobSuffix
}

// endSeededOffsets starts every file it has not seen at that file's current
// end, and remembers byte offsets thereafter.
//
// Starting at the end is the pre-migration behaviour and it matters: a fresh
// pgwatch pointed at a server with months of logs would otherwise count every
// severity in all of them and report the lot as one interval's worth. The old
// local parser did this with an explicit Seek(0, io.SeekEnd) on first run, and
// the old remote one by setting offset = size.
type endSeededOffsets struct {
	mu   sync.Mutex
	dir  string
	seen map[string]int64

	// sizeOf reports a file's current length. It is a field so the remote
	// path can supply pg_ls_logdir sizes where the local path stats the
	// filesystem.
	sizeOf func(path string) (int64, bool)
}

func newEndSeededOffsets(dir string, sizeOf func(string) (int64, bool)) *endSeededOffsets {
	return &endSeededOffsets{
		dir:    dir,
		seen:   make(map[string]int64),
		sizeOf: sizeOf,
	}
}

func (o *endSeededOffsets) Get(path string) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if off, ok := o.seen[path]; ok {
		return off, true
	}
	// First sight of this file. If it already has content, that content
	// predates this process and is not ours to report.
	if size, ok := o.sizeOf(path); ok && size > 0 {
		o.seen[path] = size
		return size, true
	}
	return 0, false
}

func (o *endSeededOffsets) Set(path string, offset int64) {
	o.mu.Lock()
	o.seen[path] = offset
	o.mu.Unlock()
}

// localFileSize stats the filesystem.
func localFileSize(path string) (int64, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return fi.Size(), true
}

// remoteFileSizes asks pg_ls_logdir once, so that seeding to the end of a
// remote directory costs one query rather than one per file.
func remoteFileSizes(ctx context.Context, lp *LogParser) func(string) (int64, bool) {
	sizes := make(map[string]int64)
	rows, err := lp.SourceConn.Conn.Query(ctx, "select name, size from pg_ls_logdir()")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var size int64
			if rows.Scan(&name, &size) == nil {
				sizes[filepath.ToSlash(filepath.Join(lp.Directory, name))] = size
			}
		}
	}
	return func(path string) (int64, bool) {
		size, ok := sizes[filepath.ToSlash(path)]
		return size, ok
	}
}
