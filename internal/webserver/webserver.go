package webserver

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sources"
)

type ReadyChecker interface {
	Ready() bool
}

type WebUIServer struct {
	CmdOpts
	http.Server
	log.Logger
	ctx                 context.Context
	uiFS                fs.FS // webui files
	metricsReaderWriter metrics.ReaderWriter
	sourcesReaderWriter sources.ReaderWriter
	readyChecker        ReadyChecker
}

func Init(ctx context.Context, opts CmdOpts, webuifs fs.FS, mrw metrics.ReaderWriter, srw sources.ReaderWriter, rc ReadyChecker) (*WebUIServer, error) {
	if opts.WebDisable == WebDisableAll {
		return nil, nil
	}
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
		Logger:              log.GetLogger(ctx),
		CmdOpts:             opts,
		uiFS:                webuifs,
		metricsReaderWriter: mrw,
		sourcesReaderWriter: srw,
		readyChecker:        rc,
	}

	mux.Handle("/source", NewEnsureAuth(s.handleSources))
	mux.Handle("/source/{name}", NewEnsureAuth(s.handleSourceItem))
	mux.Handle("/test-connect", NewEnsureAuth(s.handleTestConnect))
	mux.Handle("/metric", NewEnsureAuth(s.handleMetrics))
	mux.Handle("/preset", NewEnsureAuth(s.handlePresets))
	mux.Handle("/log", NewEnsureAuth(s.serveWsLog))
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/liveness", s.handleLiveness)
	mux.HandleFunc("/readiness", s.handleReadiness)
	if opts.WebDisable != WebDisableUI {
		mux.HandleFunc("/", s.handleStatic)
	}

	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return nil, err
	}

	go func() { panic(s.Serve(ln)) }()

	return s, nil
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
			Server.Println("file", path, "not found:", err)
			http.NotFound(w, r)
			return
		}
		Server.Println("file", path, "cannot be read:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer file.Close()

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
	Server.Debug("file", path, "copied", n, "bytes")
}

func (Server *WebUIServer) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	if Server.ctx.Err() != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status": "unavailable"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "ok"}`))
}

func (Server *WebUIServer) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	if Server.readyChecker.Ready() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"status": "busy"}`))
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
		if err := db.Ping(context.TODO(), string(p)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	default:
		w.Header().Set("Allow", "POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:4000") //check internal/webui/.env
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
