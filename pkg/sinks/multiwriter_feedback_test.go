package sinks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMulti(writers ...Writer) *MultiWriter {
	mw := &MultiWriter{}
	for _, w := range writers {
		mw.AddWriter(w)
	}
	return mw
}

// TestMultiWriterLastMeasurement walks every row of the aggregation table in
// spec/design-sink-feedback.md §4.3. MultiWriter is built through AddWriter
// rather than NewSinkWriter, which unwraps the single-sink case.
func TestMultiWriterLastMeasurement(t *testing.T) {
	const (
		older = int64(1000)
		newer = int64(2000)
	)

	for _, tc := range []struct {
		name      string
		writers   []Writer
		wantEpoch int64
		wantErr   error
	}{
		{
			name:    "no writer implements Feedbacker",
			writers: []Writer{&plainWriter{}, &plainWriter{}},
			wantErr: ErrFeedbackUnsupported,
		},
		{
			name:    "all capable writers unsupported",
			writers: []Writer{&fakeFeedbacker{canFeedback: true, err: ErrFeedbackUnsupported}},
			wantErr: ErrFeedbackUnsupported,
		},
		{
			name: "minimum across capable writers",
			writers: []Writer{
				&fakeFeedbacker{canFeedback: true, epoch: newer},
				&fakeFeedbacker{canFeedback: true, epoch: older},
			},
			wantEpoch: older,
		},
		{
			name: "unsupported writer excluded from the minimum",
			writers: []Writer{
				&fakeFeedbacker{canFeedback: true, epoch: newer},
				&fakeFeedbacker{canFeedback: true, err: ErrFeedbackUnsupported},
			},
			wantEpoch: newer,
		},
		{
			name: "non-Feedbacker writer does not veto",
			writers: []Writer{
				&fakeFeedbacker{canFeedback: true, epoch: newer},
				&plainWriter{},
			},
			wantEpoch: newer,
		},
		{
			name: "empty writer short-circuits",
			writers: []Writer{
				&fakeFeedbacker{canFeedback: true, epoch: older},
				&fakeFeedbacker{canFeedback: true, err: ErrNoFeedbackData},
				&fakeFeedbacker{canFeedback: true, epoch: newer},
			},
			wantErr: ErrNoFeedbackData,
		},
		{
			name: "transport error yields no partial minimum",
			writers: []Writer{
				&fakeFeedbacker{canFeedback: true, epoch: older},
				&fakeFeedbacker{canFeedback: true, err: assert.AnError},
			},
			wantErr: assert.AnError,
		},
		{
			name:      "single capable writer",
			writers:   []Writer{&fakeFeedbacker{canFeedback: true, epoch: newer}},
			wantEpoch: newer,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			epoch, err := newMulti(tc.writers...).LastMeasurement(ctx, "prod-db", "db_stats")
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Zero(t, epoch)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantEpoch, epoch)
		})
	}
}

func TestMultiWriterCanFeedback(t *testing.T) {
	capable := func() *fakeFeedbacker { return &fakeFeedbacker{canFeedback: true} }
	incapable := func() *fakeFeedbacker { return &fakeFeedbacker{canFeedback: false} }

	for _, tc := range []struct {
		name    string
		writers []Writer
		want    bool
	}{
		{"no writers", nil, false},
		{"only plain writers", []Writer{&plainWriter{}, &plainWriter{}}, false},
		{"only incapable feedbackers", []Writer{incapable(), incapable()}, false},
		{"one capable among plain", []Writer{&plainWriter{}, capable()}, true},
		{"one capable among incapable", []Writer{incapable(), capable()}, true},
		{"all capable", []Writer{capable(), capable()}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, newMulti(tc.writers...).CanFeedback("prod-db", "db_stats"))
		})
	}

	t.Run("empty pair is never capable", func(t *testing.T) {
		mw := newMulti(capable())
		assert.False(t, mw.CanFeedback("", "db_stats"))
		assert.False(t, mw.CanFeedback("prod-db", ""))
	})
}

// TestMultiWriterLastMeasurementPassesContext pins REQ-028: the caller's
// context reaches every contained writer unchanged, and writers are queried
// sequentially rather than fanned out.
func TestMultiWriterLastMeasurementPassesContext(t *testing.T) {
	first := &fakeFeedbacker{canFeedback: true, epoch: 2000}
	second := &fakeFeedbacker{canFeedback: true, epoch: 1000}
	plain := &plainWriter{}

	type key struct{}
	callerCtx := context.WithValue(ctx, key{}, "marker")

	epoch, err := newMulti(first, plain, second).LastMeasurement(callerCtx, "prod-db", "db_stats")
	require.NoError(t, err)
	assert.Equal(t, int64(1000), epoch)

	for name, w := range map[string]*fakeFeedbacker{"first": first, "second": second} {
		assert.Equal(t, int64(1), w.lastMeasCalls.Load(), "%s queried exactly once", name)
		assert.Same(t, callerCtx, w.gotCtx.Load(), "%s got the caller's context unchanged", name)
	}
}

// TestMultiWriterShortCircuitStopsEarly pins that ErrNoFeedbackData aborts the
// sweep instead of querying the remaining writers.
func TestMultiWriterShortCircuitStopsEarly(t *testing.T) {
	empty := &fakeFeedbacker{canFeedback: true, err: ErrNoFeedbackData}
	later := &fakeFeedbacker{canFeedback: true, epoch: 1000}

	_, err := newMulti(empty, later).LastMeasurement(ctx, "prod-db", "db_stats")
	assert.ErrorIs(t, err, ErrNoFeedbackData)
	assert.Zero(t, later.lastMeasCalls.Load(), "writers after the empty one must not be queried")
}
