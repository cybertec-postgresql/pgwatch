package webserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
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
	assert.NoError(t, json.Unmarshal(body, &got))
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
	b, _ := json.Marshal(src)
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
	b, _ := json.Marshal(src)
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
