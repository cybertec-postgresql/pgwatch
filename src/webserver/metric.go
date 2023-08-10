package webserver

import (
	"io"
	"net/http"
	"strconv"
)

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
