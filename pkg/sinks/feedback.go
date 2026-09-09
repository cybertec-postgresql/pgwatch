package sinks

import (
	"context"
	"errors"
	"time"
)

// feedbackTimeout bounds a single feedback query when the caller's context
// carries no deadline of its own.
const feedbackTimeout = 5 * time.Second

// ErrFeedbackUnsupported indicates that the sink cannot report a last-written
// epoch for the requested (sourceName, metricName) pair. It is not a failure:
// callers are expected to fall back to their default behaviour.
var ErrFeedbackUnsupported = errors.New("sink does not support feedback for this source/metric")

// ErrNoFeedbackData indicates that the pair is supported but the sink holds no
// measurement for it yet.
var ErrNoFeedbackData = errors.New("sink holds no measurements for this source/metric")

// Feedbacker is an optional interface that a Writer may implement to report
// back what it has already durably stored. It exists so that stateful,
// resumable collectors can continue from the last persisted measurement
// instead of restarting from the current instant.
//
// Implementing Feedbacker declares the sink kind capable of feedback;
// CanFeedback declares whether one specific source/metric pair can be answered.
//
// No pgwatch component calls these methods today. See spec/design-sink-feedback.md
// for the caller contract before wiring up a consumer.
type Feedbacker interface {
	// CanFeedback reports whether LastMeasurement can be answered for this
	// pair. It must not perform I/O, must not block, and must be safe for
	// concurrent use. A true result is advisory: LastMeasurement may still
	// return ErrFeedbackUnsupported if state changed in between.
	CanFeedback(sourceName, metricName string) bool

	// LastMeasurement returns the epoch_ns (Unix nanoseconds) of the newest
	// measurement the sink durably holds for the pair.
	//
	// Returns ErrFeedbackUnsupported when the pair cannot be answered, and
	// ErrNoFeedbackData when the pair is supported but empty. Both are
	// expected outcomes, not faults. The returned epoch is 0 whenever err is
	// non-nil, and strictly positive whenever err is nil.
	LastMeasurement(ctx context.Context, sourceName, metricName string) (int64, error)
}

// withFeedbackDeadline bounds a feedback query at feedbackTimeout when the
// caller supplied no deadline of its own. A caller-supplied deadline is left
// alone, whether it is tighter or looser.
func withFeedbackDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, feedbackTimeout)
}
