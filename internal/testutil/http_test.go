package testutil_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewFakeExporter asserts the fake Prometheus exporter serves the
// supplied body verbatim with the standard exposition Content-Type.
func TestNewFakeExporter(t *testing.T) {
	const body = "# HELP m help\n# TYPE m gauge\nm 42\n"
	srv := testutil.NewFakeExporter(t, body)

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain; version=0.0.4", resp.Header.Get("Content-Type"))

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(b))
}
