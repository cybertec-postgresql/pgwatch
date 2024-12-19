package main

import "fmt"

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
