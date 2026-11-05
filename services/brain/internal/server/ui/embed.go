// Package ui holds the brain.this web UI: the shell page, its scripts and
// the graph bundle, embedded into the brain binary. docs/serve.md is the
// contract the server and these files share.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed shell.html app.js graph.js assets
var files embed.FS

// Files returns the UI files: shell.html, app.js, graph.js and assets/.
func Files() fs.FS { return files }
