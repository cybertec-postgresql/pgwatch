package logparse

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

func ParseLogsLocal(
	ctx context.Context,
	mdb *sources.SourceConn,
	realDbname string,
	interval float64,
	storeCh chan<- metrics.MeasurementEnvelope,
	LogsMatchRegex *regexp.Regexp,
	LogsGlobPath string,
	serverMessagesLang string,
) {
	var latest, previous string
	var latestHandle *os.File
	var reader *bufio.Reader
	var linesRead int // to skip over already parsed lines on Postgres server restart for example
	var lastSendTime time.Time                    // to storage channel
	var eventCounts = make(map[string]int64)      // for the specific DB. [WARNING: 34, ERROR: 10, ...], zeroed on storage send
	var eventCountsTotal = make(map[string]int64) // for the whole instance
	var err error
	var firstRun = true
	var currInterval time.Duration

	logger := log.GetLogger(ctx)

	for { // re-try loop. re-start in case of FS errors or just to refresh host config
		select {
		case <-ctx.Done():
			return
		case <-time.After(currInterval):
			if currInterval == 0 {
				currInterval = time.Second * time.Duration(interval)
			}
		}

		if latest == "" || firstRun {
			globMatches, err := filepath.Glob(LogsGlobPath)
			if err != nil || len(globMatches) == 0 {
				logger.Infof("No logfiles found to parse from glob '%s'", LogsGlobPath)
				continue
			}

			logger.Debugf("Found %v logfiles from glob pattern, picking the latest", len(globMatches))
			if len(globMatches) > 1 {
				// find latest timestamp
				latest, _ = getFileWithLatestTimestamp(globMatches)
				if latest == "" {
					logger.Warningf("Could not determine the latest logfile")
					continue
				}

			} else if len(globMatches) == 1 {
				latest = globMatches[0]
			}
			logger.Infof("Starting to parse logfile: %s ", latest)
		}

		if latestHandle == nil {
			latestHandle, err = os.Open(latest)
			if err != nil {
				logger.Warningf("Failed to open logfile %s: %s", latest, err)
				continue
			}
			defer latestHandle.Close()
			reader = bufio.NewReader(latestHandle)
			if previous == latest && linesRead > 0 { // handle postmaster restarts
				i := 1
				for i <= linesRead {
					_, err = reader.ReadString('\n')
					if err == io.EOF && i < linesRead {
						logger.Warningf("Failed to open logfile %s: %s", latest, err)
						linesRead = 0
						break
					} else if err != nil {
						logger.Warningf("Failed to skip %d logfile lines for %s, there might be duplicates reported. Error: %s", linesRead, latest, err)
						linesRead = i
						break
					}
					i++
				}
				logger.Debugf("Skipped %d already processed lines from %s", linesRead, latest)
			} else if firstRun { // seek to end
				_, _ = latestHandle.Seek(0, 2)
				firstRun = false
			}
		}

		for {
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				logger.Warningf("Failed to read logfile %s: %s", latest, err)
				_ = latestHandle.Close()
				latestHandle = nil
				break
			}

			if err == io.EOF {
				// // EOF reached, wait for new files to be added
				select {
				case <-ctx.Done():
					return
				case <-time.After(currInterval):
				}
				// check for newly opened logfiles
				file, _ := getFileWithNextModTimestamp(LogsGlobPath, latest)
				if file != "" {
					previous = latest
					latest = file
					_ = latestHandle.Close()
					latestHandle = nil
					logger.Infof("Switching to new logfile: %s", file)
					linesRead = 0
					break
				}
			} else {
				linesRead++
			}

			if err == nil && line != "" {
				matches := LogsMatchRegex.FindStringSubmatch(line)
				if len(matches) == 0 {
					goto send_to_storage_if_needed
				}
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

		send_to_storage_if_needed:
			if lastSendTime.IsZero() || lastSendTime.Before(time.Now().Add(-time.Second*time.Duration(interval))) {
				logger.Debugf("Sending log event counts for last interval to storage channel. Local eventcounts: %+v, global eventcounts: %+v", eventCounts, eventCountsTotal)
				select {
				case <-ctx.Done():
					return
				case storeCh <- eventCountsToMetricStoreMessages(eventCounts, eventCountsTotal, mdb):
					zeroEventCounts(eventCounts)
					zeroEventCounts(eventCountsTotal)
					lastSendTime = time.Now()
				}
			}

		} // file read loop
	} // config loop

}

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

	for _, f := range files {
		if f == currentFile {
			continue
		}
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		if (nextMod.IsZero() || fi.ModTime().Before(nextMod)) && fi.ModTime().After(fiCurrent.ModTime()) {
			nextMod = fi.ModTime()
			nextFile = f
		}
	}
	return nextFile, nil
}
