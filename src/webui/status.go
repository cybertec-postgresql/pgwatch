package webui

import (
	"net/http"
	"time"
)

// StatusReporter is a common interface describing the current status of a connection
type StatusReporter interface {
	IsReady() bool
}

type WebUIServer struct {
	// l        log.Logger
	http.Server
}

func Init(addr string) *WebUIServer {
	s := &WebUIServer{
		// nil,
		// logger,
		http.Server{
			Addr:           addr,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
	}
	http.HandleFunc("/", s.rootHandler)
	if 8080 != 0 {
		// logger.WithField("port", opts.Port).Info("Starting REST API server...")
		go func() { panic(s.ListenAndServe()) }()
	}
	return s
}

func (Server *WebUIServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	// Server.l.Debug("Received / web request")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello world!"))
}
