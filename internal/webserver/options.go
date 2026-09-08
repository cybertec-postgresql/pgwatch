package webserver

import "github.com/cybertec-postgresql/pgwatch/v6/pkg/ui"

// Option configures the web server at initialisation time.
type Option func(*WebUIServer)

// WithUI sets the web user interface the server serves. It is required unless
// the UI is disabled with --web-disable=ui or --web-disable=all.
func WithUI(p ui.Provider) Option {
	return func(s *WebUIServer) {
		s.uiProvider = p
	}
}
