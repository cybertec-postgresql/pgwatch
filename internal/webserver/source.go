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
