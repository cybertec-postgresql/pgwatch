package webserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	jsoniter "github.com/json-iterator/go"
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
	assert.NoError(t, jsoniter.ConfigFastest.Unmarshal(body, &got))
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
	b, _ := jsoniter.ConfigFastest.Marshal(m)
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
	b, _ := jsoniter.ConfigFastest.Marshal(m)
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
	assert.Equal(t, http.StatusOK, resp.StatusCode)
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
	assert.NoError(t, jsoniter.ConfigFastest.Unmarshal(body, &got))
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
	b, _ := jsoniter.ConfigFastest.Marshal(p)
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
	b, _ := jsoniter.ConfigFastest.Marshal(p)
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
	assert.Equal(t, http.StatusOK, resp.StatusCode)
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

// Helper function to create HTTP requests with path values for testing individual metric endpoints
func newMetricItemRequest(method, name string, body io.Reader) *http.Request {
	url := "/metric/" + name
	r := httptest.NewRequest(method, url, body)
	r.SetPathValue("name", name)
	return r
}

// Tests for new REST-compliant metric endpoints

func TestHandleMetricItem_GET_Success(t *testing.T) {
	metric := metrics.Metric{Description: "test metric", SQLs: map[int]string{130000: "SELECT 1"}}
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return &metrics.Metrics{
				MetricDefs: map[string]metrics.Metric{"test-metric": metric},
			}, nil
		},
	}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodGet, "test-metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var returnedMetric metrics.Metric
	body, _ := io.ReadAll(resp.Body)
	assert.NoError(t, jsoniter.ConfigFastest.Unmarshal(body, &returnedMetric))
	assert.Equal(t, metric.Description, returnedMetric.Description)
}

func TestHandleMetricItem_GET_NotFound(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return &metrics.Metrics{MetricDefs: map[string]metrics.Metric{}}, nil
		},
	}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodGet, "nonexistent", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "metric not found")
}

func TestHandleMetricItem_PUT_Success(t *testing.T) {
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

	metric := metrics.Metric{Description: "updated metric", SQLs: map[int]string{130000: "SELECT 2"}}
	b, _ := jsoniter.ConfigFastest.Marshal(metric)
	r := newMetricItemRequest(http.MethodPut, "test-metric", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-metric", updatedName)
	assert.Equal(t, metric.Description, updatedMetric.Description)
}

func TestHandleMetricItem_DELETE_Success(t *testing.T) {
	var deletedName string
	mock := &mockMetricsReaderWriter{
		DeleteMetricFunc: func(name string) error {
			deletedName = name
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodDelete, "test-metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-metric", deletedName)
}

func TestHandleMetricItem_EmptyName(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodGet, "", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "metric name is required")
}

func TestHandleMetricItem_MethodNotAllowed(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodPost, "test", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestHandleMetricItem_Options(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodOptions, "test", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

// Error flow tests for metric endpoints

func TestHandleMetricItem_GET_GetMetricsError(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("database connection failed")
		},
	}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodGet, "test-metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "database connection failed")
}

func TestHandleMetricItem_PUT_InvalidRequestBody(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodPut, "test-metric", &errorReader{})
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "mock read error")
}

func TestHandleMetricItem_PUT_InvalidJSON(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodPut, "test-metric", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid json")
}

func TestHandleMetricItem_PUT_UpdateMetricError(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdateMetricFunc: func(string, metrics.Metric) error {
			return errors.New("update operation failed")
		},
	}
	ts := newTestMetricServer(mock)

	metric := metrics.Metric{Description: "test metric"}
	b, _ := jsoniter.ConfigFastest.Marshal(metric)
	r := newMetricItemRequest(http.MethodPut, "test-metric", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "update operation failed")
}

func TestHandleMetricItem_DELETE_DeleteMetricError(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		DeleteMetricFunc: func(string) error {
			return errors.New("delete operation failed")
		},
	}
	ts := newTestMetricServer(mock)
	r := newMetricItemRequest(http.MethodDelete, "test-metric", nil)
	w := httptest.NewRecorder()
	ts.handleMetricItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "delete operation failed")
}

// Helper function to create HTTP requests with path values for testing individual preset endpoints
func newPresetItemRequest(method, name string, body io.Reader) *http.Request {
	url := "/preset/" + name
	r := httptest.NewRequest(method, url, body)
	r.SetPathValue("name", name)
	return r
}

// Tests for new REST-compliant preset endpoints

func TestHandlePresetItem_GET_Success(t *testing.T) {
	preset := metrics.Preset{Description: "test preset", Metrics: map[string]float64{"cpu": 1.0}}
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return &metrics.Metrics{
				PresetDefs: map[string]metrics.Preset{"test-preset": preset},
			}, nil
		},
	}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodGet, "test-preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var returnedPreset metrics.Preset
	body, _ := io.ReadAll(resp.Body)
	assert.NoError(t, jsoniter.ConfigFastest.Unmarshal(body, &returnedPreset))
	assert.Equal(t, preset.Description, returnedPreset.Description)
}

func TestHandlePresetItem_GET_NotFound(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return &metrics.Metrics{PresetDefs: map[string]metrics.Preset{}}, nil
		},
	}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodGet, "nonexistent", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "preset not found")
}

func TestHandlePresetItem_PUT_Success(t *testing.T) {
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

	preset := metrics.Preset{Description: "updated preset", Metrics: map[string]float64{"memory": 2.0}}
	b, _ := jsoniter.ConfigFastest.Marshal(preset)
	r := newPresetItemRequest(http.MethodPut, "test-preset", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-preset", updatedName)
	assert.Equal(t, preset.Description, updatedPreset.Description)
}

func TestHandlePresetItem_DELETE_Success(t *testing.T) {
	var deletedName string
	mock := &mockMetricsReaderWriter{
		DeletePresetFunc: func(name string) error {
			deletedName = name
			return nil
		},
	}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodDelete, "test-preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-preset", deletedName)
}

func TestHandlePresetItem_EmptyName(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodGet, "", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "preset name is required")
}

func TestHandlePresetItem_MethodNotAllowed(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodPost, "test", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

func TestHandlePresetItem_Options(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodOptions, "test", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
}

// Error flow tests for preset endpoints

func TestHandlePresetItem_GET_GetMetricsError(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("database connection failed")
		},
	}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodGet, "test-preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "database connection failed")
}

func TestHandlePresetItem_PUT_InvalidRequestBody(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodPut, "test-preset", &errorReader{})
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid request body")
}

func TestHandlePresetItem_PUT_InvalidJSON(t *testing.T) {
	mock := &mockMetricsReaderWriter{}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodPut, "test-preset", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid JSON format")
}

func TestHandlePresetItem_PUT_UpdatePresetError(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		UpdatePresetFunc: func(string, metrics.Preset) error {
			return errors.New("update operation failed")
		},
	}
	ts := newTestMetricServer(mock)

	preset := metrics.Preset{Description: "test preset"}
	b, _ := jsoniter.ConfigFastest.Marshal(preset)
	r := newPresetItemRequest(http.MethodPut, "test-preset", bytes.NewReader(b))
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "update operation failed")
}

func TestHandlePresetItem_DELETE_DeletePresetError(t *testing.T) {
	mock := &mockMetricsReaderWriter{
		DeletePresetFunc: func(string) error {
			return errors.New("delete operation failed")
		},
	}
	ts := newTestMetricServer(mock)
	r := newPresetItemRequest(http.MethodDelete, "test-preset", nil)
	w := httptest.NewRecorder()
	ts.handlePresetItem(w, r)
	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "delete operation failed")
}
