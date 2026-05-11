package testutil

import (
	"context"
	"strings"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/sirupsen/logrus"
)

// NewTestLogger creates a capturing logger at the given level, injects it into
// a context derived from t.Context(), and returns both the context and the
// output buffer. Use the buffer in assertions to verify log output:
//
//	ctx, out := testutil.NewTestLogger(t, logrus.WarnLevel)
//	doSomething(ctx)
//	assert.Contains(t, out.String(), "expected warning")
func NewTestLogger(t *testing.T, level logrus.Level) (context.Context, *strings.Builder) {
	t.Helper()
	var out strings.Builder
	l := logrus.New()
	l.SetOutput(&out)
	l.SetLevel(level)
	return log.WithLogger(t.Context(), l), &out
}
