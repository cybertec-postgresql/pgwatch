package main

import (
	"bufio"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

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

func getFileWithLatestTimestamp(files []string) (string, time.Time) {
	var maxDate time.Time
	var latest string

	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			logger.Errorf("Failed to stat() file %s: %s", f, err)
			continue
		}
		if fi.ModTime().After(maxDate) {
			latest = f
			maxDate = fi.ModTime()
		}
	}
	return latest, maxDate
}

func getFileWithNextModTimestamp(dbUniqueName, logsGlobPath, currentFile string) (string, time.Time) {
	var nextFile string
	var nextMod time.Time

	files, err := filepath.Glob(logsGlobPath)
	if err != nil {
		logger.Error("[%s] Error globbing \"%s\"...", dbUniqueName, logsGlobPath)
		return "", time.Now()
	}

	fiCurrent, err := os.Stat(currentFile)
	if err != nil {
		logger.Errorf("Failed to stat() currentFile %s: %s", currentFile, err)
		return "", time.Now()
	}
	//log.Debugf("Stat().ModTime() for %s: %v", currentFile, fiCurrent.ModTime())

	for _, f := range files {
		if f == currentFile {
			continue
		}
		fi, err := os.Stat(f)
		if err != nil {
			logger.Errorf("Failed to stat() currentFile %s: %s", f, err)
			continue
		}
		//log.Debugf("Stat().ModTime() for %s: %v", f, fi.ModTime())
		if (nextMod.IsZero() || fi.ModTime().Before(nextMod)) && fi.ModTime().After(fiCurrent.ModTime()) {
			nextMod = fi.ModTime()
			nextFile = f
		}
	}
	return nextFile, nextMod
}

// 1. add zero counts for severity levels that didn't have any occurrences in the log
func eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal map[string]int64, mdb MonitoredDatabase) []metrics.MeasurementMessage {
	allSeverityCounts := make(metrics.Measurement)
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
	return []metrics.MeasurementMessage{{DBName: mdb.DBUniqueName, DBType: mdb.DBType,
		MetricName: specialMetricServerLogEventCounts, Data: metrics.Measurements{allSeverityCounts}, CustomTags: mdb.CustomTags}}
}

func logparseLoop(dbUniqueName, metricName string, configMap map[string]float64, controlCh <-chan ControlMessage, storeCh chan<- []metrics.MeasurementMessage) {

	var latest, previous, realDbname, serverMessagesLang string
	var latestHandle *os.File
	var reader *bufio.Reader
	var linesRead = 0 // to skip over already parsed lines on Postgres server restart for example
	var logsMatchRegex, logsMatchRegexPrev, logsGlobPath string
	var lastSendTime time.Time                    // to storage channel
	var lastConfigRefreshTime time.Time           // MonitoredDatabase info
	var eventCounts = make(map[string]int64)      // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	var eventCountsTotal = make(map[string]int64) // for the whole instance
	var mdb MonitoredDatabase
	var hostConfig HostConfigAttrs
	var config = configMap
	var interval float64
	var err error
	var firstRun = true
	var csvlogRegex *regexp.Regexp

	for { // re-try loop. re-start in case of FS errors or just to refresh host config
		select {
		case msg := <-controlCh:
			logger.Debug("got control msg", dbUniqueName, metricName, msg)
			if msg.Action == gathererStatusStart {
				config = msg.Config
				interval = config[metricName]
				logger.Debug("started MetricGathererLoop for ", dbUniqueName, metricName, " interval:", interval)
			} else if msg.Action == gathererStatusStop {
				logger.Debug("exiting MetricGathererLoop for ", dbUniqueName, metricName, " interval:", interval)
				return
			}
		default:
			if interval == 0 {
				interval = config[metricName]
			}
		}

		if lastConfigRefreshTime.IsZero() || lastConfigRefreshTime.Add(time.Second*time.Duration(opts.Connection.ServersRefreshLoopSeconds)).Before(time.Now()) {
			mdb, err = GetMonitoredDatabaseByUniqueName(dbUniqueName)
			if err != nil {
				logger.Errorf("[%s] Failed to refresh monitored DBs info: %s", dbUniqueName, err)
				time.Sleep(60 * time.Second)
				continue
			}
			hostConfig = mdb.HostConfig
			logger.Debugf("[%s] Refreshed hostConfig: %+v", dbUniqueName, hostConfig)
		}

		dbPgVersionMapLock.RLock()
		realDbname = dbPgVersionMap[dbUniqueName].RealDbname // to manage 2 sets of event counts - monitored DB + global
		dbPgVersionMapLock.RUnlock()

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
			logsGlobPath = tryDetermineLogFolder(mdb)
			if logsGlobPath == "" {
				logger.Warningf("[%s] Could not determine Postgres logs parsing folder. Configured logs_glob_path = %s", dbUniqueName, logsGlobPath)
				time.Sleep(60 * time.Second)
				continue
			}
		}
		serverMessagesLang = tryDetermineLogMessagesLanguage(mdb)
		if serverMessagesLang == "" {
			logger.Warningf("[%s] Could not determine language (lc_collate) used for server logs, cannot parse logs...", dbUniqueName)
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
		readLoopStart := time.Now()

		for {
			if readLoopStart.Add(time.Second * time.Duration(opts.Connection.ServersRefreshLoopSeconds)).Before(time.Now()) {
				break // refresh config
			}
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
				file, _ := getFileWithNextModTimestamp(dbUniqueName, logsGlobPath, latest)
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

				result := RegexMatchesToMap(csvlogRegex, matches)
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
				ZeroEventCounts(eventCounts)
				ZeroEventCounts(eventCountsTotal)
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
		logger.Warningf("Failed to map severity '%s' to english from language '%s'", errorSeverity, serverLang)
		return errorSeverity
	}
	return severityEn
}

func ZeroEventCounts(eventCounts map[string]int64) {
	for _, severity := range PgSeverities {
		eventCounts[severity] = 0
	}
}

func tryDetermineLogFolder(mdb MonitoredDatabase) string {
	sql := `select current_setting('data_directory') as dd, current_setting('log_directory') as ld`

	logger.Infof("[%s] Trying to determine server logs folder via SQL as host_config.logs_glob_path not specified...", mdb.DBUniqueName)
	data, err := DBExecReadByDbUniqueName(mainContext, mdb.DBUniqueName, sql)
	if err != nil {
		logger.Errorf("[%s] Failed to query data_directory and log_directory settings...are you superuser or have pg_monitor grant?", mdb.DBUniqueName)
		return ""
	}
	ld := data[0]["ld"].(string)
	dd := data[0]["dd"].(string)
	if strings.HasPrefix(ld, "/") {
		// we have a full path we can use
		return path.Join(ld, CSVLogDefaultGlobSuffix)
	}
	return path.Join(dd, ld, CSVLogDefaultGlobSuffix)
}

func tryDetermineLogMessagesLanguage(mdb MonitoredDatabase) string {
	sql := `select current_setting('lc_messages')::varchar(2) as lc_messages;`

	logger.Debugf("[%s] Trying to determine server log messages language...", mdb.DBUniqueName)
	data, err := DBExecReadByDbUniqueName(mainContext, mdb.DBUniqueName, sql)
	if err != nil {
		logger.Errorf("[%s] Failed to lc_messages settings: %s", mdb.DBUniqueName, err)
		return ""
	}
	lang := data[0]["lc_messages"].(string)
	if lang == "en" {
		return lang
	}
	_, ok := PgSeveritiesLocale[lang]
	if !ok {
		logger.Warningf("[%s] Server log language '%s' is not yet mapped, assuming severities in english: %+v", mdb.DBUniqueName, lang, PgSeverities)
		return "en"
	}
	return lang
}

func RegexMatchesToMap(csvlogRegex *regexp.Regexp, matches []string) map[string]string {
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
