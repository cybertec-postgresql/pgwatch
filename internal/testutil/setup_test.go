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

func TestSetupRPCServers(t *testing.T) {
	t.Run("successful setup", func(t *testing.T) {
		teardown, err := testutil.SetupRPCServers()
		require.NoError(t, err)
		require.NotNil(t, teardown)
		defer teardown()

		// Verify CA file was created
		assert.FileExists(t, testutil.CAFile)
	})

	t.Run("error writing CA file", func(t *testing.T) {
		// Save original CAFile
		origCAFile := testutil.CAFile
		defer func() {
			testutil.CAFile = origCAFile
		}()

		// Use invalid path to trigger write error
		testutil.CAFile = "/invalid/path/that/does/not/exist/ca.crt"

		teardown, err := testutil.SetupRPCServers()
		assert.Error(t, err)
		assert.NotNil(t, teardown) // teardown should still be returned

		// Call teardown to ensure it doesn't panic
		teardown()
	})
}

func TestSetupPostgresContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container test in short mode")
	}

	container, teardown, err := testutil.SetupPostgresContainer()
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
