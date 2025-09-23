package sources_test

import (
	"context"
	"errors"
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
			WithStartupTimeout(15*time.Second)))
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
	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	// Put values to etcd server
	kv := map[string]string{
		`/service/demo/config`:           `{"ttl":30,"loop_wait":10,"retry_timeout":10,"maximum_lag_on_failover":1048576,"postgresql":{"use_pg_rewind":true,"pg_hba":["local all all trust","host replication replicator all md5","host all all all md5"],"parameters":{"max_connections":100}}}`,
		`/service/demo/initialize`:       `7553211779477532695`,
		`/service/demo/leader`:           `patroni3`,
		`/service/demo/members/patroni1`: `{"conn_url":"postgres://172.18.0.8:5432/postgres","api_url":"http://172.18.0.8:8008/patroni","state":"running","role":"replica","version":"4.0.7","xlog_location":67108960,"replay_lsn":67108960,"receive_lsn":67108960,"replication_state":"streaming","timeline":1}`,
		`/service/demo/members/patroni2`: `{"conn_url":"postgres://172.18.0.4:5432/postgres","api_url":"http://172.18.0.4:8008/patroni","state":"running","role":"replica","version":"4.0.7","xlog_location":67108960,"replay_lsn":67108960,"receive_lsn":67108960,"replication_state":"streaming","timeline":1}`,
		`/service/demo/members/patroni3`: `{"conn_url":"` + pgConnStr + `","api_url":"http://172.18.0.3:8008/patroni","state":"running","role":"primary","version":"4.0.7","xlog_location":67108960,"timeline":1}`,
		`/service/demo/status`:           `{"optime":67108960,"slots":{"patroni1":67108960,"patroni2":67108960,"patroni3":67108960},"retain_slots":["patroni1","patroni2","patroni3"]}}`}

	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	for k, v := range kv {
		_, err = cli.Put(cancelCtx, k, v)
		require.NoError(t, err, "failed to put key %s to etcd", k)
	}
	cancel()

	md := sources.Source{}
	md.Name = "continuous"
	md.OnlyIfMaster = true

	t.Run("simple patroni discovery", func(t *testing.T) {
		md.Kind = sources.SourcePatroni
		md.ConnStr = "etcd://" + strings.TrimPrefix(endpoint, "http://")
		md.ConnStr += "/service"
		md.ConnStr += "/demo"

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
		md.ConnStr += "/demo"

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
