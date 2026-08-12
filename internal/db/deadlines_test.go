package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestWithFetchTimeout_BelowFloor(t *testing.T) {
	orig := MinFetchTimeout
	t.Cleanup(func() { MinFetchTimeout = orig })

	synctest.Test(t, func(t *testing.T) {
		MinFetchTimeout = 50 * time.Millisecond
		ctx, cancel := WithFetchTimeout(context.Background(), "fetch x", 5*time.Millisecond)
		defer cancel()
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline to be set")
		}
		if got := time.Until(deadline); got < 40*time.Millisecond || got > 60*time.Millisecond {
			t.Fatalf("deadline remaining %v, want ~%v (the floor)", got, MinFetchTimeout)
		}
	})
}

func TestWithFetchTimeout_AboveFloor(t *testing.T) {
	orig := MinFetchTimeout
	t.Cleanup(func() { MinFetchTimeout = orig })

	synctest.Test(t, func(t *testing.T) {
		MinFetchTimeout = 10 * time.Millisecond
		ctx, cancel := WithFetchTimeout(context.Background(), "fetch x", 250*time.Millisecond)
		defer cancel()
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline to be set")
		}
		if got := time.Until(deadline); got < 240*time.Millisecond || got > 260*time.Millisecond {
			t.Fatalf("deadline remaining %v, want ~250ms (the interval)", got)
		}
	})
}

func TestWithFetchTimeout_DeadlineFires(t *testing.T) {
	orig := MinFetchTimeout
	t.Cleanup(func() { MinFetchTimeout = orig })

	synctest.Test(t, func(t *testing.T) {
		MinFetchTimeout = 25 * time.Millisecond
		ctx, cancel := WithFetchTimeout(context.Background(), "fetch db_stats", time.Millisecond)
		defer cancel()
		time.Sleep(50 * time.Millisecond) // fake clock advances past the 25ms deadline
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got %v", ctx.Err())
		}
		cause := context.Cause(ctx)
		if !strings.Contains(cause.Error(), "fetch db_stats") {
			t.Fatalf("cause %q does not embed operation name", cause)
		}
	})
}

func TestWithOpTimeout_DeadlineEqualsD(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := WithOpTimeout(context.Background(), "ping", 75*time.Millisecond)
		defer cancel()
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline")
		}
		if got := time.Until(deadline); got < 65*time.Millisecond || got > 85*time.Millisecond {
			t.Fatalf("deadline remaining %v, want ~75ms", got)
		}
	})
}

func TestWithOpTimeout_DeadlineFires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := WithOpTimeout(context.Background(), "resolve source-x", 25*time.Millisecond)
		defer cancel()
		time.Sleep(50 * time.Millisecond)
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got %v", ctx.Err())
		}
		cause := context.Cause(ctx)
		if !strings.Contains(cause.Error(), "resolve source-x") {
			t.Fatalf("cause %q does not embed operation name", cause)
		}
	})
}

func TestWithOpTimeout_ParentCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, parentCancel := context.WithCancel(context.Background())
		ctx, cancel := WithOpTimeout(parent, "ping", time.Hour)
		defer cancel()
		parentCancel()
		time.Sleep(time.Millisecond)
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", ctx.Err())
		}
		cause := context.Cause(ctx)
		if errors.Is(cause, context.DeadlineExceeded) {
			t.Fatalf("cause should not be DeadlineExceeded for parent cancel; got %v", cause)
		}
	})
}
