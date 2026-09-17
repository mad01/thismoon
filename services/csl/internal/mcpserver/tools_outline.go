package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/outline"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// outlineInput is the typed input for the csl_outline tool.
type outlineInput struct {
	Repo         string   `json:"repo"                    jsonschema:"case-insensitive regex or substring matched against the org/repo name; must resolve to exactly one repo"`
	Path         string   `json:"path,omitempty"          jsonschema:"directory inside the repo whose files contribute definitions (e.g. services/csl/internal); default the whole repo. References are always counted across the whole repo"`
	Kinds        []string `json:"kinds,omitempty"         jsonschema:"keep only these kinds: interface, struct, class, type, typealias, enum, namespace, function, method, methodSpec, const, var, field, enumerator, section; default all but field, enumerator, and section"`
	Limit        int      `json:"limit,omitempty"         jsonschema:"maximum symbols to return (default 100, max 500)"`
	IncludeTests bool     `json:"include_tests,omitempty" jsonschema:"include test files (_test.go, test_*.py, *.test.ts, __tests__/, ...); default false leaves them out of both the definitions and the reference counts"`
	MaxFiles     int      `json:"max_files,omitempty"     jsonschema:"stop the walk after this many files (default 20000) and mark the result truncated with files_capped"`
	formatParam
}

func registerOutlineTools(s *mcp.Server) {
	addFormattedTool(s, &mcp.Tool{
		Name: "csl_outline",
		Description: "Outline a repo or a directory in it: every definition (types, functions, methods, fields, headings) " +
			"ranked by how many other files in the repo mention its name as a whole identifier, so the important types and functions surface first. " +
			"Use to answer 'how is this service structured' or 'what are the main types in internal/search' in one call instead of several searches and reads. " +
			"path narrows which files contribute definitions; references are always counted across the whole repo, so a package's public API ranks by repo-wide use. " +
			"refs counts files holding the name as a whole identifier; a lowercase Go name counts only inside its directory, and a name defined in several files shares its mentions among them, so common names don't crowd the top. " +
			"Fields, enumerators, and markdown headings are out unless kinds names them (symbols_skipped counts them); test files are out unless include_tests is true. " +
			"Reads the working tree directly (no index needed, always current). " +
			"Returns files_scanned, symbols_total, truncated, and symbols: [{name, kind, parent, file, line, refs}] ranked by refs desc, then kind (types before functions before constants), then name; " +
			"the text format groups them by file in rank order like csl_search output. Default limit 100, max 500; narrow with path or kinds when truncated.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, handleOutline, outline.Render)
}

func handleOutline(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in outlineInput,
) (*mcp.CallToolResult, outline.Result, error) {
	if strings.TrimSpace(in.Repo) == "" {
		return nil, outline.Result{}, fmt.Errorf("repo is required")
	}
	if in.Limit < 0 || in.MaxFiles < 0 {
		return nil, outline.Result{}, fmt.Errorf("limit and max_files must be >= 0")
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, outline.Result{}, fmt.Errorf("load csl config: %w", err)
	}
	matched, err := resolveRepoIn(cfg, in.Repo)
	if err != nil {
		return nil, outline.Result{}, err
	}
	res, err := outline.Build(matched, outline.Options{
		Path:         in.Path,
		Kinds:        in.Kinds,
		Limit:        in.Limit,
		IncludeTests: in.IncludeTests,
		MaxFiles:     in.MaxFiles,
		HiddenDirs:   cfg.AllowedHiddenDirs(),
	})
	if err != nil {
		return nil, outline.Result{}, err
	}
	return nil, res, nil
}
