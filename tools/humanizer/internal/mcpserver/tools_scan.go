package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/humanizer/internal/goscan"
	"github.com/mad01/thismoon/tools/humanizer/internal/rules"
)

type scanGoInput struct {
	Path      string `json:"path"                jsonschema:"absolute path to a Go file or directory to scan"`
	Detect    *bool  `json:"detect,omitempty"    jsonschema:"run humanizer detection on the extracted prose (default true)"`
	Recursive *bool  `json:"recursive,omitempty" jsonschema:"walk a directory into subdirectories; ignored for a single file (default true)"`
}

// scanGoBlock mirrors goscan.Block for the wire format: same fields, plain
// string Kind instead of the Kind type.
type scanGoBlock struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Text  string `json:"text"`
}

type scanGoFile struct {
	Path   string        `json:"path"`
	Blocks []scanGoBlock `json:"blocks"`
	// Detection is set when the caller asked for detect and reuses
	// detectOutput, the same shape humanizer_detect and
	// humanizer_detect_file return, so findings from any of the three
	// tools are interchangeable.
	Detection *detectOutput `json:"detection,omitempty"`
}

type scanGoOutput struct {
	Files      []scanGoFile `json:"files"`
	TotalFiles int          `json:"total_files"`
}

func registerScanTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "humanizer_scan_go",
		Description: "Extract human-facing prose from Go source: doc comments, cobra Use/Short/Long/Example fields, MCP tool Description strings, the message arguments of fmt.Errorf/errors.New/http.Error, and jsonschema struct tag text. " +
			"Takes an absolute path to a Go file or a directory. " +
			"detect (default true) runs the vale span rules over each file's extracted prose and reports findings against the Go source line they came from, in the same shape as humanizer_detect. " +
			"recursive (default true) walks a directory into subdirectories; set false to scan only its direct children. " +
			"Directories named vendor, node_modules, and testdata, dot-directories, and generated files are skipped; test files are included. " +
			"Under the MCP sandbox only .go files (plus .md/.markdown/.txt) beneath the sandbox profile's workspace roots, plus /tmp paths, are readable. " +
			"A denied path returns an error pointing at the CLI, which runs unsandboxed.",
	}, handleScanGo)
}

func handleScanGo(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in scanGoInput,
) (*mcp.CallToolResult, scanGoOutput, error) {
	if strings.TrimSpace(in.Path) == "" {
		return nil, scanGoOutput{}, fmt.Errorf("humanizer_scan_go: path is required")
	}
	detect := true
	if in.Detect != nil {
		detect = *in.Detect
	}
	recursive := true
	if in.Recursive != nil {
		recursive = *in.Recursive
	}
	files, err := goscan.Scan(in.Path, goscan.Options{Recursive: &recursive})
	if err != nil {
		return nil, scanGoOutput{}, fmt.Errorf(
			"humanizer_scan_go: %w",
			scanGoPathError(err, in.Path),
		)
	}
	out := scanGoOutput{Files: make([]scanGoFile, 0, len(files))}
	for _, f := range files {
		sf := scanGoFile{Path: f.Path, Blocks: make([]scanGoBlock, 0, len(f.Blocks))}
		for _, b := range f.Blocks {
			sf.Blocks = append(sf.Blocks, scanGoBlock{
				File:  b.File,
				Line:  b.Line,
				Kind:  string(b.Kind),
				Label: b.Label,
				Text:  b.Text,
			})
		}
		if detect {
			findings, err := detectBlocks(ctx, f.Blocks)
			if err != nil {
				return nil, scanGoOutput{}, fmt.Errorf("humanizer_scan_go: %s: %w", f.Path, err)
			}
			detOut := buildDetectOutput(findings)
			sf.Detection = &detOut
		}
		out.Files = append(out.Files, sf)
	}
	out.TotalFiles = len(out.Files)
	return nil, out, nil
}

// detectBlocks runs the vale span rules over one file's blocks, joined into
// a single document so vale runs once per file, then maps each finding's
// line back through the document to the Go source line it came from. Mirrors
// the CLI's detectDoc (internal/cli/scan.go).
func detectBlocks(ctx context.Context, blocks []goscan.Block) ([]rules.Finding, error) {
	doc := goscan.Render(blocks)
	findings, err := rules.Detect(ctx, doc.Text, rules.DetectOptions{})
	if err != nil {
		return nil, err
	}
	for i := range findings {
		findings[i].Line = doc.SourceLine(findings[i].Line)
	}
	return findings, nil
}

// scanGoPathError translates a sandbox permission denial into a structured
// error, mirroring detectFileError's shape. Go extraction needs filesystem
// access to a real path, so unlike humanizer_detect there is no "pass it as
// text" fallback over MCP; the hint points at the CLI instead, which runs
// unsandboxed.
func scanGoPathError(err error, path string) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	redirect, _ := json.Marshal(struct {
		Error string `json:"error"`
		Hint  string `json:"hint"`
	}{
		Error: path + " is not readable under the MCP sandbox",
		Hint: "run `humanizer scan --go` from a shell instead; the CLI is unsandboxed. " +
			"Readable here: .go files under the sandbox profile's workspace roots, and /tmp paths.",
	})
	return fmt.Errorf("%s — run the CLI instead: %w", redirect, err)
}
