package webserver_test

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/ui"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/webserver"
	"github.com/stretchr/testify/assert"
)

// testProvider is a minimal ui.Provider standing in for the React UI.
type testProvider struct{}

func (testProvider) FS() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html><html><body>{{.BasePath}}</body></html>`)},
	}
}

func (testProvider) SPARoutes() []string { return []string{"/"} }

func (testProvider) IndexData() map[string]any { return nil }

// withUI is the option every test that leaves the UI enabled must pass.
func withUI() webserver.Option { return webserver.WithUI(ui.Provider(testProvider{})) }

type Credentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type ReadyBool bool

func (ready *ReadyBool) Ready() bool {
	return bool(*ready)
}

func TestWebDisableOpt(t *testing.T) {
	var ready ReadyBool

	t.Run("all: no server at all", func(t *testing.T) {
		restsrv, err := webserver.Init(context.Background(), webserver.CmdOpts{WebDisable: "all"}, nil, nil, &ready)
		assert.Nil(t, restsrv, "no webserver should be started")
		assert.NoError(t, err)
	})

	t.Run("ui: rest api served, no provider needed", func(t *testing.T) {
		restsrv, err := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "127.0.0.1:8079", WebDisable: "ui"}, nil, nil, &ready)
		assert.NotNil(t, restsrv)
		assert.NoError(t, err, "the UI provider must not be required when the UI is disabled")
		r, err := http.Get("http://localhost:8079/")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, r.StatusCode, "no webui should be served")
		r, err = http.Get("http://localhost:8079/liveness")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, r.StatusCode, "rest api should be served though")
	})

	t.Run("listen error is reported", func(t *testing.T) {
		restsrv, err := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "127.0.0.1:8079"}, nil, nil, &ready, withUI())
		assert.Nil(t, restsrv)
		assert.Error(t, err, "port should be in use")
	})

	t.Run("ui enabled without a provider fails", func(t *testing.T) {
		restsrv, err := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "127.0.0.1:8078"}, nil, nil, &ready)
		assert.Nil(t, restsrv)
		assert.Error(t, err, "a UI-enabled server without a provider should not start")
	})
}

// wildcardProvider is an embedder-style provider: it claims every client-side
// route and contributes its own index.html template data.
type wildcardProvider struct{}

func (wildcardProvider) FS() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<!DOCTYPE html><html><body>base=[{{.BasePath}}] foo=[{{.Foo}}]</body></html>`)},
		"static/app.js": &fstest.MapFile{Data: []byte(`console.log("app")`)},
	}
}

func (wildcardProvider) SPARoutes() []string { return []string{"*"} }

func (wildcardProvider) IndexData() map[string]any {
	return map[string]any{"Foo": "bar", "BasePath": "hijacked"}
}

func TestProviderDrivenUI(t *testing.T) {
	host := "http://localhost:8083"
	restsrv, err := webserver.Init(context.Background(),
		webserver.CmdOpts{WebAddr: "localhost:8083", WebBasePath: "pgwatch"},
		nil, nil, nil, webserver.WithUI(ui.Provider(wildcardProvider{})))
	assert.NoError(t, err)
	assert.NotNil(t, restsrv)

	get := func(path string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, host+path, nil)
		assert.NoError(t, err)
		restsrv.Handler.ServeHTTP(rr, req)
		return rr
	}

	t.Run("index data is rendered, BasePath comes from the server", func(t *testing.T) {
		rr := get("/pgwatch/")
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "foo=[bar]")
		assert.Contains(t, rr.Body.String(), "base=[pgwatch]")
		assert.NotContains(t, rr.Body.String(), "hijacked")
	})

	t.Run("wildcard answers an unknown route with index.html", func(t *testing.T) {
		rr := get("/pgwatch/an/embedder/route")
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Contains(t, rr.Body.String(), "foo=[bar]")
	})

	t.Run("assets are still served from the provider FS", func(t *testing.T) {
		rr := get("/pgwatch/static/app.js")
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `console.log("app")`)
	})
}

func TestHealth(t *testing.T) {
	var ready ReadyBool
	ctx, cancel := context.WithCancel(context.Background())
	restsrv, _ := webserver.Init(ctx, webserver.CmdOpts{WebAddr: "127.0.0.1:8080"}, nil, nil, &ready, withUI())
	assert.NotNil(t, restsrv)

	r, err := http.Get("http://localhost:8080/liveness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)

	cancel()
	r, err = http.Get("http://localhost:8080/liveness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, r.StatusCode)

	r, err = http.Get("http://localhost:8080/readiness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, r.StatusCode)

	ready = true
	r, err = http.Get("http://localhost:8080/readiness")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, r.StatusCode)
}

func TestServerNoAuth(t *testing.T) {
	host := "http://localhost:8081"
	restsrv, _ := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "localhost:8081"}, nil, nil, nil, withUI())
	assert.NotNil(t, restsrv)
	rr := httptest.NewRecorder()
	// cors OPTIONS
	reqOpts, err := http.NewRequest("OPTIONS", host, nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqOpts)
	assert.Equal(t, http.StatusOK, rr.Code)

	// test request metrics
	rr = httptest.NewRecorder()
	reqMetric, err := http.NewRequest("GET", host+"/metric", nil)
	restsrv.Handler.ServeHTTP(rr, reqMetric)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

	// test request database
	rr = httptest.NewRecorder()
	reqDb, err := http.NewRequest("GET", host+"/source", nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqDb)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

	// test request
	rr = httptest.NewRecorder()
	reqLog, err := http.NewRequest("GET", host+"/log", nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqLog)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")

	// request metrics
	rr = httptest.NewRecorder()
	reqConnect, err := http.NewRequest("GET", host+"/test-connect", nil)
	assert.NoError(t, err)
	restsrv.Handler.ServeHTTP(rr, reqConnect)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "REQUEST WITHOUT AUTHENTICATION")
}

func TestGetToken(t *testing.T) {
	host := "http://localhost:8082"
	restsrv, _ := webserver.Init(context.Background(), webserver.CmdOpts{WebAddr: "localhost:8082"}, nil, nil, nil, withUI())
	rr := httptest.NewRecorder()

	credentials := Credentials{
		User:     "admin",
		Password: "admin",
	}

	payload, err := jsoniter.ConfigFastest.Marshal(credentials)
	assert.NoError(t, err)

	reqToken, err := http.NewRequest("POST", host+"/login", strings.NewReader(string(payload)))
	assert.Equal(t, err, nil)

	restsrv.Handler.ServeHTTP(rr, reqToken)

	assert.Equal(t, rr.Code, http.StatusOK, "TOKEN RESPONSE OK")

	token, err := io.ReadAll(rr.Body)
	assert.Equal(t, err, nil)
	assert.NotEqual(t, token, nil)
}
