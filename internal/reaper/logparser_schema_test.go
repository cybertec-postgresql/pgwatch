package reaper

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The envelope schema, verified against its consumer rather than against
// itself.
//
// TestEnvelopeSchemaIsExactlyTheseSixteenKeys pins the emitted key set, but a
// list in a test file agrees with whatever a developer changes it to. What
// actually breaks when a column is renamed is the shipped Grafana dashboard,
// which selects these keys by name out of the measurement's JSON:
//
//	sum((data->>'error_total')::int8) as error
//
// So this test reads the dashboard and requires every key it selects to be a
// key the parser emits. It is the closest thing to a contract test available
// without standing up Grafana.

var dashboardKeyRe = regexp.MustCompile(`data->>'([a-z_0-9]+)'`)

func TestEveryDashboardKeyIsEmitted(t *testing.T) {
	dashboard := filepath.Join("..", "..", "grafana", "postgres", "v12", "server-log-events.json")
	raw, err := os.ReadFile(dashboard) //nolint:gosec // a file in this repository
	require.NoError(t, err, "the dashboard this metric feeds must exist")

	matches := dashboardKeyRe.FindAllStringSubmatch(string(raw), -1)
	require.NotEmpty(t, matches, "the dashboard must select some keys, or this test proves nothing")

	seen := map[string]bool{}
	var keys []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	sort.Strings(keys)
	t.Logf("dashboard selects %d distinct keys: %v", len(keys), keys)

	lp := &LogParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "schema-test"}},
		eventCounts:      map[string]int64{},
		eventCountsTotal: map[string]int64{},
	}
	m := lp.GetMeasurementEnvelope().Data[0]

	for _, k := range keys {
		assert.Contains(t, m, k,
			"the dashboard selects %q; renaming or dropping it breaks a shipped panel", k)
		assert.IsType(t, int64(0), m[k],
			"%q is cast to int8 in the dashboard's SQL, so it must be an integer", k)
	}
}

// The counts survive the JSON round trip to the measurement store as integers.
//
// The envelope holds any, and a count that reached the sink as a float would
// still render -- Grafana casts to int8 -- but it would be stored as 5.0 and
// compared, summed and alerted on as a float. This checks the type the store
// actually receives rather than the type the map was built with.
func TestCountsAreIntegersAfterMarshalling(t *testing.T) {
	lp := &LogParser{
		SourceConn:       &sources.DbConn{Source: sources.Source{Name: "schema-test"}},
		eventCounts:      map[string]int64{"ERROR": 5},
		eventCountsTotal: map[string]int64{"ERROR": 9},
	}
	m := lp.GetMeasurementEnvelope().Data[0]

	for k, v := range m {
		if k == metrics.EpochColumnName {
			continue
		}
		_, ok := v.(int64)
		assert.True(t, ok, "%s is %T, not int64", k, v)
	}
	assert.Equal(t, int64(5), m["error"])
	assert.Equal(t, int64(9), m["error_total"])
}
