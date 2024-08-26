package metrics

import (
	"bufio"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
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

func getFileWithLatestTimestamp(files []string) (string, error) {
	var maxDate time.Time
	var latest string

	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			return "", err
		}
		if fi.ModTime().After(maxDate) {
			latest = f
			maxDate = fi.ModTime()
		}
	}
	return latest, nil
}

func getFileWithNextModTimestamp(logsGlobPath, currentFile string) (string, error) {
	var nextFile string
	var nextMod time.Time

	files, err := filepath.Glob(logsGlobPath)
	if err != nil {
		return "", err
	}

	fiCurrent, err := os.Stat(currentFile)
	if err != nil {
		return "", err
	}
	//log.Debugf("Stat().ModTime() for %s: %v", currentFile, fiCurrent.ModTime())

	for _, f := range files {
		if f == currentFile {
			continue
		}
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		//log.Debugf("Stat().ModTime() for %s: %v", f, fi.ModTime())
		if (nextMod.IsZero() || fi.ModTime().Before(nextMod)) && fi.ModTime().After(fiCurrent.ModTime()) {
			nextMod = fi.ModTime()
			nextFile = f
		}
	}
	return nextFile, nil
}

// 1. add zero counts for severity levels that didn't have any occurrences in the log
func eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal map[string]int64, mdb *sources.MonitoredDatabase) []MeasurementMessage {
	allSeverityCounts := make(Measurement)
	for _, s := range PgSeverities {
		parsedCount, ok := eventCounts[s]
		if ok {
			allSeverityCounts[strings.ToLower(s)] = parsedCount
		} else {
			allSeverityCounts[strings.ToLower(s)] = 0
		}
		parsedCount, ok = eventCountsTotal[s]
		if ok {
			allSeverityCounts[strings.ToLower(s)+"_total"] = parsedCount
		} else {
			allSeverityCounts[strings.ToLower(s)+"_total"] = 0
		}
	}
	allSeverityCounts["epoch_ns"] = time.Now().UnixNano()
	return []MeasurementMessage{{
		DBName:     mdb.Name,
		SourceType: string(mdb.Kind),
		MetricName: specialMetricServerLogEventCounts,
		Data:       Measurements{allSeverityCounts},
		CustomTags: mdb.CustomTags,
	}}
}

func ParseLogs(ctx context.Context, conn db.PgxIface, mdb *sources.MonitoredDatabase, realDbname, metricName string, configMap map[string]float64, storeCh chan<- []MeasurementMessage) {

	var latest, previous, serverMessagesLang string
	var latestHandle *os.File
	var reader *bufio.Reader
	var linesRead = 0 // to skip over already parsed lines on Postgres server restart for example
	var logsMatchRegex, logsMatchRegexPrev, logsGlobPath string
	var lastSendTime time.Time                    // to storage channel
	var eventCounts = make(map[string]int64)      // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	var eventCountsTotal = make(map[string]int64) // for the whole instance
	var hostConfig sources.HostConfigAttrs
	var config = configMap
	var interval float64
	var err error
	var firstRun = true
	var csvlogRegex *regexp.Regexp
	dbUniqueName := realDbname
	logger := log.GetLogger(ctx)
	for { // re-try loop. re-start in case of FS errors or just to refresh host config
		select {
		case <-ctx.Done():
			return
		default:
			if interval == 0 {
				interval = config[metricName]
			}
		}

		if hostConfig.LogsMatchRegex != "" {
			logsMatchRegex = hostConfig.LogsMatchRegex
		}
		if logsMatchRegex == "" {
			logger.Debugf("[%s] Log parsing enabled with default CSVLOG regex", dbUniqueName)
			logsMatchRegex = CSVLogDefaultRegEx
		}
		if hostConfig.LogsGlobPath != "" {
			logsGlobPath = hostConfig.LogsGlobPath
		}
		if logsGlobPath == "" {
			logsGlobPath, err = tryDetermineLogFolder(ctx, conn)
			if err != nil {
				logger.WithError(err).Printf("[%s] Could not determine Postgres logs parsing folder. Configured logs_glob_path = %s", dbUniqueName, logsGlobPath)
				time.Sleep(60 * time.Second)
				continue
			}
		}
		serverMessagesLang, err = tryDetermineLogMessagesLanguage(ctx, conn)
		if err != nil {
			logger.WithError(err).Warningf("[%s] Could not determine language (lc_collate) used for server logs, cannot parse logs...", dbUniqueName)
			time.Sleep(60 * time.Second)
			continue
		}

		if logsMatchRegexPrev != logsMatchRegex { // avoid regex recompile if no changes
			csvlogRegex, err = regexp.Compile(logsMatchRegex)
			if err != nil {
				logger.Errorf("[%s] Invalid regex: %s", dbUniqueName, logsMatchRegex)
				time.Sleep(60 * time.Second)
				continue
			}
			logger.Infof("[%s] Changing logs parsing regex to: %s", dbUniqueName, logsMatchRegex)
			logsMatchRegexPrev = logsMatchRegex
		}

		logger.Debugf("[%s] Considering log files determined by glob pattern: %s", dbUniqueName, logsGlobPath)

		if latest == "" || firstRun {

			globMatches, err := filepath.Glob(logsGlobPath)
			if err != nil || len(globMatches) == 0 {
				logger.Infof("[%s] No logfiles found to parse from glob '%s'. Sleeping 60s...", dbUniqueName, logsGlobPath)
				time.Sleep(60 * time.Second)
				continue
			}

			logger.Debugf("[%s] Found %v logfiles from glob pattern, picking the latest", dbUniqueName, len(globMatches))
			if len(globMatches) > 1 {
				// find latest timestamp
				latest, _ = getFileWithLatestTimestamp(globMatches)
				if latest == "" {
					logger.Warningf("[%s] Could not determine the latest logfile. Sleeping 60s...", dbUniqueName)
					time.Sleep(60 * time.Second)
					continue
				}

			} else if len(globMatches) == 1 {
				latest = globMatches[0]
			}
			logger.Infof("[%s] Starting to parse logfile: %s ", dbUniqueName, latest)
		}

		if latestHandle == nil {
			latestHandle, err = os.Open(latest)
			if err != nil {
				logger.Warningf("[%s] Failed to open logfile %s: %s. Sleeping 60s...", dbUniqueName, latest, err)
				time.Sleep(60 * time.Second)
				continue
			}
			reader = bufio.NewReader(latestHandle)
			if previous == latest && linesRead > 0 { // handle postmaster restarts
				i := 1
				for i <= linesRead {
					_, err = reader.ReadString('\n')
					if err == io.EOF && i < linesRead {
						logger.Warningf("[%s] Failed to open logfile %s: %s. Sleeping 60s...", dbUniqueName, latest, err)
						linesRead = 0
						break
					} else if err != nil {
						logger.Warningf("[%s] Failed to skip %d logfile lines for %s, there might be duplicates reported. Error: %s", dbUniqueName, linesRead, latest, err)
						time.Sleep(60 * time.Second)
						linesRead = i
						break
					}
					i++
				}
				logger.Debug("[%s] Skipped %d already processed lines from %s", dbUniqueName, linesRead, latest)
			} else if firstRun { // seek to end
				_, _ = latestHandle.Seek(0, 2)
				firstRun = false
			}
		}

		var eofSleepMillis = 0
		// readLoopStart := time.Now()

		for {
			// if readLoopStart.Add(time.Second * time.Duration(opts.Source.Refresh)).Before(time.Now()) {
			// 	break // refresh config
			// }
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				logger.Warningf("[%s] Failed to read logfile %s: %s. Sleeping 60s...", dbUniqueName, latest, err)
				err = latestHandle.Close()
				if err != nil {
					logger.Warningf("[%s] Failed to close logfile %s properly: %s", dbUniqueName, latest, err)
				}
				latestHandle = nil
				time.Sleep(60 * time.Second)
				break
			}

			if err == io.EOF {
				//log.Debugf("[%s] EOF reached for logfile %s", dbUniqueName, latest)
				if eofSleepMillis < 5000 && float64(eofSleepMillis) < interval*1000 {
					eofSleepMillis += 100 // progressively sleep more if nothing going on but not more that 5s or metric interval
				}
				time.Sleep(time.Millisecond * time.Duration(eofSleepMillis))

				// check for newly opened logfiles
				file, _ := getFileWithNextModTimestamp(logsGlobPath, latest)
				if file != "" {
					previous = latest
					latest = file
					err = latestHandle.Close()
					latestHandle = nil
					if err != nil {
						logger.Warningf("[%s] Failed to close logfile %s properly: %s", dbUniqueName, latest, err)
					}
					logger.Infof("[%s] Switching to new logfile: %s", dbUniqueName, file)
					linesRead = 0
					break
				}
			} else {
				eofSleepMillis = 0
				linesRead++
			}

			if err == nil && line != "" {

				matches := csvlogRegex.FindStringSubmatch(line)
				if len(matches) == 0 {
					//log.Debugf("[%s] No logline regex match for line:", dbUniqueName) // normal case actually for queries spanning multiple loglines
					//log.Debugf(line)
					goto send_to_storage_if_needed
				}

				result := regexMatchesToMap(csvlogRegex, matches)
				//log.Debugf("RegexMatchesToMap: %+v", result)
				errorSeverity, ok := result["error_severity"]
				if !ok {
					logger.Error("error_severity group must be defined in parse regex:", csvlogRegex)
					time.Sleep(time.Minute)
					break
				}
				if serverMessagesLang != "en" {
					errorSeverity = severityToEnglish(serverMessagesLang, errorSeverity)
				}

				databaseName, ok := result["database_name"]
				if !ok {
					logger.Error("database_name group must be defined in parse regex:", csvlogRegex)
					time.Sleep(time.Minute)
					break
				}
				if realDbname == databaseName {
					eventCounts[errorSeverity]++
				}
				eventCountsTotal[errorSeverity]++
			}

		send_to_storage_if_needed:
			if lastSendTime.IsZero() || lastSendTime.Before(time.Now().Add(-1*time.Second*time.Duration(interval))) {
				logger.Debugf("[%s] Sending log event counts for last interval to storage channel. Local eventcounts: %+v, global eventcounts: %+v", dbUniqueName, eventCounts, eventCountsTotal)
				metricStoreMessages := eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal, mdb)
				storeCh <- metricStoreMessages
				zeroEventCounts(eventCounts)
				zeroEventCounts(eventCountsTotal)
				lastSendTime = time.Now()
			}

		} // file read loop
	} // config loop

}

func severityToEnglish(serverLang, errorSeverity string) string {
	//log.Debug("severityToEnglish", serverLang, errorSeverity)
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
