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
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/log"
)

// The pglogwatch-backed parsing engine: one loop over pglogwatch.Parser, fed by
// either a local or a remote io.Reader. Counts go to eventCounts (records whose
// Database matches the source) and eventCountsTotal (every record), keyed by the
// English severity, which pglogwatch normalises from Config.MessagesLang.

// parseStream drives the parser over one reader until it ends or ctx is done.
//
// The send ticks in THIS goroutine while the parse runs in another: a following
// reader blocks on a quiet server, and a quiet interval still has to report its
// zeroes rather than leave a gap.
func (lp *LogParser) parseStream(rc io.ReadCloser) error {
	defer func() { _ = rc.Close() }()

	parsed := make(chan error, 1)
	go func() { parsed <- lp.consume(rc) }()

	tick := time.NewTicker(lp.tickInterval())
	defer tick.Stop()

	for {
		select {
		case <-lp.ctx.Done():
			<-parsed // the deferred Close races consume if it is still reading
			return nil
		case err := <-parsed:
			lp.sendCounts() // don't discard the last partial interval
			return err
		case <-tick.C:
			if lp.HasSendIntervalElapsed() {
				lp.sendCounts()
			}
		}
	}
}

// minSendTickInterval keeps a zero Interval (tests only) off time.NewTicker,
// which panics on one, while preserving "send as fast as possible" at zero.
const minSendTickInterval = 100 * time.Millisecond

// sendTicksPerInterval must be > 1: HasSendIntervalElapsed wants STRICTLY more
// than Interval, so a ticker of exactly Interval is always a hair early and the
// send is skipped on alternate ticks -- half the configured rate.
const sendTicksPerInterval = 4

func (lp *LogParser) tickInterval() time.Duration {
	if t := lp.Interval / sendTicksPerInterval; t >= minSendTickInterval {
		return t
	}
	return minSendTickInterval
}

// consume is the parse loop. It runs in its own goroutine; everything it
// touches on lp is guarded by lp.countsMu.
func (lp *LogParser) consume(r io.Reader) error {
	logger := log.GetLogger(lp.ctx)

	p := pglogwatch.New(r, pglogwatch.Config{
		Format:       lp.parserFormat(),
		MessagesLang: lp.ServerMessagesLang,

		// stderr only; empty would mean "detect from the log", and the
		// server's own setting beats a guess from a sample.
		LinePrefix: lp.LinePrefix,

		// Malformed lines are normal after a rotation or a resume, and not
		// worth a log line each. Stats() reports the count at stream end.
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
		return // unrecognised; the regex this replaces would not have matched either
	}

	lp.countsMu.Lock()
	defer lp.countsMu.Unlock()

	// Compared inline: the compiler elides the conversion. Assigning
	// string(rec.Database) to a variable first would allocate per record.
	if lp.realDbname == string(rec.Database) {
		lp.eventCounts[severity]++
	}
	lp.eventCountsTotal[severity]++
}

// sendCounts emits an envelope and zeroes the tallies.
//
// Read and zero share one critical section, BEFORE the send: zeroing after a
// send that blocked on sink backpressure would discard everything the parser
// counted meanwhile. Records arriving mid-send land in the emptied maps and go
// out with the next envelope.
func (lp *LogParser) sendCounts() {
	lp.countsMu.Lock()
	envelope := lp.getMeasurementEnvelopeLocked()
	zeroEventCounts(lp.eventCounts)
	zeroEventCounts(lp.eventCountsTotal)
	lp.lastSendTime = time.Now()
	lp.countsMu.Unlock()

	select {
	case <-lp.ctx.Done():
	case lp.StoreCh <- envelope:
	}
}

// parserFormat maps log_destination to a parser format. It is a LIST, so this
// is a precedence: csvlog first, to keep pre-migration counts identical.
func (lp *LogParser) parserFormat() pglogwatch.Format {
	switch {
	case lp.CSVDestination:
		return pglogwatch.FormatCSV
	case lp.JSONDestination:
		return pglogwatch.FormatJSON
	default:
		return pglogwatch.FormatStderr
	}
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

// openRemote presents pg_read_file as one stream. Follow mirrors openLocal:
// without it the reader ends at the server's current last byte and never
// reports again.
func (lp *LogParser) openRemote() (io.ReadCloser, error) {
	return pgremote.Open(lp.ctx, lp.SourceConn.Conn, pgremote.Config{
		Dir:          lp.Directory,
		Glob:         lp.remoteGlob(),
		ChunkSize:    int64(maxChunkSize),
		Follow:       true,
		PollInterval: lp.Interval,
		Offsets:      lp.offsets,
	})
}

// remoteGlob selects the files the chosen destination writes. pg_ls_logdir
// lists everything, and a server writing two formats would otherwise be counted
// twice.
func (lp *LogParser) remoteGlob() string {
	switch {
	case lp.CSVDestination:
		return "*.csv"
	case lp.JSONDestination:
		return "*.json"
	default:
		return "*.log"
	}
}

// endSeededOffsets records how far each log file has been read, in BYTES --
// what pg_read_file takes natively, and a seek locally.
//
// An unseen file starts at its CURRENT END, so a fresh pgwatch reports what
// happens next rather than every severity in months of retained logs.
type endSeededOffsets struct {
	mu    sync.Mutex
	seen  map[string]offsetEntry
	max   int
	clock uint64 // monotonic tick, for eviction order

	sizeOf func(path string) (int64, bool) // os.Stat locally, pg_ls_logdir remotely
}

type offsetEntry struct {
	offset   int64
	lastUsed uint64
}

func newEndSeededOffsets(sizeOf func(string) (int64, bool)) *endSeededOffsets {
	return &endSeededOffsets{
		seen:   make(map[string]offsetEntry),
		max:    maxTrackedFiles,
		sizeOf: sizeOf,
	}
}

func (o *endSeededOffsets) Get(path string) (int64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if e, ok := o.seen[path]; ok {
		o.touchLocked(path, e.offset)
		return e.offset, true
	}
	// First sight: existing content predates this process, so skip it.
	if size, ok := o.sizeOf(path); ok && size > 0 {
		o.touchLocked(path, size)
		return size, true
	}
	return 0, false
}

func (o *endSeededOffsets) Set(path string, offset int64) {
	o.mu.Lock()
	o.touchLocked(path, offset)
	o.mu.Unlock()
}

// touchLocked stores an offset, evicting the least recently used entry past the
// bound. LRU rather than clearing the map: the active file is the most recently
// used, so it can never be the one dropped.
func (o *endSeededOffsets) touchLocked(path string, offset int64) {
	o.clock++
	o.seen[path] = offsetEntry{offset: offset, lastUsed: o.clock}
	if len(o.seen) <= o.max {
		return
	}
	var oldest string
	var oldestUse uint64
	for p, e := range o.seen {
		if oldest == "" || e.lastUsed < oldestUse {
			oldest, oldestUse = p, e.lastUsed
		}
	}
	delete(o.seen, oldest)
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
//
// The error must not be swallowed: an empty map reads as "never seen" for every
// file, so one failed query re-reads the whole retained backlog and reports it
// as a single interval.
func remoteFileSizes(ctx context.Context, lp *LogParser) (func(string) (int64, bool), error) {
	sizes := make(map[string]int64)
	rows, err := lp.SourceConn.Conn.Query(ctx, "select name, size from pg_ls_logdir()")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var size int64
		if err := rows.Scan(&name, &size); err != nil {
			return nil, err
		}
		sizes[filepath.ToSlash(filepath.Join(lp.Directory, name))] = size
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return func(path string) (int64, bool) {
		size, ok := sizes[filepath.ToSlash(path)]
		return size, ok
	}, nil
}
