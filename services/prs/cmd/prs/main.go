// Command prs is a local open-PR dashboard: a poller over the GitHub hosts of
// every locally checked-out repo, a CLI, an HTTP server with an embedded
// webkit web UI, and an MCP server, all over a single JSON cache.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/prs/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "prs:", err)
		os.Exit(1)
	}
}
