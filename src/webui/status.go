package webui

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

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
	fs := http.FileServer(http.Dir("webui/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/versions", s.versionsHandler)
	http.HandleFunc("/dbs", s.rootHandler)
	http.HandleFunc("/", s.rootHandler)
	if 8080 != 0 {
		// logger.WithField("port", opts.Port).Info("Starting REST API server...")
		go func() { panic(s.ListenAndServe()) }()
	}
	return s
}

type Version struct {
	PgWatchVersion  string
	PostgresVersion string
	GrafanaVersion  string
}

func (Server *WebUIServer) versionsHandler(w http.ResponseWriter, r *http.Request) {
	lp := filepath.Join("webui/templates", "layout.html")
	fp := filepath.Join("webui/templates", "versions.html")
	tmpl, err := template.ParseFiles(lp, fp)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
		return
	}

	V := Version{
		PgWatchVersion:  "0.0.1",
		PostgresVersion: "9.6.11",
		GrafanaVersion:  "6.6.2",
	}

	err = tmpl.ExecuteTemplate(w, "layout", V)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}

func (Server *WebUIServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	// Server.l.Debug("Received / web request")

	// The "/" pattern matches everything, so we need to check
	// that we're at the root here.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	lp := filepath.Join("webui/templates", "layout.html")
	fp := filepath.Join("webui/templates", "dbs.html")

	// Return a 404 if the template doesn't exist
	info, err := os.Stat(fp)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
	}

	// Return a 404 if the request is for a directory
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}

	tmpl, err := template.ParseFiles(lp, fp)
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
