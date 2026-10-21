package sources_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cybertec-postgresql/pgwatch/v6/internal/sources"
)

// writeTestCert writes a self-signed certificate and its key as PEM files
// into dir and returns their paths.
func writeTestCert(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pgwatch-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDer, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}), 0o600))
	return certFile, keyFile
}

func TestGetTransport(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeTestCert(t, dir)
	garbage := filepath.Join(dir, "garbage.pem")
	require.NoError(t, os.WriteFile(garbage, []byte("not a certificate"), 0o600))

	t.Run("no TLS params", func(t *testing.T) {
		cfg, err := sources.GetTransport(sources.HostConfig{})
		require.NoError(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("CA only", func(t *testing.T) {
		cfg, err := sources.GetTransport(sources.HostConfig{CAFile: certFile})
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.NotNil(t, cfg.RootCAs)
		assert.Empty(t, cfg.Certificates)
	})

	t.Run("client cert without CA", func(t *testing.T) {
		cfg, err := sources.GetTransport(sources.HostConfig{CertFile: certFile, KeyFile: keyFile})
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Nil(t, cfg.RootCAs, "system CA pool must be used")
		assert.Len(t, cfg.Certificates, 1)
	})

	t.Run("CA and client cert", func(t *testing.T) {
		cfg, err := sources.GetTransport(sources.HostConfig{CAFile: certFile, CertFile: certFile, KeyFile: keyFile})
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.NotNil(t, cfg.RootCAs)
		assert.Len(t, cfg.Certificates, 1)
	})

	t.Run("missing CA file", func(t *testing.T) {
		_, err := sources.GetTransport(sources.HostConfig{CAFile: filepath.Join(dir, "missing.pem")})
		assert.Error(t, err)
	})

	t.Run("CA file without certificates", func(t *testing.T) {
		_, err := sources.GetTransport(sources.HostConfig{CAFile: garbage})
		assert.ErrorContains(t, err, "no PEM certificates")
	})

	t.Run("bad client key", func(t *testing.T) {
		_, err := sources.GetTransport(sources.HostConfig{CertFile: certFile, KeyFile: garbage})
		assert.Error(t, err)
	})
}
