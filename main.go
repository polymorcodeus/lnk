// Package main is the entry point for the lnk CLI.
package main

import "github.com/polymorcodeus/lnk/cmd"

// version and buildTime are set by GoReleaser via ldflags at build time.
var (
	version   = "v1.0.0"
	buildTime = "local"
)

func main() {
	cmd.SetVersion(version, buildTime)
	cmd.Execute()
}
