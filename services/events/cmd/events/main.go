// Command events is a local event/audit log service: a CLI, an HTTP server with
// an embedded webkit web UI, and an MCP server, all over a single-writer JSON
// log store. It is archive-only — events are recorded, never fired.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/events/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "events:", err)
		os.Exit(1)
	}
}
