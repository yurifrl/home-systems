// Package main is the nostos CLI entrypoint.
//
// Invocation contract: always `go run ./cmd/nostos <subcommand>`.
// The repository does not ship compiled binaries in v0.1.
package main

import (
	"fmt"
	"os"

	"github.com/yurifrl/nostos/internal/cli"
)

// Version is the tool version. Hardcoded in v0.1; `go generate`d from CHANGELOG in a later iteration.
const Version = "0.1.0"

func main() {
	root := cli.NewRoot(Version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
