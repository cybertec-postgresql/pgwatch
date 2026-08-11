package db

import (
	"context"
	"fmt"
	"time"
)

// Deadline defaults for the various database round-trips pgwatch performs.
//
// These are package-level vars rather than consts so tests can shrink them
// (e.g. to a few milliseconds) to exercise fault-injection paths quickly.
//
// `internal/db` is a leaf package — it MUST NOT import `internal/reaper` or
// `internal/sources`. Tests of the call sites shrink these vars locally and
// restore them on cleanup.
var (
	// MinFetchTimeout is the floor applied to fetch-path deadlines. Very
	// short metric intervals still get a usable window to dial, authenticate,
	// and return a meaningful error rather than aborting before the round-trip
	// starts.
	MinFetchTimeout = 30 * time.Second

	// ChangeDetectionTimeout bounds the Detect*Changes family and the
	// QueryMeasurements helper used by them.
	ChangeDetectionTimeout = 60 * time.Second

	// RuntimeInfoTimeout bounds each sub-query of FetchRuntimeInfo:
	// version, platform discovery, approximate size, extensions, available
	// extensions.
	RuntimeInfoTimeout = 30 * time.Second

	// ResolverTimeout bounds a single ResolveDatabasesFromPostgres call
	// (pool creation + discovery query).
	ResolverTimeout = 15 * time.Second

	// PingTimeoutMargin is added on top of the configured ConnectTimeout when
	// bounding the main-loop Ping gate. Default 5s + 5s = 10s total.
	PingTimeoutMargin = 5 * time.Second
)

// deadlineCause builds the canonical cause string attached to a derived
// context via context.WithTimeoutCause. The format "<op> deadline" is
// greppable and distinct per call site.
func deadlineCause(op string) error {
	return fmt.Errorf("%s deadline", op)
}

// WithFetchTimeout returns a child of ctx whose deadline is
// max(interval, MinFetchTimeout). The op string is embedded in the context's
// cause so a context.DeadlineExceeded is distinguishable per call site.
//
// The cancel func MUST be called by the caller to release the timer once
// the work completes (or fails) — same convention as context.WithTimeout.
func WithFetchTimeout(ctx context.Context, op string, interval time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(ctx, max(interval, MinFetchTimeout), deadlineCause(op))
}

// WithOpTimeout returns a child of ctx with a fixed deadline of d. The op
// string is embedded in the context's cause so a context.DeadlineExceeded is
// distinguishable per call site.
//
// The cancel func MUST be called by the caller to release the timer.
func WithOpTimeout(ctx context.Context, op string, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(ctx, d, deadlineCause(op))
}
