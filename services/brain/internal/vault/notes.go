package vault

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// Defaults for the read-side operations when the caller passes zero.
const (
	defaultPeekLines = 20
	defaultListLimit = 100
)

// BodyLine is one numbered line of a note body.
type BodyLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Note is a note's frontmatter plus a slice of its body, as read returns it.
type Note struct {
	Vault       string          `json:"vault"`
	Path        string          `json:"path"`
	SHA         string          `json:"sha"         jsonschema:"sha256 of the note's bytes; pass it to edit as if_match to refuse an edit when the note changed since"`
	Frontmatter FrontmatterView `json:"frontmatter"`
	BodyLines   []BodyLine      `json:"body_lines"`

	fm markdown.Frontmatter
}

// Raw returns the parsed frontmatter, for rendering it back as YAML.
func (n Note) Raw() markdown.Frontmatter { return n.fm }

// Peek is a cheap overview of a note: frontmatter, the first body lines and
// the number of notes linking to it.
type Peek struct {
	Vault         string          `json:"vault"`
	Path          string          `json:"path"`
	Frontmatter   FrontmatterView `json:"frontmatter"`
	Preview       []string        `json:"preview"`
	BacklinkCount int             `json:"backlink_count" jsonschema:"number of notes in the vault that contain a [[stem]] wikilink to this note"`

	fm markdown.Frontmatter
}

// Raw returns the parsed frontmatter, for rendering it back as YAML.
func (p Peek) Raw() markdown.Frontmatter { return p.fm }

// Read returns the note at the vault-relative path rel with body lines
// start..end, 1-based and inclusive; zero leaves that side open. The caller
// validates the range.
func (v *Vault) Read(rel string, start, end int) (Note, error) {
	full, err := v.Resolve(rel)
	if err != nil {
		return Note{}, err
	}
	data, fm, body, err := v.readNote(full)
	if err != nil {
		return Note{}, err
	}
	view, err := viewOf(fm)
	if err != nil {
		return Note{}, fmt.Errorf("%s: %w", rel, err)
	}
	return Note{
		Vault:       v.Name,
		Path:        v.Rel(full),
		SHA:         ContentSHA(data),
		Frontmatter: view,
		BodyLines:   sliceBodyLines(body, start, end),
		fm:          fm,
	}, nil
}

// Peek returns the frontmatter, the first maxLines body lines (20 when zero)
// and the inbound backlink count of the note at the vault-relative path rel,
// with links resolved within this vault alone.
func (v *Vault) Peek(rel string, maxLines int) (Peek, error) {
	ref, err := v.locate(rel)
	if err != nil {
		return Peek{}, err
	}
	return NewSet(v).Peek(ref, maxLines)
}

// Peek returns the frontmatter, the first maxLines body lines (20 when zero)
// and the number of other notes in the set that link to the note at ref.
func (s *Set) Peek(ref Ref, maxLines int) (Peek, error) {
	if maxLines <= 0 {
		maxLines = defaultPeekLines
	}
	fm, body, err := ref.Vault.ReadFrontmatter(ref.Abs())
	if err != nil {
		return Peek{}, err
	}
	view, err := viewOf(fm)
	if err != nil {
		return Peek{}, fmt.Errorf("%s: %w", ref.Path, err)
	}
	count, err := s.backlinkCount(ref)
	if err != nil {
		return Peek{}, fmt.Errorf("count backlinks: %w", err)
	}
	return Peek{
		Vault:         ref.Vault.Name,
		Path:          ref.Path,
		Frontmatter:   view,
		Preview:       firstBodyLines(body, maxLines),
		BacklinkCount: count,
		fm:            fm,
	}, nil
}

// ListOptions selects the notes List returns. Empty filters match every note.
type ListOptions struct {
	Tier   Tier
	Tag    string
	Status string
	Type   string
	Limit  int // 100 when zero
}

// ListedNote is one row of a tier listing.
type ListedNote struct {
	Vault    string   `json:"vault"              jsonschema:"name of the vault holding the note"`
	Path     string   `json:"path"               jsonschema:"vault-relative note path"`
	Title    string   `json:"title,omitempty"`
	Status   string   `json:"status,omitempty"`
	Type     string   `json:"type,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
	Evidence string   `json:"evidence,omitempty" jsonschema:"trust tier (A/B/C/D/mixed) if set"`
	Updated  string   `json:"updated,omitempty"`
}

// Listing is the notes of one tier, newest update first. Total counts every
// match before the limit.
type Listing struct {
	Tier  string       `json:"tier"`
	Total int          `json:"total"`
	Notes []ListedNote `json:"notes"`
}

// List returns the notes in one tier of this vault that match the filters,
// sorted by frontmatter.updated descending, then path. A missing tier
// directory is an empty listing; unreadable or unparsable notes are skipped.
func (v *Vault) List(opts ListOptions) (Listing, error) {
	return NewSet(v).List(opts)
}

func (v *Vault) collectTier(opts ListOptions) ([]ListedNote, error) {
	dir := v.TierDir(opts.Tier)
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	var out []ListedNote
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
			return nil // skip an unreadable note, keep listing
		}
		if !matchesFilters(fm, opts) {
			return nil
		}
		out = append(out, ListedNote{
			Vault:    v.Name,
			Path:     v.Rel(path),
			Title:    fm.Title,
			Status:   fm.Status,
			Type:     fm.Type,
			Tags:     fm.Tags,
			Aliases:  fm.Aliases,
			Evidence: markdown.EvidenceOf(fm),
			Updated:  fm.Updated,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func matchesFilters(fm markdown.Frontmatter, opts ListOptions) bool {
	if opts.Tag != "" && !fm.HasTag(opts.Tag) {
		return false
	}
	if opts.Status != "" && !strings.EqualFold(fm.Status, opts.Status) {
		return false
	}
	if opts.Type != "" && !strings.EqualFold(fm.Type, opts.Type) {
		return false
	}
	return true
}

// sliceBodyLines returns body lines in [start, end] (1-based inclusive). Zero
// on either bound means unbounded on that side.
func sliceBodyLines(body []byte, start, end int) []BodyLine {
	var out []BodyLine
	scanner := newLineScanner(body)
	n := 0
	for scanner.Scan() {
		n++
		if start > 0 && n < start {
			continue
		}
		if end > 0 && n > end {
			break
		}
		out = append(out, BodyLine{Line: n, Text: scanner.Text()})
	}
	return out
}

func firstBodyLines(body []byte, maxLines int) []string {
	out := make([]string, 0, maxLines)
	scanner := newLineScanner(body)
	for scanner.Scan() {
		out = append(out, scanner.Text())
		if len(out) >= maxLines {
			break
		}
	}
	return out
}

// newLineScanner scans lines of up to 1 MiB, enough for long markdown lines.
func newLineScanner(data []byte) *bufio.Scanner {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return scanner
}
