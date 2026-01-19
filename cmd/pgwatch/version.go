package main

import "fmt"

// version output variables
var (
	commit       = "unknown"
	version      = "unknown"
	date         = "unknown"
	ConfigSchema = "00824"
	SinkSchema   = "01110"
)

func printVersion() {
	fmt.Printf(`
Version info:
  Version:       %s
  ConfigSchema:  %s
  SinkSchema:    %s
  Git Commit:    %s
  Built:         %s

`, version, ConfigSchema, SinkSchema, commit, date)
}
