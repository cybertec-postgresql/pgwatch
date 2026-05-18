package reaper

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/sirupsen/logrus"
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
			got, err := (&PromReaper{md: sc}).ScrapeAll(t.Context())
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
	got, err := (&PromReaper{md: sc}).ScrapeAll(t.Context())
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

	_, err := (&PromReaper{md: sc}).ScrapeAll(t.Context())
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

// roundTripFunc is a fake http.RoundTripper backed by a plain function.
// Using it instead of a real httptest.Server avoids spawning IO-blocked goroutines
// inside a synctest bubble (which would prevent synctest.Wait from ever returning).
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fakePromConn builds a PromConn whose HTTP client uses fn as its transport.
// ConnStr can be any syntactically valid URL; fn intercepts every request.
func fakePromConn(t *testing.T, src sources.Source, fn roundTripFunc) *sources.PromConn {
	t.Helper()
	src.ConnStr = "http://fake.local"
	pc := &sources.PromConn{
		Source:     src,
		HTTPClient: &http.Client{Transport: fn},
	}
	require.NoError(t, pc.ParseConfig())
	return pc
}

// fakeResponse builds a minimal 200 Prometheus text-format response.
func fakeResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/plain; version=0.0.4"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// compile-time check that PromReaper implements Reaper.
var _ Reaper = (*PromReaper)(nil)

// TestCalcScrapeInterval verifies GCD-based interval calculation and defaults.
func TestCalcScrapeInterval(t *testing.T) {
	tests := []struct {
		name    string
		metrics metrics.MetricIntervals
		want    time.Duration
	}{
		{
			name:    "multiple intervals produce GCD",
			metrics: metrics.MetricIntervals{"a": 30, "b": 60},
			want:    30 * time.Second,
		},
		{
			name:    "single interval returns that value",
			metrics: metrics.MetricIntervals{"a": 60},
			want:    60 * time.Second,
		},
		{
			name:    "empty intervals scrape-all defaults to 60s",
			metrics: metrics.MetricIntervals{},
			want:    defaultScrapeInterval,
		},
		{
			name:    "all intervals below min floored to minTickInterval",
			metrics: metrics.MetricIntervals{"a": 0, "b": -1},
			want:    minTickInterval * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pr := &PromReaper{
				md: &sources.PromConn{
					Source: sources.Source{Metrics: tc.metrics},
				},
			}
			assert.Equal(t, tc.want, pr.calcScrapeInterval())
		})
	}
}

// when Metrics is empty, Reap uses 60 s interval and logs a warning.
func TestPromReaper_ScrapeAllMode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var scrapeCount atomic.Int32

		ctx, out := testutil.NewTestLogger(t, logrus.WarnLevel)

		md := fakePromConn(t, sources.Source{Name: "prom_scrapeall", Kind: sources.SourcePrometheus}, func(*http.Request) (*http.Response, error) {
			scrapeCount.Add(1)
			return fakeResponse("# HELP up Up\n# TYPE up gauge\nup 1\n"), nil
		})

		pr := NewPromSourceReaper(&reaper{measurementCh: make(chan metrics.MeasurementEnvelope, 100)}, md)
		go pr.Reap(ctx)

		// t=0: first scrape completes; goroutine blocks on time.After(60s).
		synctest.Wait()
		assert.Equal(t, int32(1), scrapeCount.Load(), "first scrape should occur at t=0")
		assert.Contains(t, out.String(), "scrape-all", "scrape-all mode warning should be logged")

		// Advance 60 s → second scrape.
		time.Sleep(60 * time.Second)
		synctest.Wait()
		assert.Equal(t, int32(2), scrapeCount.Load(), "second scrape should occur after 60 s")
	})
}

// per-family emit gating — families respect their configured intervals.
func TestPromReaper_EmitGating(t *testing.T) {
	const gatingBody = "# HELP fast Fast metric\n# TYPE fast gauge\nfast 1\n" +
		"# HELP slow Slow metric\n# TYPE slow gauge\nslow 2\n"

	synctest.Test(t, func(t *testing.T) {
		const fastEnv = "fast"
		const slowEnv = "slow"

		r := &reaper{measurementCh: make(chan metrics.MeasurementEnvelope, 100)}
		md := fakePromConn(t, sources.Source{
			Name:    "gating_test",
			Kind:    sources.SourcePrometheus,
			Metrics: metrics.MetricIntervals{"fast": 30, "slow": 60},
		}, func(*http.Request) (*http.Response, error) {
			return fakeResponse(gatingBody), nil
		})

		pr := NewPromSourceReaper(r, md)

		ctx := log.WithLogger(t.Context(), log.NewNoopLogger())
		go pr.Reap(ctx)

		drain := func() []string {
			var names []string
			for {
				select {
				case env := <-r.measurementCh:
					names = append(names, env.MetricName)
				default:
					return names
				}
			}
		}

		// Tick 1 (t=0): lastEmitted is zero for both → both emitted.
		synctest.Wait()
		assert.ElementsMatch(t, []string{fastEnv, slowEnv}, drain(), "tick 1: both families emitted")

		// Tick 2 (t=30s): only "fast" due; "slow" still within its 60 s window.
		time.Sleep(30*time.Second + time.Millisecond)
		synctest.Wait()
		assert.ElementsMatch(t, []string{fastEnv}, drain(), "tick 2: only 'fast' emitted")

		// Tick 3 (t=60s): both families now due.
		time.Sleep(30 * time.Second)
		synctest.Wait()
		assert.ElementsMatch(t, []string{fastEnv, slowEnv}, drain(), "tick 3: both families emitted again")
	})
}

// ScrapeAll error path — warning is logged, lastEmitted stays unchanged, loop continues.
func TestPromReaper_ScrapeError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, out := testutil.NewTestLogger(t, logrus.WarnLevel)
		r := &reaper{measurementCh: make(chan metrics.MeasurementEnvelope, 10)}

		md := fakePromConn(t, sources.Source{
			Name:    "error_test",
			Kind:    sources.SourcePrometheus,
			Metrics: metrics.MetricIntervals{"some_metric": 30},
		}, func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})

		pr := NewPromSourceReaper(r, md)
		go pr.Reap(ctx)

		// First (failing) scrape completes; goroutine blocks on time.After.
		synctest.Wait()
		assert.Contains(t, out.String(), "scrape failed", "warning should be logged on scrape error")
		assert.Empty(t, pr.lastEmitted, "lastEmitted should not change on error")

		select {
		case <-r.measurementCh:
			t.Error("no measurements should be dispatched on scrape error")
		default:
		}

		// Advance to trigger a second tick; loop must continue without crashing.
		time.Sleep(30*time.Second + time.Millisecond)
		synctest.Wait()

		select {
		case <-r.measurementCh:
			t.Error("no measurements should be dispatched on repeated scrape errors")
		default:
		}
	})
}
