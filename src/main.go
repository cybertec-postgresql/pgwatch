package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/cybertec-postgresql/pgwatch3/log"
	"github.com/cybertec-postgresql/pgwatch3/metrics"
	"github.com/cybertec-postgresql/pgwatch3/reaper"
	"github.com/cybertec-postgresql/pgwatch3/webserver"
)

// version output variables
var (
	commit  = "unknown"
	version = "unknown"
	date    = "unknown"
	dbapi   = "00179"
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
		log.GetLogger(mainContext).Debug("SetupCloseHandler received an interrupt from OS. Closing session...")
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

// UpgradeConfiguration upgrades the configuration if needed.
func UpgradeConfiguration(m metrics.Migrator) (err error) {
	if opts.Upgrade {
		err = m.Migrate()
	} else {
		var upgrade bool
		upgrade, err = m.NeedsMigration()
		if upgrade {
			err = errors.Join(err, errors.New("configuration needs upgrade, use --upgrade option"))
		}
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

	if err := opts.InitConfigReaders(mainContext); err != nil {
		exitCode.Store(ExitCodeInitError)
		logger.Error(err)
		return
	}

	//check if we want to upgrade the configuration, which can be one of the following:
	// - PostgreSQL database schema
	// - YAML file schema
	if m, ok := opts.MetricsReaderWriter.(metrics.Migrator); ok {
		if err = UpgradeConfiguration(m); err != nil {
			exitCode.Store(ExitCodeUpgradeError)
			logger.Error(err)
			return
		}
	}

	if opts.Init {
		// At this point we have initialised the sources, metrics and presets configurations.
		// Any fatal errors are handled by the configuration readers. So we may exit gracefully.
		return
	}

	if !opts.Ping {
		uifs, _ := fs.Sub(webuifs, "webui/build")
		ui := webserver.Init(opts.WebUI, uifs, opts.MetricsReaderWriter, opts.SourcesReaderWriter, logger)
		if ui == nil {
			os.Exit(int(ExitCodeWebUIError))
		}
	}

	reaper := reaper.NewReaper(opts, opts.SourcesReaderWriter, opts.MetricsReaderWriter)
	if err = reaper.Reap(mainContext); err != nil {
		logger.Error(err)
		exitCode.Store(ExitCodeFatalError)
	}
}
