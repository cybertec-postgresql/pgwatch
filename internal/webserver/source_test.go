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
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

type mockSourcesReaderWriter struct {
	GetSourcesFunc   func() (sources.Sources, error)
	UpdateSourceFunc func(md sources.Source) error
	DeleteSourceFunc func(name string) error
	WriteSourcesFunc func(sources.Sources) error
}

func (m *mockSourcesReaderWriter) GetSources() (sources.Sources, error) {
	return m.GetSourcesFunc()
}
func (m *mockSourcesReaderWriter) UpdateSource(md sources.Source) error {
	return m.UpdateSourceFunc(md)
}
func (m *mockSourcesReaderWriter) DeleteSource(name string) error {
	return m.DeleteSourceFunc(name)
}
func (m *mockSourcesReaderWriter) WriteSources(srcs sources.Sources) error {
	return m.WriteSourcesFunc(srcs)
}

func newTestSourceServer(mrw *mockSourcesReaderWriter) *WebUIServer {
	return &WebUIServer{
		sourcesReaderWriter: mrw,
	}
}

func TestHandleSources_GET(t *testing.T) {
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSources_GET_Fail(t *testing.T) {
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSources_POST(t *testing.T) {
	var updatedSource sources.Source
	mock := &mockSourcesReaderWriter{
		UpdateSourceFunc: func(md sources.Source) error {
			updatedSource = md
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
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, src, updatedSource)
}

func TestHandleSources_POST_ReaderFail(t *testing.T) {
	mock := &mockSourcesReaderWriter{
		UpdateSourceFunc: func(sources.Source) error {
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
}

func TestHandleSources_POST_Fail(t *testing.T) {
	mock := &mockSourcesReaderWriter{
		UpdateSourceFunc: func(sources.Source) error {
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
}

func TestHandleSources_DELETE(t *testing.T) {
	var deletedName string
	mock := &mockSourcesReaderWriter{
		DeleteSourceFunc: func(name string) error {
			deletedName = name
			return nil
		},
	}
	ts := newTestSourceServer(mock)
	r := httptest.NewRequest(http.MethodDelete, "/source?name=foo", nil)
	w := httptest.NewRecorder()
	ts.handleSources(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "foo", deletedName)
}

func TestHandleSources_Options(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := httptest.NewRequest(http.MethodOptions, "/source", nil)
	w := httptest.NewRecorder()
	ts.handleSources(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "GET, POST, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestHandleSources_MethodNotAllowed(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := httptest.NewRequest(http.MethodPut, "/source", nil)
	w := httptest.NewRecorder()
	ts.handleSources(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, POST, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestGetSources_Error(t *testing.T) {
	mock := &mockSourcesReaderWriter{
		GetSourcesFunc: func() (sources.Sources, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestSourceServer(mock)
	_, err := ts.GetSources()
	assert.Error(t, err)
}

func TestUpdateSource_Error(t *testing.T) {
	mock := &mockSourcesReaderWriter{
		UpdateSourceFunc: func(sources.Source) error {
			return errors.New("fail")
		},
	}
	ts := newTestSourceServer(mock)
	err := ts.UpdateSource([]byte("notjson"))
	assert.Error(t, err)
}

func TestDeleteSource_Error(t *testing.T) {
	mock := &mockSourcesReaderWriter{
		DeleteSourceFunc: func(string) error {
			return errors.New("fail")
		},
	}
	ts := newTestSourceServer(mock)
	err := ts.DeleteSource("foo")
	assert.Error(t, err)
}

func TestHandleSources_ReadAllError(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := httptest.NewRequest(http.MethodPost, "/source", &errorReader{})
	w := httptest.NewRecorder()
	ts.handleSources(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// Helper function to create HTTP requests with path values for testing individual source endpoints
func newSourceItemRequest(method, name string, body io.Reader) *http.Request {
	url := "/source/" + name
	r := httptest.NewRequest(method, url, body)
	r.SetPathValue("name", name)
	return r
}

// Tests for new REST-compliant source endpoints

func TestHandleSourceItem_GET_Success(t *testing.T) {
	source := sources.Source{Name: "test-source", ConnStr: "postgresql://test"}
	mock := &mockSourcesReaderWriter{
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
	jsoniter.ConfigFastest.Unmarshal(body, &returnedSource)
	assert.Equal(t, source.Name, returnedSource.Name)
}

func TestHandleSourceItem_GET_NotFound(t *testing.T) {
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSourceItem_PUT_Success(t *testing.T) {
	existingSource := sources.Source{Name: "test-source", ConnStr: "postgresql://old"}
	var updatedSource sources.Source
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSourceItem_PUT_CreateNew(t *testing.T) {
	var updatedSource sources.Source
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSourceItem_PUT_NameMismatch(t *testing.T) {
	existingSource := sources.Source{Name: "test-source"}
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSourceItem_DELETE_Success(t *testing.T) {
	existingSource := sources.Source{Name: "test-source"}
	var deletedName string
	mock := &mockSourcesReaderWriter{
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
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "test-source", deletedName)
}

func TestHandleSourceItem_DELETE_Idempotent(t *testing.T) {
	var deletedName string
	mock := &mockSourcesReaderWriter{
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
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "nonexistent", deletedName)
}

func TestHandleSourceItem_EmptyName(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := newSourceItemRequest(http.MethodGet, "", nil)
	w := httptest.NewRecorder()
	ts.handleSourceItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "source name is required")
}

func TestHandleSourceItem_MethodNotAllowed(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := newSourceItemRequest(http.MethodPost, "test", nil)
	w := httptest.NewRecorder()
	ts.handleSourceItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestHandleSourceItem_Options(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := newSourceItemRequest(http.MethodOptions, "test", nil)
	w := httptest.NewRecorder()
	ts.handleSourceItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

// Error flow tests for xxSourceByName methods

func TestHandleSourceItem_GET_GetSourcesError(t *testing.T) {
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSourceItem_PUT_InvalidRequestBody(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := newSourceItemRequest(http.MethodPut, "test-source", &errorReader{})
	w := httptest.NewRecorder()
	ts.handleSourceItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid request body")
}

func TestHandleSourceItem_PUT_InvalidJSON(t *testing.T) {
	mock := &mockSourcesReaderWriter{}
	ts := newTestSourceServer(mock)
	r := newSourceItemRequest(http.MethodPut, "test-source", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()
	ts.handleSourceItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid JSON format")
}

func TestHandleSourceItem_PUT_UpdateSourceError(t *testing.T) {
	mock := &mockSourcesReaderWriter{
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
}

func TestHandleSourceItem_DELETE_DeleteSourceError(t *testing.T) {
	mock := &mockSourcesReaderWriter{
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
}
