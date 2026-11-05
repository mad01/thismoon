package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// frontmatterOut is the JSON-friendly projection of markdown.Frontmatter we
// return in read/peek/list responses. Mirrors the fields users actually care
// about; drops the yaml.Node Extra since it serializes poorly.
//
// Evidence is the computed trust tier (A/B/C/D/mixed/empty) — read from the
// explicit frontmatter.evidence field or parsed from an "evidence/*" tag
// when the field is absent. Callers route trust decisions on this value.
type frontmatterOut struct {
	Title    string   `json:"title,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Type     string   `json:"type,omitempty"`
	Status   string   `json:"status,omitempty"`
	Source   string   `json:"source,omitempty"`
	Evidence string   `json:"evidence,omitempty" jsonschema:"trust tier: A (replicated RCT / meta-analysis), B (single well-designed study), C (preliminary / survey / mechanism-plausible), D (refuted / apocryphal), mixed, or empty for non-empirical notes"`
	Created  string   `json:"created,omitempty"`
	Updated  string   `json:"updated,omitempty"`
	ID       string   `json:"id,omitempty"`
}

func toFrontmatterOut(fm markdown.Frontmatter) frontmatterOut {
	return frontmatterOut{
		Title:    fm.Title,
		Aliases:  fm.Aliases,
		Tags:     fm.Tags,
		Type:     fm.Type,
		Status:   fm.Status,
		Source:   fm.Source,
		Evidence: markdown.EvidenceOf(fm),
		Created:  fm.Created,
		Updated:  fm.Updated,
		ID:       fm.ID,
	}
}

type readInput struct {
	Path      string `json:"path"                 jsonschema:"vault-relative note path, e.g. notes/topic/foo.md"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first body line to return, 1-based; 0 or omit for start of body"`
	EndLine   int    `json:"end_line,omitempty"   jsonschema:"last body line to return, 1-based inclusive; 0 or omit for end of body"`
}

type readBodyLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type readOutput struct {
	Path        string         `json:"path"`
	Frontmatter frontmatterOut `json:"frontmatter"`
	BodyLines   []readBodyLine `json:"body_lines"`
}

type peekInput struct {
	Path         string `json:"path"                     jsonschema:"vault-relative note path"`
	MaxBodyLines int    `json:"max_body_lines,omitempty" jsonschema:"number of body lines to include in preview (default 20)"`
}

type peekOutput struct {
	Path          string         `json:"path"`
	Frontmatter   frontmatterOut `json:"frontmatter"`
	Preview       []string       `json:"preview"`
	BacklinkCount int            `json:"backlink_count" jsonschema:"number of notes in the vault that contain a [[stem]] wikilink to this note"`
}

func registerReadTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_read",
		Description: "Read a note's frontmatter and body (optionally a line slice). " +
			"Use after brain_search finds a relevant note and you need the full text, or when you already know a vault-relative path. " +
			"start_line / end_line are 1-based inclusive over the body (frontmatter excluded); omit both to read the whole body.",
	}, handleRead)

	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_peek",
		Description: "Cheap overview of a note: frontmatter, the first N body lines (default 20), and a count of inbound backlinks. " +
			"Use to decide whether a note is worth reading in full or to check its connectivity before diving in.",
	}, handlePeek)
}

func handleRead(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in readInput,
) (*mcp.CallToolResult, readOutput, error) {
	if in.StartLine < 0 || in.EndLine < 0 {
		return nil, readOutput{}, fmt.Errorf("brain_read: start_line and end_line must be >= 0")
	}
	if in.StartLine > 0 && in.EndLine > 0 && in.StartLine > in.EndLine {
		return nil, readOutput{}, fmt.Errorf(
			"brain_read: start_line (%d) must be <= end_line (%d)",
			in.StartLine,
			in.EndLine,
		)
	}

	cfg, _, err := loadVault()
	if err != nil {
		return nil, readOutput{}, fmt.Errorf("brain_read: %w", err)
	}
	full, err := resolveVaultPath(cfg, in.Path)
	if err != nil {
		return nil, readOutput{}, fmt.Errorf("brain_read: %w", err)
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return nil, readOutput{}, fmt.Errorf("brain_read: read %s: %w", in.Path, err)
	}
	fm, body, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return nil, readOutput{}, fmt.Errorf("brain_read: parse %s: %w", in.Path, err)
	}

	lines := sliceBodyLines(body, in.StartLine, in.EndLine)
	return nil, readOutput{
		Path:        in.Path,
		Frontmatter: toFrontmatterOut(fm),
		BodyLines:   lines,
	}, nil
}

func handlePeek(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in peekInput,
) (*mcp.CallToolResult, peekOutput, error) {
	max := in.MaxBodyLines
	if max <= 0 {
		max = 20
	}
	cfg, _, err := loadVault()
	if err != nil {
		return nil, peekOutput{}, fmt.Errorf("brain_peek: %w", err)
	}
	full, err := resolveVaultPath(cfg, in.Path)
	if err != nil {
		return nil, peekOutput{}, fmt.Errorf("brain_peek: %w", err)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, peekOutput{}, fmt.Errorf("brain_peek: read %s: %w", in.Path, err)
	}
	fm, body, err := markdown.ParseFrontmatter(data)
	if err != nil {
		return nil, peekOutput{}, fmt.Errorf("brain_peek: parse %s: %w", in.Path, err)
	}

	preview := firstBodyLines(body, max)
	stem := strings.TrimSuffix(filepath.Base(in.Path), filepath.Ext(in.Path))
	count, err := countBacklinks(cfg.Vault, stem, full)
	if err != nil {
		return nil, peekOutput{}, fmt.Errorf("brain_peek: count backlinks: %w", err)
	}
	return nil, peekOutput{
		Path:          in.Path,
		Frontmatter:   toFrontmatterOut(fm),
		Preview:       preview,
		BacklinkCount: count,
	}, nil
}

// sliceBodyLines returns body lines in [start, end] (1-based inclusive). Zero
// on either bound means unbounded on that side.
func sliceBodyLines(body []byte, start, end int) []readBodyLine {
	var out []readBodyLine
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for scanner.Scan() {
		n++
		if start > 0 && n < start {
			continue
		}
		if end > 0 && n > end {
			break
		}
		out = append(out, readBodyLine{Line: n, Text: scanner.Text()})
	}
	return out
}

func firstBodyLines(body []byte, max int) []string {
	out := make([]string, 0, max)
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		out = append(out, scanner.Text())
		if len(out) >= max {
			break
		}
	}
	return out
}

// countBacklinks walks the vault and counts markdown files whose content
// contains a [[<stem> wikilink prefix. Skips the note at selfPath itself so a
// note's own stem doesn't self-reference. Also skips .git/.obsidian/.trash.
func countBacklinks(vaultRoot, stem, selfPath string) (int, error) {
	if stem == "" {
		return 0, nil
	}
	needle := []byte("[[" + stem)
	count := 0
	err := filepath.WalkDir(vaultRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".obsidian" || name == ".trash" || name == "attachments" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		if path == selfPath {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if bytes.Contains(data, needle) {
			count++
		}
		return nil
	})
	return count, err
}
