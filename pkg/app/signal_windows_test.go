//go:build windows

package app

import "errors"

// canSelfSignal reports whether the test process can deliver a termination
// signal to itself. Windows has no such thing: the only way in is a console
// control event, which reaches the whole process group and would take the
// `go test` process down with it.
const canSelfSignal = false

// raiseTerm is never reached on Windows; see canSelfSignal.
func raiseTerm() error { return errors.New("cannot raise SIGTERM on windows") }
