package webserver

import (
	"io"
	"net/http"
)

func (Server *WebUIServer) handleSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// return monitored databases
		dbs, err := Server.GetSources()
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
		if err := Server.UpdateSource(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case http.MethodDelete:
		// delete monitored database
		if err := Server.DeleteSource(r.URL.Query().Get("name")); err != nil {
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
