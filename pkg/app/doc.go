// Package app bootstraps pgwatch: it wires the logger, the configuration
// readers, the sink writer, the schema check, the reaper and the web server
// into the same startup sequence the pgwatch binary runs, and owns signal
// handling, panic recovery and exit-code mapping.
//
// An embedder builds cmdopts.Options, hands them to New together with its own
// ui.Provider and route hook, and lets Run drive the collector:
//
//	opts, err := cmdopts.New(os.Stdout)
//	// ... error and subcommand handling ...
//	a, err := app.New(ctx, opts, app.WithUI(myProvider))
//	// ... error handling ...
//	os.Exit(a.Run(ctx))
package app
