package sources_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	client "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMonitoredDatabase_ResolveDatabasesFromPostgres(t *testing.T) {
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	require.NoError(t, err)
	defer func() { assert.NoError(t, pgContainer.Terminate(ctx)) }()

	// Create a new MonitoredDatabase instance
	md := sources.Source{}
	md.Name = "continuous"
	md.Kind = sources.SourcePostgresContinuous
	md.ConnStr, err = pgContainer.ConnectionString(ctx)
	assert.NoError(t, err)

	// Call the ResolveDatabasesFromPostgres method
	dbs, err := sources.ResolveDatabasesFromPostgres(md)
	assert.NoError(t, err)
	assert.True(t, len(dbs) == 2) //postgres and mydatabase

	// check the "continuous_mydatabase"
	db := dbs.GetMonitoredDatabase(md.Name + "_mydatabase")
	assert.NotNil(t, db)
	assert.Equal(t, "mydatabase", db.GetDatabaseName())

	//check unexpected database
	db = dbs.GetMonitoredDatabase(md.Name + "_unexpected")
	assert.Nil(t, db)
}

func TestMonitoredDatabase_ResolveDatabasesFromPatroni(t *testing.T) {
	// Start embedded etcd server
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error"
	e, err := embed.StartEtcd(cfg)
	require.NoError(t, err)
	defer e.Close()

	select {
	case <-e.Server.ReadyNotify():
		// ready
	case <-time.After(5 * time.Second):
		t.Fatal("etcd server took too long to start")
	}

	endpoint := e.Clients[0].Addr().String()

	cli, err := client.New(client.Config{
		Endpoints:   []string{"http://" + endpoint},
		DialTimeout: 2 * time.Second,
	})
	require.NoError(t, err, "failed to create etcd client")
	defer cli.Close()

	// Start postgres server for testing
	pgContainer, err := postgres.Run(ctx,
		ImageName,
		postgres.WithDatabase("mydatabase"),
		postgres.WithInitScripts("../../docker/bootstrap/create_role_db.sql"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	require.NoError(t, err)
	defer func() { assert.NoError(t, pgContainer.Terminate(ctx)) }()

	// Put values to etcd server
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	connStr, err := pgContainer.ConnectionString(ctx)
	require.NoError(t, err)
	_, err = cli.Put(ctx, "/service/batman/members/pg1",
		fmt.Sprintf(`{"role":"master","conn_url":"%s"}`, connStr))
	require.NoError(t, err)
	_, err = cli.Put(ctx, "/service/batman/members/pg2",
		`{"role":"standby","conn_url":"must_be_skipped"}`)
	cancel()
	require.NoError(t, err)

	// Set up Source to use embedded etcd
	md := sources.Source{}
	md.Name = "continuous"
	md.Kind = sources.SourcePatroni
	md.OnlyIfMaster = true
	md.HostConfig.DcsType = "etcd"
	md.HostConfig.DcsEndpoints = []string{"http://" + endpoint}
	md.HostConfig.Scope = "/batman/"
	md.HostConfig.Namespace = "/service"

	// Run ResolveDatabasesFromPatroni
	dbs, err := sources.ResolveDatabasesFromPatroni(md)
	assert.NoError(t, err)
	assert.NotNil(t, dbs)
	assert.Len(t, dbs, 4) // postgres, mydatrabase, pgwatch, pgwatch_metrics
}
