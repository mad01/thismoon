// Command moondb records the file edits agent sessions make between commits
// and links each one to the turn that made it: a capture hook, an HTTP
// server with an embedded webkit web UI, a CLI and an MCP server, all over
// one local store.
package main

import (
	"fmt"
	"os"

	"github.com/mad01/thismoon/services/moondb/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "moondb:", err)
		os.Exit(1)
	}
}
