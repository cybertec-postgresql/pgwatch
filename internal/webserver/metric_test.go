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
	"github.com/cybertec-postgresql/pgwatch/v3/internal/testutil"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
)

func newTestMetricServer(mrw *testutil.MockMetricsReaderWriter) *WebUIServer {
	return &WebUIServer{
		metricsReaderWriter: mrw,
	}
}

func TestHandleMetrics(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("Failure", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("POST", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var createdName string
			var createdMetric metrics.Metric
			mock := &testutil.MockMetricsReaderWriter{
				CreateMetricFunc: func(name string, m metrics.Metric) error {
					createdName = name
					createdMetric = m
					return nil
				},
			}
			ts := newTestMetricServer(mock)

			// Test the map-based JSON format expected by collection endpoint
			metricData := map[string]metrics.Metric{
				"bar": {Description: "bar"},
			}
			b, _ := jsoniter.ConfigFastest.Marshal(metricData)
			r := httptest.NewRequest(http.MethodPost, "/metric", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleMetrics(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusCreated, resp.StatusCode)
			assert.Equal(t, "bar", createdName)
			assert.Equal(t, metrics.Metric{Description: "bar"}, createdMetric)
		})

		t.Run("ReaderFailure", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
				CreateMetricFunc: func(_ string, _ metrics.Metric) error {
					return nil
				},
			}
			ts := newTestMetricServer(mock)
			r := httptest.NewRequest(http.MethodPost, "/metric", &errorReader{})
			w := httptest.NewRecorder()
			ts.handleMetrics(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "mock read error")
		})

		t.Run("CreateFailure", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
				CreateMetricFunc: func(_ string, _ metrics.Metric) error {
					return errors.New("fail")
				},
			}
			ts := newTestMetricServer(mock)
			metricData := map[string]metrics.Metric{
				"bar": {Description: "bar"},
			}
			b, _ := jsoniter.ConfigFastest.Marshal(metricData)
			r := httptest.NewRequest(http.MethodPost, "/metric", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleMetrics(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "fail")
		})

		t.Run("Conflict", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
				CreateMetricFunc: func(_ string, _ metrics.Metric) error {
					return metrics.ErrMetricExists
				},
			}
			ts := newTestMetricServer(mock)
			metricData := map[string]metrics.Metric{
				"bar": {Description: "bar"},
			}
			b, _ := jsoniter.ConfigFastest.Marshal(metricData)
			r := httptest.NewRequest(http.MethodPost, "/metric", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handleMetrics(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusConflict, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "metric already exists")
		})
	})

	t.Run("OPTIONS", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := httptest.NewRequest(http.MethodOptions, "/metric", nil)
		w := httptest.NewRecorder()
		ts.handleMetrics(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "GET, POST, OPTIONS", resp.Header.Get("Allow"))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := httptest.NewRequest(http.MethodPut, "/metric", nil)
		w := httptest.NewRecorder()
		ts.handleMetrics(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, "GET, POST, OPTIONS", resp.Header.Get("Allow"))
	})
}

type errorReader struct{}

func (e *errorReader) Read([]byte) (n int, err error) {
	return 0, errors.New("mock read error")
}

func TestGetMetrics_Error(t *testing.T) {
	mock := &testutil.MockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	_, err := ts.GetMetrics()
	assert.Error(t, err)
}

func TestUpdateMetric_Error(t *testing.T) {
	mock := &testutil.MockMetricsReaderWriter{
		UpdateMetricFunc: func(_ string, _ metrics.Metric) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.UpdateMetric("foo", []byte("notjson"))
	assert.Error(t, err)
}

func TestDeleteMetric_Error(t *testing.T) {
	mock := &testutil.MockMetricsReaderWriter{
		DeleteMetricFunc: func(_ string) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.DeleteMetric("foo")
	assert.Error(t, err)
}

func TestHandlePresets(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("Failure", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("POST", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var createdName string
			var createdPreset metrics.Preset
			mock := &testutil.MockMetricsReaderWriter{
				CreatePresetFunc: func(name string, p metrics.Preset) error {
					createdName = name
					createdPreset = p
					return nil
				},
			}
			ts := newTestMetricServer(mock)
			p := metrics.Preset{Description: "bar"}
			// Use map format for collection endpoint
			presetMap := map[string]metrics.Preset{"bar": p}
			b, _ := jsoniter.ConfigFastest.Marshal(presetMap)
			r := httptest.NewRequest(http.MethodPost, "/preset", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handlePresets(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusCreated, resp.StatusCode)
			assert.Equal(t, "bar", createdName)
			assert.Equal(t, p, createdPreset)
		})

		t.Run("ReaderFailure", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("CreateFailure", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
				CreatePresetFunc: func(string, metrics.Preset) error {
					return errors.New("fail")
				},
			}
			ts := newTestMetricServer(mock)
			p := metrics.Preset{Description: "bar"}
			// Use map format for collection endpoint
			presetMap := map[string]metrics.Preset{"bar": p}
			b, _ := jsoniter.ConfigFastest.Marshal(presetMap)
			r := httptest.NewRequest(http.MethodPost, "/preset", bytes.NewReader(b))
			w := httptest.NewRecorder()
			ts.handlePresets(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "fail")
		})
	})

	t.Run("OPTIONS", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := httptest.NewRequest(http.MethodOptions, "/preset", nil)
		w := httptest.NewRecorder()
		ts.handlePresets(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "GET, POST, OPTIONS", resp.Header.Get("Allow"))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := httptest.NewRequest(http.MethodPut, "/preset", nil)
		w := httptest.NewRecorder()
		ts.handlePresets(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, "GET, POST, OPTIONS", resp.Header.Get("Allow"))
	})
}

func TestGetPresets_Error(t *testing.T) {
	mock := &testutil.MockMetricsReaderWriter{
		GetMetricsFunc: func() (*metrics.Metrics, error) {
			return nil, errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	_, err := ts.GetPresets()
	assert.Error(t, err)
}

func TestUpdatePreset_Error(t *testing.T) {
	mock := &testutil.MockMetricsReaderWriter{
		UpdatePresetFunc: func(string, metrics.Preset) error {
			return errors.New("fail")
		},
	}
	ts := newTestMetricServer(mock)
	err := ts.UpdatePreset("foo", []byte("notjson"))
	assert.Error(t, err)
}

func TestDeletePreset_Error(t *testing.T) {
	mock := &testutil.MockMetricsReaderWriter{
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

func TestHandleMetricItem(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			metric := metrics.Metric{Description: "test metric", SQLs: map[int]string{130000: "SELECT 1"}}
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("NotFound", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("GetMetricsError", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("PUT", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var updatedName string
			var updatedMetric metrics.Metric
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("InvalidRequestBody", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{}
			ts := newTestMetricServer(mock)
			r := newMetricItemRequest(http.MethodPut, "test-metric", &errorReader{})
			w := httptest.NewRecorder()
			ts.handleMetricItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "mock read error")
		})

		t.Run("InvalidJSON", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{}
			ts := newTestMetricServer(mock)
			r := newMetricItemRequest(http.MethodPut, "test-metric", strings.NewReader("invalid json"))
			w := httptest.NewRecorder()
			ts.handleMetricItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "invalid json")
		})

		t.Run("UpdateError", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("DELETE", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var deletedName string
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("DeleteError", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("EmptyName", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := newMetricItemRequest(http.MethodGet, "", nil)
		w := httptest.NewRecorder()
		ts.handleMetricItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "metric name is required")
	})

	t.Run("OPTIONS", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := newMetricItemRequest(http.MethodOptions, "test", nil)
		w := httptest.NewRecorder()
		ts.handleMetricItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := newMetricItemRequest(http.MethodPost, "test", nil)
		w := httptest.NewRecorder()
		ts.handleMetricItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
	})
}

// Helper function to create HTTP requests with path values for testing individual preset endpoints
func newPresetItemRequest(method, name string, body io.Reader) *http.Request {
	url := "/preset/" + name
	r := httptest.NewRequest(method, url, body)
	r.SetPathValue("name", name)
	return r
}

// Tests for new REST-compliant preset endpoints

func TestHandlePresetItem(t *testing.T) {
	t.Run("GET", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			preset := metrics.Preset{Description: "test preset", Metrics: map[string]float64{"cpu": 1.0}}
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("NotFound", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("GetMetricsError", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("PUT", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var updatedName string
			var updatedPreset metrics.Preset
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("InvalidRequestBody", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{}
			ts := newTestMetricServer(mock)
			r := newPresetItemRequest(http.MethodPut, "test-preset", &errorReader{})
			w := httptest.NewRecorder()
			ts.handlePresetItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "invalid request body")
		})

		t.Run("InvalidJSON", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{}
			ts := newTestMetricServer(mock)
			r := newPresetItemRequest(http.MethodPut, "test-preset", strings.NewReader("invalid json"))
			w := httptest.NewRecorder()
			ts.handlePresetItem(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "invalid JSON format")
		})

		t.Run("UpdateError", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("DELETE", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			var deletedName string
			mock := &testutil.MockMetricsReaderWriter{
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
		})

		t.Run("DeleteError", func(t *testing.T) {
			mock := &testutil.MockMetricsReaderWriter{
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
		})
	})

	t.Run("EmptyName", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := newPresetItemRequest(http.MethodGet, "", nil)
		w := httptest.NewRecorder()
		ts.handlePresetItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "preset name is required")
	})

	t.Run("OPTIONS", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := newPresetItemRequest(http.MethodOptions, "test", nil)
		w := httptest.NewRecorder()
		ts.handlePresetItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		mock := &testutil.MockMetricsReaderWriter{}
		ts := newTestMetricServer(mock)
		r := newPresetItemRequest(http.MethodPost, "test", nil)
		w := httptest.NewRecorder()
		ts.handlePresetItem(w, r)
		resp := w.Result()
		defer resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, "GET, PUT, DELETE, OPTIONS", resp.Header.Get("Allow"))
	})
}
