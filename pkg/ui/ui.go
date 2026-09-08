// Package ui defines the contract between the pgwatch web server and the web
// user interface it serves.
//
// pgwatch ships its own React UI, but the server itself knows nothing about
// it: it asks a Provider for the assets to serve. An embedder that wants a
// different UI implements Provider and passes it to webserver.Init through
// webserver.WithUI.
package ui

import "io/fs"

// Provider supplies the static assets and the client-side routing of a web UI.
type Provider interface {
	// FS returns the file system holding the UI's static assets. A file named
	// index.html must exist at its root and is parsed as a Go html/template.
	FS() fs.FS

	// SPARoutes returns the client-side routes that must be answered with
	// index.html rather than looked up as files. A single "*" entry means
	// every path without a file extension.
	SPARoutes() []string

	// IndexData returns extra data for the index.html template. It is merged
	// with the data the server supplies; keys returned here never override
	// the server's own, notably BasePath. It may return nil.
	IndexData() map[string]any
}
