package sources_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch3/sources"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMonitoredDatabase_ResolveDatabasesFromPostgres(t *testing.T) {
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("docker.io/postgres:16-alpine"),
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	assert.NoError(t, err)
	defer func() { assert.NoError(t, pgContainer.Terminate(ctx)) }()

	// Create a new MonitoredDatabase instance
	md := &sources.MonitoredDatabase{DBUniqueName: "continuous", Kind: sources.SourcePostgresContinuous}
	md.ConnStr, err = pgContainer.ConnectionString(ctx)
	assert.NoError(t, err)

	// Call the ResolveDatabasesFromPostgres method
	dbs, err := sources.ResolveDatabasesFromPostgres(md)
	assert.NoError(t, err)
	assert.True(t, len(dbs) == 2) //postgres and mydatabase

	// check the "continuous_mydatabase"
	db := dbs.GetMonitoredDatabase(md.DBUniqueName + "_mydatabase")
	assert.NotNil(t, db)
	assert.Equal(t, "mydatabase", db.GetDatabaseName())

	//check unexpected database
	db = dbs.GetMonitoredDatabase(md.DBUniqueName + "_unexpected")
	assert.Nil(t, db)
}
