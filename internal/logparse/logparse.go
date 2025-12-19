package logparse

import (
	"context"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/jackc/pgx/v5"
)

type LogParser struct {
	ctx                context.Context
	LogsMatchRegex     *regexp.Regexp
	LogFolder   	   string
	ServerMessagesLang string
	Mdb  		       *sources.SourceConn
	RealDbname		   string
	Interval           float64
	StoreCh            chan<- metrics.MeasurementEnvelope
}

func NewLogParser(
	ctx context.Context,
	mdb *sources.SourceConn,
	realDbname string,
	interval float64,
	storeCh chan<- metrics.MeasurementEnvelope,
) (*LogParser, error) {

	logger := log.GetLogger(ctx).WithField("source", mdb.Name).WithField("metric", specialMetricServerLogEventCounts)
	ctx = log.WithLogger(ctx, logger)

	logsRegex, err := regexp.Compile(CSVLogDefaultRegEx)
	if err != nil {
		logger.WithError(err).Error("Invalid log parsing regex")
		return nil, err
	}
	logger.Debugf("Using %s as log parsing regex", logsRegex)

	var logFolder string
	if logFolder, err = tryDetermineLogFolder(ctx, mdb.Conn); err != nil {
		logger.WithError(err).Error("Could not determine Postgres logs folder")
		return nil, err
	}
	logger.Debugf("Considering log files in folder: %s", logFolder)

	var serverMessagesLang string
	if serverMessagesLang, err = tryDetermineLogMessagesLanguage(ctx, mdb.Conn); err != nil {
		logger.WithError(err).Error("Could not determine language (lc_collate) used for server logs")
		return nil, err
	}

	return &LogParser{
		ctx: ctx,
		LogsMatchRegex: logsRegex,
		LogFolder: logFolder,
		ServerMessagesLang: serverMessagesLang,
		Mdb: mdb,
		RealDbname: realDbname,
		Interval: interval,
		StoreCh: storeCh,
	}, nil
}

func (lp *LogParser) ParseLogs() error {
	l := log.GetLogger(lp.ctx)
	if ok, err := db.IsClientOnSameHost(lp.Mdb.Conn); ok && err == nil {
		l.Info("DB is on the same host. parsing logs locally")
		// TODO: check privileges before invoking local parsing
		return lp.ParseLogsLocal()
	}

	l.Info("DB is not detected to be on the same host. parsing logs remotely")
	if err := checkHasPrivileges(lp.ctx, lp.Mdb, lp.LogFolder); err == nil {
		return lp.ParseLogsRemote()
	} else {
		l.WithError(err).Error("Could't parse logs remotely. lacking required privileges")
		return err
	}
}

func tryDetermineLogFolder(ctx context.Context, conn db.PgxIface) (string, error) {
	sql := `select current_setting('data_directory') as dd, current_setting('log_directory') as ld`
	var dd, ld string
	err := conn.QueryRow(ctx, sql).Scan(&dd, &ld)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(ld, "/") {
		// we have a full path we can use
		return ld, nil
	}
	return path.Join(dd, ld), nil
}

func tryDetermineLogMessagesLanguage(ctx context.Context, conn db.PgxIface) (string, error) {
	sql := `select current_setting('lc_messages')::varchar(2) as lc_messages;`
	var lc string
	err := conn.QueryRow(ctx, sql).Scan(&lc)
	if err != nil {
		return "", err
	}
	if _, ok := PgSeveritiesLocale[lc]; !ok {
		return "en", nil
	}
	return lc, nil
}

func checkHasPrivileges(ctx context.Context, mdb *sources.SourceConn, logsDirPath string) error {
	var logFile string
	err := mdb.Conn.QueryRow(ctx, "select name from pg_ls_logdir() limit 1").Scan(&logFile)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	var dummy string
	err = mdb.Conn.QueryRow(ctx, "select pg_read_file($1, 0, 0)", filepath.Join(logsDirPath, logFile)).Scan(&dummy)
	return err
}
