// Command keep is a local assertion store with evidence pins: a CLI, an HTTP
// server with an embedded webkit web UI, and an MCP server, all over a single
// JSON store.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/keep/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "keep:", err)
		os.Exit(1)
	}
}
