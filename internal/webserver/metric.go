package webserver

import (
	"errors"
	"io"
	"net/http"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
)

func (server *WebUIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		status = http.StatusInternalServerError
		params []byte
		res    string
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), status)
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
		// add new stored metric (REST-compliant: POST for creation only)
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		// For collection endpoint POST, extract name from request body
		// The individual endpoint PUT /metric/{name} should be used for updates
		err = server.CreateMetric(params)
		if err != nil {
			if errors.Is(err, metrics.ErrMetricExists) {
				status = http.StatusConflict
				return
			}
			return
		}
		w.WriteHeader(http.StatusCreated)

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusOK)

	default:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
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

// CreateMetric creates new metrics (for REST collection endpoint)
// Supports both single and bulk creation from a map of metric names to metric definitions
func (server *WebUIServer) CreateMetric(params []byte) error {
	// For collection endpoint, we expect the JSON to be a map with name as key and metric as value
	var namedMetrics map[string]metrics.Metric
	err := jsoniter.ConfigFastest.Unmarshal(params, &namedMetrics)
	if err != nil {
		return err
	}
	if len(namedMetrics) == 0 {
		return metrics.ErrInvalidMetric
	}

	// Create all metrics, returning the first error encountered
	for metricName, metric := range namedMetrics {
		if err := server.metricsReaderWriter.CreateMetric(metricName, metric); err != nil {
			return err
		}
	}
	return nil
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
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getMetricByName returns a specific metric by name
func (server *WebUIServer) getMetricByName(w http.ResponseWriter, name string) {
	var (
		err    error
		status = http.StatusInternalServerError
		mr     *metrics.Metrics
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), status)
		}
	}()

	if mr, err = server.metricsReaderWriter.GetMetrics(); err != nil {
		return
	}

	if metric, exists := mr.MetricDefs[name]; exists {
		b, _ := jsoniter.ConfigFastest.Marshal(metric)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(b)
		return
	}

	err = metrics.ErrMetricNotFound
	status = http.StatusNotFound
}

// updateMetricByName updates an existing metric using PUT semantics
func (server *WebUIServer) updateMetricByName(w http.ResponseWriter, r *http.Request, name string) {
	var (
		err    error
		params []byte
		status = http.StatusInternalServerError
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), status)
		}
	}()

	if params, err = io.ReadAll(r.Body); err != nil {
		status = http.StatusBadRequest
		return
	}

	var m metrics.Metric
	if err = jsoniter.ConfigFastest.Unmarshal(params, &m); err != nil {
		status = http.StatusBadRequest
		return
	}

	if err = server.metricsReaderWriter.UpdateMetric(name, m); err != nil {
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

	w.WriteHeader(http.StatusOK)
}

func (server *WebUIServer) handlePresets(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		status = http.StatusInternalServerError
		params []byte
		res    string
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), status)
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
		// add new stored Preset (REST-compliant: POST for creation only)
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = server.CreatePreset(params)
		if err != nil {
			if errors.Is(err, metrics.ErrPresetExists) {
				status = http.StatusConflict
			}
			return
		}
		w.WriteHeader(http.StatusCreated)

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusOK)

	default:
		w.Header().Set("Allow", "GET, POST, OPTIONS")
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

// CreatePreset creates new presets (for REST collection endpoint)
// Supports both single and bulk creation
func (server *WebUIServer) CreatePreset(params []byte) error {
	// We expect the JSON to be a map with name as key and preset as value
	var namedPresets map[string]metrics.Preset
	err := jsoniter.ConfigFastest.Unmarshal(params, &namedPresets)
	if err != nil {
		return err
	}
	if len(namedPresets) == 0 {
		return metrics.ErrInvalidPreset
	}

	// Create all presets, returning the first error encountered
	for presetName, preset := range namedPresets {
		if err := server.metricsReaderWriter.CreatePreset(presetName, preset); err != nil {
			return err
		}
	}
	return nil
}

// GetPresets returns the list of available presets
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

// handlePresetItem handles individual preset operations using REST-compliant HTTP methods
// and path parameters like /preset/{name}
func (server *WebUIServer) handlePresetItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "preset name is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		server.getPresetByName(w, name)
	case http.MethodPut:
		server.updatePresetByName(w, r, name)
	case http.MethodDelete:
		server.deletePresetByName(w, name)
	case http.MethodOptions:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getPresetByName returns a specific preset by name
func (server *WebUIServer) getPresetByName(w http.ResponseWriter, name string) {

	var (
		err error
		mr  *metrics.Metrics
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if mr, err = server.metricsReaderWriter.GetMetrics(); err != nil {
		return
	}

	if preset, exists := mr.PresetDefs[name]; exists {
		b, _ := jsoniter.ConfigFastest.Marshal(preset)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(b)
		return
	}

	http.Error(w, "preset not found", http.StatusNotFound)
}

// updatePresetByName updates an existing preset using PUT semantics
func (server *WebUIServer) updatePresetByName(w http.ResponseWriter, r *http.Request, name string) {
	params, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var p metrics.Preset
	if err := jsoniter.ConfigFastest.Unmarshal(params, &p); err != nil {
		http.Error(w, "invalid JSON format", http.StatusBadRequest)
		return
	}

	if err := server.metricsReaderWriter.UpdatePreset(name, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// deletePresetByName deletes a preset by name
func (server *WebUIServer) deletePresetByName(w http.ResponseWriter, name string) {
	if err := server.metricsReaderWriter.DeletePreset(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
