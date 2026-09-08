// Package pkg is the root of pgwatch's public API.
//
// It contains no code itself. Every subdirectory of pkg is an importable
// package that external modules may build on:
//
//	pkg/cmdopts    command-line options, config readers, schema constants
//	pkg/db         database helpers shared by the engine
//	pkg/log        logging setup and context plumbing
//	pkg/metrics    metric definitions and their reader/writer
//	pkg/reaper     the measurement gathering loop
//	pkg/sinks      measurement sinks
//	pkg/sources    monitored source definitions and their reader/writer
//	pkg/webserver  the REST API and static UI server
//	pkg/ui         the Provider interface for pluggable web UIs
//	pkg/app        the optional bootstrap wrapping the whole wiring sequence
//
// Anything under internal/ is private to pgwatch and may change without
// notice. See docs/developer/api_stability.md for the stability policy that
// governs the packages listed above.
package pkg
