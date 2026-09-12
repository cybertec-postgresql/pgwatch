package webserver

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"maps"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/db"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/log"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/metrics"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/sources"
	"github.com/cybertec-postgresql/pgwatch/v7/pkg/ui"
)

type Readier interface {
	Ready() bool
}

type WebUIServer struct {
	CmdOpts
	http.Server
	log.Logger
	ctx                 context.Context
	basePath            string // computed base path with slashes
	indexHTML           []byte // pre-rendered index.html content
	uiProvider          ui.Provider
	corsOrigin          string
	routes              func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)
	metricsReaderWriter metrics.ReaderWriter
	sourcesReaderWriter sources.ReaderWriter
	readyChecker        Readier
}

func Init(ctx context.Context, opts CmdOpts, mrw metrics.ReaderWriter, srw sources.ReaderWriter,
	rc Readier, options ...Option) (_ *WebUIServer, err error) {
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
		},
		ctx:                 ctx,
		Logger:              log.GetLogger(ctx),
		CmdOpts:             opts,
		metricsReaderWriter: mrw,
		sourcesReaderWriter: srw,
		readyChecker:        rc,
	}

	for _, o := range options {
		o(s)
	}

	// WithCORSOrigin wins over the flag, which wins over the built-in default.
	s.corsOrigin = cmp.Or(s.corsOrigin, opts.WebCORSOrigin, DefaultCORSOrigin)

	s.basePath = "/" + opts.WebBasePath
	if opts.WebBasePath != "" {
		s.basePath += "/"
	}

	mux.Handle(s.basePath+"source", NewEnsureAuth(s.handleSources))
	mux.Handle(s.basePath+"source/{name}", NewEnsureAuth(s.handleSourceItem))
	mux.Handle(s.basePath+"test-connect", NewEnsureAuth(s.handleTestConnect))
	mux.Handle(s.basePath+"metric", NewEnsureAuth(s.handleMetrics))
	mux.Handle(s.basePath+"metric/{name}", NewEnsureAuth(s.handleMetricItem))
	mux.Handle(s.basePath+"preset", NewEnsureAuth(s.handlePresets))
	mux.Handle(s.basePath+"preset/{name}", NewEnsureAuth(s.handlePresetItem))
	mux.Handle(s.basePath+"log", NewEnsureAuth(s.serveWsLog))
	mux.HandleFunc(s.basePath+"login", s.handleLogin)
	mux.HandleFunc(s.basePath+"liveness", s.handleLiveness)
	mux.HandleFunc(s.basePath+"readiness", s.handleReadiness)

	// Extension routes are registered on their own mux so that a pattern
	// clashing with a built-in one neither panics nor takes over.
	var extMux *http.ServeMux
	if s.routes != nil {
		extMux = http.NewServeMux()
		s.routes(extMux, s.basePath, func(h http.HandlerFunc) http.Handler { return NewEnsureAuth(h) })
	}

	if opts.WebDisable != WebDisableUI {
		if s.uiProvider == nil {
			return nil, errors.New("no web UI provider configured: pass webserver.WithUI() or disable the UI with --web-disable=ui")
		}
		if err = s.prepareIndexHTML(); err != nil {
			return nil, err
		}
		mux.HandleFunc(s.basePath, s.handleStatic)
	}

	s.Handler = s.corsMiddleware(s.dispatcher(mux, extMux))

	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return nil, err
	}

	go func() { panic(s.Serve(ln)) }()

	return s, nil
}

// prepareIndexHTML renders the index.html template once at startup
func (s *WebUIServer) prepareIndexHTML() error {
	tmpl, err := template.ParseFS(s.uiProvider.FS(), "index.html")
	if err != nil {
		return err
	}

	// The provider's data comes first so that the server's own keys, notably
	// BasePath, always win.
	data := map[string]any{}
	maps.Copy(data, s.uiProvider.IndexData())
	data["BasePath"] = s.WebBasePath

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	s.indexHTML = buf.Bytes()
	return nil
}

// isSPARoute reports whether path is a client-side route of the UI and must
// therefore be answered with index.html. The provider owns the route set; a
// single "*" entry means every path that does not look like a file.
func (s *WebUIServer) isSPARoute(path string) bool {
	routes := s.uiProvider.SPARoutes()
	if len(routes) == 1 && routes[0] == "*" {
		return filepath.Ext(path) == ""
	}
	return slices.Contains(routes, path)
}

func (s *WebUIServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Strip base path if present
	path := strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(s.basePath, "/"))

	if s.isSPARoute(path) { // is index.html
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.indexHTML)))
		_, _ = w.Write(s.indexHTML)
		s.Debug("index.html served")
		return
	}

	path = strings.TrimPrefix(path, "/")
	file, err := s.uiProvider.FS().Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Println("file", path, "not found:", err)
			http.NotFound(w, r)
			return
		}
		s.Println("file", path, "cannot be read:", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Determine content type
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
	s.Debug("file", path, "copied", n, "bytes")
}

func (s *WebUIServer) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	if s.ctx.Err() != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status": "unavailable"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status": "ok"}`))
}

func (s *WebUIServer) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	if s.readyChecker.Ready() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"status": "busy"}`))
}

func (s *WebUIServer) handleTestConnect(w http.ResponseWriter, r *http.Request) {
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

// dispatcher routes a request to the built-in mux, falling back to the
// extension mux registered with WithRoutes. Built-in routes always win; the
// extension mux is only consulted where the built-in mux has nothing to offer
// but the catch-all static handler.
func (s *WebUIServer) dispatcher(mux, extMux *http.ServeMux) http.Handler {
	if extMux == nil {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, pattern := mux.Handler(r); pattern != "" && pattern != s.basePath {
			h.ServeHTTP(w, r)
			return
		}
		if _, pattern := extMux.Handler(r); pattern != "" {
			extMux.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *WebUIServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.corsOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
