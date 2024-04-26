package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/reaper"
	"github.com/cybertec-postgresql/pgwatch3/sources"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
)

// version output variables
var (
	commit  = "unknown"
	version = "unknown"
	date    = "unknown"
	dbapi   = "00534"
)

func printVersion() {
	fmt.Printf(`pgwatch3:
  Version:      %s
  DB Schema:    %s
  Git Commit:   %s
  Built:        %s
`, version, dbapi, commit, date)
}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func SetupCloseHandler(cancel context.CancelFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Debug("SetupCloseHandler received an interrupt from OS. Closing session...")
		cancel()
		exitCode.Store(ExitCodeUserCancel)
	}()
}

const (
	ExitCodeOK int32 = iota
	ExitCodeConfigError
	ExitCodeInitError
	ExitCodeWebUIError
	ExitCodeUpgradeError
	ExitCodeUserCancel
	ExitCodeShutdownCommand
	ExitCodeFatalError
)

var (
	exitCode    atomic.Int32          // Exit code to be returned to the OS
	mainContext context.Context       // Main context for the application
	logger      log.LoggerHookerIface // Logger for the application
	opts        *config.Options       // Command line options for the application
)

// NewConfigurationReaders creates the configuration readers based on the configuration kind from the options.
func NewConfigurationReaders(opts *config.Options) (sourcesReader sources.ReaderWriter, metricsReader metrics.ReaderWriter, err error) {
	var configKind config.Kind
	configKind, err = opts.GetConfigKind()
	switch {
	case err != nil:
		return
	case configKind != config.ConfigPgURL:
		ctx := log.WithLogger(mainContext, logger.WithField("config", "files"))
		if sourcesReader, err = sources.NewYAMLSourcesReaderWriter(ctx, opts.Sources.Config); err != nil {
			return
		}
		metricsReader, err = metrics.NewYAMLMetricReaderWriter(ctx, opts.Metrics.Metrics)
	default:
		var configDb db.PgxPoolIface
		ctx := log.WithLogger(mainContext, logger.WithField("config", "postgres"))
		if configDb, err = db.New(ctx, opts.Sources.Config); err != nil {
			return
		}
		if metricsReader, err = metrics.NewPostgresMetricReaderWriter(ctx, configDb); err != nil {
			return
		}
		sourcesReader, err = sources.NewPostgresSourcesReaderWriter(ctx, configDb)
	}
	return
}

func main() {
	var (
		err    error
		cancel context.CancelFunc
	)
	exitCode.Store(ExitCodeOK)
	defer func() {
		if err := recover(); err != nil {
			exitCode.Store(ExitCodeFatalError)
			log.GetLogger(mainContext).WithField("callstack", string(debug.Stack())).Error(err)
		}
		os.Exit(int(exitCode.Load()))
	}()

	mainContext, cancel = context.WithCancel(context.Background())
	SetupCloseHandler(cancel)
	defer cancel()

	if opts, err = config.New(os.Stdout); err != nil {
		printVersion()
		fmt.Println(err)
		if !opts.Help {
			exitCode.Store(ExitCodeConfigError)
		}
		return
	}
	logger = log.Init(opts.Logging)
	mainContext = log.WithLogger(mainContext, logger)

	logger.Debugf("opts: %+v", opts)

	// sourcesReaderWriter reads/writes the monitored sources (databases, patroni clusters, pgpools, etc.) information
	// metricsReaderWriter reads/writes the metric and preset definitions
	sourcesReaderWriter, metricsReaderWriter, err := NewConfigurationReaders(opts)
	if err != nil {
		exitCode.Store(ExitCodeInitError)
		logger.Error(err)
		return
	}
	if opts.Sources.Init {
		// At this point we have initialised the sources, metrics and presets configurations.
		// Any fatal errors are handled by the configuration readers. So we may exit gracefully.
		return
	}

	if !opts.Ping {
		uifs, _ := fs.Sub(webuifs, "webui/build")
		ui := webserver.Init(opts.WebUI, uifs, metricsReaderWriter, sourcesReaderWriter, logger)
		if ui == nil {
			os.Exit(int(ExitCodeWebUIError))
		}
	}

	reaper := reaper.NewReaper(opts, sourcesReaderWriter, metricsReaderWriter)
	if err = reaper.Reap(mainContext); err != nil {
		logger.Error(err)
		exitCode.Store(ExitCodeFatalError)
	}
}
