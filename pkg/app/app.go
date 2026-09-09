package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"

	"github.com/cybertec-postgresql/pgwatch/v6/pkg/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/log"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/reaper"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/ui"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/webserver"
)

// App holds the wiring of a pgwatch instance. Create it with New and start it
// with Run.
//
// Experimental: the bootstrap is new API and may still grow methods.
type App struct {
	opts       *cmdopts.Options
	logger     log.LoggerHooker
	reaper     reaper.ReadierReaper
	web        *webserver.WebUIServer
	uiProvider ui.Provider
	routes     func(mux *http.ServeMux, basePath string, auth func(http.HandlerFunc) http.Handler)
	// needsUpgrade defaults to opts.NeedsSchemaUpgrade; it is a seam for tests.
	needsUpgrade func() (bool, error)
	exitCode     atomic.Int32
}

// New creates the application from already parsed options: it initialises the
// logger from opts.Logging and constructs the reaper, but touches neither the
// configuration nor the sinks — that happens in Run. ctx is used for the
// reaper's logging context only; the context Run is given governs the run
// itself. It fails only when opts is nil.
func New(ctx context.Context, opts *cmdopts.Options, options ...Option) (*App, error) {
	if opts == nil {
		return nil, errors.New("no command-line options provided")
	}
	a := &App{opts: opts}
	for _, o := range options {
		o(a)
	}
	a.logger = log.Init(opts.Logging)
	a.needsUpgrade = opts.NeedsSchemaUpgrade
	a.reaper = reaper.NewReaper(log.WithLogger(ctx, a.logger), opts)
	return a, nil
}

// Options returns the options the application was created with. They are
// enriched by Run with the configuration readers and the sink writer.
func (a *App) Options() *cmdopts.Options { return a.opts }

// Logger returns the application logger, also carried by the context Run
// passes to every component.
func (a *App) Logger() log.Logger { return a.logger }

// Ready reports whether the collector is up and gathering measurements. It
// backs the /readiness endpoint.
func (a *App) Ready() bool { return a.reaper != nil && a.reaper.Ready() }

// Run executes the startup sequence — configuration readers, sink writer,
// schema check, web server — and then blocks in the measurement gathering loop
// until ctx is cancelled or an interrupt arrives. It returns the process exit
// code: cmdopts.ExitCodeOK on a clean stop, ExitCodeConfigError,
// ExitCodeUpgradeError or ExitCodeWebUIError when a startup step fails,
// ExitCodeUserCancel when the OS interrupted it and ExitCodeFatalError on a
// panic, which Run recovers and logs with its callstack.
func (a *App) Run(ctx context.Context) (code int) {
	a.exitCode.Store(cmdopts.ExitCodeOK)
	ctx, cancel := context.WithCancel(log.WithLogger(ctx, a.logger))
	defer cancel()
	defer func() {
		if err := recover(); err != nil {
			a.exitCode.Store(cmdopts.ExitCodeFatalError)
			a.logger.WithField("callstack", string(debug.Stack())).Error(err)
		}
		code = int(a.exitCode.Load())
	}()
	a.setupCloseHandler(ctx, cancel)

	a.logger.Debugf("opts: %+v", a.opts)

	if err := a.opts.InitConfigReaders(ctx); err != nil {
		a.exitCode.Store(cmdopts.ExitCodeConfigError)
		a.logger.Error(err)
		return
	}

	if err := a.opts.InitSinkWriter(ctx); err != nil {
		a.exitCode.Store(cmdopts.ExitCodeConfigError)
		a.logger.Error(err)
		return
	}

	if upgrade, err := a.needsUpgrade(); upgrade || err != nil {
		if upgrade {
			err = errors.Join(err, errors.New(`configuration needs upgrade, use "config upgrade" command`))
		}
		a.exitCode.Store(cmdopts.ExitCodeUpgradeError)
		a.logger.Error(err)
		return
	}

	if a.opts.Metrics.DirectOSStats {
		a.logger.Warning("--direct-os-stats flag is deprecated, direct OS access is now applied automatically for relevant metrics if on same host.")
	}

	var err error
	if a.web, err = webserver.Init(ctx, a.opts.WebUI, a.opts.MetricsReaderWriter,
		a.opts.SourcesReaderWriter, a.reaper, a.webserverOptions()...); err != nil {
		a.exitCode.Store(cmdopts.ExitCodeWebUIError)
		a.logger.Error("failed to initialize web UI: ", err)
		return
	}

	a.reaper.Reap(ctx)
	return
}

// webserverOptions translates the application options into web server ones.
// Nothing is forwarded unless it was set, so the web server keeps deciding
// what a missing UI provider means.
func (a *App) webserverOptions() (options []webserver.Option) {
	if a.uiProvider != nil {
		options = append(options, webserver.WithUI(a.uiProvider))
	}
	if a.routes != nil {
		options = append(options, webserver.WithRoutes(a.routes))
	}
	return
}

// setupCloseHandler notifies the application when it receives an interrupt
// from the OS: the run is cancelled and the exit code set before cancelling,
// so Run observes it once the reaper returns. The listener stops with ctx, so
// repeated Run calls in one process leave nothing behind.
func (a *App) setupCloseHandler(ctx context.Context, cancel context.CancelFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer signal.Stop(c)
		select {
		case <-c:
			a.logger.Debug("received an interrupt from OS. Closing session...")
			a.exitCode.Store(cmdopts.ExitCodeUserCancel)
			cancel()
		case <-ctx.Done():
		}
	}()
}
