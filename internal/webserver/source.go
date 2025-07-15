package webserver

import (
	"io"
	"net/http"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

func (server *WebUIServer) handleSources(w http.ResponseWriter, r *http.Request) {
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
		// return monitored databases
		if res, err = server.GetSources(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new monitored database
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = server.UpdateSource(params)

	case http.MethodDelete:
		// delete monitored database
		err = server.DeleteSource(r.URL.Query().Get("name"))

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GetSources returns the list of sources fo find databases for monitoring
func (server *WebUIServer) GetSources() (res string, err error) {
	var dbs sources.Sources
	if dbs, err = server.sourcesReaderWriter.GetSources(); err != nil {
		return
	}
	b, _ := jsoniter.ConfigFastest.Marshal(dbs)
	res = string(b)
	return
}

// DeleteSource removes the source from the list of configured sources
func (server *WebUIServer) DeleteSource(database string) error {
	return server.sourcesReaderWriter.DeleteSource(database)
}

// UpdateSource updates the configured source information
func (server *WebUIServer) UpdateSource(params []byte) error {
	var md sources.Source
	err := jsoniter.ConfigFastest.Unmarshal(params, &md)
	if err != nil {
		return err
	}
	return server.sourcesReaderWriter.UpdateSource(md)
}

// handleSourceItem handles individual source operations using REST-compliant HTTP methods
// and path parameters like /source/{name}
func (server *WebUIServer) handleSourceItem(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "source name is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		server.getSourceByName(w, name)
	case http.MethodPut:
		server.updateSourceByName(w, r, name)
	case http.MethodDelete:
		server.deleteSourceByName(w, name)
	case http.MethodOptions:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getSourceByName returns a specific source by name
func (server *WebUIServer) getSourceByName(w http.ResponseWriter, name string) {
	sources, err := server.sourcesReaderWriter.GetSources()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, source := range sources {
		if source.Name == name {
			b, _ := jsoniter.ConfigFastest.Marshal(source)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
	}

	http.Error(w, "source not found", http.StatusNotFound)
}

// updateSourceByName updates an existing source using PUT semantics
func (server *WebUIServer) updateSourceByName(w http.ResponseWriter, r *http.Request, name string) {
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

	if err := server.sourcesReaderWriter.UpdateSource(md); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// deleteSourceByName deletes a source by name
func (server *WebUIServer) deleteSourceByName(w http.ResponseWriter, name string) {
	if err := server.sourcesReaderWriter.DeleteSource(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
