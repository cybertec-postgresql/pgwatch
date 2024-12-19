package webserver

import (
	"io"
	"net/http"
)

func (Server *WebUIServer) handlePresets(w http.ResponseWriter, r *http.Request) {
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
		if res, err = Server.GetPresets(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new stored Preset
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = Server.UpdatePreset(r.URL.Query().Get("name"), params)

	case http.MethodDelete:
		// delete stored Preset
		err = Server.DeletePreset(r.URL.Query().Get("name"))

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
