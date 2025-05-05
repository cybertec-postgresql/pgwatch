package webserver

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type mockFS struct {
	OpenFunc func(name string) (fs.File, error)
}

func (m mockFS) Open(name string) (fs.File, error) {
	return m.OpenFunc(name)
}

func TestServer_handleStatic(t *testing.T) {
	tempFile := path.Join(t.TempDir(), "file.ext")
	assert.NoError(t, os.WriteFile(tempFile, []byte(`{"foo": {"bar": 1}}`), 0644))
	ts := &WebUIServer{
		Logger: logrus.StandardLogger(),
		uiFS: mockFS{
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
		},
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
		body, _ := io.ReadAll(resp.Body)
		var got map[string]metrics.Metric
		assert.NoError(t, json.Unmarshal(body, &got))
		assert.Contains(t, got, "foo")
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
