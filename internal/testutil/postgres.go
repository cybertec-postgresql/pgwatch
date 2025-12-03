package testutil

import (
	"context"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var ctx = log.WithLogger(context.Background(), log.NewNoopLogger())

func SetupPostgresContainer() (*postgres.PostgresContainer, func(), error) {
	const ImageName = "docker.io/postgres:17-alpine"
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)

	tearDown := func () {
		_ = pgContainer.Terminate(ctx)
	}

	return pgContainer, tearDown, err
}