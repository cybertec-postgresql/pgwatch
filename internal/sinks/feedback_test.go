package sinks

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/metrics"
	"github.com/stretchr/testify/assert"
)

// fakeFeedbacker is a scriptable Writer + Feedbacker double. It lets the
// MultiWriter aggregation rules be exercised without standing up four real
// sinks, and records call counts so tests can assert that a query never
// reached a writer at all.
type fakeFeedbacker struct {
	canFeedback bool
	epoch       int64
	err         error

	canFeedbackCalls atomic.Int64
	lastMeasCalls    atomic.Int64
	writeCalls       atomic.Int64
	syncCalls        atomic.Int64

	// gotCtx records the context handed to the most recent LastMeasurement
	// call, so tests can assert it was passed through unchanged.
	gotCtx atomic.Value
}

var (
	_ Writer     = (*fakeFeedbacker)(nil)
	_ Feedbacker = (*fakeFeedbacker)(nil)
)

func (f *fakeFeedbacker) CanFeedback(sourceName, metricName string) bool {
	f.canFeedbackCalls.Add(1)
	if sourceName == "" || metricName == "" {
		return false
	}
	return f.canFeedback
}

func (f *fakeFeedbacker) LastMeasurement(ctx context.Context, _, _ string) (int64, error) {
	f.lastMeasCalls.Add(1)
	f.gotCtx.Store(ctx)
	if f.err != nil {
		return 0, f.err
	}
	return f.epoch, nil
}

func (f *fakeFeedbacker) Write(metrics.MeasurementEnvelope) error {
	f.writeCalls.Add(1)
	return nil
}

func (f *fakeFeedbacker) SyncMetric(_, _ string, _ SyncOp) error {
	f.syncCalls.Add(1)
	return nil
}

// plainWriter is a Writer that deliberately does not implement Feedbacker,
// standing in for the Prometheus and JSON sinks in aggregation tests.
type plainWriter struct{ writeCalls atomic.Int64 }

var _ Writer = (*plainWriter)(nil)

func (p *plainWriter) Write(metrics.MeasurementEnvelope) error {
	p.writeCalls.Add(1)
	return nil
}

func (p *plainWriter) SyncMetric(_, _ string, _ SyncOp) error { return nil }

// TestFeedbackNotImplemented pins the deliberate non-implementations. The
// Prometheus sink only knows what pgwatch offered, not what a scraper stored,
// and the JSON sink would have to scan rotated, compressed files to answer;
// see spec/design-sink-feedback.md §7.3 and §7.4. Both must nevertheless stay
// usable as sinks.
func TestFeedbackNotImplemented(t *testing.T) {
	// The map type asserts Writer conformance at compile time.
	for name, w := range map[string]Writer{
		"prometheus": (*PrometheusWriter)(nil),
		"jsonfile":   (*JSONWriter)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := w.(Feedbacker)
			assert.False(t, ok, "%s sink must not implement Feedbacker", name)
		})
	}
}
