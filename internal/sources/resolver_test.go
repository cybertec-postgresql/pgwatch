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

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
)

func TestMonitoredDatabase_ResolveDatabasesFromPostgres(t *testing.T) {
	pgContainer, pgTeardown, err := testutil.SetupPostgresContainer()
	require.NoError(t, err)
	defer pgTeardown()

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
	etcdContainer, etcdTeardown, err := testutil.SetupEtcdContainer()
	require.NoError(t, err)
	defer etcdTeardown()

	endpoint, err := etcdContainer.ClientEndpoint(ctx)
	require.NoError(t, err)

	cli, err := client.New(client.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 10 * time.Second,
	})
	require.NoError(t, err, "failed to create etcd client")
	defer cli.Close()

	// Start postgres server for testing
	pgContainer, pgTeardown, err := testutil.SetupPostgresContainerWithInitScripts("../../docker/bootstrap/create_role_db.sql")
	require.NoError(t, err)
	defer pgTeardown()

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

func TestNewHostConfig_BasicParsing(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected sources.HostConfig
		wantErr  bool
	}{
		{
			name: "simple etcd URI",
			uri:  "etcd://localhost:2379/service/demo",
			expected: sources.HostConfig{
				DcsType:      "etcd",
				DcsEndpoints: []string{"http://localhost:2379"},
				Path:         "/service/demo",
			},
		},
		{
			name: "etcd with multiple hosts",
			uri:  "etcd://host1:2379,host2:2379,host3:2379/service/demo",
			expected: sources.HostConfig{
				DcsType:      "etcd",
				DcsEndpoints: []string{"http://host1:2379", "http://host2:2379", "http://host3:2379"},
				Path:         "/service/demo",
			},
		},
		{
			name: "zookeeper URI",
			uri:  "zookeeper://localhost:2181/patroni",
			expected: sources.HostConfig{
				DcsType:      "zookeeper",
				DcsEndpoints: []string{"localhost:2181"},
				Path:         "/patroni",
			},
		},
		{
			name: "consul URI",
			uri:  "consul://localhost:8500/service",
			expected: sources.HostConfig{
				DcsType:      "consul",
				DcsEndpoints: []string{"localhost:8500"},
				Path:         "/service",
			},
		},
		{
			name:    "invalid URI - no scheme",
			uri:     "localhost:2379/service",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			uri:     "redis://localhost:6379",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc, err := sources.NewHostConfig(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected.DcsType, hc.DcsType)
			assert.Equal(t, tt.expected.DcsEndpoints, hc.DcsEndpoints)
			assert.Equal(t, tt.expected.Path, hc.Path)
		})
	}
}

func TestNewHostConfig_WithUserInfo(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		username string
		password string
	}{
		{
			name:     "username only",
			uri:      "etcd://admin@localhost:2379/service",
			username: "admin",
			password: "",
		},
		{
			name:     "username and password",
			uri:      "etcd://admin:secret@localhost:2379/service",
			username: "admin",
			password: "secret",
		},
		{
			name:     "multiple hosts with auth",
			uri:      "etcd://user:pass@host1:2379,host2:2379/service",
			username: "user",
			password: "pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc, err := sources.NewHostConfig(tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.username, hc.Username)
			assert.Equal(t, tt.password, hc.Password)
		})
	}
}

func TestNewHostConfig_WithQueryParameters(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		caFile   string
		certFile string
		keyFile  string
	}{
		{
			name:     "all TLS parameters",
			uri:      "etcd://localhost:2379/service?ca_file=/path/to/ca.crt&cert_file=/path/to/cert.crt&key_file=/path/to/key.key",
			caFile:   "/path/to/ca.crt",
			certFile: "/path/to/cert.crt",
			keyFile:  "/path/to/key.key",
		},
		{
			name:   "only ca_file",
			uri:    "etcd://localhost:2379/service?ca_file=/ca.crt",
			caFile: "/ca.crt",
		},
		{
			name:     "cert and key only",
			uri:      "etcd://localhost:2379/service?cert_file=/cert.crt&key_file=/key.key",
			certFile: "/cert.crt",
			keyFile:  "/key.key",
		},
		{
			name: "no TLS parameters",
			uri:  "etcd://localhost:2379/service",
		},
		{
			name:     "TLS params with multiple hosts",
			uri:      "etcd://host1:2379,host2:2379/service?ca_file=/ca.crt&cert_file=/cert.crt",
			caFile:   "/ca.crt",
			certFile: "/cert.crt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc, err := sources.NewHostConfig(tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.caFile, hc.CAFile)
			assert.Equal(t, tt.certFile, hc.CertFile)
			assert.Equal(t, tt.keyFile, hc.KeyFile)
		})
	}
}

func TestNewHostConfig_WithAuthAndTLS(t *testing.T) {
	uri := "etcd://admin:secret@host1:2379,host2:2379/service/demo?ca_file=/ca.crt&cert_file=/cert.crt&key_file=/key.key"
	hc, err := sources.NewHostConfig(uri)
	require.NoError(t, err)

	assert.Equal(t, "etcd", hc.DcsType)
	assert.Equal(t, []string{"http://host1:2379", "http://host2:2379"}, hc.DcsEndpoints)
	assert.Equal(t, "/service/demo", hc.Path)
	assert.Equal(t, "admin", hc.Username)
	assert.Equal(t, "secret", hc.Password)
	assert.Equal(t, "/ca.crt", hc.CAFile)
	assert.Equal(t, "/cert.crt", hc.CertFile)
	assert.Equal(t, "/key.key", hc.KeyFile)
}

func TestNewHostConfig_PathVariations(t *testing.T) {
	tests := []struct {
		name  string
		uri   string
		path  string
		scope bool
	}{
		{
			name:  "namespace only",
			uri:   "etcd://localhost:2379/service",
			path:  "/service",
			scope: false,
		},
		{
			name:  "namespace and scope",
			uri:   "etcd://localhost:2379/service/demo",
			path:  "/service/demo",
			scope: true,
		},
		{
			name:  "deep path",
			uri:   "etcd://localhost:2379/service/demo/v1",
			path:  "/service/demo/v1",
			scope: true,
		},
		{
			name:  "no path",
			uri:   "etcd://localhost:2379",
			path:  "",
			scope: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc, err := sources.NewHostConfig(tt.uri)
			require.NoError(t, err)
			assert.Equal(t, tt.path, hc.Path)
			assert.Equal(t, tt.scope, hc.IsScopeSpecified())
		})
	}
}

func TestNewHostConfig_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{
			name:    "empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "URI without scheme separator",
			uri:     "etcdlocalhost:2379",
			wantErr: true,
		},
		{
			name:    "URI with invalid host format",
			uri:     "etcd://[::1:2379/service", // malformed IPv6
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sources.NewHostConfig(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
