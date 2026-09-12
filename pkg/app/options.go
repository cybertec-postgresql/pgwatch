package app

import (
	"net/http"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/ui"
)

// Option configures the application before it is run.
//
// Experimental: the set of options is still growing.
type Option func(*App)

// WithUI sets the web user interface the application serves. It is forwarded
// to webserver.WithUI and is required unless the UI is disabled with
// --web-disable=ui or --web-disable=all.
func WithUI(p ui.Provider) Option {
	return func(a *App) {
		a.uiProvider = p
	}
}

// WithRoutes registers additional HTTP routes on the web server. The hook is
// forwarded to webserver.WithRoutes, so the routes sit under the same base
// path as the REST API and auth puts them behind the same JWT check.
//
// Experimental: the hook signature may still change.
func WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option {
	return func(a *App) {
		a.routes = fn
	}
}
