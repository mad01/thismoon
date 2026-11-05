package vault

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// maxExcerpt caps the length of an inbound backlink excerpt.
const maxExcerpt = 200

// Inbound is a line in another note that links to the analyzed note.
type Inbound struct {
	Path     string `json:"path"               jsonschema:"vault-relative path of the linking note"`
	Line     int    `json:"line"               jsonschema:"1-based line number where the wikilink appears"`
	Excerpt  string `json:"excerpt"            jsonschema:"the matching line trimmed to ~200 chars"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the linking note, if set (A/B/C/D/mixed)"`
}

// Outbound is a wikilink in the analyzed note.
type Outbound struct {
	Target   string `json:"target"             jsonschema:"wikilink target as written, e.g. [[Target]]"`
	Path     string `json:"path,omitempty"     jsonschema:"resolved vault-relative path if the target matches a note in the vault"`
	Line     int    `json:"line"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the target note, if resolved and set"`
}

// Backlinks is the link neighbourhood of one note.
type Backlinks struct {
	NotePath string     `json:"note_path"          jsonschema:"vault-relative path of the note whose links were analyzed"`
	Evidence string     `json:"evidence,omitempty" jsonschema:"trust tier of the analyzed note, if set"`
	Inbound  []Inbound  `json:"inbound"`
	Outbound []Outbound `json:"outbound"`
}

// Backlinks returns the inbound and outbound wikilinks of the note at the
// vault-relative path rel. Inbound links match [[stem]], [[title]] or
// [[alias]]; outbound targets resolve to a path when a note with that stem
// exists.
func (v *Vault) Backlinks(rel string) (Backlinks, error) {
	absNote, err := v.Resolve(rel)
	if err != nil {
		return Backlinks{}, err
	}
	fm, body, err := v.ReadFrontmatter(absNote)
	if err != nil {
		return Backlinks{}, err
	}

	stem := strings.TrimSuffix(filepath.Base(absNote), filepath.Ext(absNote))
	inbound, err := findInboundLinks(v.Root, absNote, linkNeedles(stem, fm))
	if err != nil {
		return Backlinks{}, err
	}
	// Small N: cache evidence per path so each note is parsed at most once.
	evidence := make(map[string]string)
	for i := range inbound {
		inbound[i].Evidence = v.evidenceOf(evidence, inbound[i].Path)
	}

	targetMap, err := buildTargetMap(v.Root)
	if err != nil {
		return Backlinks{}, err
	}
	targets := markdown.ExtractWikilinks(body)
	outbound := make([]Outbound, 0, len(targets))
	for _, wl := range targets {
		entry := Outbound{
			Target: wl.Target,
			Path:   targetMap[strings.ToLower(wl.Target)],
			Line:   wl.Line,
		}
		if entry.Path != "" {
			entry.Evidence = v.evidenceOf(evidence, entry.Path)
		}
		outbound = append(outbound, entry)
	}

	return Backlinks{
		NotePath: v.Rel(absNote),
		Evidence: markdown.EvidenceOf(fm),
		Inbound:  inbound,
		Outbound: outbound,
	}, nil
}

// FindByTitle returns the vault-relative path of the first note whose
// frontmatter title or alias equals title (case-insensitive), or
// ErrNotFound.
func (v *Vault) FindByTitle(title string) (string, error) {
	var matched string
	err := filepath.WalkDir(v.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		fm, _, err := v.ReadFrontmatter(path)
		if err != nil {
			return nil // skip an unreadable note, keep looking
		}
		if strings.EqualFold(fm.Title, title) || hasAlias(fm, title) {
			matched = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if matched == "" {
		return "", fmt.Errorf("%w: no note with title or alias %q", ErrNotFound, title)
	}
	return v.Rel(matched), nil
}

func hasAlias(fm markdown.Frontmatter, name string) bool {
	for _, a := range fm.Aliases {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

// linkNeedles returns the names an inbound wikilink may use for a note: its
// stem, then its title and aliases when they differ.
func linkNeedles(stem string, fm markdown.Frontmatter) []string {
	needles := []string{stem}
	if fm.Title != "" && fm.Title != stem {
		needles = append(needles, fm.Title)
	}
	for _, a := range fm.Aliases {
		if a != "" && a != stem && a != fm.Title {
			needles = append(needles, a)
		}
	}
	return needles
}

// evidenceOf returns the evidence tier of the note at rel, caching by path.
// A note that cannot be read has no tier.
func (v *Vault) evidenceOf(cache map[string]string, rel string) string {
	if ev, ok := cache[rel]; ok {
		return ev
	}
	ev := ""
	if fm, _, err := v.ReadFrontmatter(filepath.Join(v.Root, filepath.FromSlash(rel))); err == nil {
		ev = markdown.EvidenceOf(fm)
	}
	cache[rel] = ev
	return ev
}

// countBacklinks walks the vault and counts markdown files whose content
// contains a [[<stem> wikilink prefix. Skips the note at selfPath itself so a
// note's own stem doesn't self-reference.
func countBacklinks(vaultRoot, stem, selfPath string) (int, error) {
	if stem == "" {
		return 0, nil
	}
	needle := []byte("[[" + stem)
	count := 0
	err := walkNotes(vaultRoot, func(path string, data []byte) {
		if path != selfPath && bytes.Contains(data, needle) {
			count++
		}
	})
	return count, err
}

// findInboundLinks walks the vault looking for files whose content contains
// `[[<needle>` for any of the supplied needles. Returns the linking file's
// path, the 1-based line number, and an excerpt of the matching line.
func findInboundLinks(vaultRoot, selfPath string, needles []string) ([]Inbound, error) {
	byteNeedles := make([][]byte, 0, len(needles))
	for _, n := range needles {
		if n != "" {
			byteNeedles = append(byteNeedles, []byte("[["+n))
		}
	}
	var out []Inbound
	err := walkNotes(vaultRoot, func(path string, data []byte) {
		if path == selfPath || !containsAny(data, byteNeedles) {
			return
		}
		rel, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		scanner := newLineScanner(data)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Bytes()
			if !containsAny(line, byteNeedles) {
				continue
			}
			text := string(line)
			if len(text) > maxExcerpt {
				text = text[:maxExcerpt]
			}
			out = append(out, Inbound{Path: rel, Line: lineNum, Excerpt: text})
		}
	})
	return out, err
}

func containsAny(data []byte, needles [][]byte) bool {
	for _, n := range needles {
		if bytes.Contains(data, n) {
			return true
		}
	}
	return false
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
			if skipDir(d.Name()) {
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
			return nil // a path we cannot relativize is not a target
		}
		out[strings.ToLower(stem)] = filepath.ToSlash(rel)
		return nil
	})
	return out, err
}

// walkNotes calls visit with the path and content of every readable markdown
// note in the vault, skipping tool directories. Unreadable files are skipped.
func walkNotes(vaultRoot string, visit func(path string, data []byte)) error {
	return filepath.WalkDir(vaultRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip an unreadable note, keep walking
		}
		visit(path, data)
		return nil
	})
}
