package pglogparse

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

const maxChunkSize int32 = 10 * 1024 * 1024 // 10 MB

func ParseLogsRemote(
	ctx context.Context,
	mdb *sources.SourceConn,
	realDbname string,
	interval float64,
	storeCh chan<- metrics.MeasurementEnvelope,
	LogsMatchRegex *regexp.Regexp,
	LogsDirPath string,
	serverMessagesLang string,
) {
	var latestLogFile string
	var linesRead int // to skip over already parsed lines on Postgres server restart for example
	var lastSendTime time.Time                    // to storage channel
	var eventCounts = make(map[string]int64)      // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	var eventCountsTotal = make(map[string]int64) // for the whole instance
	var firstRun = true
	var currInterval time.Duration

	var size int32
	var offset int32
	var chunk string
	var lines []string
	var numOfLines int

	logger := log.GetLogger(ctx)

	for { // detect current log file. read new chunks. re-start in case of errors
		select {
		case <-ctx.Done():
			return
		case <-time.After(currInterval):
			if currInterval == 0 {
				currInterval = time.Second * time.Duration(interval)
			}
		}

		if latestLogFile == "" || firstRun {
			sql := "select name, size from pg_ls_logdir() where name like '%csv' order by modification desc limit 1;"
			err := mdb.Conn.QueryRow(ctx, sql).Scan(&latestLogFile, &size)
			if err != nil {
				logger.Infof("No logfiles found in log dir: '%s'", LogsDirPath)
				continue
			}
			offset = size // Seek to an end
			firstRun = false
			logger.Infof("Starting to parse logfile: %s", latestLogFile)
		}

		if linesRead == numOfLines && size != offset {
			logFilePath := filepath.Join(LogsDirPath, latestLogFile)
			sizeToRead := min(maxChunkSize, size - offset)
			err := mdb.Conn.QueryRow(ctx, "select pg_read_file($1, $2, $3)", logFilePath, offset, sizeToRead).Scan(&chunk)
			offset += sizeToRead
			if err != nil {
				logger.Warningf("Failed to read logfile %s: %s", latestLogFile, err)
				continue
			}
			lines = strings.Split(chunk, "\n")
			if sizeToRead == maxChunkSize {
				// last line may be incomplete, re-read it next time
				offset -= int32(len(lines[len(lines)-1]))
			}
			numOfLines = len(lines)
			linesRead = 0
		}

		for {
			if linesRead == numOfLines {
				select {
				case <-ctx.Done():
					return
				case <-time.After(currInterval):
				}

				var latestSize int32
				var modification time.Time

				err := mdb.Conn.QueryRow(ctx, "select size, modification from pg_ls_logdir() where name = $1;", latestLogFile).Scan(&latestSize, &modification)
				if err != nil {
					logger.Warn("Failed to read state info of logfile %s", latestLogFile)
					latestLogFile = ""
					break
				}

				var fileName string
				if size == latestSize && offset == size {
					sql := "select name, size from pg_ls_logdir() where modification > $1 and name like '%csv' order by modification, name limit 1;"
					err := mdb.Conn.QueryRow(ctx, sql, modification).Scan(&fileName, &latestSize)
					if err == nil && latestLogFile != fileName {
						latestLogFile = fileName
						size = latestSize
						offset = 0
						logger.Infof("Switching to new logfile: %s", fileName)
						currInterval = 0 // We already slept. It will be resetted.
						break
					}
				} else {
					size = latestSize
					currInterval = 0 // We already slept. It will be resetted.
					break
				}
			}

			if linesRead < numOfLines {
				line := lines[linesRead]
				linesRead++

				matches := LogsMatchRegex.FindStringSubmatch(line)
				if len(matches) != 0 {
					result := regexMatchesToMap(LogsMatchRegex, matches)
					errorSeverity, ok := result["error_severity"]
					if !ok {
						logger.Error("error_severity group must be defined in parse regex:", LogsMatchRegex)
						time.Sleep(time.Minute)
						break
					}
					if serverMessagesLang != "en" {
						errorSeverity = severityToEnglish(serverMessagesLang, errorSeverity)
					}

					databaseName, ok := result["database_name"]
					if !ok {
						logger.Error("database_name group must be defined in parse regex:", LogsMatchRegex)
						time.Sleep(time.Minute)
						break
					}
					if realDbname == databaseName {
						eventCounts[errorSeverity]++
					}
					eventCountsTotal[errorSeverity]++
				}
			}

			if lastSendTime.IsZero() || lastSendTime.Before(time.Now().Add(-time.Second*time.Duration(interval))) {
				select {
				case <-ctx.Done():
					return
				case storeCh <- eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal, mdb):
					zeroEventCounts(eventCounts)
					zeroEventCounts(eventCountsTotal)
					lastSendTime = time.Now()
				}
			}

		} // line read loop
	} // chunk read loop

}
