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

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

type backlinksInput struct {
	Path  string `json:"path,omitempty"  jsonschema:"vault-relative note path; one of path or title is required"`
	Title string `json:"title,omitempty" jsonschema:"note title or alias to look up; matched against frontmatter.title and aliases"`
}

type backlinkInbound struct {
	Path     string `json:"path"               jsonschema:"vault-relative path of the linking note"`
	Line     int    `json:"line"               jsonschema:"1-based line number where the wikilink appears"`
	Excerpt  string `json:"excerpt"            jsonschema:"the matching line trimmed to ~200 chars"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the linking note, if set (A/B/C/D/mixed)"`
}

type backlinkOutbound struct {
	Target   string `json:"target"             jsonschema:"wikilink target as written, e.g. [[Target]]"`
	Path     string `json:"path,omitempty"     jsonschema:"resolved vault-relative path if the target matches a note in the vault"`
	Line     int    `json:"line"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the target note, if resolved and set"`
}

type backlinksOutput struct {
	NotePath string             `json:"note_path"          jsonschema:"vault-relative path of the note whose links were analyzed"`
	Evidence string             `json:"evidence,omitempty" jsonschema:"trust tier of the analyzed note, if set"`
	Inbound  []backlinkInbound  `json:"inbound"`
	Outbound []backlinkOutbound `json:"outbound"`
}

func registerGraphTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "brain_backlinks",
		Description: "Return inbound and outbound wikilinks for a note. " +
			"Inbound: every note that contains [[stem]] or [[title]] pointing at this note (with the matching line and excerpt). " +
			"Outbound: every [[...]] wikilink in the note itself, resolved to a vault-relative path when the target matches an existing note. " +
			"Supply either path (preferred) or title; title is matched against frontmatter.title and aliases.",
	}, handleBacklinks)
}

func handleBacklinks(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in backlinksInput,
) (*mcp.CallToolResult, backlinksOutput, error) {
	if in.Path == "" && in.Title == "" {
		return nil, backlinksOutput{}, fmt.Errorf(
			"brain_backlinks: one of path or title is required",
		)
	}
	cfg, _, err := loadVault()
	if err != nil {
		return nil, backlinksOutput{}, fmt.Errorf("brain_backlinks: %w", err)
	}

	var absNote string
	var relNote string
	if in.Path != "" {
		abs, err := resolveVaultPath(cfg, in.Path)
		if err != nil {
			return nil, backlinksOutput{}, fmt.Errorf("brain_backlinks: %w", err)
		}
		absNote = abs
		relNote = filepath.ToSlash(filepath.Clean(in.Path))
	} else {
		abs, rel, err := findNoteByTitle(cfg, in.Title)
		if err != nil {
			return nil, backlinksOutput{}, fmt.Errorf("brain_backlinks: %w", err)
		}
		absNote = abs
		relNote = rel
	}

	stem := strings.TrimSuffix(filepath.Base(absNote), filepath.Ext(absNote))
	fm, body, err := readNote(absNote)
	if err != nil {
		return nil, backlinksOutput{}, fmt.Errorf("brain_backlinks: %w", err)
	}

	needles := []string{stem}
	if fm.Title != "" && fm.Title != stem {
		needles = append(needles, fm.Title)
	}
	for _, a := range fm.Aliases {
		if a != "" && a != stem && a != fm.Title {
			needles = append(needles, a)
		}
	}

	inbound, err := findInboundLinks(cfg.Vault, absNote, needles)
	if err != nil {
		return nil, backlinksOutput{}, fmt.Errorf("brain_backlinks: %w", err)
	}
	// Annotate each inbound entry with the linking note's evidence tier.
	// Small N — cache parses per path to avoid re-reading files.
	evidenceCache := make(map[string]string)
	for i, ib := range inbound {
		absLinker := filepath.Join(cfg.Vault, filepath.FromSlash(ib.Path))
		if ev, ok := evidenceCache[absLinker]; ok {
			inbound[i].Evidence = ev
			continue
		}
		if lfm, _, err := readNote(absLinker); err == nil {
			ev := markdown.EvidenceOf(lfm)
			evidenceCache[absLinker] = ev
			inbound[i].Evidence = ev
		}
	}

	// Outbound: parse wikilinks in body, try to resolve targets.
	targets := markdown.ExtractWikilinks(body)
	targetMap, err := buildTargetMap(cfg.Vault)
	if err != nil {
		return nil, backlinksOutput{}, fmt.Errorf("brain_backlinks: %w", err)
	}
	outbound := make([]backlinkOutbound, 0, len(targets))
	for _, wl := range targets {
		resolved := targetMap[strings.ToLower(wl.Target)]
		entry := backlinkOutbound{
			Target: wl.Target,
			Path:   resolved,
			Line:   wl.Line,
		}
		if resolved != "" {
			absTarget := filepath.Join(cfg.Vault, filepath.FromSlash(resolved))
			if ev, ok := evidenceCache[absTarget]; ok {
				entry.Evidence = ev
			} else if tfm, _, err := readNote(absTarget); err == nil {
				ev := markdown.EvidenceOf(tfm)
				evidenceCache[absTarget] = ev
				entry.Evidence = ev
			}
		}
		outbound = append(outbound, entry)
	}

	return nil, backlinksOutput{
		NotePath: relNote,
		Evidence: markdown.EvidenceOf(fm),
		Inbound:  inbound,
		Outbound: outbound,
	}, nil
}

func readNote(absPath string) (markdown.Frontmatter, []byte, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return markdown.Frontmatter{}, nil, err
	}
	return markdown.ParseFrontmatter(data)
}

func findNoteByTitle(cfg *config.Config, title string) (string, string, error) {
	var matched string
	err := filepath.WalkDir(cfg.Vault, func(path string, d fs.DirEntry, err error) error {
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
		if matched != "" {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		fm, _, err := markdown.ParseFrontmatter(data)
		if err != nil {
			return nil
		}
		if strings.EqualFold(fm.Title, title) {
			matched = path
			return nil
		}
		for _, a := range fm.Aliases {
			if strings.EqualFold(a, title) {
				matched = path
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if matched == "" {
		return "", "", fmt.Errorf("no note with title or alias %q", title)
	}
	rel, err := filepath.Rel(cfg.Vault, matched)
	if err != nil {
		return "", "", err
	}
	return matched, filepath.ToSlash(rel), nil
}

// findInboundLinks walks the vault looking for files whose content contains
// `[[<needle>` for any of the supplied needles. Returns the linking file's
// path, the 1-based line number, and an excerpt of the matching line.
func findInboundLinks(vaultRoot, selfPath string, needles []string) ([]backlinkInbound, error) {
	var out []backlinkInbound
	byteNeedles := make([][]byte, 0, len(needles))
	for _, n := range needles {
		if n == "" {
			continue
		}
		byteNeedles = append(byteNeedles, []byte("[["+n))
	}
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
		if path == selfPath {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hit := false
		for _, n := range byteNeedles {
			if bytes.Contains(data, n) {
				hit = true
				break
			}
		}
		if !hit {
			return nil
		}
		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Bytes()
			for _, n := range byteNeedles {
				if bytes.Contains(line, n) {
					text := string(line)
					if len(text) > 200 {
						text = text[:200]
					}
					out = append(out, backlinkInbound{
						Path:    rel,
						Line:    lineNum,
						Excerpt: text,
					})
					break
				}
			}
		}
		return nil
	})
	return out, err
}

// buildTargetMap walks the vault once and returns a lowercase stem → relpath
// index used to resolve outbound wikilink targets.
func buildTargetMap(vaultRoot string) (map[string]string, error) {
	out := make(map[string]string)
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
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return nil
		}
		out[strings.ToLower(stem)] = filepath.ToSlash(rel)
		return nil
	})
	return out, err
}
