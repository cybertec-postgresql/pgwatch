package reaper

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const scrapeAllFixture = `# HELP go_goroutines Number of goroutines
# TYPE go_goroutines gauge
go_goroutines{job="pgwatch",instance="local"} 42 1700000000000
# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1024
`

func TestScrapeAll(t *testing.T) {
	testCases := []struct {
		name  string
		check func(t *testing.T, got []metrics.MeasurementEnvelope)
	}{
		{
			name: "returns expected measurement count",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				require.Len(t, got, 2)
				names := make([]string, len(got))
				for i, e := range got {
					names[i] = e.MetricName
				}
				assert.ElementsMatch(t, []string{"go_goroutines", "http_requests_total"}, names)
				require.Len(t, requireEnvelope(t, got, "go_goroutines").Data, 1)
				require.Len(t, requireEnvelope(t, got, "http_requests_total").Data, 1)
			},
		},
		{
			name: "source kind set",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				assert.Equal(t, string(sources.SourcePrometheus), requireEnvelope(t, got, "go_goroutines").SourceKind)
			},
		},
		{
			name: "tag labels present",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				measurement := requireSingleMeasurement(t, requireEnvelope(t, got, "go_goroutines").Data)
				assert.Equal(t, "pgwatch", measurement[metrics.TagPrefix+"job"])
				assert.Equal(t, "local", measurement[metrics.TagPrefix+"instance"])
			},
		},
		{
			name: "no __name__ label",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				measurement := requireSingleMeasurement(t, requireEnvelope(t, got, "go_goroutines").Data)
				assert.NotContains(t, measurement, metrics.TagPrefix+"__name__")
				assert.NotContains(t, measurement, "__name__")
			},
		},
		{
			name: "value column",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				measurement := requireSingleMeasurement(t, requireEnvelope(t, got, "go_goroutines").Data)
				assert.Equal(t, float64(42), measurement["go_goroutines"])
			},
		},
		{
			name: "epoch_ns from timestamp",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				measurement := requireSingleMeasurement(t, requireEnvelope(t, got, "go_goroutines").Data)
				assert.Equal(t, int64(1700000000000*1_000_000), measurement[metrics.EpochColumnName])
			},
		},
		{
			name: "epoch_ns fallback",
			check: func(t *testing.T, got []metrics.MeasurementEnvelope) {
				t.Helper()
				measurement := requireSingleMeasurement(t, requireEnvelope(t, got, "http_requests_total").Data)
				epoch, ok := measurement[metrics.EpochColumnName].(int64)
				require.True(t, ok)
				assert.WithinDuration(t, time.Now(), time.Unix(0, epoch), time.Second)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sc := newTestPromConn(t, scrapeAllFixture)
			got, err := ScrapeAll(t.Context(), sc)
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestScrapeAll_NonFinite(t *testing.T) {
	const fixture = `# HELP mymetric A metric
# TYPE mymetric gauge
mymetric{} +Inf
mymetric{foo="bar"} -Inf
mymetric{baz="qux"} NaN
`

	sc := newTestPromConn(t, fixture)
	got, err := ScrapeAll(t.Context(), sc)
	require.NoError(t, err)

	env := requireEnvelope(t, got, "mymetric")
	require.Len(t, env.Data, 3)

	testCases := []struct {
		name  string
		tags  map[string]string
		check func(float64) bool
	}{
		{
			name:  "positive infinity",
			tags:  map[string]string{},
			check: func(v float64) bool { return math.IsInf(v, 1) },
		},
		{
			name:  "negative infinity",
			tags:  map[string]string{"foo": "bar"},
			check: func(v float64) bool { return math.IsInf(v, -1) },
		},
		{
			name:  "nan",
			tags:  map[string]string{"baz": "qux"},
			check: math.IsNaN,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			measurement := requireMeasurementByTags(t, env.Data, tc.tags)
			value, ok := measurement["mymetric"].(float64)
			require.True(t, ok)
			assert.True(t, tc.check(value))
		})
	}
}

func TestScrapeAll_AcceptHeader(t *testing.T) {
	var accept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# HELP up Up\n# TYPE up gauge\nup 1\n"))
	}))
	t.Cleanup(srv.Close)

	sc := &sources.PromConn{
		Source:     sources.Source{ConnStr: srv.URL},
		HTTPClient: srv.Client(),
	}
	require.NoError(t, sc.ParseConfig())

	_, err := ScrapeAll(t.Context(), sc)
	require.NoError(t, err)
	assert.Equal(t, "text/plain", accept)
}

func newTestPromConn(t *testing.T, body string) *sources.PromConn {
	t.Helper()
	srv := testutil.NewFakeExporter(t, body)
	pc := &sources.PromConn{
		Source: sources.Source{
			Kind:    sources.SourcePrometheus,
			ConnStr: srv.URL,
		},
		HTTPClient: srv.Client(),
	}
	require.NoError(t, pc.ParseConfig())
	return pc
}

func requireEnvelope(t *testing.T, envelopes []metrics.MeasurementEnvelope, metricName string) metrics.MeasurementEnvelope {
	t.Helper()
	for _, e := range envelopes {
		if e.MetricName == metricName {
			return e
		}
	}
	require.FailNowf(t, "envelope not found", "no envelope with MetricName %q", metricName)
	return metrics.MeasurementEnvelope{}
}

func requireSingleMeasurement(t *testing.T, data metrics.Measurements) metrics.Measurement {
	t.Helper()
	require.Len(t, data, 1)
	return metrics.Measurement(data[0])
}

func requireMeasurementByTags(t *testing.T, measurements metrics.Measurements, tags map[string]string) metrics.Measurement {
	t.Helper()
	for _, measurement := range measurements {
		if hasExactTags(measurement, tags) {
			return metrics.Measurement(measurement)
		}
	}
	require.FailNowf(t, "measurement not found", "expected tags %v", tags)
	return nil
}

func hasExactTags(measurement map[string]any, tags map[string]string) bool {
	tagCount := 0
	for key, value := range measurement {
		label, ok := strings.CutPrefix(key, metrics.TagPrefix)
		if !ok {
			continue
		}
		tagCount++
		expected, ok := tags[label]
		if !ok || value != expected {
			return false
		}
	}
	return tagCount == len(tags)
}
