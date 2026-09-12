package webserver

import (
	"net/http"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/ui"
)

// Option configures the web server at initialisation time.
//
// Experimental: the set of options is still growing.
type Option func(*WebUIServer)

// WithUI sets the web user interface the server serves. It is required unless
// the UI is disabled with --web-disable=ui or --web-disable=all.
func WithUI(p ui.Provider) Option {
	return func(s *WebUIServer) {
		s.uiProvider = p
	}
}

// WithRoutes registers additional HTTP routes. fn is called after the built-in
// routes are in place and before the static UI handler, so embedder routes sit
// under the same base path and can never shadow a built-in one. Wrap a handler
// with auth to put it behind the same JWT check the REST API uses.
//
// Experimental: the hook signature may still change.
func WithRoutes(fn func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)) Option {
	return func(s *WebUIServer) {
		s.routes = fn
	}
}

// WithCORSOrigin sets the origin the CORS middleware allows, overriding
// --web-cors-origin. Defaults to DefaultCORSOrigin.
func WithCORSOrigin(origin string) Option {
	return func(s *WebUIServer) {
		s.corsOrigin = origin
	}
}
