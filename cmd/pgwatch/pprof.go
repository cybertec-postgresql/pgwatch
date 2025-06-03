//go:build pprof

package main

import (
	"net/http"

	_ "net/http/pprof" // Import for pprof HTTP server
)

func init() {
	// Start pprof HTTP server for debugging when build tag 'pprof' is enabled
	go func() {
		// Using panic here will cause the application to exit if pprof server fails to start
		// This is intentional behavior for debugging builds
		panic(http.ListenAndServe(":6060", nil))
	}()
}
