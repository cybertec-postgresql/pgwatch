package webserver

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

func (server *WebUIServer) handleSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// return monitored databases
		dbs, err := server.GetSources()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(dbs))

	case http.MethodPost:
		// add new monitored database
		p, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := server.UpdateSource(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case http.MethodDelete:
		// delete monitored database
		if err := server.DeleteSource(r.URL.Query().Get("name")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

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
	b, _ := json.Marshal(dbs)
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
	err := json.Unmarshal(params, &md)
	if err != nil {
		return err
	}
	return server.sourcesReaderWriter.UpdateSource(md)
}
