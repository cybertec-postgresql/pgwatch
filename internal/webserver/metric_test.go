package webserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/stretchr/testify/assert"
)

type mockMetricsReaderWriter struct {
	GetMetricsFunc   func() (*metrics.Metrics, error)
	UpdateMetricFunc func(name string, m metrics.Metric) error
	DeleteMetricFunc func(name string) error
	DeletePresetFunc func(name string) error
	UpdatePresetFunc func(name string, preset metrics.Preset) error
	WriteMetricsFunc func(metricDefs *metrics.Metrics) error
}

func (m *mockMetricsReaderWriter) GetMetrics() (*metrics.Metrics, error) {
	return m.GetMetricsFunc()
}
func (m *mockMetricsReaderWriter) UpdateMetric(name string, metric metrics.Metric) error {
	return m.UpdateMetricFunc(name, metric)
}
func (m *mockMetricsReaderWriter) DeleteMetric(name string) error {
	return m.DeleteMetricFunc(name)
}
func (m *mockMetricsReaderWriter) DeletePreset(name string) error {
	return m.DeletePresetFunc(name)
}
func (m *mockMetricsReaderWriter) UpdatePreset(name string, preset metrics.Preset) error {
	return m.UpdatePresetFunc(name, preset)
}
func (m *mockMetricsReaderWriter) WriteMetrics(metricDefs *metrics.Metrics) error {
	return m.WriteMetricsFunc(metricDefs)
}

func newTestMetricServer(mrw *mockMetricsReaderWriter) *WebUIServer {
	return &WebUIServer{
		metricsReaderWriter: mrw,
	}
}

func TestHandleMetrics_GET(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return &metrics.Metrics{MetricDefs: map[string]metrics.Metric{"foo": {Description: "foo"}}}, nil
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodGet, "/metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var got map[string]metrics.Metric
	assert.NoError(t, json.Unmarshal(body, &got))
	assert.Contains(t, got, "foo")
}

func TestHandleMetrics_GET_Fail(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodGet, "/metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "fail")
}

func TestHandleMetrics_POST(t *testing.T) {
	var updatedName string
	var updatedMetric metrics.Metric
	mock := &mockMetricsReaderWriter{
		UpdateMetricFunc: func(name string, m metrics.Metric) error {
			updatedName = name
			updatedMetric = m
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	m := metrics.Metric{Description: "bar"}
	b, _ := json.Marshal(m)
	r := httptest.NewRequest(http.MethodPost, "/metric?name=bar", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "bar", updatedName)
	assert.Equal(t, m, updatedMetric)
}

type errorReader struct{}

func (e *errorReader) Read([]byte) (n int, err error) {
	return 0, errors.New("mock read error")
}

func TestHandleMetrics_POST_ReaderFail(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdateMetricFunc: func(_ string, _ metrics.Metric) error {
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodPost, "/metric?name=bar", &errorReader{})
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "mock read error")
}

func TestHandleMetrics_POST_Fail(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdateMetricFunc: func(_ string, _ metrics.Metric) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	m := metrics.Metric{Description: "bar"}
	b, _ := json.Marshal(m)
	r := httptest.NewRequest(http.MethodPost, "/metric?name=bar", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "fail")
}

func TestHandleMetrics_DELETE(t *testing.T) {
	var deletedName string
	mock := &mockMetricsReaderWriter{
		DeleteMetricFunc: func(name string) error {
			deletedName = name
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodDelete, "/metric?name=foo", nil)
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "foo", deletedName)
}

func TestHandleMetrics_Options(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodOptions, "/metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "GET, POST, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestHandleMetrics_MethodNotAllowed(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodPut, "/metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetrics(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, POST, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestGetMetrics_Error(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	_, err := ts.GetMetrics()
	assert.Error(t, err)
}

func TestUpdateMetric_Error(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdateMetricFunc: func(_ string, _ metrics.Metric) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.UpdateMetric("foo", []byte("notjson"))
	assert.Error(t, err)
}

func TestDeleteMetric_Error(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		DeleteMetricFunc: func(_ string) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.DeleteMetric("foo")
	assert.Error(t, err)
}

func TestHandlePreset_GET(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return &metrics.Metrics{PresetDefs: map[string]metrics.Preset{"foo": {Description: "foo"}}}, nil
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodGet, "/preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var got map[string]metrics.Preset
	assert.NoError(t, json.Unmarshal(body, &got))
	assert.Contains(t, got, "foo")
}

func TestHandlePreset_GET_Fail(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodGet, "/preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "fail")
}

func TestHandlePreset_POST(t *testing.T) {
	var updatedName string
	var updatedPreset metrics.Preset
	mock := &mockMetricsReaderWriter{
		UpdatePresetFunc: func(name string, p metrics.Preset) error {
			updatedName = name
			updatedPreset = p
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	p := metrics.Preset{Description: "bar"}
	b, _ := json.Marshal(p)
	r := httptest.NewRequest(http.MethodPost, "/preset?name=bar", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "bar", updatedName)
	assert.Equal(t, p, updatedPreset)
}

func TestHandlePreset_POST_ReaderFail(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdatePresetFunc: func(string, metrics.Preset) error {
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodPost, "/preset?name=bar", &errorReader{})
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "mock read error")
}

func TestHandlePreset_POST_Fail(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdatePresetFunc: func(string, metrics.Preset) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	p := metrics.Preset{Description: "bar"}
	b, _ := json.Marshal(p)
	r := httptest.NewRequest(http.MethodPost, "/preset?name=bar", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "fail")
}

func TestHandlePreset_DELETE(t *testing.T) {
	var deletedName string
	mock := &mockMetricsReaderWriter{
		DeletePresetFunc: func(name string) error {
			deletedName = name
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodDelete, "/preset?name=foo", nil)
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "foo", deletedName)
}

func TestHandlePreset_Options(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodOptions, "/preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "GET, POST, PATCH, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestHandlePreset_MethodNotAllowed(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := httptest.NewRequest(http.MethodPut, "/preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresets(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, POST, PATCH, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestGetPresets_Error(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	_, err := ts.GetPresets()
	assert.Error(t, err)
}

func TestUpdatePreset_Error(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdatePresetFunc: func(string, metrics.Preset) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.UpdatePreset("foo", []byte("notjson"))
	assert.Error(t, err)
}

func TestDeletePreset_Error(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		DeletePresetFunc: func(string) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.DeletePreset("foo")
	assert.Error(t, err)
}
