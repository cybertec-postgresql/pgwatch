package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/reaper"
	"github.com/cybertec-postgresql/pgwatch/v5/internal/webserver"
)

// setupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func setupCloseHandler(cancel context.CancelFunc) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.GetLogger(mainCtx).Debug("SetupCloseHandler received an interrupt from OS. Closing session...")
		cancel()
		exitCode.Store(cmdopts.ExitCodeUserCancel)
	}()
}

var (
	exitCode atomic.Int32       // Exit code to be returned to the OS
	mainCtx  context.Context    // Main context for the application
	cancel   context.CancelFunc // Cancel function to stop the main context
	logger   log.LoggerHooker   // Logger for the application
	opts     *cmdopts.Options   // Command line options for the application
	err      error
)

var Exit = os.Exit

func main() {

	exitCode.Store(cmdopts.ExitCodeOK)
	defer func() {
		if err := recover(); err != nil {
			exitCode.Store(cmdopts.ExitCodeFatalError)
			log.GetLogger(mainCtx).WithField("callstack", string(debug.Stack())).Error(err)
		}
		Exit(int(exitCode.Load()))
	}()

	mainCtx, cancel = context.WithCancel(context.Background())
	setupCloseHandler(cancel)
	defer cancel()

	if opts, err = cmdopts.New(os.Stdout); err != nil {
		printVersion()
		fmt.Println(err)
		if !opts.Help {
			exitCode.Store(cmdopts.ExitCodeConfigError)
		}
		return
	}

	// check if some sub-command was executed and exit
	if opts.CommandCompleted {
		exitCode.Store(opts.ExitCode)
		return
	}

	logger = log.Init(opts.Logging)
	mainCtx = log.WithLogger(mainCtx, logger)

	logger.Debugf("opts: %+v", opts)

	if err := opts.InitConfigReaders(mainCtx); err != nil {
		exitCode.Store(cmdopts.ExitCodeConfigError)
		logger.Error(err)
		return
	}

	if err := opts.InitSinkWriter(mainCtx); err != nil {
		exitCode.Store(cmdopts.ExitCodeConfigError)
		logger.Error(err)
		return
	}

	if upgrade, err := opts.NeedsSchemaUpgrade(); upgrade || err != nil {
		if upgrade {
			err = errors.Join(err, errors.New(`configuration needs upgrade, use "config upgrade" command`))
		}
		exitCode.Store(cmdopts.ExitCodeUpgradeError)
		logger.Error(err)
		return
	}

	if opts.Metrics.DirectOSStats {
		logger.Warning("--direct-os-stats flag is deprecated, direct OS access is now applied automatically for relevant metrics if on same host.")
	}

	reaper := reaper.NewReaper(mainCtx, opts)

	if _, err = webserver.Init(mainCtx, opts.WebUI, opts.MetricsReaderWriter,
		opts.SourcesReaderWriter, reaper); err != nil {
		exitCode.Store(cmdopts.ExitCodeWebUIError)
		logger.Error("failed to initialize web UI: ", err)
		return
	}

	reaper.Reap(mainCtx)
}
