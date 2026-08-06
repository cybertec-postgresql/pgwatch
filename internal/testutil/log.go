package testutil

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/sirupsen/logrus"
)

// SafeBuffer is a goroutine-safe string buffer that satisfies io.Writer.
// Use String() to read the accumulated output.
type SafeBuffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *SafeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

// String returns a snapshot of the accumulated log output.
func (b *SafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

// NewTestLogger creates a capturing logger at the given level, injects it into
// a context derived from t.Context(), and returns both the context and the
// output buffer. Use the buffer in assertions to verify log output:
//
//	ctx, out := testutil.NewTestLogger(t, logrus.WarnLevel)
//	doSomething(ctx)
//	assert.Contains(t, out.String(), "expected warning")
func NewTestLogger(t *testing.T, level logrus.Level) (context.Context, *SafeBuffer) {
	t.Helper()
	var out SafeBuffer
	l := logrus.New()
	l.SetOutput(&out)
	l.SetLevel(level)
	return log.WithLogger(t.Context(), l), &out
}
