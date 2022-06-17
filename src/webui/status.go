package webui

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"time"
)

type WebUIServer struct {
	// l        log.Logger
	http.Server
	PgWatchVersion  string
	PostgresVersion string
	GrafanaVersion  string
}

//go:embed static/* templates/*
var assetsFS embed.FS

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
		"3.0.0", "14.4", "8.7.0",
	}
	fs := http.FileServer(http.FS(assetsFS))
	http.Handle("/static/", fs)
	http.HandleFunc("/versions", s.versionsHandler)
	http.HandleFunc("/dbs", s.rootHandler)
	http.HandleFunc("/", s.rootHandler)
	if 8080 != 0 {
		// logger.WithField("port", opts.Port).Info("Starting REST API server...")
		go func() { panic(s.ListenAndServe()) }()
	}
	return s
}

func (Server *WebUIServer) versionsHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(assetsFS, "templates/layout.html", "templates/versions.html")
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
		return
	}

	err = tmpl.ExecuteTemplate(w, "layout", Server)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}

func (Server *WebUIServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	// Server.l.Debug("Received / web request")

	tmpl, err := template.ParseFS(assetsFS, "templates/layout.html", "templates/dbs.html")
	if err != nil {
		// Log the detailed error
		log.Print(err.Error())
		// Return a generic "Internal Server Error" message
		http.Error(w, http.StatusText(500), 500)
		return
	}

	err = tmpl.ExecuteTemplate(w, "layout", nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}
