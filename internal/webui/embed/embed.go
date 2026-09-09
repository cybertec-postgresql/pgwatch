// Package embed carries the React web UI built from internal/webui and exposes
// it as the default ui.Provider.
//
// It is the only package that embeds the React build output, and it is
// deliberately internal: nothing but cmd/pgwatch imports it, so an embedder
// never has to compile — or even possess — the React assets.
package embed

import (
	"embed"
	"io/fs"

	"github.com/cybertec-postgresql/pgwatch/v6/pkg/ui"
)

// build is produced by `yarn build` in internal/webui (see vite.config.ts) and
// is not checked into git.
//
//go:embed build
var buildFS embed.FS

// spaRoutes are the client-side routes of the React application.
var spaRoutes = []string{"/", "/sources", "/metrics", "/presets", "/logs"}

type provider struct {
	fsys fs.FS
}

func (p provider) FS() fs.FS { return p.fsys }

func (p provider) SPARoutes() []string { return spaRoutes }

func (p provider) IndexData() map[string]any { return nil }

// Provider returns the default pgwatch web UI.
func Provider() ui.Provider {
	fsys, err := fs.Sub(buildFS, "build")
	if err != nil {
		// Unreachable: "build" is embedded above, so the subtree always exists.
		panic(err)
	}
	return provider{fsys: fsys}
}
