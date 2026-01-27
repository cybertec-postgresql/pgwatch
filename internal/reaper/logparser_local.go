package reaper

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
)

func (lp *LogParser) parseLogsLocal() error {
	var latest, previous string
	var latestHandle *os.File
	var reader *bufio.Reader
	var linesRead int // to skip over already parsed lines on Postgres server restart for example
	var err error
	var firstRun = true
	var currInterval time.Duration

	// current byte offset for the file currently opened; kept in local variable while file is open
	var offset int64

	logger := log.GetLogger(lp.ctx)
	logsGlobPath := filepath.Join(lp.LogFolder, csvLogDefaultGlobSuffix)

	for { // re-try loop. re-start in case of FS errors or just to refresh host config
		select {
		case <-lp.ctx.Done():
			return nil
		case <-time.After(currInterval):
			if currInterval == 0 {
				currInterval = time.Second * time.Duration(lp.Interval)
			}
		}

		if latest == "" || firstRun {
			globMatches, err := filepath.Glob(logsGlobPath)
			if err != nil || len(globMatches) == 0 {
				logger.Infof("No logfiles found to parse from glob '%s'", logsGlobPath)
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

			// Determine the offset to resume from:
			// 1) If we have a saved offset for this filename, use it (resume).
			// 2) Else if firstRun, seek to end and store that offset so we don't read historical content.
			// 3) Else (no saved offset, not first run) fall back to previous behavior: try skipping already-parsed lines.
			offset = 0
			if v, ok := lp.readOffsets[latest]; ok && v > 0 {
				// use saved offset
				fi, ferr := latestHandle.Stat()
				if ferr == nil {
					// if saved offset is beyond current file size (truncated), reset to 0
					if v > fi.Size() {
						logger.Debugf("Saved offset %d beyond filesize %d for %s, resetting to 0", v, fi.Size(), latest)
						offset = 0
					} else {
						offset = v
					}
				} else {
					offset = v
				}
				if _, err = latestHandle.Seek(offset, io.SeekStart); err != nil {
					logger.Warningf("Failed to seek logfile %s to offset %d: %s", latest, offset, err)
					// continue with offset = 0
					offset = 0
					_, _ = latestHandle.Seek(0, io.SeekStart)
				}
			} else if previous == latest && linesRead > 0 { // handle postmaster restarts (legacy fallback)
				reader = bufio.NewReader(latestHandle)
				i := 1
				for i <= linesRead {
					s, rerr := reader.ReadString('\n')
					if rerr == io.EOF && i < linesRead {
						logger.Warningf("Failed to open logfile %s: %s", latest, rerr)
						linesRead = 0
						offset = 0
						break
					} else if rerr != nil {
						logger.Warningf("Failed to skip %d logfile lines for %s, there might be duplicates reported. Error: %s", linesRead, latest, rerr)
						linesRead = i
						// update offset by what we skipped so far
						offset += int64(len(s))
						break
					}
					offset += int64(len(s))
					i++
				}
				logger.Debugf("Skipped %d already processed lines from %s", linesRead, latest)
				lp.readOffsets[latest] = offset
				// Ensure file is positioned at offset for subsequent reads
				_, _ = latestHandle.Seek(offset, io.SeekStart)
			} else if firstRun { // seek to end
				off, _ := latestHandle.Seek(0, 2)
				offset = off
				firstRun = false
				lp.readOffsets[latest] = offset
			}

			// create a reader positioned at the chosen offset
			reader = bufio.NewReader(latestHandle)
			// ensure linesRead is reset for a newly opened file
			linesRead = 0
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
				// update stored offset to current in-memory offset
				lp.readOffsets[latest] = offset

				// EOF reached, wait for new files to be added / new data appended
				select {
				case <-lp.ctx.Done():
					return nil
				case <-time.After(currInterval):
				}
				// check for newly opened logfiles
				file, _ := getFileWithNextModTimestamp(logsGlobPath, latest)
				if file != "" {
					previous = latest
					latest = file
					_ = latestHandle.Close()
					latestHandle = nil
					logger.Infof("Switching to new logfile: %s", file)
					// reset offset for the new file; it will be set when the file is opened
					offset = 0
					linesRead = 0
					break
				}
			} else {
				// successfully read a line; advance offset and counters
				offset += int64(len(line))
				lp.readOffsets[latest] = offset
				linesRead++
			}

			if err == nil && line != "" {
				matches := lp.LogsMatchRegex.FindStringSubmatch(line)
				if len(matches) == 0 {
					goto send_to_storage_if_needed
				}
				result := lp.regexMatchesToMap(matches)
				errorSeverity, ok := result["error_severity"]
				if !ok {
					logger.Error("error_severity group must be defined in parse regex:", lp.LogsMatchRegex)
					time.Sleep(time.Minute)
					break
				}
				if lp.ServerMessagesLang != "en" {
					errorSeverity = severityToEnglish(lp.ServerMessagesLang, errorSeverity)
				}

				databaseName, ok := result["database_name"]
				if !ok {
					logger.Error("database_name group must be defined in parse regex:", lp.LogsMatchRegex)
					time.Sleep(time.Minute)
					break
				}
				if lp.SourceConn.RealDbname == databaseName {
					lp.eventCounts[errorSeverity]++
				}
				lp.eventCountsTotal[errorSeverity]++
			}

		send_to_storage_if_needed:
			if lp.HasSendIntervalElapsed() {
				logger.Debugf("Sending log event counts for last interval to storage channel. Local eventcounts: %+v, global eventcounts: %+v", lp.eventCounts, lp.eventCountsTotal)
				select {
				case <-lp.ctx.Done():
					return nil
				case lp.StoreCh <- lp.GetMeasurementEnvelope():
					zeroEventCounts(lp.eventCounts)
					zeroEventCounts(lp.eventCountsTotal)
					lp.lastSendTime = time.Now()
				}
			}

		} // file read loop
	} // config loop

}

// Helper: pick the file with the latest modification time from a list
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

// Helper: find the next file that has modification time after currentFile's mod time
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
