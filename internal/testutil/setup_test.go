package testutil_test

import (
	"context"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestLoadServerTLSCredentials(t *testing.T) {
	t.Run("valid credentials", func(t *testing.T) {
		creds, err := testutil.LoadServerTLSCredentials()
		assert.NoError(t, err)
		assert.NotNil(t, creds)
		assert.Equal(t, "tls", creds.Info().SecurityProtocol)
	})

	t.Run("invalid certificate", func(t *testing.T) {
		// Save original values
		origCert := testutil.Cert
		origKey := testutil.PrivateKey
		defer func() {
			testutil.Cert = origCert
			testutil.PrivateKey = origKey
		}()

		// Test with invalid cert/key pair
		testutil.Cert = []byte("invalid cert")
		testutil.PrivateKey = []byte("invalid key")

		creds, err := testutil.LoadServerTLSCredentials()
		assert.Error(t, err)
		assert.Nil(t, creds)
	})
}

func TestAuthInterceptor(t *testing.T) {
	handler := func(context.Context, any) (any, error) {
		return "success", nil
	}

	t.Run("valid credentials", func(t *testing.T) {
		md := metadata.Pairs("username", "pgwatch", "password", "pgwatch")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		result, err := testutil.AuthInterceptor(ctx, nil, nil, handler)
		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})

	t.Run("empty credentials", func(t *testing.T) {
		md := metadata.Pairs("username", "", "password", "")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		result, err := testutil.AuthInterceptor(ctx, nil, nil, handler)
		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})

	t.Run("invalid credentials", func(t *testing.T) {
		md := metadata.Pairs("username", "wrong", "password", "wrong")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		result, err := testutil.AuthInterceptor(ctx, nil, nil, handler)
		assert.Error(t, err)
		assert.Nil(t, result)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
	})
}

func TestSetupPostgresContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container test in short mode")
	}

	container, teardown, err := testutil.SetupPostgresContainer()
	for i := range 2 {
		if i == 1 {
			container, teardown, err = testutil.SetupPostgresContainerWithInitScripts("../../docker/bootstrap/create_role_db.sql")
		}

		if err != nil {
			t.Skipf("Skipping postgres container test: %v", err)
			return
		}
		defer teardown()

		assert.NotNil(t, container)

		// Verify container is running
		state, err := container.State(context.Background())
		require.NoError(t, err)
		assert.True(t, state.Running)

		// Verify connection string is available
		connStr, err := container.ConnectionString(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, connStr)
	}
}

func TestSetupEtcdContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping etcd container test in short mode")
	}

	etcdContainer, etcdTeardown, err := testutil.SetupEtcdContainer()
	require.NoError(t, err)
	defer etcdTeardown()

	// Verify container is running
	state, err := etcdContainer.State(context.Background())
	require.NoError(t, err)
	assert.True(t, state.Running)
}