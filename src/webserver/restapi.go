package webserver

import (
	"io"
	"net/http"
	"strconv"
)

func (Server *WebUIServer) handleTestConnect(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// test database connection
		p, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := Server.api.TryConnectToDB(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	default:
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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
		if res, err = Server.api.GetPresets(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new stored Preset
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = Server.api.AddPreset(params)

	case http.MethodPatch:
		// update stored preset
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = Server.api.UpdatePreset(r.URL.Query().Get("id"), params)

	case http.MethodDelete:
		// delete stored Preset
		err = Server.api.DeletePreset(r.URL.Query().Get("id"))

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (Server *WebUIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		params []byte
		res    string
		id     int
	)

	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	switch r.Method {
	case http.MethodGet:
		// return stored metrics
		if res, err = Server.api.GetMetrics(); err != nil {
			return
		}
		_, err = w.Write([]byte(res))

	case http.MethodPost:
		// add new stored metric
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		err = Server.api.AddMetric(params)

	case http.MethodPatch:
		// update monitored database
		if params, err = io.ReadAll(r.Body); err != nil {
			return
		}
		if id, err = strconv.Atoi(r.URL.Query().Get("id")); err != nil {
			return
		}
		err = Server.api.UpdateMetric(id, params)

	case http.MethodDelete:
		// delete stored metric
		if id, err = strconv.Atoi(r.URL.Query().Get("id")); err != nil {
			return
		}
		err = Server.api.DeleteMetric(id)

	case http.MethodOptions:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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
			return
		}
		if err := Server.api.AddDatabase(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	case http.MethodPatch:
		// update monitored database
		p, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
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
