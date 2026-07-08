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

func registerReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_read",
		Description: "Read a file from a named local repo by repo name and relative path. " +
			"Use when you already know which repo holds a file and want to read a specific line range without first resolving the repo's absolute path via csl_repo_lookup. " +
			"The repo param is a case-insensitive regex (like every csl repo param) and must resolve to exactly one repo — an ambiguous name returns an error listing the candidates. " +
			"Supply start_line / end_line (1-based inclusive) to slice; omit both to read the whole file (default caps at 500 lines). " +
			"The output always includes total_lines (the file's full line count) — when truncated is true, page through the rest with start_line/end_line.",
	}, handleRead)
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

	absPath := filepath.Join(matched.Path, in.File)
	f, err := os.Open(absPath)
	if err != nil {
		return nil, readOutput{}, fmt.Errorf("open %s: %w", absPath, err)
	}
	defer f.Close()

	const defaultMaxLines = 500

	// Apply default cap when no range is specified.
	effectiveEnd := in.EndLine
	unbounded := in.StartLine <= 0 && in.EndLine <= 0
	if unbounded {
		effectiveEnd = defaultMaxLines
	}

	scanner := bufio.NewScanner(f)
	// Increase the buffer so lines longer than 64 KB don't trip the scanner.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([]readLine, 0, 256)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if in.StartLine > 0 && lineNum < in.StartLine {
			continue
		}
		if effectiveEnd > 0 && lineNum > effectiveEnd {
			// Keep counting for total_lines but stop collecting.
			continue
		}
		lines = append(lines, readLine{Number: lineNum, Text: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		return nil, readOutput{}, fmt.Errorf("read %s: %w", absPath, err)
	}

	totalLines := lineNum
	truncated := unbounded && totalLines > defaultMaxLines

	return nil, readOutput{
		Repo:       matched.Name,
		Path:       in.File,
		Lines:      lines,
		TotalLines: totalLines,
		Truncated:  truncated,
	}, nil
}
