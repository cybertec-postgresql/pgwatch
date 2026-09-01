package reaper

import (
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server_log_event_counts envelope schema.
//
// This file is written against the PRE-migration implementation on purpose. It
// is a characterisation test: it pins what pgwatch emits today so that the
// switch to pglogwatch can be shown not to have changed it. A test written
// after the migration would only prove the new code agrees with itself.
//
// The schema is not pgwatch's to change. Dashboards select these column names,
// and the measurement store has them as columns; a renamed or missing key is a
// broken dashboard, not a failing test, so it has to fail here first.

// wantEnvelopeKeys is the complete set of keys the measurement carries: one
// per severity in pgSeverities, plus one _total per severity.
//
// Spelled out rather than generated from pgSeverities, because a test that
// derives its expectation from the code under test agrees with any change that
// code makes -- including a change to this list, which is the one thing this
// test exists to catch.
var wantEnvelopeKeys = []string{
	"debug", "info", "notice", "warning", "error", "log", "fatal", "panic",
	"debug_total", "info_total", "notice_total", "warning_total",
	"error_total", "log_total", "fatal_total", "panic_total",
}

func TestEnvelopeSchemaIsExactlyTheseSixteenKeys(t *testing.T) {
	lp := &LogParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "test-db"}},
		eventCounts:      map[string]int64{},
		eventCountsTotal: map[string]int64{},
	}
	m := lp.GetMeasurementEnvelope().Data[0]

	// NewMeasurement stamps the timestamp; every other key is a severity.
	got := make([]string, 0, len(m))
	for k := range m {
		if k == metrics.EpochColumnName {
			continue
		}
		got = append(got, k)
	}
	assert.ElementsMatch(t, wantEnvelopeKeys, got,
		"the envelope's key set is fixed; dashboards select these names")

	// Every count is an int64, including the zeroes. A JSON round trip that
	// turned an absent count into a float64 or a nil would reach the sink
	// looking like a schema change.
	for _, k := range wantEnvelopeKeys {
		require.Contains(t, m, k)
		assert.IsType(t, int64(0), m[k], "%s must be int64", k)
	}
}

func TestEnvelopeCountsAndIdentityAreUnchanged(t *testing.T) {
	mdb := &sources.DbConn{
		Source: sources.Source{
			Name:       "test-db",
			Kind:       sources.SourcePostgres,
			CustomTags: map[string]string{"env": "test"},
		},
	}
	lp := &LogParser{
		SourceConn: mdb,
		// Keyed by ENGLISH severity name. Where those keys come from is
		// exactly what the migration changes -- a regex capture group
		// before, pglogwatch's normalised Severity after -- so pinning
		// the mapping from key to emitted column is what makes the two
		// implementations comparable.
		eventCounts:      map[string]int64{"ERROR": 5, "WARNING": 10},
		eventCountsTotal: map[string]int64{"ERROR": 15, "WARNING": 25, "INFO": 50},
	}

	env := lp.GetMeasurementEnvelope()
	assert.Equal(t, "test-db", env.DBName)
	assert.Equal(t, specialMetricServerLogEventCounts, env.MetricName)
	assert.Equal(t, map[string]string{"env": "test"}, env.CustomTags)
	require.Len(t, env.Data, 1)

	m := env.Data[0]
	assert.Equal(t, int64(5), m["error"])
	assert.Equal(t, int64(10), m["warning"])
	assert.Equal(t, int64(15), m["error_total"])
	assert.Equal(t, int64(25), m["warning_total"])
	assert.Equal(t, int64(50), m["info_total"])

	// A severity counted only per-instance leaves the per-database column
	// at zero rather than absent, and a severity counted nowhere leaves
	// both at zero. Absent and zero are different on the wire.
	assert.Equal(t, int64(0), m["info"])
	assert.Equal(t, int64(0), m["debug"])
	assert.Equal(t, int64(0), m["debug_total"])
}

// DEBUG1..DEBUG5 are dropped, and that is the pre-migration behaviour.
//
// PostgreSQL writes DEBUG1 through DEBUG5, never a bare "DEBUG", so the
// "debug" column has always been zero on a real server: the regex captured
// "DEBUG3", the count landed under that key, and GetMeasurementEnvelope only
// ever reads the eight names in pgSeverities.
//
// Recorded here because pglogwatch reports the same severities and the obvious
// adapter -- fold DEBUG1..5 into DEBUG -- would look like a bug fix while
// changing a column that has emitted zero for years. The migration is meant to
// leave the output identical, so the drop is preserved and made deliberate
// rather than incidental.
func TestNumberedDebugSeveritiesAreDropped(t *testing.T) {
	lp := &LogParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "test-db"}},
		eventCounts:      map[string]int64{"DEBUG1": 7, "DEBUG3": 9},
		eventCountsTotal: map[string]int64{"DEBUG1": 7, "DEBUG3": 9},
	}
	m := lp.GetMeasurementEnvelope().Data[0]

	assert.Equal(t, int64(0), m["debug"],
		"numbered debug severities do not reach the debug column")
	assert.Equal(t, int64(0), m["debug_total"])
	assert.NotContains(t, m, "debug1")
	assert.NotContains(t, m, "debug3")
}
