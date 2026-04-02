package reaper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/jackc/pgx/v5"
)

// Constants and types
var pgSeverities = [...]string{"DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "LOG", "FATAL", "PANIC"}
var pgSeveritiesLocale = map[string]map[string]string{
	"C.": {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "NOTICE": "NOTICE", "WARNING": "WARNING", "ERROR": "ERROR", "FATAL": "FATAL", "PANIC": "PANIC"},
	"C":  {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "NOTICE": "NOTICE", "WARNING": "WARNING", "ERROR": "ERROR", "FATAL": "FATAL", "PANIC": "PANIC"},
	"de": {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "HINWEIS": "NOTICE", "WARNUNG": "WARNING", "FEHLER": "ERROR", "FATAL": "FATAL", "PANIK": "PANIC"},
	"fr": {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "NOTICE": "NOTICE", "ATTENTION": "WARNING", "ERREUR": "ERROR", "FATAL": "FATAL", "PANIK": "PANIC", "PANIQUE": "PANIC"},
	"it": {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "NOTIFICA": "NOTICE", "ATTENZIONE": "WARNING", "ERRORE": "ERROR", "FATALE": "FATAL", "PANICO": "PANIC"},
	"ko": {"디버그": "DEBUG", "로그": "LOG", "정보": "INFO", "알림": "NOTICE", "경고": "WARNING", "오류": "ERROR", "치명적오류": "FATAL", "손상": "PANIC"},
	"pl": {"DEBUG": "DEBUG", "DZIENNIK": "LOG", "INFORMACJA": "INFO", "UWAGA": "NOTICE", "OSTRZEŻENIE": "WARNING", "BŁĄD": "ERROR", "KATASTROFALNY": "FATAL", "PANIKA": "PANIC"},
	"ru": {"ОТЛАДКА": "DEBUG", "СООБЩЕНИЕ": "LOG", "ИНФОРМАЦИЯ": "INFO", "ЗАМЕЧАНИЕ": "NOTICE", "ПРЕДУПРЕЖДЕНИЕ": "WARNING", "ОШИБКА": "ERROR", "ВАЖНО": "FATAL", "ПАНИКА": "PANIC"},
	"sv": {"DEBUG": "DEBUG", "LOGG": "LOG", "INFO": "INFO", "NOTIS": "NOTICE", "VARNING": "WARNING", "FEL": "ERROR", "FATALT": "FATAL", "PANIK": "PANIC"},
	"tr": {"DEBUG": "DEBUG", "LOG": "LOG", "BİLGİ": "INFO", "NOT": "NOTICE", "UYARI": "WARNING", "HATA": "ERROR", "ÖLÜMCÜL (FATAL)": "FATAL", "KRİTİK": "PANIC"},
	"zh": {"调试": "DEBUG", "日志": "LOG", "信息": "INFO", "注意": "NOTICE", "警告": "WARNING", "错误": "ERROR", "致命错误": "FATAL", "比致命错误还过分的错误": "PANIC"},
}

const csvLogDefaultRegEx = `^^(?P<log_time>.*?),"?(?P<user_name>.*?)"?,"?(?P<database_name>.*?)"?,(?P<process_id>\d+),"?(?P<connection_from>.*?)"?,(?P<session_id>.*?),(?P<session_line_num>\d+),"?(?P<command_tag>.*?)"?,(?P<session_start_time>.*?),(?P<virtual_transaction_id>.*?),(?P<transaction_id>.*?),(?P<error_severity>\w+),`
const csvLogDefaultGlobSuffix = "*.csv"

const maxChunkSize uint64 = 10 * 1024 * 1024 // 10 MB
const maxTrackedFiles = 2500

type LogParser struct {
	*LogConfig
	ctx              context.Context
	LogsMatchRegex   *regexp.Regexp
	SourceConn       *sources.SourceConn
	Interval         time.Duration
	StoreCh          chan<- metrics.MeasurementEnvelope
	eventCounts      map[string]int64 // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	eventCountsTotal map[string]int64 // for the whole instance
	lastSendTime     time.Time
	fileOffsets      map[string]uint64 // map of log file paths to last read offsets
}

type LogConfig struct {
	CollectorEnabled   bool
	CSVDestination     bool
	TruncateOnRotation bool
	Directory          string
	ServerMessagesLang string
}

func NewLogParser(ctx context.Context, mdb *sources.SourceConn, storeCh chan<- metrics.MeasurementEnvelope) (lp *LogParser, err error) {

	logger := log.GetLogger(ctx).WithField("source", mdb.Name).WithField("metric", specialMetricServerLogEventCounts)
	ctx = log.WithLogger(ctx, logger)

	logsRegex := regexp.MustCompile(csvLogDefaultRegEx)

	logger.Debugf("Using %s as log parsing regex", logsRegex)

	var cfg *LogConfig
	if cfg, err = tryDetermineLogSettings(ctx, mdb.Conn); err != nil {
		return nil, fmt.Errorf("could not determine Postgres logs settings: %w", err)
	}

	if !cfg.CollectorEnabled {
		return nil, errors.New("logging_collector is not enabled on the db server")
	}

	if !cfg.CSVDestination {
		return nil, errors.New("log_destination must contain 'csvlog' for log parsing to work")
	}

	logger.Debugf("Considering log files in folder: %s", cfg.Directory)

	return &LogParser{
		ctx:              ctx,
		LogsMatchRegex:   logsRegex,
		SourceConn:       mdb,
		Interval:         mdb.GetMetricInterval(specialMetricServerLogEventCounts),
		StoreCh:          storeCh,
		LogConfig:        cfg,
		eventCounts:      make(map[string]int64),
		eventCountsTotal: make(map[string]int64),
		fileOffsets:      make(map[string]uint64),
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
			return lp.parseLogsLocal()
		}
		l.WithError(err).Error("Couldn't parse logs locally, lacking required privileges")
	}

	l.Info("DB is not detected to be on the same host, parsing logs remotely")
	if err := checkHasRemotePrivileges(lp.ctx, lp.SourceConn, lp.Directory); err != nil {
		l.WithError(err).Error("couldn't parse logs remotely, lacking required privileges")
		return err
	}
	return lp.parseLogsRemote()
}

func tryDetermineLogSettings(ctx context.Context, conn db.PgxIface) (cfg *LogConfig, err error) {
	sql := `select 
	current_setting('logging_collector') = 'on' as is_enabled,
	strpos(current_setting('log_destination'), 'csvlog') > 0 as csvlog_dest,
	current_setting('log_truncate_on_rotation') = 'on' as log_trunc,
	case 
		when current_setting('log_directory') ~ '^(\w:)?\/.+' then current_setting('log_directory') 
		else current_setting('data_directory') || '/' || current_setting('log_directory') 
	end as log_dir,
	current_setting('lc_messages')::varchar(2) as lc_messages`
	var res pgx.Rows
	if res, err = conn.Query(ctx, sql); err == nil {
		if cfg, err = pgx.CollectOneRow(res, pgx.RowToAddrOfStructByPos[LogConfig]); err == nil {
			if _, ok := pgSeveritiesLocale[cfg.ServerMessagesLang]; !ok {
				cfg.ServerMessagesLang = "en"
			}
			return cfg, nil
		}
	}
	return nil, err
}

func checkHasRemotePrivileges(ctx context.Context, mdb *sources.SourceConn, logsDirPath string) error {
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

func severityToEnglish(serverLang, errorSeverity string) string {
	normalizedSeverity := strings.ToUpper(errorSeverity)
	if serverLang == "en" {
		return normalizedSeverity
	}
	severityMap := pgSeveritiesLocale[serverLang]
	severityEn, ok := severityMap[normalizedSeverity]
	if !ok {
		return normalizedSeverity
	}
	return severityEn
}

func (lp *LogParser) regexMatchesToMap(matches []string) map[string]string {
	result := make(map[string]string)
	if len(matches) == 0 || lp.LogsMatchRegex == nil {
		return result
	}
	for i, name := range lp.LogsMatchRegex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}
	return result
}

// GetMeasurementEnvelope converts current event counts to a MeasurementEnvelope
func (lp *LogParser) GetMeasurementEnvelope() metrics.MeasurementEnvelope {
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
