// Package main is the nostos CLI entrypoint.
//
// Invocation contract: always `go run ./cmd/nostos <subcommand>`.
package main

import (
	"github.com/yurifrl/nostos/internal/cli"
)

// Version is the tool version. Hardcoded in v0.1.
const Version = "0.3.0"

func main() {
	root := cli.NewRoot(Version)
	cli.HandleExit(root.Execute())
}
