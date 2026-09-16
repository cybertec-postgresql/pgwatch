//go:build !windows

package app

import "syscall"

// canSelfSignal reports whether the test process can deliver a termination
// signal to itself.
const canSelfSignal = true

// raiseTerm delivers SIGTERM to the test process.
func raiseTerm() error { return syscall.Kill(syscall.Getpid(), syscall.SIGTERM) }
