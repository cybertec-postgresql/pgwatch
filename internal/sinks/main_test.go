package sinks

import (
	"context"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func TestMain(m *testing.M) {
	// Setup

	err := RPCTestsSetup()
	if err != nil {
		panic(err)
	}

	err = PGTestsSetup()
	if err != nil {
		panic(err)
	}

	// Execute all tests
	exitCode := m.Run()

	// Teardown
	RPCTestsTeardown()
	PGTestsTeardown()

	os.Exit(exitCode)
}
