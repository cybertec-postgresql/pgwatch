package sinks

import (
	"context"
	"os"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func TestMain(m *testing.M) {
	// Setup
	rpcTeardown, err := testutil.SetupRPCServers()
	if err != nil {
		rpcTeardown()
		panic(err)
	}
	var pgTearDown func()
	pgContainer, pgTearDown, err = testutil.SetupPostgresContainer()
	if err != nil {
		panic(err)
	}

	// Execute all tests
	exitCode := m.Run()

	// Teardown
	pgTearDown()
	rpcTeardown()
	os.Exit(exitCode)
}
