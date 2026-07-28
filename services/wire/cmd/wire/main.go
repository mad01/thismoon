// Command wire is a message bus between agent sessions: a CLI, an HTTP server
// with an embedded webkit web UI, and an MCP server, all over one append-only
// JSONL store.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/wire/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "wire:", err)
		os.Exit(1)
	}
}
