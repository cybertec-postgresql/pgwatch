package webserver

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cybertec-postgresql/pgwatch/v6/pkg/metrics"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type mockFS struct {
	OpenFunc func(name string) (fs.File, error)
}

func (m mockFS) Open(name string) (fs.File, error) {
	return m.OpenFunc(name)
}

// mockProvider serves a caller-supplied file system as the web UI.
type mockProvider struct {
	fsys   fs.FS
	routes []string
	data   map[string]any
}

func (p mockProvider) FS() fs.FS { return p.fsys }

func (p mockProvider) SPARoutes() []string {
	if p.routes == nil {
		return []string{"/", "/sources", "/metrics", "/presets", "/logs"}
	}
	return p.routes
}

func (p mockProvider) IndexData() map[string]any { return p.data }

func TestServer_handleStatic(t *testing.T) {
	tempFile := path.Join(t.TempDir(), "file.ext")
	assert.NoError(t, os.WriteFile(tempFile, []byte(`{"foo": {"bar": 1}}`), 0644))

	indexHTML := []byte(`<!DOCTYPE html><html><head><script>window.__PGWATCH_BASE_PATH__='';</script></head><body>{"foo": {"bar": 1}}</body></html>`)

	uiFS := mockFS{
		OpenFunc: func(name string) (fs.File, error) {
			switch name {
			case "index.html", "static/file.ext":
				return os.Open(tempFile)
			case "badfile.ext":
				return nil, fs.ErrInvalid
			default:
				return nil, fs.ErrNotExist
			}
		},
	}

	ts := &WebUIServer{
		Logger:     logrus.StandardLogger(),
		indexHTML:  indexHTML,
		uiProvider: mockProvider{fsys: uiFS},
	}

	t.Run("not GET", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/static/file.ext", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "Method Not Allowed\n", string(body))
	})

	t.Run("some static file", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/static/file.ext", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		var got map[string]metrics.Metric
		assert.NoError(t, json.Unmarshal(body, &got))
		assert.Contains(t, got, "foo")
	})

	t.Run("predefined route", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "<!DOCTYPE html>")
		assert.Contains(t, bodyStr, "window.__PGWATCH_BASE_PATH__")
	})

	t.Run("file not found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/static/notfound.ext", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "404 page not found\n", string(body))
	})

	t.Run("file cannot be read", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/badfile.ext", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func TestServer_handleTestConnect(t *testing.T) {
	ts := &WebUIServer{
		Logger: logrus.StandardLogger(),
	}

	t.Run("POST", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/testconnect", strings.NewReader("bad connection string"))
		w := httptest.NewRecorder()
		ts.handleTestConnect(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("failed reader", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/testconnect", &errorReader{})
		w := httptest.NewRecorder()
		ts.handleTestConnect(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("GET", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/testconnect", nil)
		w := httptest.NewRecorder()
		ts.handleTestConnect(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, "Method Not Allowed\n", string(body))
	})
}

func TestServer_isSPARoute(t *testing.T) {
	tests := []struct {
		name   string
		routes []string
		path   string
		want   bool
	}{
		{"default route set, root", nil, "/", true},
		{"default route set, known route", nil, "/metrics", true},
		{"default route set, unknown route", nil, "/unknown", false},
		{"default route set, asset", nil, "/static/app.js", false},
		{"provider route set, listed", []string{"/dash"}, "/dash", true},
		{"provider route set, not listed", []string{"/dash"}, "/sources", false},
		{"empty route set serves nothing as index", []string{}, "/", false},
		{"wildcard, root", []string{"*"}, "/", true},
		{"wildcard, arbitrary path", []string{"*"}, "/anything/deep", true},
		{"wildcard, path with extension", []string{"*"}, "/static/app.js", false},
		{"wildcard, dot in a non-final segment", []string{"*"}, "/v1.2/page", true},
		{"literal star among others is not a wildcard", []string{"*", "/dash"}, "/anything", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &WebUIServer{uiProvider: mockProvider{routes: tt.routes}}
			assert.Equal(t, tt.want, s.isSPARoute(tt.path))
		})
	}
}

func TestServer_handleStatic_wildcardRoutes(t *testing.T) {
	indexHTML := []byte(`<!DOCTYPE html><html><body>index</body></html>`)
	ts := &WebUIServer{
		Logger:     logrus.StandardLogger(),
		indexHTML:  indexHTML,
		uiProvider: mockProvider{fsys: mockFS{OpenFunc: func(string) (fs.File, error) { return nil, fs.ErrNotExist }}, routes: []string{"*"}},
	}

	t.Run("extension-less path serves index.html", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/whatever/the/embedder/wants", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, string(indexHTML), string(body))
	})

	t.Run("path with extension is looked up as a file", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/static/missing.js", nil)
		w := httptest.NewRecorder()
		ts.handleStatic(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestServer_prepareIndexHTML(t *testing.T) {
	indexFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html><html><body>base={{.BasePath}} foo={{.Foo}}</body></html>`)},
	}

	t.Run("provider data is merged", func(t *testing.T) {
		s := &WebUIServer{
			CmdOpts:    CmdOpts{WebBasePath: "pgwatch"},
			uiProvider: mockProvider{fsys: indexFS, data: map[string]any{"Foo": "bar"}},
		}
		assert.NoError(t, s.prepareIndexHTML())
		assert.Contains(t, string(s.indexHTML), "base=pgwatch")
		assert.Contains(t, string(s.indexHTML), "foo=bar")
	})

	t.Run("provider cannot override BasePath", func(t *testing.T) {
		s := &WebUIServer{
			CmdOpts:    CmdOpts{WebBasePath: "pgwatch"},
			uiProvider: mockProvider{fsys: indexFS, data: map[string]any{"BasePath": "hijacked", "Foo": "bar"}},
		}
		assert.NoError(t, s.prepareIndexHTML())
		assert.Contains(t, string(s.indexHTML), "base=pgwatch")
		assert.NotContains(t, string(s.indexHTML), "hijacked")
	})

	t.Run("nil provider data is fine", func(t *testing.T) {
		s := &WebUIServer{uiProvider: mockProvider{fsys: indexFS}}
		assert.NoError(t, s.prepareIndexHTML())
		assert.Contains(t, string(s.indexHTML), "base= foo=")
	})
}
