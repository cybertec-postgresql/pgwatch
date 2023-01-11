package webserver

import (
	"encoding/json"
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
		json.NewEncoder(w).Encode(dbs)

	case http.MethodPost:
		// add new monitored database

	case http.MethodPatch:
		// update monitored database

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
