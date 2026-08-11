package testutil

import (
	"context"
	"net"
	"sync"
	"testing"
)

// BlackholeListener starts a TCP listener that accepts connections but
// never reads, writes, or closes them. Each accepted connection runs in
// its own goroutine that blocks until the listener is shut down.
//
// This simulates a half-open TCP connection: the client sees an
// established TCP session, but no application-layer response ever arrives.
// The kernel keeps the connection open until the local read deadline (if
// any) fires or the peer gives up. Callers pair this with a context-bound
// round-trip to convert such stalls into bounded client-side failures.
//
// The address is registered for cleanup via t.Cleanup, so a single
// BlackholeListener(t) call is leak-free under normal test runs. The
// returned close func is idempotent and may also be invoked explicitly by
// the test for early teardown.
//
// Concurrency: the accept loop and any number of accepted connections run
// concurrently. Accepted goroutines block until ctx is cancelled; the close
// func waits for them via wg.Wait before returning.
func BlackholeListener(t *testing.T) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("blackhole: listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Accept loop. Stops as soon as the listener is closed.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				// Hold the connection until the listener is closed.
				// Intentionally no read/write/close — we are simulating
				// a peer that silently dropped packets.
				<-ctx.Done()
			}(conn)
		}
	}()

	close := func() {
		cancel()
		_ = ln.Close()
		wg.Wait()
	}
	t.Cleanup(close)
	return ln.Addr().String(), close
}
