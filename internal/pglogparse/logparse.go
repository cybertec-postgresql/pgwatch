package pglogparse

import (
	"cmp"
	"context"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/jackc/pgx/v5"
)

const specialMetricServerLogEventCounts = "server_log_event_counts"

var PgSeverities = [...]string{"DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "LOG", "FATAL", "PANIC"}
var PgSeveritiesLocale = map[string]map[string]string{
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

const CSVLogDefaultRegEx = `^^(?P<log_time>.*?),"?(?P<user_name>.*?)"?,"?(?P<database_name>.*?)"?,(?P<process_id>\d+),"?(?P<connection_from>.*?)"?,(?P<session_id>.*?),(?P<session_line_num>\d+),"?(?P<command_tag>.*?)"?,(?P<session_start_time>.*?),(?P<virtual_transaction_id>.*?),(?P<transaction_id>.*?),(?P<error_severity>\w+),`
const CSVLogDefaultGlobSuffix = "*.csv"

func ParseLogs(
	ctx context.Context,
	mdb *sources.SourceConn,
	realDbname string,
	interval float64,
	storeCh chan<- metrics.MeasurementEnvelope,
	LogsMatchRegex string,
	LogsGlobPath string,
) {
	var logsGlobPath string
	var serverMessagesLang string

	logger := log.GetLogger(ctx).WithField("source", mdb.Name).WithField("metric", specialMetricServerLogEventCounts)
	ctx = log.WithLogger(ctx, logger)

	logsRegex, err := regexp.Compile(cmp.Or(LogsMatchRegex, CSVLogDefaultRegEx))
	if err != nil {
		logger.WithError(err).Printf("Invalid log parsing regex: %s", logsRegex)
		return
	}
	logger.Debugf("Using %s as log parsing regex", logsRegex)

	if LogsGlobPath != "" {
		logsGlobPath = LogsGlobPath
	} else {
		if logsGlobPath, err = tryDetermineLogFolder(ctx, mdb.Conn); err != nil {
			logger.WithError(err).Print("Could not determine Postgres logs folder")
			return
		}
	}
	logger.Debugf("Considering log files determined by glob pattern: %s", logsGlobPath)

	if serverMessagesLang, err = tryDetermineLogMessagesLanguage(ctx, mdb.Conn); err != nil {
		logger.WithError(err).Print("Could not determine language (lc_collate) used for server logs")
		return
	}

	if ok, err := db.IsClientOnSameHost(mdb.Conn); ok && err == nil {
		logger.Info("DB is on the same host. parsing logs locally")
		ParseLogsLocal(ctx, mdb, realDbname, interval, storeCh, logsRegex, logsGlobPath, serverMessagesLang)
	} else {
		logger.Info("DB is not detected to be on the same host. parsing logs remotely")
		if err := checkHasPrivileges(ctx, mdb, filepath.Dir(logsGlobPath)); err == nil {
			ParseLogsRemote(ctx, mdb, realDbname, interval, storeCh, logsRegex, filepath.Dir(logsGlobPath), serverMessagesLang)
		} else {
			logger.WithError(err).Warn("Can't parse logs remotely. lacking required privileges")
		}
	}
}

// 1. add zero counts for severity levels that didn't have any occurrences in the log
func eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal map[string]int64, mdb *sources.SourceConn) metrics.MeasurementEnvelope {
	allSeverityCounts := metrics.NewMeasurement(time.Now().UnixNano())
	for _, s := range PgSeverities {
		parsedCount, ok := eventCounts[s]
		if ok {
			allSeverityCounts[strings.ToLower(s)] = parsedCount
		} else {
			allSeverityCounts[strings.ToLower(s)] = int64(0)
		}
		parsedCount, ok = eventCountsTotal[s]
		if ok {
			allSeverityCounts[strings.ToLower(s)+"_total"] = parsedCount
		} else {
			allSeverityCounts[strings.ToLower(s)+"_total"] = int64(0)
		}
	}
	return metrics.MeasurementEnvelope{
		DBName:     mdb.Name,
		MetricName: specialMetricServerLogEventCounts,
		Data:       metrics.Measurements{allSeverityCounts},
		CustomTags: mdb.CustomTags,
	}
}

func severityToEnglish(serverLang, errorSeverity string) string {
	if serverLang == "en" {
		return errorSeverity
	}
	severityMap := PgSeveritiesLocale[serverLang]
	severityEn, ok := severityMap[errorSeverity]
	if !ok {
		return errorSeverity
	}
	return severityEn
}

func zeroEventCounts(eventCounts map[string]int64) {
	for _, severity := range PgSeverities {
		eventCounts[severity] = 0
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
		return path.Join(ld, CSVLogDefaultGlobSuffix), nil
	}
	return path.Join(dd, ld, CSVLogDefaultGlobSuffix), nil
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

func regexMatchesToMap(csvlogRegex *regexp.Regexp, matches []string) map[string]string {
	result := make(map[string]string)
	if len(matches) == 0 || csvlogRegex == nil {
		return result
	}
	for i, name := range csvlogRegex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}
	return result
}

func checkHasPrivileges(ctx context.Context, mdb *sources.SourceConn, logsDirPath string) error {
	var logFile string
	err := mdb.Conn.QueryRow(ctx, "select name from pg_ls_logdir() limit 1").Scan(&logFile)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	var dummy string
	err = mdb.Conn.QueryRow(ctx, "select pg_read_file($1, 0, 0)", logsDirPath + "/" + logFile).Scan(&dummy)
	return err
}
