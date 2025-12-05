package webserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

func newTestSourceServer(mrw *testutil.MockSourcesReaderWriter) *WebUIServer {
	return &WebUIServer{
		sourcesReaderWriter: mrw,
	}
}

func TestHandleSources(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{{Name: "foo"}}, nil
				},
			}
			ts := newTestSourceServer(mock)
			r := httptest.NewRequest(http.MethodGet, "/source", nil)
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			var got []sources.Source
			assert.NoError(t, jsoniter.ConfigFastest.Unmarshal(body, &got))
			assert.Equal(t, "foo", got[0].Name)
		})

		t.Run("Failure", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return nil, errors.New("fail")
				},
			}
			ts := newTestSourceServer(mock)
			r := httptest.NewRequest(http.MethodGet, "/source", nil)
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "fail")
		})
	})

	t.Run("POST", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var createdSource sources.Source
			mock := &testutil.MockSourcesReaderWriter{
				CreateSourceFunc: func(md sources.Source) error {
					createdSource = md
					return nil
				},
			}
			ts := newTestSourceServer(mock)
			src := sources.Source{Name: "bar"}
			b, _ := jsoniter.ConfigFastest.Marshal(src)
			r := httptest.NewRequest(http.MethodPost, "/source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusCreated, resp.StatusCode)
			assert.Equal(t, src, createdSource)
		})

		t.Run("ReaderFailure", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				CreateSourceFunc: func(sources.Source) error {
					return nil
				},
			}
			ts := newTestSourceServer(mock)
			r := httptest.NewRequest(http.MethodPost, "/Source?name=bar", &errorReader{})
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "mock read error")
		})

		t.Run("Conflict", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				CreateSourceFunc: func(sources.Source) error {
					return sources.ErrSourceExists
				},
			}
			ts := newTestSourceServer(mock)
			src := sources.Source{Name: "bar"}
			b, _ := jsoniter.ConfigFastest.Marshal(src)
			r := httptest.NewRequest(http.MethodPost, "/source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusConflict, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "source already exists")
		})

		t.Run("CreateFailure", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				CreateSourceFunc: func(sources.Source) error {
					return errors.New("fail")
				},
			}
			ts := newTestSourceServer(mock)
			src := sources.Source{Name: "bar"}
			b, _ := jsoniter.ConfigFastest.Marshal(src)
			r := httptest.NewRequest(http.MethodPost, "/source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "fail")
		})

		t.Run("ReadAllError", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{}
			ts := newTestSourceServer(mock)
			r := httptest.NewRequest(http.MethodPost, "/source", &errorReader{})
			w := httptest.NewRecorder()
			ts.handleSources(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		})
	})

	t.Run("OPTIONS", func(t *testing.T) {
		mock := &testutil.MockSourcesReaderWriter{}
		ts := newTestSourceServer(mock)
		r := httptest.NewRequest(http.MethodOptions, "/source", nil)
		w := httptest.NewRecorder()
		ts.handleSources(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "GET, POST, OPTIONS", resp.Header.Get("Allow"))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		mock := &testutil.MockSourcesReaderWriter{}
		ts := newTestSourceServer(mock)
		r := httptest.NewRequest(http.MethodPut, "/source", nil)
		w := httptest.NewRecorder()
		ts.handleSources(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, "GET, POST, OPTIONS", resp.Header.Get("Allow"))
	})
}

func TestGetSources_Error(t *testing.T) {
	mock := &testutil.MockSourcesReaderWriter{
		GetSourcesFunc: func() (sources.Sources, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestSourceServer(mock)
	_, err := ts.GetSources()
	assert.Error(t, err)
}

func TestUpdateSource_Error(t *testing.T) {
	mock := &testutil.MockSourcesReaderWriter{
		UpdateSourceFunc: func(sources.Source) error {
			return errors.New("fail")
		},
	}
	ts := newTestSourceServer(mock)
	err := ts.UpdateSource([]byte("notjson"))
	assert.Error(t, err)
}

func TestDeleteSource_Error(t *testing.T) {
	mock := &testutil.MockSourcesReaderWriter{
		DeleteSourceFunc: func(string) error {
			return errors.New("fail")
		},
	}
	ts := newTestSourceServer(mock)
	err := ts.DeleteSource("foo")
	assert.Error(t, err)
}

// Helper function to create HTTP requests with path values for testing individual source endpoints
func newSourceItemRequest(method, name string, body io.Reader) *http.Request {
	url := "/source/" + name
	r := httptest.NewRequest(method, url, body)
	r.SetPathValue("name", name)
	return r
}

func TestHandleSourceItem(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			source := sources.Source{Name: "test-source", ConnStr: "postgresql://test"}
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{source}, nil
				},
			}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodGet, "test-source", nil)
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			var returnedSource sources.Source
			body, _ := io.ReadAll(resp.Body)
			assert.NoError(t, jsoniter.ConfigFastest.Unmarshal(body, &returnedSource))
			assert.Equal(t, source.Name, returnedSource.Name)
		})

		t.Run("NotFound", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{}, nil
				},
			}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodGet, "nonexistent", nil)
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "source not found")
		})

		t.Run("GetSourcesError", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return nil, errors.New("database connection failed")
				},
			}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodGet, "test-source", nil)
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "database connection failed")
		})
	})

	t.Run("PUT", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			existingSource := sources.Source{Name: "test-source", ConnStr: "postgresql://old"}
			var updatedSource sources.Source
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{existingSource}, nil
				},
				UpdateSourceFunc: func(md sources.Source) error {
					updatedSource = md
					return nil
				},
			}
			ts := newTestSourceServer(mock)

			newSource := sources.Source{Name: "test-source", ConnStr: "postgresql://new"}
			b, _ := jsoniter.ConfigFastest.Marshal(newSource)
			r := newSourceItemRequest(http.MethodPut, "test-source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, newSource.ConnStr, updatedSource.ConnStr)
		})

		t.Run("CreateNew", func(t *testing.T) {
			var updatedSource sources.Source
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{}, nil // No existing sources
				},
				UpdateSourceFunc: func(md sources.Source) error {
					updatedSource = md
					return nil
				},
			}
			ts := newTestSourceServer(mock)

			source := sources.Source{Name: "new-source", ConnStr: "postgresql://new"}
			b, _ := jsoniter.ConfigFastest.Marshal(source)
			r := newSourceItemRequest(http.MethodPut, "new-source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, source.Name, updatedSource.Name)
			assert.Equal(t, source.ConnStr, updatedSource.ConnStr)
		})

		t.Run("NameMismatch", func(t *testing.T) {
			existingSource := sources.Source{Name: "test-source"}
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{existingSource}, nil
				},
			}
			ts := newTestSourceServer(mock)

			// Body has different name than URL path
			source := sources.Source{Name: "different-name"}
			b, _ := jsoniter.ConfigFastest.Marshal(source)
			r := newSourceItemRequest(http.MethodPut, "test-source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "name in URL and body must match")
		})

		t.Run("InvalidRequestBody", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodPut, "test-source", &errorReader{})
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "invalid request body")
		})

		t.Run("InvalidJSON", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodPut, "test-source", strings.NewReader("invalid json"))
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "invalid JSON format")
		})

		t.Run("UpdateError", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				UpdateSourceFunc: func(sources.Source) error {
					return errors.New("update operation failed")
				},
			}
			ts := newTestSourceServer(mock)

			source := sources.Source{Name: "test-source", ConnStr: "postgresql://test"}
			b, _ := jsoniter.ConfigFastest.Marshal(source)
			r := newSourceItemRequest(http.MethodPut, "test-source", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "update operation failed")
		})
	})

	t.Run("DELETE", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			existingSource := sources.Source{Name: "test-source"}
			var deletedName string
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{existingSource}, nil
				},
				DeleteSourceFunc: func(name string) error {
					deletedName = name
					return nil
				},
			}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodDelete, "test-source", nil)
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "test-source", deletedName)
		})

		t.Run("Idempotent", func(t *testing.T) {
			var deletedName string
			mock := &testutil.MockSourcesReaderWriter{
				GetSourcesFunc: func() (sources.Sources, error) {
					return sources.Sources{}, nil // No existing sources
				},
				DeleteSourceFunc: func(name string) error {
					deletedName = name
					return nil // DELETE is idempotent - succeeds even if source doesn't exist
				},
			}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodDelete, "nonexistent", nil)
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "nonexistent", deletedName)
		})

		t.Run("DeleteError", func(t *testing.T) {
			mock := &testutil.MockSourcesReaderWriter{
				DeleteSourceFunc: func(string) error {
					return errors.New("delete operation failed")
				},
			}
			ts := newTestSourceServer(mock)
			r := newSourceItemRequest(http.MethodDelete, "test-source", nil)
			w := httptest.NewRecorder()
			ts.handleSourceItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "delete operation failed")
		})
	})

	t.Run("EmptyName", func(t *testing.T) {
		mock := &testutil.MockSourcesReaderWriter{}
		ts := newTestSourceServer(mock)
		r := newSourceItemRequest(http.MethodGet, "", nil)
		w := httptest.NewRecorder()
		ts.handleSourceItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "source name is required")
	})

	t.Run("OPTIONS", func(t *testing.T) {
		mock := &testutil.MockSourcesReaderWriter{}
		ts := newTestSourceServer(mock)
		r := newSourceItemRequest(http.MethodOptions, "test", nil)
		w := httptest.NewRecorder()
		ts.handleSourceItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		mock := &testutil.MockSourcesReaderWriter{}
		ts := newTestSourceServer(mock)
		r := newSourceItemRequest(http.MethodPost, "test", nil)
		w := httptest.NewRecorder()
		ts.handleSourceItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
	})
}
