package main

import (
	"fmt"

	"github.com/cybertec-postgresql/pgwatch/v7/pkg/cmdopts"
)

// version output variables
var (
	commit  = "unknown"
	version = "unknown"
	date    = "unknown"
)

func printVersion() {
	fmt.Printf(`
Version info:
  Version:       %s
  Config Schema: %s
  Sink Schema:   %s
  Git Commit:    %s
  Built:         %s

`, version, cmdopts.ConfigSchema, cmdopts.SinkSchema, commit, date)
}
