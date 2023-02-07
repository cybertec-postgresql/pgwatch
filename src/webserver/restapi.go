package webserver

import (
	"io"
	"net/http"
)

func (Server *WebUIServer) handleDBs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// return monitored databases
		dbs, err := Server.api.GetDatabases()
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
		}
		if err := Server.api.AddDatabase(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	case http.MethodPatch:
		// update monitored database
		p, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		if err := Server.api.UpdateDatabase(r.URL.Query().Get("id"), p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case http.MethodDelete:
		// delete monitored database
		if err := Server.api.DeleteDatabase(r.URL.Query().Get("id")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
