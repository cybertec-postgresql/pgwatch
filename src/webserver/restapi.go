package webserver

import (
	"net/http"
)

func (Server *WebUIServer) handleApi(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// return monitored databases

	case http.MethodPost:
		// add new monitored database

	case http.MethodPatch:
		// update monitored database

	case http.MethodDelete:
		// delete monitored database

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
