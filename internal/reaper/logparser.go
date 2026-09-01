package reaper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/jackc/pgx/v5"
)

// Constants and types
var pgSeverities = [...]string{"DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "LOG", "FATAL", "PANIC"}

// supportedMessageLangs is the set of lc_messages prefixes whose severities
// can be normalised to English.
//
// This used to be the translation tables themselves -- eight severities in ten
// languages, spelled out here. pglogwatch carries them now, and keeping a
// second copy would mean two lists to update and no way to notice they had
// drifted apart. Only the KEYS were ever read outside severityToEnglish, to
// decide whether a language is known.
//
// "C." is in the set and is not in pglogwatch's tables, deliberately: the C
// locale writes English severities, so passing it through unchanged -- which
// is what pglogwatch does with a language it does not know -- is correct.
var supportedMessageLangs = map[string]bool{
	"C.": true, "de": true, "fr": true, "it": true, "ko": true,
	"pl": true, "ru": true, "sv": true, "tr": true, "zh": true,
}

// maxChunkSize is how much pg_read_file fetches per round trip, and
// maxTrackedFiles bounds the offset store. Both keep their pre-migration
// values so the remote read pattern and the memory ceiling are unchanged.
const maxChunkSize uint64 = 10 * 1024 * 1024 // 10 MB
const maxTrackedFiles = 2500

type LogParser struct {
	*LogConfig
	ctx              context.Context
	SourceConn       *sources.DbConn
	realDbname       string // snapshot of SourceConn.RealDbname at construction time (avoids lock per log line)
	Interval         time.Duration
	StoreCh          chan<- metrics.MeasurementEnvelope
	eventCounts      map[string]int64 // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	eventCountsTotal map[string]int64 // for the whole instance
	lastSendTime     time.Time

	// countsMu guards eventCounts, eventCountsTotal and lastSendTime.
	//
	// It is new with the pglogwatch engine: parsing now runs in its own
	// goroutine while the send interval ticks in another, because a
	// following reader blocks when the server is quiet and the interval
	// has to elapse anyway. The pre-migration parsers were single
	// goroutine loops and needed no lock.
	countsMu sync.Mutex

	// offsets records how far each log file has been read, in BYTES.
	offsets *endSeededOffsets
}

// LogConfig is the server's logging configuration, as resolved from its GUCs.
//
// The field ORDER is load-bearing: pgx.RowToAddrOfStructByPos maps columns to
// fields by position, so a field added here must be matched by a column added
// at the same position in tryDetermineLogSettings' query.
type LogConfig struct {
	CollectorEnabled bool

	// CSVDestination and JSONDestination report whether log_destination
	// contains csvlog and jsonlog. Neither being set means stderr, which
	// is PostgreSQL's default and which pgwatch could not read at all
	// before this migration.
	CSVDestination  bool
	JSONDestination bool

	TruncateOnRotation bool
	Directory          string
	ServerMessagesLang string

	// LinePrefix is log_line_prefix, and matters only for stderr: it is
	// what tells the parser where the timestamp, user and database sit in
	// a line that has no columns.
	//
	// pglogwatch can detect a prefix from the log itself, but asking the
	// server is better where the server is right there to ask -- detection
	// has to guess from a sample, and a log whose first lines are unusual
	// can be guessed wrong.
	LinePrefix string
}

func NewLogParser(ctx context.Context, mdb *sources.DbConn, storeCh chan<- metrics.MeasurementEnvelope) (lp *LogParser, err error) {

	logger := log.GetLogger(ctx).WithField("source", mdb.Name).WithField("metric", specialMetricServerLogEventCounts)
	ctx = log.WithLogger(ctx, logger)

	var cfg *LogConfig
	if cfg, err = tryDetermineLogSettings(ctx, mdb.Conn); err != nil {
		return nil, fmt.Errorf("could not determine Postgres logs settings: %w", err)
	}

	// This error stays, where the log_destination one went.
	//
	// The two look alike and are not. log_destination only chooses a
	// FORMAT, and pglogwatch reads all three of them, so rejecting a server
	// over it was a limitation of the old regex rather than a fact about
	// the server. logging_collector is different in kind: with it off,
	// PostgreSQL writes to the postmaster's stderr and there are no log
	// files in log_directory to read. No parser fixes that, and reporting
	// zero events would be indistinguishable from a healthy quiet server.
	if !cfg.CollectorEnabled {
		return nil, errors.New("logging_collector is not enabled on the db server")
	}

	logger.Debugf("Considering log files in folder: %s", cfg.Directory)

	mdb.RLock()
	realDbname := mdb.RealDbname
	mdb.RUnlock()
	return &LogParser{
		ctx:              ctx,
		SourceConn:       mdb,
		realDbname:       realDbname,
		Interval:         mdb.GetMetricInterval(specialMetricServerLogEventCounts),
		StoreCh:          storeCh,
		LogConfig:        cfg,
		eventCounts:      make(map[string]int64),
		eventCountsTotal: make(map[string]int64),
	}, nil
}

func (lp *LogParser) HasSendIntervalElapsed() bool {
	return lp.lastSendTime.IsZero() || lp.lastSendTime.Before(time.Now().Add(-lp.Interval))
}

func (lp *LogParser) ParseLogs() error {
	l := log.GetLogger(lp.ctx)
	if ok, err := db.IsClientOnSameHost(lp.SourceConn.Conn); ok && err == nil {
		l.Info("DB is on the same host, parsing logs locally")
		if err = checkHasLocalPrivileges(lp.Directory); err == nil {
			lp.offsets = newEndSeededOffsets(localFileSize)
			rc, err := lp.openLocal()
			if err != nil {
				return err
			}
			return lp.parseStream(rc)
		}
		l.WithError(err).Error("Couldn't parse logs locally, lacking required privileges")
	}

	l.Info("DB is not detected to be on the same host, parsing logs remotely")
	if err := checkHasRemotePrivileges(lp.ctx, lp.SourceConn, lp.Directory); err != nil {
		l.WithError(err).Error("couldn't parse logs remotely, lacking required privileges")
		return err
	}
	lp.offsets = newEndSeededOffsets(remoteFileSizes(lp.ctx, lp))
	rc, err := lp.openRemote()
	if err != nil {
		return err
	}
	return lp.parseStream(rc)
}

func tryDetermineLogSettings(ctx context.Context, conn db.PgxIface) (cfg *LogConfig, err error) {
	sql := `select 
	current_setting('logging_collector') = 'on' as is_enabled,
	strpos(current_setting('log_destination'), 'csvlog') > 0 as csvlog_dest,
	strpos(current_setting('log_destination'), 'jsonlog') > 0 as jsonlog_dest,
	current_setting('log_truncate_on_rotation') = 'on' as log_trunc,
	case 
		when current_setting('log_directory') ~ '^(\w:)?\/.+' then current_setting('log_directory') 
		else current_setting('data_directory') || '/' || current_setting('log_directory') 
	end as log_dir,
	current_setting('lc_messages')::varchar(2) as lc_messages,
	current_setting('log_line_prefix') as line_prefix`
	var res pgx.Rows
	if res, err = conn.Query(ctx, sql); err == nil {
		if cfg, err = pgx.CollectOneRow(res, pgx.RowToAddrOfStructByPos[LogConfig]); err == nil {
			if !supportedMessageLangs[cfg.ServerMessagesLang] {
				cfg.ServerMessagesLang = "en"
			}
			return cfg, nil
		}
	}
	return nil, err
}

func checkHasRemotePrivileges(ctx context.Context, mdb *sources.DbConn, logsDirPath string) error {
	var logFile string
	err := mdb.Conn.QueryRow(ctx, "select name from pg_ls_logdir() limit 1").Scan(&logFile)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	var dummy string
	err = mdb.Conn.QueryRow(ctx, "select pg_read_file($1, 0, 0)", filepath.Join(logsDirPath, logFile)).Scan(&dummy)
	return err
}

func checkHasLocalPrivileges(logsDirPath string) error {
	_, err := os.ReadDir(logsDirPath)
	if err != nil {
		return err
	}
	return nil
}

// GetMeasurementEnvelope converts current event counts to a MeasurementEnvelope
func (lp *LogParser) GetMeasurementEnvelope() metrics.MeasurementEnvelope {
	lp.countsMu.Lock()
	defer lp.countsMu.Unlock()
	return lp.getMeasurementEnvelopeLocked()
}

// getMeasurementEnvelopeLocked is GetMeasurementEnvelope with countsMu already
// held, which is how the send path reads the counts and zeroes them without
// letting a record land in between.
func (lp *LogParser) getMeasurementEnvelopeLocked() metrics.MeasurementEnvelope {
	allSeverityCounts := metrics.NewMeasurement(time.Now().UnixNano())
	for _, s := range pgSeverities {
		parsedCount, ok := lp.eventCounts[s]
		if ok {
			allSeverityCounts[strings.ToLower(s)] = parsedCount
		} else {
			allSeverityCounts[strings.ToLower(s)] = int64(0)
		}
		parsedCount, ok = lp.eventCountsTotal[s]
		if ok {
			allSeverityCounts[strings.ToLower(s)+"_total"] = parsedCount
		} else {
			allSeverityCounts[strings.ToLower(s)+"_total"] = int64(0)
		}
	}
	return metrics.MeasurementEnvelope{
		DBName:     lp.SourceConn.Name,
		MetricName: specialMetricServerLogEventCounts,
		Data:       metrics.Measurements{allSeverityCounts},
		CustomTags: lp.SourceConn.CustomTags,
	}
}

func zeroEventCounts(eventCounts map[string]int64) {
	for _, severity := range pgSeverities {
		eventCounts[severity] = 0
	}
}
