package testutil_test

import (
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNewTestLogger(t *testing.T) {
	t.Run("captures output at configured level", func(t *testing.T) {
		ctx, out := testutil.NewTestLogger(t, logrus.WarnLevel)
		log.GetLogger(ctx).Warn("hello warning")
		assert.Contains(t, out.String(), "hello warning")
	})

	t.Run("suppresses output below configured level", func(t *testing.T) {
		ctx, out := testutil.NewTestLogger(t, logrus.WarnLevel)
		log.GetLogger(ctx).Debug("debug noise")
		assert.Empty(t, out.String())
	})

	t.Run("returns non-nil logger in context", func(t *testing.T) {
		ctx, _ := testutil.NewTestLogger(t, logrus.InfoLevel)
		assert.NotNil(t, log.GetLogger(ctx))
	})

	t.Run("each call returns independent buffer", func(t *testing.T) {
		ctx1, out1 := testutil.NewTestLogger(t, logrus.InfoLevel)
		_, out2 := testutil.NewTestLogger(t, logrus.InfoLevel)
		log.GetLogger(ctx1).Info("only in one")
		assert.Contains(t, out1.String(), "only in one")
		assert.Empty(t, out2.String())
	})
}
