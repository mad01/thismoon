package mcpserver

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// showFileInput is the typed input for the csl_show_file tool.
type showFileInput struct {
	Repo      string `json:"repo"                 jsonschema:"case-insensitive regex or substring matched against the org/repo name; must resolve to exactly one repo"`
	File      string `json:"file"                 jsonschema:"file path relative to the repo root"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line of the section to highlight, 1-based; omit to show the whole file"`
	EndLine   int    `json:"end_line,omitempty"   jsonschema:"last line of the section to highlight, 1-based inclusive; defaults to start_line"`
	NoOpen    bool   `json:"no_open,omitempty"    jsonschema:"return the URL without opening the browser"`
}

// showFileOutput is the structured output of the csl_show_file tool.
type showFileOutput struct {
	URL       string `json:"url"               jsonschema:"the csl web file-view URL for the section"`
	Repo      string `json:"repo"              jsonschema:"the resolved repo name (canonical org/repo form)"`
	File      string `json:"file"              jsonschema:"file path relative to the repo root"`
	LocalPath string `json:"local_path"        jsonschema:"absolute on-disk path to the file"`
	Opened    bool   `json:"opened"            jsonschema:"true when the browser was opened"`
	Warning   string `json:"warning,omitempty" jsonschema:"non-fatal problem, e.g. the browser could not be opened"`
}

// openURL launches the default browser. A package var so tests can stub it.
var openURL = func(u string) error {
	return exec.Command("/usr/bin/open", u).Start()
}

func registerShowTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_show_file",
		Description: "Show a file section to the USER in the csl web UI (opens their browser). " +
			"Use when the user should look at a piece of code you are referencing — instead of pasting it or having them hunt for the file in an editor. " +
			"The page renders the section like a search match, with controls to widen the context up to the full file and a copy-local-path button. " +
			"The view reads the file live from disk via the running `csl web` server, so it must be up (it is a t-man service on this machine). " +
			"Set no_open=true to just get the URL. This shows content to the human; to read file content yourself, use csl_read.",
	}, handleShowFile)
}

func handleShowFile(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in showFileInput,
) (*mcp.CallToolResult, showFileOutput, error) {
	if strings.TrimSpace(in.Repo) == "" {
		return nil, showFileOutput{}, fmt.Errorf("repo is required")
	}
	if strings.TrimSpace(in.File) == "" {
		return nil, showFileOutput{}, fmt.Errorf("file is required")
	}
	if in.StartLine < 0 || in.EndLine < 0 {
		return nil, showFileOutput{}, fmt.Errorf("start_line and end_line must be >= 0")
	}
	if in.StartLine > 0 && in.EndLine > 0 && in.StartLine > in.EndLine {
		return nil, showFileOutput{}, fmt.Errorf(
			"start_line (%d) must be <= end_line (%d)", in.StartLine, in.EndLine)
	}

	matched, err := resolveRepo(in.Repo)
	if err != nil {
		return nil, showFileOutput{}, err
	}

	// Confine the path to the repo root and check the file exists, so the tool
	// fails here with a clear error instead of opening a browser tab on a 404.
	clean := filepath.Clean("/" + in.File)
	absPath := filepath.Join(matched.Path, clean)
	if !strings.HasPrefix(absPath, matched.Path+string(os.PathSeparator)) {
		return nil, showFileOutput{}, fmt.Errorf("invalid path %q", in.File)
	}
	if fi, err := os.Stat(absPath); err != nil {
		return nil, showFileOutput{}, fmt.Errorf("stat %s: %w", absPath, err)
	} else if fi.IsDir() {
		return nil, showFileOutput{}, fmt.Errorf("%s is a directory", absPath)
	}
	relPath := strings.TrimPrefix(clean, "/")

	cfg, err := config.Load()
	if err != nil {
		return nil, showFileOutput{}, fmt.Errorf("load csl config: %w", err)
	}

	params := url.Values{"repo": {matched.Name}, "file": {relPath}}
	if in.StartLine > 0 {
		params.Set("start", strconv.Itoa(in.StartLine))
		end := in.EndLine
		if end == 0 {
			end = in.StartLine
		}
		params.Set("end", strconv.Itoa(end))
	}

	out := showFileOutput{
		URL:       cfg.EffectiveWebBaseURL() + "/file?" + params.Encode(),
		Repo:      matched.Name,
		File:      relPath,
		LocalPath: absPath,
	}

	if !in.NoOpen {
		if err := openURL(out.URL); err != nil {
			out.Warning = fmt.Sprintf("could not open browser: %v — share the URL instead", err)
		} else {
			out.Opened = true
		}
	}

	return nil, out, nil
}
