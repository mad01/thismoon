package server

import (
	"bytes"
	"errors"
	"regexp"
)

// Standalone is what the page view needs from this package to run with no
// server behind it: the shell's own stylesheet, the fixation targets its
// header names, viz.js, and app.js. internal/export assembles them into one
// file, so an exported page keeps the look and behaviour of the served one.
type Standalone struct {
	ShellCSS        string
	FixationTargets string
	VizJS           string
	AppJS           string
}

// shellStyle is the shell's one inline stylesheet; reFixation the header's
// fixation-targets attribute.
var (
	shellStyleOpen  = []byte("<style>\n")
	shellStyleClose = []byte("</style>")
	reFixation      = regexp.MustCompile(`<wk-header [^>]*fixation-targets="([^"]+)"`)
)

// errShellMarkup means shell.html lost the markup StandaloneAssets cuts out.
var errShellMarkup = errors.New(
	"present: shell.html has no inline <style> or header fixation targets",
)

// StandaloneAssets cuts the stylesheet and the fixation targets out of the
// embedded shell.html and returns them with viz.js and app.js. An error
// means the shell no longer carries the markup the cut looks for.
func StandaloneAssets() (Standalone, error) {
	start := bytes.Index(shellHTML, shellStyleOpen)
	end := bytes.Index(shellHTML, shellStyleClose)
	targets := reFixation.FindSubmatch(shellHTML)
	if start < 0 || end < start || targets == nil {
		return Standalone{}, errShellMarkup
	}
	return Standalone{
		ShellCSS:        string(shellHTML[start+len(shellStyleOpen) : end]),
		FixationTargets: string(targets[1]),
		VizJS:           string(vizJS),
		AppJS:           string(appJS),
	}, nil
}
