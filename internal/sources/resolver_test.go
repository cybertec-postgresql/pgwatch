package sources_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	client "go.etcd.io/etcd/client/v3"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
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
	md.Kind = sources.SourcePostgresDiscovery
	md.ConnStr, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	// Call the ResolveDatabasesFromPostgres method
	dbs, err := md.ResolveDatabases()
	assert.NoError(t, err)
	assert.True(t, len(dbs) == 2) //postgres and mydatabase

	// check the "continuous_mydatabase"
	db := dbs.GetMonitoredDatabase(md.Name + "_mydatabase")
	assert.NotNil(t, db)
	dbConn, ok := db.(*sources.DbConn)
	assert.True(t, ok)
	assert.Equal(t, "mydatabase", dbConn.GetDatabaseName())

	//check unexpected database
	db = dbs.GetMonitoredDatabase(md.Name + "_unexpected")
	assert.Nil(t, db)
}

func TestResolveDatabasesFromPostgres_ResolverTimeout(t *testing.T) {
	// Shrink the resolver timeout so the test completes quickly.
	orig := db.ResolverTimeout
	db.ResolverTimeout = 150 * time.Millisecond
	t.Cleanup(func() { db.ResolverTimeout = orig })

	// BlackholeListener accepts TCP but never completes the pgwire handshake,
	// causing NewConn (or the subsequent Query) to stall indefinitely when
	// called with context.Background() / context.TODO().
	addr, closeFn := testutil.BlackholeListener(t)
	defer closeFn()

	// connect_timeout is set longer than ResolverTimeout so that only the
	// ResolverTimeout deadline (once wired) terminates the call.
	connStr := fmt.Sprintf("postgres://postgres@%s/postgres?connect_timeout=10&sslmode=disable", addr)

	md := sources.Source{}
	md.Name = "stall_test"
	md.Kind = sources.SourcePostgresDiscovery
	md.ConnStr = connStr

	start := time.Now()
	_, err := sources.NewResolver().ResolveDatabasesFromPostgres(md)
	elapsed := time.Since(start)

	require.Error(t, err, "expected an error from a stalled resolver")

	// Must return within the deadline (with generous test-scheduling slack).
	assert.Less(t, elapsed, db.ResolverTimeout+2*time.Second,
		"ResolveDatabasesFromPostgres took too long: %v", elapsed)

	// The error must name the resolver operation (surfaced via WithOpTimeout cause).
	assert.Contains(t, err.Error(), "resolve stall_test",
		"error should name the resolver operation; got: %v", err)
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
		md.Kind = sources.SourcePatroniDiscovery
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
		md.Kind = sources.SourcePatroniDiscovery
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
		md.Kind = sources.SourcePatroniDiscovery
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
	md.Kind = sources.SourcePatroniDiscovery

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

// stubNewConnWithDatnames replaces sources.NewConn with a stub that builds a fresh
// pgxmock pool per call so each invocation gets its own mocked discovery query.
// The returned rows closure is invoked inside the stub to construct the row set
// for the current call; this lets a single stub serve concurrent goroutines.
func stubNewConnWithDatnames(t *testing.T, rowsFn func() []string) {
	t.Helper()
	orig := sources.NewConn
	sources.NewConn = func(_ context.Context, _ string, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
		mock, err := pgxmock.NewPool()
		if err != nil {
			return nil, err
		}
		rows := pgxmock.NewRows([]string{"datname"})
		for _, n := range rowsFn() {
			rows.AddRow(n)
		}
		mock.ExpectQuery("pg_database").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(rows)
		return mock, nil
	}
	t.Cleanup(func() { sources.NewConn = orig })
}

// stubNewConnWithError makes sources.NewConn return the given error verbatim.
func stubNewConnWithError(t *testing.T, err error) {
	t.Helper()
	orig := sources.NewConn
	sources.NewConn = func(_ context.Context, _ string, _ ...db.ConnConfigCallback) (db.PgxPoolIface, error) {
		return nil, err
	}
	t.Cleanup(func() { sources.NewConn = orig })
}

func TestResolveDatabasesFromPostgres_LKGFallbackOnFailure(t *testing.T) {
	resolver := sources.NewResolver()

	md := sources.Source{
		Name:    "lkg_failure",
		Kind:    sources.SourcePostgresDiscovery,
		ConnStr: "postgres://user:pw@a:5432/postgres?sslmode=disable",
	}

	// Wave 1: success populates the cache.
	stubNewConnWithDatnames(t, func() []string { return []string{"db1", "db2", "db3"} })
	dbs, err := resolver.ResolveDatabasesFromPostgres(md)
	require.NoError(t, err)
	require.Len(t, dbs, 3)
	firstNames := []string{dbs[0].GetSource().Name, dbs[1].GetSource().Name, dbs[2].GetSource().Name}

	// Wave 2: discovery fails; cache MUST be served with nil error.
	sentinel := errors.New("boom")
	stubNewConnWithError(t, sentinel)
	dbs, err = resolver.ResolveDatabasesFromPostgres(md)
	require.NoError(t, err, "expected cached fallback to swallow the error")
	require.Len(t, dbs, 3, "expected the previously cached list to be returned")
	gotNames := []string{dbs[0].GetSource().Name, dbs[1].GetSource().Name, dbs[2].GetSource().Name}
	assert.Equal(t, firstNames, gotNames)
}

func TestResolveDatabasesFromPostgres_LKGReplacementOnSuccess(t *testing.T) {
	resolver := sources.NewResolver()

	md := sources.Source{
		Name:    "lkg_replace",
		Kind:    sources.SourcePostgresDiscovery,
		ConnStr: "postgres://user:pw@b:5432/postgres?sslmode=disable",
	}

	// Seed cache with the first list.
	stubNewConnWithDatnames(t, func() []string { return []string{"alpha", "beta"} })
	dbs, err := resolver.ResolveDatabasesFromPostgres(md)
	require.NoError(t, err)
	require.Len(t, dbs, 2)

	// Replace with a new, different list via a successful resolution.
	stubNewConnWithDatnames(t, func() []string { return []string{"gamma", "delta", "epsilon"} })
	dbs, err = resolver.ResolveDatabasesFromPostgres(md)
	require.NoError(t, err)
	require.Len(t, dbs, 3)
	newNames := []string{dbs[0].GetSource().Name, dbs[1].GetSource().Name, dbs[2].GetSource().Name}
	assert.Contains(t, newNames, "lkg_replace_gamma")
	assert.Contains(t, newNames, "lkg_replace_delta")
	assert.Contains(t, newNames, "lkg_replace_epsilon")

	// Discovery fails now: the NEW list must be served, not the old one.
	stubNewConnWithError(t, errors.New("still down"))
	dbs, err = resolver.ResolveDatabasesFromPostgres(md)
	require.NoError(t, err)
	require.Len(t, dbs, 3)
	for _, d := range dbs {
		n := d.GetSource().Name
		assert.NotContains(t, []string{"lkg_replace_alpha", "lkg_replace_beta"}, n,
			"stale entry %q was served from cache", n)
	}
}

func TestResolveDatabasesFromPostgres_EmptyCacheErrorPropagates(t *testing.T) {
	resolver := sources.NewResolver()

	sentinel := errors.New("no cache yet")
	stubNewConnWithError(t, sentinel)

	md := sources.Source{
		Name:    "lkg_empty",
		Kind:    sources.SourcePostgresDiscovery,
		ConnStr: "postgres://user:pw@c:5432/postgres?sslmode=disable",
	}

	dbs, err := resolver.ResolveDatabasesFromPostgres(md)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Empty(t, dbs, "no cached entry should have been served")
}

func TestResolveDatabasesFromPostgres_CacheKeyIdentity(t *testing.T) {
	resolver := sources.NewResolver()

	base := sources.Source{
		Name:    "lkg_key",
		Kind:    sources.SourcePostgresDiscovery,
		ConnStr: "postgres://user:pw@d:5432/postgres?sslmode=disable",
	}

	stubNewConnWithDatnames(t, func() []string { return []string{"one", "two"} })
	dbs, err := resolver.ResolveDatabasesFromPostgres(base)
	require.NoError(t, err)
	require.Len(t, dbs, 2)

	// Same Name, DIFFERENT ConnStr: failing must not serve the cached entry.
	sentinelA := errors.New("connstr-A down")
	stubNewConnWithError(t, sentinelA)
	other := sources.Source{
		Name:    base.Name,
		Kind:    sources.SourcePostgresDiscovery,
		ConnStr: "postgres://user:pw@d-other:5432/postgres?sslmode=disable",
	}
	dbs, err = resolver.ResolveDatabasesFromPostgres(other)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelA)
	assert.Empty(t, dbs, "a reconfigured ConnStr must not serve the previous target's list")

	// Same ConnStr, changed IncludePattern: failing must not serve the cached entry either.
	sentinelB := errors.New("include-pattern changed")
	stubNewConnWithError(t, sentinelB)
	repattern := sources.Source{
		Name:           base.Name,
		Kind:           sources.SourcePostgresDiscovery,
		ConnStr:        base.ConnStr,
		IncludePattern: "^foo_",
	}
	dbs, err = resolver.ResolveDatabasesFromPostgres(repattern)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelB)
	assert.Empty(t, dbs, "a reconfigured include_pattern must not serve the previous result set")
}

func TestResolveDatabasesFromPostgres_ConcurrentFallback(t *testing.T) {
	resolver := sources.NewResolver()

	const n = 8
	srcs := make(sources.Sources, n)
	for i := range n {
		srcs[i] = sources.Source{
			Name:    fmt.Sprintf("concurrent_%d", i),
			Kind:    sources.SourcePostgresDiscovery,
			ConnStr: fmt.Sprintf("postgres://user:pw@h%d:5432/postgres?sslmode=disable", i),
		}
	}

	// Wave 1: every source resolves successfully, each to a single distinct datname.
	stubNewConnWithDatnames(t, func() []string {
		// The stub doesn't know which source is calling; that's fine - the source
		// name is taken from the Source struct, not from the mocked rows. We just
		// need at least one row so resolution does not error.
		return []string{"only"}
	})
	dbs, err := resolver.ResolveDatabases(srcs, func(string) {})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(dbs), n, "each source should produce one resolved DB")

	// Wave 2: discovery fails for every source. Cached lists must be served.
	stubNewConnWithError(t, errors.New("discovery down"))
	// Add one brand-new source that has no cached entry.
	extra := sources.Source{
		Name:    "concurrent_extra",
		Kind:    sources.SourcePostgresDiscovery,
		ConnStr: "postgres://user:pw@extra:5432/postgres?sslmode=disable",
	}
	srcs2 := append(sources.Sources{}, srcs...)
	srcs2 = append(srcs2, extra)

	var onErrorNames sync.Map
	dbs2, err := resolver.ResolveDatabases(srcs2, func(name string) {
		onErrorNames.Store(name, struct{}{})
	})
	require.Error(t, err, "the never-cached extra source must propagate its error")
	// Every original (cached) source still contributes its last-known DBs.
	for i := range n {
		require.NotNil(t, dbs2.GetMonitoredDatabase(fmt.Sprintf("concurrent_%d_only", i)),
			"source %d should be served from cache", i)
	}
	// Extra source never cached: its name must appear in onError.
	if _, ok := onErrorNames.Load(extra.Name); !ok {
		t.Fatalf("expected onError to fire for never-cached source %q", extra.Name)
	}
}
