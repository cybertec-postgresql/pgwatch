package main

import "fmt"

// version output variables
var (
	commit       = "unknown"
	version      = "unknown"
	date         = "unknown"
	configSchema = "00824"
	sinkSchema   = "01110"
)

func printVersion() {
	fmt.Printf(`
Version info:
  Version:       %s
  Config Schema: %s
  Sink Schema:   %s
  Git Commit:    %s
  Built:         %s

`, version, configSchema, sinkSchema, commit, date)
}
