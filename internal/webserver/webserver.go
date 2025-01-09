package webserver

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

type ReadyChecker interface {
	Ready() bool
}

type WebUIServer struct {
	ctx context.Context
	l   log.LoggerIface
	http.Server
	CmdOpts
	uiFS                fs.FS
	metricsReaderWriter metrics.ReaderWriter
	sourcesReaderWriter sources.ReaderWriter
	readyChecker        ReadyChecker
}

func Init(ctx context.Context, opts CmdOpts, webuifs fs.FS, mrw metrics.ReaderWriter, srw sources.ReaderWriter, rc ReadyChecker) *WebUIServer {
	mux := http.NewServeMux()
	s := &WebUIServer{
		Server: http.Server{
			Addr:           opts.WebAddr,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			Handler:        corsMiddleware(mux),
		},
		ctx:                 ctx,
		l:                   log.GetLogger(ctx),
		CmdOpts:             opts,
		uiFS:                webuifs,
		metricsReaderWriter: mrw,
		sourcesReaderWriter: srw,
		readyChecker:        rc,
	}

	mux.Handle("/source", NewEnsureAuth(s.handleSources))
	mux.Handle("/test-connect", NewEnsureAuth(s.handleTestConnect))
	mux.Handle("/metric", NewEnsureAuth(s.handleMetrics))
	mux.Handle("/preset", NewEnsureAuth(s.handlePresets))
	mux.Handle("/log", NewEnsureAuth(s.serveWsLog))
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/liveness", s.handleLiveness)
	mux.HandleFunc("/readiness", s.handleReadiness)
	if !opts.WebDisable {
		mux.HandleFunc("/", s.handleStatic)
	}

	go func() { panic(s.ListenAndServe()) }()

	return s
}

func (Server *WebUIServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	routes := []string{"/", "/sources", "/metrics", "/presets", "/logs"}
	path := r.URL.Path
	if slices.Contains(routes, path) {
		path = "index.html"
	} else {
		path = strings.TrimPrefix(path, "/")
	}

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

func (Server *WebUIServer) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	if Server.ctx.Err() != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status": "unavailable"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}

func (Server *WebUIServer) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	if Server.readyChecker.Ready() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`{"status": "busy"}`))
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
		if err := Server.TryConnectToDB(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	default:
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
