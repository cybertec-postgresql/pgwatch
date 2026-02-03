package reaper

import (
	"context"
	"os"
	"path"
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
	"de": {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "HINWEIS": "NOTICE", "WARNUNG": "WARNING", "FEHLER": "ERROR", "FATAL": "FATAL", "PANIK": "PANIC"},
	"fr": {"DEBUG": "DEBUG", "LOG": "LOG", "INFO": "INFO", "NOTICE": "NOTICE", "ATTENTION": "WARNING", "ERREUR": "ERROR", "FATAL": "FATAL", "PANIK": "PANIC"},
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
	ctx                context.Context
	LogsMatchRegex     *regexp.Regexp
	LogFolder          string
	ServerMessagesLang string
	LogTruncOnRotation string
	SourceConn         *sources.SourceConn
	Interval           float64
	StoreCh            chan<- metrics.MeasurementEnvelope
	eventCounts        map[string]int64 // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	eventCountsTotal   map[string]int64 // for the whole instance
	lastSendTime       time.Time
	fileOffsets        map[string]uint64 // map of log file paths to last read offsets
}

func NewLogParser(ctx context.Context, mdb *sources.SourceConn, storeCh chan<- metrics.MeasurementEnvelope) (*LogParser, error) {

	logger := log.GetLogger(ctx).WithField("source", mdb.Name).WithField("metric", specialMetricServerLogEventCounts)
	ctx = log.WithLogger(ctx, logger)

	logsRegex, err := regexp.Compile(csvLogDefaultRegEx)
	if err != nil {
		logger.WithError(err).Error("Invalid log parsing regex")
		return nil, err
	}
	logger.Debugf("Using %s as log parsing regex", logsRegex)

	var logFolder, serverMessagesLang, logTrunc string
	if logFolder, serverMessagesLang, logTrunc, err = tryDetermineLogSettings(ctx, mdb.Conn); err != nil {
		logger.WithError(err).Error("Could not determine Postgres logs settings")
		return nil, err
	}
	logger.Debugf("Considering log files in folder: %s", logFolder)

	return &LogParser{
		ctx:                ctx,
		LogsMatchRegex:     logsRegex,
		LogFolder:          logFolder,
		ServerMessagesLang: serverMessagesLang,
		LogTruncOnRotation: logTrunc,
		SourceConn:         mdb,
		Interval:           mdb.GetMetricInterval(specialMetricServerLogEventCounts),
		StoreCh:            storeCh,
		eventCounts:        make(map[string]int64),
		eventCountsTotal:   make(map[string]int64),
		fileOffsets:        make(map[string]uint64),
	}, nil
}

func (lp *LogParser) HasSendIntervalElapsed() bool {
	return lp.lastSendTime.IsZero() || lp.lastSendTime.Before(time.Now().Add(-time.Second*time.Duration(lp.Interval)))
}

func (lp *LogParser) ParseLogs() error {
	l := log.GetLogger(lp.ctx)
	if ok, err := db.IsClientOnSameHost(lp.SourceConn.Conn); ok && err == nil {
		l.Info("DB is on the same host. parsing logs locally")
		if err = checkHasLocalPrivileges(lp.LogFolder); err == nil {
			return lp.parseLogsLocal()
		}
		l.WithError(err).Error("Could't parse logs locally. lacking required privileges")
	}

	l.Info("DB is not detected to be on the same host. parsing logs remotely")
	if err := checkHasRemotePrivileges(lp.ctx, lp.SourceConn, lp.LogFolder); err != nil {
		l.WithError(err).Error("Could't parse logs remotely. lacking required privileges")
		return err
	}
	return lp.parseLogsRemote()
}

func tryDetermineLogSettings(ctx context.Context, conn db.PgxIface) (string, string, string, error) {
	sql := `select 
			current_setting('data_directory') as dd, 
			current_setting('log_directory') as ld,
			current_setting('lc_messages')::varchar(2) as lc_messages,
			current_setting('log_truncate_on_rotation') as log_trunc`
	var dd, ld, lc, log_trunc string
	err := conn.QueryRow(ctx, sql).Scan(&dd, &ld, &lc, &log_trunc)
	if err != nil {
		return "", "", "", err
	}
	if !strings.HasPrefix(ld, "/") {
		ld = path.Join(dd, ld)
	}
	if _, ok := pgSeveritiesLocale[lc]; !ok {
		lc = "en"
	}
	return ld, lc, log_trunc, nil
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
	if serverLang == "en" {
		return errorSeverity
	}
	severityMap := pgSeveritiesLocale[serverLang]
	severityEn, ok := severityMap[errorSeverity]
	if !ok {
		return errorSeverity
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
