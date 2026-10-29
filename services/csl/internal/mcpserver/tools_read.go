package mcpserver

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// readInput is the typed input for the csl_read tool.
type readInput struct {
	Repo      string `json:"repo"                 jsonschema:"case-insensitive regex or substring matched against the org/repo name (e.g. 'myrepo', 'mad01/thismoon'); must resolve to exactly one repo"`
	File      string `json:"file"                 jsonschema:"file path relative to the repo root"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line to return, 1-based; 0 or omit for start of file"`
	EndLine   int    `json:"end_line,omitempty"   jsonschema:"last line to return, 1-based inclusive; 0 or omit for end of file"`
	formatParam
}

// readLine is one line in the csl_read result.
type readLine struct {
	Number int    `json:"number" jsonschema:"1-based line number in the source file"`
	Text   string `json:"text"`
}

// readOutput is the typed output of the csl_read tool.
type readOutput struct {
	Repo       string     `json:"repo"        jsonschema:"the resolved repo name (canonical org/repo form)"`
	Path       string     `json:"path"        jsonschema:"file path relative to the repo root"`
	Lines      []readLine `json:"lines"`
	TotalLines int        `json:"total_lines" jsonschema:"total line count of the file"`
	Truncated  bool       `json:"truncated"   jsonschema:"true if output was capped; use start_line/end_line to read specific ranges"`
}

// maxReadLines caps a csl_read call that sets no range.
const maxReadLines = 500

func registerReadTools(s *mcp.Server) {
	addFormattedTool(s, &mcp.Tool{
		Name: "csl_read",
		Description: "Read a file from a named local repo by repo name and relative path. " +
			"Use when you already know which repo holds a file and want to read a specific line range without first resolving the repo's absolute path via csl_repo_lookup. " +
			"The repo param is a case-insensitive regex (like every csl repo param) and must resolve to exactly one repo; an ambiguous name returns an error listing the candidates. " +
			"Supply start_line / end_line (1-based inclusive) to slice; omit both to read the whole file (default caps at 500 lines). " +
			"The output always includes total_lines (the file's full line count). When truncated is true, page through the rest with start_line/end_line. " +
			"Several independent files or ranges: " + parallelCallsTail,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
		Meta: mcp.Meta{alwaysLoadKey: true},
	}, handleRead, renderReadText)
}

func handleRead(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in readInput,
) (*mcp.CallToolResult, readOutput, error) {
	if strings.TrimSpace(in.Repo) == "" {
		return nil, readOutput{}, fmt.Errorf("repo is required")
	}
	if strings.TrimSpace(in.File) == "" {
		return nil, readOutput{}, fmt.Errorf("file is required")
	}
	if in.StartLine < 0 || in.EndLine < 0 {
		return nil, readOutput{}, fmt.Errorf("start_line and end_line must be >= 0")
	}
	if in.StartLine > 0 && in.EndLine > 0 && in.StartLine > in.EndLine {
		return nil, readOutput{}, fmt.Errorf(
			"start_line (%d) must be <= end_line (%d)",
			in.StartLine,
			in.EndLine,
		)
	}

	matched, err := resolveRepo(in.Repo)
	if err != nil {
		return nil, readOutput{}, err
	}

	// Apply the default cap when no range is specified.
	effectiveEnd := in.EndLine
	unbounded := in.StartLine <= 0 && in.EndLine <= 0
	if unbounded {
		effectiveEnd = maxReadLines
	}

	lines, totalLines, err := readLines(
		filepath.Join(matched.Path, in.File),
		in.StartLine,
		effectiveEnd,
	)
	if err != nil {
		return nil, readOutput{}, err
	}
	truncated := unbounded && totalLines > maxReadLines

	return nil, readOutput{
		Repo:       matched.Name,
		Path:       in.File,
		Lines:      lines,
		TotalLines: totalLines,
		Truncated:  truncated,
	}, nil
}

// readLines returns lines start through end of the file at path, 1-based and
// inclusive (0 leaves that side open), plus the file's total line count. The
// scan reads to the end either way, so the count is always the whole file's.
func readLines(path string, start, end int) ([]readLine, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase the buffer so lines longer than 64 KB don't trip the scanner.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([]readLine, 0, 256)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if start > 0 && lineNum < start {
			continue
		}
		if end > 0 && lineNum > end {
			// Keep counting for the total but stop collecting.
			continue
		}
		lines = append(lines, readLine{Number: lineNum, Text: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	return lines, lineNum, nil
}
