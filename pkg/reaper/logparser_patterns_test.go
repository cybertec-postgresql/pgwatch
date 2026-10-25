package reaper

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/pashagolub/pglogwatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logEvent is one record to render into a fixture log.
type logEvent struct {
	severity string // ERROR, LOG, ...
	state    string // five characters, or empty
	db       string
	message  string
}

// patternsLinePrefix carries %e so that stderr records have a SQLSTATE too.
const patternsLinePrefix = `%m [%p] %e %u@%d `

const fixtureTime = "2023-12-01 10:00:00.000 UTC"

// sqlstateOrSuccess is what csvlog and %e write for a record without a code.
func sqlstateOrSuccess(s string) string {
	if s == "" {
		return "00000"
	}
	return s
}

func renderJSON(t testing.TB, events []logEvent) []byte {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, e := range events {
		require.NoError(t, enc.Encode(struct {
			Timestamp string `json:"timestamp"`
			User      string `json:"user"`
			Dbname    string `json:"dbname,omitempty"`
			Pid       int    `json:"pid"`
			Severity  string `json:"error_severity"`
			State     string `json:"state_code"`
			Message   string `json:"message"`
		}{fixtureTime, "app", e.db, 4242, e.severity, sqlstateOrSuccess(e.state), e.message}))
	}
	return b.Bytes()
}

// renderCSV writes the 26-column layout of PostgreSQL 14 and later.
func renderCSV(t testing.TB, events []logEvent) []byte {
	t.Helper()
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	for _, e := range events {
		row := make([]string, 26)
		row[0], row[1], row[2], row[3] = fixtureTime, "app", e.db, "4242"
		row[11], row[12], row[13] = e.severity, sqlstateOrSuccess(e.state), e.message
		row[22], row[23], row[25] = "psql", "client backend", "0"
		require.NoError(t, w.Write(row))
	}
	w.Flush()
	require.NoError(t, w.Error())
	return b.Bytes()
}

// renderStderr writes patternsLinePrefix lines. A stderr record ends where the
// next begins, so a sentinel of severity INFO closes the last one.
func renderStderr(_ testing.TB, events []logEvent) []byte {
	var b bytes.Buffer
	for _, e := range append(events, logEvent{severity: "INFO", message: "sentinel"}) {
		fmt.Fprintf(&b, "%s [4242] %s app@%s %s:  %s\n", fixtureTime, sqlstateOrSuccess(e.state), e.db,
			e.severity, strings.ReplaceAll(e.message, "\n", "\n\t"))
	}
	return b.Bytes()
}

var fixtureFormats = []struct {
	name   string
	cfg    logConfig
	render func(testing.TB, []logEvent) []byte
}{
	{"stderr", logConfig{LinePrefix: patternsLinePrefix}, renderStderr},
	{"csvlog", logConfig{CSVDestination: true}, renderCSV},
	{"jsonlog", logConfig{JSONDestination: true}, renderJSON},
}

func newStreamParser(t testing.TB, cfg logConfig) (*logParser, chan metrics.MeasurementEnvelope) {
	t.Helper()
	ch := make(chan metrics.MeasurementEnvelope, 32)
	return &logParser{
		ctx:              t.Context(),
		logConfig:        &cfg,
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "patterns-test"}},
		realDbname:       "shop",
		Interval:         time.Minute,
		StoreCh:          ch,
		eventCounts:      make(map[string]int64),
		eventCountsTotal: make(map[string]int64),
	}, ch
}

// benchEvents is a mix of repeated and distinct patterns, with and without
// durations, quoting and SQLSTATEs.
func benchEvents(n int) []logEvent {
	events := make([]logEvent, 0, n)
	for i := range n {
		var e logEvent
		switch i % 5 {
		case 0:
			e = logEvent{"LOG", "", "shop", fmt.Sprintf("duration: %d.%03d ms  statement: SELECT * FROM orders WHERE id = %d AND note = 'x%d'", i%97, i%1000, i, i)}
		case 1:
			e = logEvent{"ERROR", "42P01", "shop", fmt.Sprintf(`relation "orders_%d" does not exist`, i%7)}
		case 2:
			e = logEvent{"LOG", "", "", fmt.Sprintf("checkpoint complete: wrote %d buffers (%d.%d%%)", i, i%100, i%10)}
		case 3:
			e = logEvent{"WARNING", "01000", "other", fmt.Sprintf("there is already a transaction in progress %d", i)}
		default:
			e = logEvent{"LOG", "", "shop", fmt.Sprintf("connection authorized: user=u%d database=shop application_name=psql", i)}
		}
		events = append(events, e)
	}
	return events
}

// BenchmarkConsume measures the parse loop over each format, with
// server_log_event_counts alone and with server_log_patterns as well.
func BenchmarkConsume(b *testing.B) {
	events := benchEvents(10_000)
	for _, f := range fixtureFormats {
		input := f.render(b, events)
		for _, patterns := range []bool{false, true} {
			name := f.name + "/event_counts"
			if patterns {
				name += "+patterns"
			}
			b.Run(name, func(b *testing.B) {
				lp, _ := newStreamParser(b, f.cfg)
				if patterns {
					lp.patterns = newPatternTally(time.Minute)
				}
				b.SetBytes(int64(len(input)))
				b.ReportAllocs()
				for b.Loop() {
					if err := lp.consume(bytes.NewReader(input)); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// consumePatterns runs input through consume and returns the
// server_log_patterns rows and the server_log_event_counts row it sends.
func consumePatterns(t *testing.T, cfg logConfig, input []byte) (patterns metrics.Measurements, counts metrics.Measurement) {
	t.Helper()
	lp, ch := newStreamParser(t, cfg)
	lp.patterns = newPatternTally(time.Minute)
	require.NoError(t, lp.consume(bytes.NewReader(input)))
	lp.sendDue(true)
	close(ch)
	for env := range ch {
		switch env.MetricName {
		case specialMetricServerLogPatterns:
			patterns = env.Data
		case specialMetricServerLogEventCounts:
			counts = env.Data[0]
		}
	}
	return
}

func rowsOfSeverity(rows metrics.Measurements, severity string) metrics.Measurements {
	var out metrics.Measurements
	for _, r := range rows {
		if r["tag_severity"] == severity {
			out = append(out, r)
		}
	}
	return out
}

func sumCount(rows metrics.Measurements) (n int64) {
	for _, r := range rows {
		n += r["count"].(int64)
	}
	return
}

func repeatEvent(e logEvent, n int) []logEvent {
	out := make([]logEvent, n)
	for i := range out {
		out[i] = e
	}
	return out
}

// AC-001, with one pattern id per event whatever the format (GUD-002).
func TestPatternsGroupRecordsByTemplate(t *testing.T) {
	events := append(
		repeatEvent(logEvent{"ERROR", "42P01", "shop", `relation "x" does not exist`}, 3),
		repeatEvent(logEvent{"ERROR", "42P01", "shop", `relation "y" does not exist`}, 2)...)
	var first metrics.Measurements
	for _, f := range fixtureFormats {
		t.Run(f.name, func(t *testing.T) {
			rows, _ := consumePatterns(t, f.cfg, f.render(t, events))
			errs := rowsOfSeverity(rows, "error")
			require.Len(t, errs, 2)
			assert.Equal(t, int64(3), errs[0]["count"])
			assert.Equal(t, `relation "x" does not exist`, errs[0]["template"])
			assert.Equal(t, int64(2), errs[1]["count"])
			assert.Equal(t, `relation "y" does not exist`, errs[1]["template"])
			for _, r := range errs {
				assert.Regexp(t, `^[0-9a-f]{16}$`, r["tag_pattern_id"])
				assert.Equal(t, "42P01", r["tag_sqlstate"])
				assert.Equal(t, "shop", r["tag_database"])
				assert.Equal(t, int64(pglogwatch.TemplateRulesVersion), r["rules_version"])
				assert.Nil(t, r["duration_sum_ms"])
				assert.Nil(t, r["duration_max_ms"])
			}
			if first == nil {
				first = errs
				return
			}
			for i := range errs {
				assert.Equal(t, first[i]["tag_pattern_id"], errs[i]["tag_pattern_id"])
			}
		})
	}
}

// csvlog and jsonlog escape quotes differently; unquoting first gives one id.
func TestPatternsUnquoteBeforeTemplating(t *testing.T) {
	events := []logEvent{{"ERROR", "42601", "shop", `syntax error at or near "selec" in 'x' \ y`}}
	var ids []any
	for _, f := range fixtureFormats {
		rows, _ := consumePatterns(t, f.cfg, f.render(t, events))
		errs := rowsOfSeverity(rows, "error")
		require.Len(t, errs, 1, f.name)
		ids = append(ids, errs[0]["tag_pattern_id"])
	}
	assert.Equal(t, ids[0], ids[1])
	assert.Equal(t, ids[0], ids[2])
}

// AC-002.
func TestPatternsSumAndMaxDurations(t *testing.T) {
	events := make([]logEvent, 100)
	for i := range events {
		events[i] = logEvent{"LOG", "", "shop", fmt.Sprintf(
			"duration: %d.%03d ms  statement: SELECT * FROM t WHERE id = %d", (i+1)/4, (i+1)%4*250, i)}
	}
	for _, f := range fixtureFormats {
		t.Run(f.name, func(t *testing.T) {
			rows, _ := consumePatterns(t, f.cfg, f.render(t, events))
			logs := rowsOfSeverity(rows, "log")
			require.Len(t, logs, 1)
			assert.Equal(t, int64(100), logs[0]["count"])
			assert.Equal(t, "duration: $1 statement: SELECT * FROM t WHERE id = $2", logs[0]["template"])
			assert.InDelta(t, 1262.5, logs[0]["duration_sum_ms"], 1e-9) // 0.25 ms × (1 + … + 100)
			assert.InDelta(t, 25.0, logs[0]["duration_max_ms"], 1e-9)
		})
	}
}

// AC-003.
func TestPatternsAreCappedPerSeverity(t *testing.T) {
	var events []logEvent
	for i := range 105 {
		events = append(events, repeatEvent(logEvent{"ERROR", "42P01", "shop", fmt.Sprintf(`relation "t%d" does not exist`, i)}, i%3+1)...)
	}
	errors := len(events)
	events = append(events, logEvent{"LOG", "", "", "checkpoint starting: time"})
	for _, f := range fixtureFormats {
		t.Run(f.name, func(t *testing.T) {
			rows, counts := consumePatterns(t, f.cfg, f.render(t, events))
			errs := rowsOfSeverity(rows, "error")
			require.Len(t, errs, patternRowsPerSeverity+1)

			other := errs[len(errs)-1]
			assert.Equal(t, otherPatternID, other["tag_pattern_id"])
			assert.Equal(t, "", other["tag_sqlstate"])
			assert.Equal(t, "", other["tag_database"])
			assert.Nil(t, other["template"])
			for _, r := range errs[:patternRowsPerSeverity] {
				assert.NotEqual(t, otherPatternID, r["tag_pattern_id"])
				assert.NotNil(t, r["template"])
			}

			assert.Equal(t, int64(errors), sumCount(errs))
			assert.Equal(t, counts["error_total"], sumCount(errs), "per-severity sums equal the _total column")
			assert.Equal(t, counts["log_total"], sumCount(rowsOfSeverity(rows, "log")))
		})
	}
}

// The highest counts are kept, and ties go to the lower pattern id.
func TestPatternCapKeepsHighestCountsThenLowestIDs(t *testing.T) {
	tally := newPatternTally(time.Minute)
	rec := &pglogwatch.Record{Severity: pglogwatch.SeverityError}
	for i := range 150 {
		n := 1
		if i >= 140 {
			n = 2
		}
		for range n {
			tally.addLocked(rec, [8]byte{0, byte(i)}, []byte("t"))
		}
	}
	rows := tally.measurementsLocked(0)
	require.Len(t, rows, patternRowsPerSeverity+1)
	for i := range 10 {
		assert.Equal(t, fmt.Sprintf("00%02x000000000000", 140+i), rows[i]["tag_pattern_id"])
	}
	for i := range 90 {
		assert.Equal(t, fmt.Sprintf("00%02x000000000000", i), rows[10+i]["tag_pattern_id"])
	}
	assert.Equal(t, int64(50), rows[100]["count"])
}

// REQ-009: memory stays bounded however many distinct patterns arrive.
func TestPatternsHeldPerSeverityAreBounded(t *testing.T) {
	lp, _ := newStreamParser(t, logConfig{JSONDestination: true})
	lp.patterns = newPatternTally(time.Minute)
	const extra = 7
	for i := range patternRowsHeldPerSeverity + extra {
		lp.count(&pglogwatch.Record{
			Severity: pglogwatch.SeverityError,
			SQLState: [5]byte{'4', '2', 'P', '0', '1'},
			Database: []byte("shop"),
			Message:  fmt.Appendf(nil, `relation "t%d" does not exist`, i),
			Flags:    pglogwatch.FlagHasDuration,
			Duration: time.Millisecond,
		})
	}
	sp := lp.patterns.bySeverity[pglogwatch.SeverityError]
	assert.Equal(t, patternRowsHeldPerSeverity, sp.held)
	assert.Equal(t, int64(extra), sp.other.count)

	rows := lp.patterns.measurementsLocked(0)
	require.Len(t, rows, patternRowsPerSeverity+1)
	assert.Equal(t, int64(patternRowsHeldPerSeverity+extra), sumCount(rows))
	assert.InDelta(t, float64(patternRowsHeldPerSeverity+extra-patternRowsPerSeverity), rows[100]["duration_sum_ms"], 1e-9)
}

// REQ-005: rows count every database, and a record without one has "".
func TestPatternsCountTheWholeInstance(t *testing.T) {
	events := []logEvent{
		{"ERROR", "42P01", "shop", `relation "x" does not exist`},
		{"ERROR", "42P01", "other", `relation "x" does not exist`},
		{"LOG", "", "", "checkpoint starting: time"},
	}
	rows, counts := consumePatterns(t, logConfig{JSONDestination: true}, renderJSON(t, events))
	errs := rowsOfSeverity(rows, "error")
	require.Len(t, errs, 2)
	assert.Equal(t, errs[0]["tag_pattern_id"], errs[1]["tag_pattern_id"])
	assert.ElementsMatch(t, []any{"shop", "other"}, []any{errs[0]["tag_database"], errs[1]["tag_database"]})
	logs := rowsOfSeverity(rows, "log")
	require.Len(t, logs, 1)
	assert.Equal(t, "", logs[0]["tag_database"])
	assert.Equal(t, int64(1), counts["error"], "event counts still split by the source's database")
}

// REQ-006: the send empties the tally, and an empty interval sends no rows.
func TestPatternsResetOnSend(t *testing.T) {
	lp, ch := newStreamParser(t, logConfig{JSONDestination: true})
	lp.patterns = newPatternTally(time.Minute)
	require.NoError(t, lp.consume(bytes.NewReader(renderJSON(t, []logEvent{{"ERROR", "42P01", "shop", `relation "x" does not exist`}}))))
	lp.sendPatterns()
	require.Len(t, ch, 1)
	<-ch
	assert.Empty(t, lp.patterns.bySeverity)
	assert.False(t, lp.patterns.due())

	lp.sendPatterns()
	assert.Empty(t, ch, "an interval with no records sends no rows")
}

// AC-005 and REQ-003: each metric is sent only when configured, and adding
// server_log_patterns does not change server_log_event_counts.
func TestEventCountsAreUnchangedByPatterns(t *testing.T) {
	input := renderJSON(t, benchEvents(500))
	run := func(countsOff, patterns bool) []metrics.MeasurementEnvelope {
		lp, ch := newStreamParser(t, logConfig{JSONDestination: true})
		lp.countsOff = countsOff
		if patterns {
			lp.patterns = newPatternTally(time.Minute)
		}
		require.NoError(t, lp.consume(bytes.NewReader(input)))
		lp.sendDue(true)
		close(ch)
		var out []metrics.MeasurementEnvelope
		for env := range ch {
			out = append(out, env)
		}
		return out
	}
	withoutEpoch := func(m metrics.Measurement) metrics.Measurement {
		c := maps.Clone(m)
		delete(c, metrics.EpochColumnName)
		return c
	}

	countsOnly := run(false, false)
	require.Len(t, countsOnly, 1, "no server_log_patterns rows without the metric")
	assert.Equal(t, specialMetricServerLogEventCounts, countsOnly[0].MetricName)

	both := run(false, true)
	require.Len(t, both, 2)
	assert.Equal(t, specialMetricServerLogEventCounts, both[0].MetricName)
	assert.Equal(t, withoutEpoch(countsOnly[0].Data[0]), withoutEpoch(both[0].Data[0]))
	assert.Equal(t, specialMetricServerLogPatterns, both[1].MetricName)

	patternsOnly := run(true, true)
	require.Len(t, patternsOnly, 1)
	assert.Equal(t, specialMetricServerLogPatterns, patternsOnly[0].MetricName)
}

func TestSendIntervalIsTheShortestConfigured(t *testing.T) {
	lp := &logParser{Interval: time.Minute}
	assert.Equal(t, time.Minute, lp.sendInterval())
	lp.patterns = newPatternTally(30 * time.Second)
	assert.Equal(t, 30*time.Second, lp.sendInterval())
	lp.patterns.interval = 2 * time.Minute
	assert.Equal(t, time.Minute, lp.sendInterval())
	lp.countsOff = true
	assert.Equal(t, 2*time.Minute, lp.sendInterval())
}

// AC-004: no privacy marker of the pglogwatch template corpus reaches a row.
func TestPatternsCarryNoValueFromTheLog(t *testing.T) {
	corpus := loadPrivacyCorpus(t)
	require.GreaterOrEqual(t, len(corpus), 200)
	markers := []string{"SECRET", "424242", "secret4"} // covers every marker of the corpus README
	for _, f := range fixtureFormats {
		t.Run(f.name, func(t *testing.T) {
			rows, _ := consumePatterns(t, f.cfg, f.render(t, corpus))
			assert.GreaterOrEqual(t, sumCount(rows), int64(len(corpus)), "every corpus record must be counted")
			for _, r := range rows {
				for k, v := range r {
					if s, ok := v.(string); ok {
						for _, m := range markers {
							assert.NotContains(t, s, m, "column %s", k)
						}
					}
				}
			}
		})
	}
}

// loadPrivacyCorpus reads testdata/template/privacy.txt of the pglogwatch
// module in use; its README describes the format.
func loadPrivacyCorpus(t *testing.T) []logEvent {
	t.Helper()
	dir, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/pashagolub/pglogwatch").Output()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(dir)), "testdata", "template", "privacy.txt"))
	require.NoError(t, err)
	unescape := strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\t`, "\t", "{email}", "secret4@example.com")
	var events []logEvent
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" || line[0] == '#' {
			continue
		}
		fields := strings.SplitN(line, " ", 3)
		require.Len(t, fields, 3, line)
		state := fields[1]
		if state == "-" {
			state = ""
		}
		events = append(events, logEvent{fields[0], state, "shop", unescape.Replace(fields[2])})
	}
	return events
}

// CON-002: no allocation per record once its pattern row exists.
func TestCountDoesNotAllocatePerRecord(t *testing.T) {
	lp, _ := newStreamParser(t, logConfig{CSVDestination: true})
	lp.patterns = newPatternTally(time.Minute)
	recs := []*pglogwatch.Record{
		{Severity: pglogwatch.SeverityError, SQLState: [5]byte{'4', '2', 'P', '0', '1'}, Database: []byte("shop"),
			Message: []byte(`relation ""x"" does not exist`), Flags: pglogwatch.FlagNeedsUnquote},
		{Severity: pglogwatch.SeverityLog, Database: []byte("shop"), Flags: pglogwatch.FlagHasDuration, Duration: time.Millisecond,
			Message: []byte("duration: 1.000 ms  statement: SELECT * FROM t WHERE id = 42")},
	}
	for _, r := range recs {
		lp.count(r)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		for _, r := range recs {
			lp.count(r)
		}
	})
	assert.Zero(t, allocs)
}

// CON-001: the unquoted copy of a message is not kept past count.
func TestCountKeepsNoRawMessage(t *testing.T) {
	lp, _ := newStreamParser(t, logConfig{CSVDestination: true})
	lp.patterns = newPatternTally(time.Minute)
	lp.count(&pglogwatch.Record{Severity: pglogwatch.SeverityError, SQLState: [5]byte{'2', '2', '0', '0', '0'},
		Message: []byte(`invalid value ""SECRET-3"" 424242`), Flags: pglogwatch.FlagNeedsUnquote})
	assert.NotContains(t, string(lp.patterns.unquoted[:cap(lp.patterns.unquoted)]), "SECRET")
	assert.NotContains(t, string(lp.patterns.template), "SECRET")
	assert.Nil(t, lp.patterns.scratch.Message)
}
