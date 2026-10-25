package reaper

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/pashagolub/pglogwatch"
)

// server_log_patterns: records grouped by their pglogwatch template and counted
// per interval. Only templates are kept, never a value from the log.

const (
	patternRowsPerSeverity     = 100                         // rows sent per severity; the rest is summed into "other"
	patternRowsHeldPerSeverity = 10 * patternRowsPerSeverity // distinct rows held between sends; beyond it a record goes to "other"
	otherPatternID             = "other"
)

type patternRow struct {
	id       [8]byte
	sqlState [5]byte
	db       string
	template string
	count    int64
	durSum   time.Duration
	durMax   time.Duration
	hasDur   bool
}

func (r *patternRow) add(rec *pglogwatch.Record) {
	r.count++
	if rec.Flags&pglogwatch.FlagHasDuration != 0 {
		r.hasDur = true
		r.durSum += rec.Duration
		r.durMax = max(r.durMax, rec.Duration)
	}
}

func (r *patternRow) merge(o *patternRow) {
	r.count += o.count
	if o.hasDur {
		r.hasDur = true
		r.durSum += o.durSum
		r.durMax = max(r.durMax, o.durMax)
	}
}

func (r *patternRow) measurement(epoch int64, severity string, other bool) metrics.Measurement {
	m := metrics.NewMeasurement(epoch)
	m["tag_severity"] = severity
	m["tag_database"] = r.db
	m["rules_version"] = int64(pglogwatch.TemplateRulesVersion)
	m["count"] = r.count
	if other {
		m["tag_pattern_id"] = otherPatternID
		m["tag_sqlstate"] = ""
		m["template"] = nil
	} else {
		m["tag_pattern_id"] = hex.EncodeToString(r.id[:])
		m["tag_sqlstate"] = ""
		if r.sqlState != [5]byte{} {
			m["tag_sqlstate"] = string(r.sqlState[:])
		}
		m["template"] = r.template
	}
	m["duration_sum_ms"], m["duration_max_ms"] = nil, nil
	if r.hasDur {
		m["duration_sum_ms"] = float64(r.durSum) / float64(time.Millisecond)
		m["duration_max_ms"] = float64(r.durMax) / float64(time.Millisecond)
	}
	return m
}

type severityPatterns struct {
	byDB  map[string]map[[8]byte]*patternRow // two levels so a lookup by rec.Database does not allocate
	held  int
	other patternRow
}

// patternTally is guarded by logParser.countsMu, except for the buffers, which
// only the consume goroutine touches.
type patternTally struct {
	interval   time.Duration
	lastSend   time.Time
	bySeverity map[pglogwatch.Severity]*severityPatterns

	unquoted, template []byte
	scratch            pglogwatch.Record
}

func newPatternTally(interval time.Duration) *patternTally {
	return &patternTally{
		interval:   interval,
		bySeverity: make(map[pglogwatch.Severity]*severityPatterns),
	}
}

func (t *patternTally) due() bool {
	return t.lastSend.IsZero() || t.lastSend.Before(time.Now().Add(-t.interval))
}

// templateOf computes the template and pattern id of rec into reused buffers.
func (t *patternTally) templateOf(rec *pglogwatch.Record, format pglogwatch.Format) ([]byte, [8]byte) {
	t.scratch.Severity, t.scratch.SQLState, t.scratch.Message = rec.Severity, rec.SQLState, rec.Message
	if rec.Flags&pglogwatch.FlagNeedsUnquote != 0 {
		t.unquoted = pglogwatch.AppendUnquoted(t.unquoted[:0], rec.Message, format)
		t.scratch.Message = t.unquoted
	}
	t.template = pglogwatch.AppendTemplate(t.template[:0], &t.scratch)
	id := pglogwatch.PatternID(&t.scratch, t.template)
	if rec.Flags&pglogwatch.FlagNeedsUnquote != 0 {
		clear(t.unquoted) // raw text; keep none of it
	}
	t.scratch.Message = nil
	return t.template, id
}

func (t *patternTally) addLocked(rec *pglogwatch.Record, id [8]byte, template []byte) {
	sp := t.bySeverity[rec.Severity]
	if sp == nil {
		sp = &severityPatterns{byDB: make(map[string]map[[8]byte]*patternRow)}
		t.bySeverity[rec.Severity] = sp
	}
	rows := sp.byDB[string(rec.Database)]
	row := rows[id]
	if row == nil {
		if sp.held >= patternRowsHeldPerSeverity {
			sp.other.add(rec)
			return
		}
		db := string(rec.Database)
		if rows == nil {
			rows = make(map[[8]byte]*patternRow)
			sp.byDB[db] = rows
		}
		row = &patternRow{id: id, sqlState: rec.SQLState, db: db, template: string(template)}
		rows[id] = row
		sp.held++
	}
	row.add(rec)
}

// measurementsLocked returns the rows of the interval, capped per severity.
func (t *patternTally) measurementsLocked(epoch int64) metrics.Measurements {
	var out metrics.Measurements
	for _, sev := range slices.Sorted(maps.Keys(t.bySeverity)) {
		sp := t.bySeverity[sev]
		name := strings.ToLower(sev.String())
		rows := make([]*patternRow, 0, sp.held)
		for _, byID := range sp.byDB {
			for _, r := range byID {
				rows = append(rows, r)
			}
		}
		slices.SortFunc(rows, func(a, b *patternRow) int {
			if c := cmp.Compare(b.count, a.count); c != 0 {
				return c
			}
			if c := bytes.Compare(a.id[:], b.id[:]); c != 0 {
				return c
			}
			return strings.Compare(a.db, b.db)
		})
		other := sp.other
		for i, r := range rows {
			if i < patternRowsPerSeverity {
				out = append(out, r.measurement(epoch, name, false))
			} else {
				other.merge(r)
			}
		}
		if other.count > 0 {
			out = append(out, other.measurement(epoch, name, true))
		}
	}
	return out
}

func (t *patternTally) resetLocked() {
	clear(t.bySeverity)
	t.lastSend = time.Now()
}

// sendPatterns emits the pattern rows and empties the tally, reading and
// resetting under one lock as sendCounts does. An empty interval sends nothing.
func (lp *logParser) sendPatterns() {
	lp.countsMu.Lock()
	data := lp.patterns.measurementsLocked(time.Now().UnixNano())
	lp.patterns.resetLocked()
	lp.countsMu.Unlock()

	if len(data) == 0 {
		return
	}
	select {
	case <-lp.ctx.Done():
	case lp.StoreCh <- metrics.MeasurementEnvelope{
		DBName:     lp.SourceConn.Name,
		MetricName: specialMetricServerLogPatterns,
		Data:       data,
		CustomTags: lp.SourceConn.CustomTags,
	}:
	}
}
