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

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/db"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/log"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/jackc/pgx/v5"
)

// Constants and types
var pgSeverities = [...]string{"DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "LOG", "FATAL", "PANIC"}

// supportedMessageLangs is the set of lc_messages prefixes pglogwatch can
// normalise to English. "C." is included though pglogwatch lacks a table for it:
// the C locale already writes English, and unknown languages pass through.
var supportedMessageLangs = map[string]bool{
	"C.": true, "de": true, "fr": true, "it": true, "ko": true,
	"pl": true, "ru": true, "sv": true, "tr": true, "zh": true,
}

// Both keep their pre-migration values: read pattern and memory ceiling unchanged.
const maxChunkSize uint64 = 10 * 1024 * 1024 // 10 MB
const maxTrackedFiles = 2500

type logParser struct {
	*logConfig
	ctx              context.Context
	SourceConn       *sources.DbConn
	realDbname       string // snapshot of SourceConn.RealDbname at construction time (avoids lock per log line)
	Interval         time.Duration
	StoreCh          chan<- metrics.MeasurementEnvelope
	eventCounts      map[string]int64 // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	eventCountsTotal map[string]int64 // for the whole instance
	lastSendTime     time.Time

	countsMu sync.Mutex        // guards eventCounts, eventCountsTotal, lastSendTime
	offsets  *endSeededOffsets // how far each log file has been read, in bytes
}

// logConfig is the server's logging configuration, resolved from its GUCs.
//
// Field ORDER is load-bearing: pgx.RowToAddrOfStructByPos maps by position, so a
// new field needs a column at the same position in tryDetermineLogSettings.
type logConfig struct {
	CollectorEnabled   bool
	CSVDestination     bool // neither set means stderr, PostgreSQL's default
	JSONDestination    bool
	TruncateOnRotation bool
	Directory          string
	ServerMessagesLang string
	LinePrefix         string // log_line_prefix; stderr only, and beats pglogwatch's own detection
}

func newLogParser(ctx context.Context, mdb *sources.DbConn, storeCh chan<- metrics.MeasurementEnvelope) (lp *logParser, err error) {

	logger := log.GetLogger(ctx).WithField("source", mdb.Name).WithField("metric", specialMetricServerLogEventCounts)
	ctx = log.WithLogger(ctx, logger)

	var cfg *logConfig
	if cfg, err = tryDetermineLogSettings(ctx, mdb.Conn); err != nil {
		return nil, fmt.Errorf("could not determine Postgres logs settings: %w", err)
	}

	// Unlike the log_destination check this replaces, this one is real: with
	// the collector off there are no files in log_directory at all.
	if !cfg.CollectorEnabled {
		return nil, errors.New("logging_collector is not enabled on the db server")
	}

	logger.Debugf("Considering log files in folder: %s", cfg.Directory)

	mdb.RLock()
	realDbname := mdb.RealDbname
	mdb.RUnlock()
	return &logParser{
		ctx:              ctx,
		SourceConn:       mdb,
		realDbname:       realDbname,
		Interval:         mdb.GetMetricInterval(specialMetricServerLogEventCounts),
		StoreCh:          storeCh,
		logConfig:        cfg,
		eventCounts:      make(map[string]int64),
		eventCountsTotal: make(map[string]int64),
	}, nil
}

func (lp *logParser) hasSendIntervalElapsed() bool {
	return lp.lastSendTime.IsZero() || lp.lastSendTime.Before(time.Now().Add(-lp.Interval))
}

func (lp *logParser) parseLogs() error {
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
	sizeOf, err := remoteFileSizes(lp.ctx, lp)
	if err != nil {
		return fmt.Errorf("could not list the remote log directory: %w", err)
	}
	lp.offsets = newEndSeededOffsets(sizeOf)
	rc, err := lp.openRemote()
	if err != nil {
		return err
	}
	return lp.parseStream(rc)
}

func tryDetermineLogSettings(ctx context.Context, conn db.PgxIface) (cfg *logConfig, err error) {
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
		if cfg, err = pgx.CollectOneRow(res, pgx.RowToAddrOfStructByPos[logConfig]); err == nil {
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

// getMeasurementEnvelope converts current event counts to a MeasurementEnvelope
func (lp *logParser) getMeasurementEnvelope() metrics.MeasurementEnvelope {
	lp.countsMu.Lock()
	defer lp.countsMu.Unlock()
	return lp.getMeasurementEnvelopeLocked()
}

// getMeasurementEnvelopeLocked is getMeasurementEnvelope with countsMu already
// held, which is how the send path reads the counts and zeroes them without
// letting a record land in between.
func (lp *logParser) getMeasurementEnvelopeLocked() metrics.MeasurementEnvelope {
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
