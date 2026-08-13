package testutil_test

import (
	"net"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlackholeListener asserts the half-open peer contract:
// - the listener accepts TCP connections,
// - it never writes or reads from them, so a client Read times out,
// - the returned close func is safe to call while a connection is held,
// - calling the close func twice is a documented no-op.
func TestBlackholeListener(t *testing.T) {
	addr, closeFn := testutil.BlackholeListener(t)
	require.NotEmpty(t, addr)

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Exercise the accept loop and the per-connection holder goroutine
	// by reading; the peer never responds, so the deadline must fire.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, err = conn.Read(make([]byte, 1))
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout())

	// Early close while a holder goroutine is still parked on <-ctx.Done();
	// second call is idempotent.
	closeFn()
	closeFn()
}
