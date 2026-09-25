package webserver_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/webserver"
)

// login returns a valid JWT issued by the given server.
func login(t *testing.T, srv *webserver.WebUIServer, host string) string {
	t.Helper()
	payload, err := jsoniter.ConfigFastest.Marshal(Credentials{User: "admin", Password: "admin"})
	assert.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, host+"/login", strings.NewReader(string(payload)))
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	token, err := io.ReadAll(rr.Body)
	assert.NoError(t, err)
	return string(token)
}

func TestWithRoutes(t *testing.T) {
	host := "http://localhost:8084"
	hook := func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler) {
		mux.Handle(basePath+"hello", auth(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hello"))
		}))
		mux.HandleFunc(basePath+"public", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("public"))
		})
		// an embedder must not be able to take over a built-in route
		mux.HandleFunc(basePath+"source", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hijacked"))
		})
	}
	srv, err := webserver.Init(context.Background(),
		webserver.CmdOpts{WebAddr: "localhost:8084", WebBasePath: "pgwatch"},
		nil, nil, nil, withUI(), webserver.WithRoutes(hook))
	assert.NoError(t, err)
	assert.NotNil(t, srv)

	get := func(path, token string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, host+path, nil)
		assert.NoError(t, err)
		if token != "" {
			req.Header.Set("Token", token)
		}
		rr := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rr, req)
		return rr
	}

	t.Run("hook route is behind the shared JWT check", func(t *testing.T) {
		rr := get("/pgwatch/hello", "")
		assert.Equal(t, http.StatusUnauthorized, rr.Code)

		rr = get("/pgwatch/hello", login(t, srv, host+"/pgwatch"))
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "hello", rr.Body.String())
	})

	t.Run("hook routes are mounted under the base path", func(t *testing.T) {
		rr := get("/pgwatch/public", "")
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "public", rr.Body.String())

		// outside the base path the SPA handler answers, not the hook
		rr = get("/public", "")
		assert.NotEqual(t, "public", rr.Body.String())
	})

	t.Run("hook routes cannot shadow built-in ones", func(t *testing.T) {
		rr := get("/pgwatch/source", "")
		assert.Equal(t, http.StatusUnauthorized, rr.Code, "the built-in handler must still be in place")
		assert.NotContains(t, rr.Body.String(), "hijacked")
	})
}

func TestWithRoutesUIDisabled(t *testing.T) {
	host := "http://localhost:8085"
	srv, err := webserver.Init(context.Background(),
		webserver.CmdOpts{WebAddr: "localhost:8085", WebDisable: webserver.WebDisableUI},
		nil, nil, nil, webserver.WithRoutes(func(mux *http.ServeMux, basePath string, _ func(http.HandlerFunc) http.Handler) {
			mux.HandleFunc(basePath+"hello", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("hello"))
			})
		}))
	assert.NoError(t, err)
	assert.NotNil(t, srv)

	req, err := http.NewRequest(http.MethodGet, host+"/hello", nil)
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "hello", rr.Body.String())
}

func TestCORSOrigin(t *testing.T) {
	origin := func(t *testing.T, srv *webserver.WebUIServer) string {
		t.Helper()
		req, err := http.NewRequest(http.MethodOptions, "http://localhost/liveness", nil)
		assert.NoError(t, err)
		rr := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		return rr.Header().Get("Access-Control-Allow-Origin")
	}

	t.Run("default is unchanged", func(t *testing.T) {
		srv, err := webserver.Init(context.Background(),
			webserver.CmdOpts{WebAddr: "localhost:8086"}, nil, nil, nil, withUI())
		assert.NoError(t, err)
		assert.Equal(t, "http://localhost:4000", origin(t, srv))
	})

	t.Run("flag sets the origin", func(t *testing.T) {
		srv, err := webserver.Init(context.Background(),
			webserver.CmdOpts{WebAddr: "localhost:8087", WebCORSOrigin: "https://flag.test"},
			nil, nil, nil, withUI())
		assert.NoError(t, err)
		assert.Equal(t, "https://flag.test", origin(t, srv))
	})

	t.Run("option overrides the flag", func(t *testing.T) {
		srv, err := webserver.Init(context.Background(),
			webserver.CmdOpts{WebAddr: "localhost:8088", WebCORSOrigin: "https://flag.test"},
			nil, nil, nil, withUI(), webserver.WithCORSOrigin("https://example.test"))
		assert.NoError(t, err)
		assert.Equal(t, "https://example.test", origin(t, srv))
	})
}

// TestWithRoutesPathValues makes sure the dispatcher installed by WithRoutes
// keeps the wildcards of the built-in routes populated.
func TestWithRoutesPathValues(t *testing.T) {
	host := "http://localhost:8089"
	dir := t.TempDir()
	sourcesFile := filepath.Join(dir, "sources.yaml")
	assert.NoError(t, os.WriteFile(sourcesFile, []byte("[]\n"), 0644))
	metricsFile := filepath.Join(dir, "metrics.yaml")
	assert.NoError(t, os.WriteFile(metricsFile, []byte("metrics:\npresets:\n"), 0644))

	srw, err := sources.NewYAMLSourcesReaderWriter(context.Background(), sourcesFile)
	assert.NoError(t, err)
	mrw, err := metrics.NewYAMLMetricReaderWriter(context.Background(), metricsFile)
	assert.NoError(t, err)

	srv, err := webserver.Init(context.Background(),
		webserver.CmdOpts{WebAddr: "localhost:8089"}, mrw, srw, nil, withUI(),
		webserver.WithRoutes(func(mux *http.ServeMux, basePath string, _ func(http.HandlerFunc) http.Handler) {
			mux.HandleFunc(basePath+"ext", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ext"))
			})
		}))
	assert.NoError(t, err)
	assert.NotNil(t, srv)

	token := login(t, srv, host)
	get := func(path string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, host+path, nil)
		assert.NoError(t, err)
		req.Header.Set("Token", token)
		rr := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rr, req)
		return rr
	}

	for _, path := range []string{"/source/does-not-exist", "/metric/does-not-exist", "/preset/does-not-exist"} {
		t.Run(path, func(t *testing.T) {
			rr := get(path)
			assert.Equal(t, http.StatusNotFound, rr.Code, "the handler must see the {name} path value: %s", rr.Body.String())
			assert.NotContains(t, rr.Body.String(), "is required")
		})
	}

	t.Run("extension route still works", func(t *testing.T) {
		rr := get("/ext")
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "ext", rr.Body.String())
	})
}

// TestBasePathSpellings makes sure every spelling of the base path gives the
// same routes, the same rendered BasePath and the same hook prefix.
func TestBasePathSpellings(t *testing.T) {
	for _, basePath := range []string{"pgwatch", "/pgwatch", "pgwatch/", "/pgwatch/"} {
		t.Run(basePath, func(t *testing.T) {
			var hookBasePath string
			srv, err := webserver.Init(context.Background(),
				webserver.CmdOpts{WebAddr: "localhost:0", WebBasePath: basePath},
				nil, nil, nil, withUI(),
				webserver.WithRoutes(func(_ *http.ServeMux, basePath string, _ func(http.HandlerFunc) http.Handler) {
					hookBasePath = basePath
				}))
			assert.NoError(t, err)
			assert.NotNil(t, srv)
			assert.Equal(t, "/pgwatch/", hookBasePath)

			get := func(path string) *httptest.ResponseRecorder {
				req, err := http.NewRequest(http.MethodGet, "http://localhost"+path, nil)
				assert.NoError(t, err)
				rr := httptest.NewRecorder()
				srv.Handler.ServeHTTP(rr, req)
				return rr
			}

			rr := get("/pgwatch/liveness")
			assert.Equal(t, http.StatusOK, rr.Code)

			rr = get("/pgwatch/")
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "<!DOCTYPE html><html><body>pgwatch</body></html>", rr.Body.String())
		})
	}
}

func TestBasePathInvalid(t *testing.T) {
	addr := "localhost:8090"
	srv, err := webserver.Init(context.Background(),
		webserver.CmdOpts{WebAddr: addr, WebBasePath: "a//b"}, nil, nil, nil, withUI())
	assert.ErrorContains(t, err, "--web-base-path")
	assert.ErrorContains(t, err, "a//b")
	assert.Nil(t, srv)

	// the port must still be free
	ln, err := net.Listen("tcp", addr)
	assert.NoError(t, err)
	if ln != nil {
		_ = ln.Close()
	}
}
