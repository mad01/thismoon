// Command reminder is a local reminder service: a CLI, an HTTP server with an
// embedded webkit web UI, and an MCP server, all over a single JSON store.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/reminder/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "reminder:", err)
		os.Exit(1)
	}
}
