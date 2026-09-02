package sinks

import (
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
