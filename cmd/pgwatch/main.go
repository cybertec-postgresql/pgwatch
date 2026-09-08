package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	webui "github.com/cybertec-postgresql/pgwatch/v6/internal/webui/embed"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/app"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/cmdopts"
	"github.com/cybertec-postgresql/pgwatch/v6/pkg/log"
)

var (
	mainCtx context.Context    // Main context for the application
	cancel  context.CancelFunc // Cancel function to stop the main context
)

var Exit = os.Exit

func main() {
	mainCtx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// Panics raised before the application exists are on us: pkg/app recovers
	// the ones raised during the run itself.
	defer func() {
		if p := recover(); p != nil {
			log.GetLogger(mainCtx).WithField("callstack", string(debug.Stack())).Error(p)
			Exit(int(cmdopts.ExitCodeFatalError))
		}
	}()

	opts, err := cmdopts.New(os.Stdout)
	if err != nil {
		printVersion()
		fmt.Println(err)
		if opts.Help {
			Exit(int(cmdopts.ExitCodeOK))
			return
		}
		Exit(int(cmdopts.ExitCodeConfigError))
		return
	}

	// check if some sub-command was executed and exit
	if opts.CommandCompleted {
		Exit(int(opts.ExitCode))
		return
	}

	a, err := app.New(mainCtx, opts, app.WithUI(webui.Provider()))
	if err != nil {
		fmt.Println(err)
		Exit(int(cmdopts.ExitCodeConfigError))
		return
	}

	Exit(a.Run(mainCtx))
}
