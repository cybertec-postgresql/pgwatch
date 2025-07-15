package webserver

import (
	"io"
	"net/http"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

func (server *WebUIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		params []byte
		res    string
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	switch r.Method {
	case http.MethodGet:
		// return stored metrics
		if res, err = server.GetMetrics(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new stored metric
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = server.UpdateMetric(r.URL.Query().Get("name"), params)

	case http.MethodDelete:
		// delete stored metric
		err = server.DeleteMetric(r.URL.Query().Get("name"))

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GetMetrics returns the list of metrics
func (server *WebUIServer) GetMetrics() (res string, err error) {
	var mr *metrics.Metrics
	if mr, err = server.metricsReaderWriter.GetMetrics(); err != nil {
		return
	}
	b, _ := jsoniter.ConfigFastest.Marshal(mr.MetricDefs)
	res = string(b)
	return
}

// UpdateMetric updates the stored metric information
func (server *WebUIServer) UpdateMetric(name string, params []byte) error {
	var m metrics.Metric
	err := jsoniter.ConfigFastest.Unmarshal(params, &m)
	if err != nil {
		return err
	}
	return server.metricsReaderWriter.UpdateMetric(name, m)
}

// DeleteMetric removes the metric from the configuration
func (server *WebUIServer) DeleteMetric(name string) error {
	return server.metricsReaderWriter.DeleteMetric(name)
}

// handleMetricItem handles individual metric operations using REST-compliant HTTP methods
// and path parameters like /metric/{name}
func (server *WebUIServer) handleMetricItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "metric name is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		server.getMetricByName(w, name)
	case http.MethodPut:
		server.updateMetricByName(w, r, name)
	case http.MethodDelete:
		server.deleteMetricByName(w, name)
	case http.MethodOptions:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getMetricByName returns a specific metric by name
func (server *WebUIServer) getMetricByName(w http.ResponseWriter, name string) {
	mr, err := server.metricsReaderWriter.GetMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if metric, exists := mr.MetricDefs[name]; exists {
		b, _ := jsoniter.ConfigFastest.Marshal(metric)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	http.Error(w, "metric not found", http.StatusNotFound)
}

// updateMetricByName updates an existing metric using PUT semantics
func (server *WebUIServer) updateMetricByName(w http.ResponseWriter, r *http.Request, name string) {
	params, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var m metrics.Metric
	if err := jsoniter.ConfigFastest.Unmarshal(params, &m); err != nil {
		http.Error(w, "invalid JSON format", http.StatusBadRequest)
		return
	}

	if err := server.metricsReaderWriter.UpdateMetric(name, m); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// deleteMetricByName deletes a metric by name
func (server *WebUIServer) deleteMetricByName(w http.ResponseWriter, name string) {
	if err := server.metricsReaderWriter.DeleteMetric(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (server *WebUIServer) handlePresets(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		params []byte
		res    string
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	switch r.Method {
	case http.MethodGet:
		// return stored Presets
		if res, err = server.GetPresets(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new stored Preset
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = server.UpdatePreset(r.URL.Query().Get("name"), params)

	case http.MethodDelete:
		// delete stored Preset
		err = server.DeletePreset(r.URL.Query().Get("name"))

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// UpdatePreset updates the stored preset
func (server *WebUIServer) UpdatePreset(name string, params []byte) error {
	var p metrics.Preset
	err := jsoniter.ConfigFastest.Unmarshal(params, &p)
	if err != nil {
		return err
	}
	return server.metricsReaderWriter.UpdatePreset(name, p)
}

// GetPresets ret	urns the list of available presets
func (server *WebUIServer) GetPresets() (res string, err error) {
	var mr *metrics.Metrics
	if mr, err = server.metricsReaderWriter.GetMetrics(); err != nil {
		return
	}
	b, _ := jsoniter.ConfigFastest.Marshal(mr.PresetDefs)
	res = string(b)
	return
}

// DeletePreset removes the preset from the configuration
func (server *WebUIServer) DeletePreset(name string) error {
	return server.metricsReaderWriter.DeletePreset(name)
}
