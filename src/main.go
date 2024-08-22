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

	"github.com/cybertec-postgresql/pgwatch/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/log"
	"github.com/cybertec-postgresql/pgwatch/reaper"
)

// version output variables
var (
	commit  = "unknown"
	version = "unknown"
	date    = "unknown"
	dbapi   = "00179"
)

func printVersion() {
	fmt.Printf(`
Version info:
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
		log.GetLogger(mainCtx).Debug("SetupCloseHandler received an interrupt from OS. Closing session...")
		cancel()
		exitCode.Store(cmdopts.ExitCodeUserCancel)
	}()
}

var (
	exitCode atomic.Int32          // Exit code to be returned to the OS
	mainCtx  context.Context       // Main context for the application
	logger   log.LoggerHookerIface // Logger for the application
	opts     *cmdopts.Options      // Command line options for the application
)

func main() {
	var (
		err    error
		cancel context.CancelFunc
	)
	exitCode.Store(cmdopts.ExitCodeOK)
	defer func() {
		if err := recover(); err != nil {
			exitCode.Store(cmdopts.ExitCodeFatalError)
			log.GetLogger(mainCtx).WithField("callstack", string(debug.Stack())).Error(err)
		}
		os.Exit(int(exitCode.Load()))
	}()

	mainCtx, cancel = context.WithCancel(context.Background())
	SetupCloseHandler(cancel)
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
		exitCode.Store(cmdopts.ExitCodeCmdError)
		logger.Error(err)
		return
	}

	if upgrade, err := opts.NeedsSchemaUpgrade(); upgrade || err != nil {
		if upgrade {
			err = errors.Join(err, errors.New(`configuration needs upgrade, use "init --upgrade" command`))
		}
		exitCode.Store(cmdopts.ExitCodeUpgradeError)
		logger.Error(err)
		return
	}

	if err = opts.InitWebUI(webuifs, logger); err != nil {
		exitCode.Store(cmdopts.ExitCodeWebUIError)
		logger.Error(err)
		return
	}

	reaper := reaper.NewReaper(opts, opts.SourcesReaderWriter, opts.MetricsReaderWriter)
	if err = reaper.Reap(mainCtx); err != nil {
		logger.Error(err)
		exitCode.Store(cmdopts.ExitCodeFatalError)
	}
}
