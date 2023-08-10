package webserver

import (
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
)

type apiHandler interface {
	GetDatabases() (string, error)
	AddDatabase(params []byte) error
	DeleteDatabase(id string) error
	UpdateDatabase(id string, params []byte) error
	GetMetrics() (res string, err error)
	AddMetric(params []byte) error
	DeleteMetric(id int) error
	UpdateMetric(id int, params []byte) error
	GetPresets() (res string, err error)
	AddPreset(params []byte) error
	DeletePreset(name string) error
	UpdatePreset(id string, params []byte) error
	GetStats() string
	TryConnectToDB(params []byte) error
}

type WebUIServer struct {
	l log.LoggerIface
	http.Server
	config.WebUIOpts
	uiFS fs.FS
	api  apiHandler
}

func Init(opts config.WebUIOpts, webuifs fs.FS, api apiHandler, logger log.LoggerIface) *WebUIServer {
	mux := http.NewServeMux()
	s := &WebUIServer{
		logger,
		http.Server{
			Addr:           opts.WebAddr,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			Handler:        mux,
		},
		opts,
		webuifs,
		api,
	}

	mux.Handle("/db", NewEnsureAuth(s.handleDBs))
	mux.Handle("/test-connect", NewEnsureAuth(s.handleTestConnect))
	mux.Handle("/metric", NewEnsureAuth(s.handleMetrics))
	mux.Handle("/preset", NewEnsureAuth(s.handlePresets))
	mux.Handle("/stats", NewEnsureAuth(s.handleStats))
	mux.Handle("/log", NewEnsureAuth(s.serveWsLog))
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/", s.handleStatic)

	go func() { panic(s.ListenAndServe()) }()

	return s
}

func (Server *WebUIServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	}
	path = strings.TrimPrefix(path, "/")

	file, err := Server.uiFS.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			Server.l.Println("file", path, "not found:", err)
			http.NotFound(w, r)
			return
		}
		Server.l.Println("file", path, "cannot be read:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	w.Header().Set("Content-Type", contentType)
	if strings.HasPrefix(path, "static/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000")
	}
	stat, err := file.Stat()
	if err == nil && stat.Size() > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}

	n, _ := io.Copy(w, file)
	Server.l.Debug("file", path, "copied", n, "bytes")
}

func (Server *WebUIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_, _ = w.Write([]byte(Server.api.GetStats()))
	default:
		w.Header().Set("Allow", "GET, POST, PATCH, DELETE, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

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
