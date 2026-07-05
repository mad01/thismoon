// Command deps is a local supply-chain dependency scanner: it discovers every
// external package across the mad01 tool repos (Go, npm, …), checks each
// version against the OSV.dev advisory database, surfaces findings over an HTTP
// web page + MCP, and fires a macOS notification when a package is flagged.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/deps/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "deps:", err)
		os.Exit(1)
	}
}
