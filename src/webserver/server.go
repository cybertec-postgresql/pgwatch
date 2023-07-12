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

	"github.com/cybertec-postgresql/pgwatch3/log"
)

type WebUIServer struct {
	l log.LoggerIface
	http.Server
	uiFS fs.FS
	api  apiHandler
}

func Init(addr string, webuifs fs.FS, api apiHandler, logger log.LoggerIface) *WebUIServer {
	mux := http.NewServeMux()
	s := &WebUIServer{
		logger,
		http.Server{
			Addr:           addr,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			Handler:        mux,
		},
		webuifs,
		api,
	}

	mux.Handle("/db", NewEnsureAuth(s.handleDBs))
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
