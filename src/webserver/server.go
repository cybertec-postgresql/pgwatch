package webserver

import (
	"fmt"
	"html/template"
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
}

type WebUIServer struct {
	l log.LoggerIface
	http.Server
	PgWatchVersion  string
	PostgresVersion string
	GrafanaVersion  string
	uiFS            fs.FS
	api             apiHandler
}

func Init(addr string, webuifs fs.FS, api apiHandler, logger log.LoggerIface) *WebUIServer {
	mux := http.NewServeMux()
	s := &WebUIServer{
		// nil,
		logger,
		http.Server{
			Addr:           addr,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			Handler:        mux,
		},
		"3.0.0", "14.4", "8.7.0",
		webuifs,
		api,
	}

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/db", s.handleDBs)
	mux.HandleFunc("/metric", s.handleMetrics)
	mux.HandleFunc("/preset", s.handlePresets)
	mux.HandleFunc("/", s.handleStatic)

	if 8080 != 0 {
		go func() { panic(s.ListenAndServe()) }()
	}
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
	Server.l.Println("file", path, "copied", n, "bytes")
}

func (Server *WebUIServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	tmpl, err := template.New("versions").Parse(`{{define "title"}}Versions{{end}}
<html>
<body>
<ul>
    <li>pgwatch3 {{ .PgWatchVersion }}</li>
    <li>Grafana {{ .GrafanaVersion }}</li>
    <li>Postgres {{ .PostgresVersion }}</li>
</ul>
</body>
</html>`)

	if err != nil {
		Server.l.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
		return
	}

	err = tmpl.ExecuteTemplate(w, "versions", Server)
	if err != nil {
		Server.l.Print(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}
