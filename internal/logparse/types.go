package logparse

import (
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
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

const maxChunkSize int32 = 10 * 1024 * 1024 // 10 MB

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

// Add zero counts for severity levels that didn't have any occurrences in the log
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

func zeroEventCounts(eventCounts map[string]int64) {
	for _, severity := range PgSeverities {
		eventCounts[severity] = 0
	}
}
