package reaper

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
)

func (lp *LogParser) parseLogsRemote() error {
	var latestLogFile string
	var linesRead int // to skip over already parsed lines on Postgres server restart for example
	var firstRun = true
	var currInterval time.Duration
	var size int32
	var offset int32
	var modification time.Time
	var chunk string
	var lines []string
	var numOfLines int

	logger := log.GetLogger(lp.ctx)

	for { // detect current log file. read new chunks. re-start in case of errors
		select {
		case <-lp.ctx.Done():
			return nil
		case <-time.After(currInterval):
			if currInterval == 0 {
				currInterval = time.Second * time.Duration(lp.Interval)
			}
		}

		if latestLogFile == "" || firstRun {
			sql := "select name, size, modification from pg_ls_logdir() where name like '%csv' order by modification desc limit 1;"
			err := lp.SourceConn.Conn.QueryRow(lp.ctx, sql).Scan(&latestLogFile, &size, &modification)
			if err != nil {
				logger.Infof("No logfiles found in log dir: '%s'", lp.LogFolder)
				continue
			}
			offset = size // Seek to an end
			firstRun = false
			logger.Infof("Starting to parse logfile: '%s'", latestLogFile)
		}

		if linesRead == numOfLines && size != offset {
			logFilePath := filepath.Join(lp.LogFolder, latestLogFile)
			sizeToRead := min(maxChunkSize, size-offset)
			err := lp.SourceConn.Conn.QueryRow(lp.ctx, "select pg_read_file($1, $2, $3)", logFilePath, offset, sizeToRead).Scan(&chunk)
			offset += sizeToRead
			if err != nil {
				logger.Warningf("Failed to read logfile '%s': %s", latestLogFile, err)
				continue
			}
			lines = strings.Split(chunk, "\n")
			if sizeToRead == maxChunkSize {
				// last line may be incomplete, re-read it next time
				offset -= int32(len(lines[len(lines)-1]))
			}
			numOfLines = len(lines)
			linesRead = 0
			logger.WithField("lines", len(lines)).Info("logs fetched")
		}

		for {
			if linesRead == numOfLines {
				select {
				case <-lp.ctx.Done():
					return nil
				case <-time.After(currInterval):
				}

				var latestSize int32
				err := lp.SourceConn.Conn.QueryRow(lp.ctx, "select size, modification from pg_ls_logdir() where name = $1;", latestLogFile).Scan(&latestSize, &modification)
				if err != nil {
					logger.Warnf("Failed to read state info of logfile: '%s'", latestLogFile)
				}

				var fileName string
				if size == latestSize && offset == size || err != nil {
					sql := "select name, size from pg_ls_logdir() where modification > $1 and name like '%csv' order by modification, name limit 1;"
					err := lp.SourceConn.Conn.QueryRow(lp.ctx, sql, modification).Scan(&fileName, &latestSize)
					if err == nil && latestLogFile != fileName {
						latestLogFile = fileName
						size = latestSize
						offset = 0
						logger.Infof("Switching to new logfile: '%s'", fileName)
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

				matches := lp.LogsMatchRegex.FindStringSubmatch(line)
				if len(matches) != 0 {
					result := lp.regexMatchesToMap(matches)
					errorSeverity := result["error_severity"]
					if lp.ServerMessagesLang != "en" {
						errorSeverity = severityToEnglish(lp.ServerMessagesLang, errorSeverity)
					}

					databaseName := result["database_name"]
					if lp.SourceConn.RealDbname == databaseName {
						lp.eventCounts[errorSeverity]++
					}
					lp.eventCountsTotal[errorSeverity]++
				}
			}

			if lp.HasSendIntervalElapsed() {
				select {
				case <-lp.ctx.Done():
					return nil
				case lp.StoreCh <- lp.GetMeasurementEnvelope():
					zeroEventCounts(lp.eventCounts)
					zeroEventCounts(lp.eventCountsTotal)
					lp.lastSendTime = time.Now()
				}
			}

		} // line read loop
	} // chunk read loop

}
