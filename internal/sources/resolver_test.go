package sources_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	client "go.etcd.io/etcd/client/v3"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
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
	md.ConnStr, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	// Call the ResolveDatabasesFromPostgres method
	dbs, err := md.ResolveDatabases()
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
	etcdContainer, err := etcd.Run(ctx, "gcr.io/etcd-development/etcd:v3.5.14",
		testcontainers.WithWaitStrategy(wait.ForLog("ready to serve client requests").
			WithStartupTimeout(5*time.Second)))
	require.NoError(t, err)
	defer func() { assert.NoError(t, etcdContainer.Terminate(ctx)) }()

	endpoint, err := etcdContainer.ClientEndpoint(ctx)
	require.NoError(t, err)

	cli, err := client.New(client.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 10 * time.Second,
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
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connStr, err := pgContainer.ConnectionString(cancelCtx, "sslmode=disable")
	require.NoError(t, err)
	_, err = cli.Put(cancelCtx, "/service/batman/members/pg1",
		fmt.Sprintf(`{"role":"master","conn_url":"%s"}`, connStr))
	require.NoError(t, err)
	_, err = cli.Put(cancelCtx, "/service/batman/members/pg2",
		`{"role":"standby","conn_url":"must_be_skipped"}`)
	require.NoError(t, err)

	md := sources.Source{}
	md.Name = "continuous"
	md.OnlyIfMaster = true

	t.Run("simple patroni discovery", func(t *testing.T) {
		md.Kind = sources.SourcePatroni
		md.ConnStr = "etcd://" + strings.TrimPrefix(endpoint, "http://")
		md.ConnStr += "/service"
		md.ConnStr += "/batman"

		// Run ResolveDatabasesFromPatroni
		dbs, err := md.ResolveDatabases()
		assert.NoError(t, err)
		assert.NotNil(t, dbs)
		assert.Len(t, dbs, 4) // postgres, mydatrabase, pgwatch, pgwatch_metrics}
	})

	t.Run("several endpoints patroni discovery", func(t *testing.T) {
		md.Kind = sources.SourcePatroni
		e := strings.TrimPrefix(endpoint, "http://")
		md.ConnStr = "etcd://" + strings.Join([]string{e, e, e}, ",")
		md.ConnStr += "/service"
		md.ConnStr += "/batman"

		// Run ResolveDatabasesFromPatroni
		dbs, err := md.ResolveDatabases()
		assert.NoError(t, err)
		assert.NotNil(t, dbs)
		assert.Len(t, dbs, 4) // postgres, mydatrabase, pgwatch, pgwatch_metrics}
	})

	t.Run("namespace patroni discovery", func(t *testing.T) {
		md.Kind = sources.SourcePatroni
		md.ConnStr = "etcd://" + strings.TrimPrefix(endpoint, "http://")

		// Run ResolveDatabasesFromPatroni
		dbs, err := md.ResolveDatabases()
		assert.NoError(t, err)
		assert.NotNil(t, dbs)
		assert.Len(t, dbs, 4) // postgres, mydatrabase, pgwatch, pgwatch_metrics}
	})
}

func TestMonitoredDatabase_UnsupportedDCS(t *testing.T) {
	md := sources.Source{}
	md.Name = "continuous"
	md.Kind = sources.SourcePatroni

	md.ConnStr = "consul://foo"
	_, err := md.ResolveDatabases()
	assert.ErrorIs(t, err, errors.ErrUnsupported)

	md.ConnStr = "zookeeper://foo"
	_, err = md.ResolveDatabases()
	assert.ErrorIs(t, err, errors.ErrUnsupported)

	md.ConnStr = "unknown://foo"
	_, err = md.ResolveDatabases()
	assert.EqualError(t, err, "unsupported DCS type: unknown")

}
