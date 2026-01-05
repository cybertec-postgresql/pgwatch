package webserver

import (
	"errors"
	"io"
	"net/http"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/sources"
)

func (s *WebUIServer) handleSources(w http.ResponseWriter, r *http.Request) {
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
		// return monitored databases
		if res, err = s.GetSources(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new monitored database (REST-compliant: POST for creation only)
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = s.CreateSource(params)
		if err != nil {
			if errors.Is(err, sources.ErrSourceExists) {
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

// GetSources returns the list of sources fo find databases for monitoring
func (s *WebUIServer) GetSources() (res string, err error) {
	var dbs sources.Sources
	if dbs, err = s.sourcesReaderWriter.GetSources(); err != nil {
		return
	}
	b, _ := jsoniter.ConfigFastest.Marshal(dbs)
	res = string(b)
	return
}

// DeleteSource removes the source from the list of configured sources
func (s *WebUIServer) DeleteSource(database string) error {
	return s.sourcesReaderWriter.DeleteSource(database)
}

// UpdateSource updates the configured source information
func (s *WebUIServer) UpdateSource(params []byte) error {
	var md sources.Source
	err := jsoniter.ConfigFastest.Unmarshal(params, &md)
	if err != nil {
		return err
	}
	return s.sourcesReaderWriter.UpdateSource(md)
}

// CreateSource creates a new source (for REST collection endpoint)
func (s *WebUIServer) CreateSource(params []byte) error {
	var md sources.Source
	err := jsoniter.ConfigFastest.Unmarshal(params, &md)
	if err != nil {
		return err
	}
	return s.sourcesReaderWriter.CreateSource(md)
}

// handleSourceItem handles individual source operations using REST-compliant HTTP methods
// and path parameters like /source/{name}
func (s *WebUIServer) handleSourceItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "source name is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getSourceByName(w, name)
	case http.MethodPut:
		s.updateSourceByName(w, r, name)
	case http.MethodDelete:
		s.deleteSourceByName(w, name)
	case http.MethodOptions:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getSourceByName returns a specific source by name
func (s *WebUIServer) getSourceByName(w http.ResponseWriter, name string) {
	var (
		err    error
		status = http.StatusInternalServerError
		srcs   sources.Sources
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), status)
		}
	}()

	if srcs, err = s.sourcesReaderWriter.GetSources(); err != nil {
		return
	}

	for _, src := range srcs {
		if src.Name == name {
			b, _ := jsoniter.ConfigFastest.Marshal(src)
			w.Header().Set("Content-Type", "application/json")
			_, err = w.Write(b)
			return
		}
	}

	err = sources.ErrSourceNotFound
	status = http.StatusNotFound
}

// updateSourceByName updates an existing source using PUT semantics
func (s *WebUIServer) updateSourceByName(w http.ResponseWriter, r *http.Request, name string) {
	params, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var md sources.Source
	if err := jsoniter.ConfigFastest.Unmarshal(params, &md); err != nil {
		http.Error(w, "invalid JSON format", http.StatusBadRequest)
		return
	}

	// Name in body must match the URL parameter
	if md.Name != name {
		http.Error(w, "name in URL and body must match", http.StatusBadRequest)
		return
	}

	if err := s.sourcesReaderWriter.UpdateSource(md); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// deleteSourceByName deletes a source by name
func (s *WebUIServer) deleteSourceByName(w http.ResponseWriter, name string) {
	if err := s.sourcesReaderWriter.DeleteSource(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
