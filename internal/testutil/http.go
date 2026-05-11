package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NewFakeExporter starts an httptest.Server that responds to any request with
// the provided Prometheus exposition-format body and Content-Type
// "text/plain; version=0.0.4". The server is automatically closed when the
// test finishes via t.Cleanup.
func NewFakeExporter(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}
