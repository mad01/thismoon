package vault

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// maxExcerpt caps the length of an inbound backlink excerpt.
const maxExcerpt = 200

// How a wikilink target matched the note it resolved to.
type linkMatch int

const (
	matchNone  linkMatch = iota
	matchStem            // the target is the note's stem
	matchPath            // the target is a path ending in the note's path
	matchAlias           // the target is the note's title or one of its aliases
)

// linkIndex resolves the wikilinks of every note in a set. It is the one
// resolver behind backlinks, peek's backlink count, promote's link rewrite
// and the graph, so they never disagree about where a link goes.
//
// A target resolves case-insensitively, first by stem (or by path when it
// holds a slash), then by title or alias. A note in a private vault looks in
// its own vault first, then in the shared vaults in config order; a note in
// a shared vault looks in its own vault, then in the other shared vaults,
// never in a private one. A target nothing matches is dangling; a target is
// never matched by prefix.
type linkIndex struct {
	set   *Set
	notes []*indexedNote
	byRef map[string]*indexedNote
	names map[*Vault]*nameTable
	// dupIDs holds the note ids more than one note carries.
	dupIDs map[string]bool
	// linked holds the markdown files that are symbolic links. They are
	// never read, link to nothing and resolve no link; Snapshot lists them
	// so brain check reports each one.
	linked []*indexedNote
}

// nameTable is one vault's notes by the names a wikilink can use. Each list
// is in resolution order: tier precedence, then path.
type nameTable struct {
	notes   []*indexedNote
	stems   map[string][]*indexedNote
	aliases map[string][]*indexedNote
}

// indexedNote is one note as the link index sees it.
type indexedNote struct {
	ref      Ref
	stem     string
	fm       markdown.Frontmatter
	parseErr error
	body     []byte
	data     []byte
	links    []noteLink
}

// noteLink is one wikilink in a note's body and where it resolved.
type noteLink struct {
	markdown.Wikilink // Line and Offset are in the whole file
	excerpt           string
	to                *indexedNote // nil when dangling
	match             linkMatch
}

// buildLinkIndex reads every markdown note in the set and resolves its
// wikilinks.
func buildLinkIndex(s *Set) (*linkIndex, error) {
	idx := &linkIndex{
		set:    s,
		byRef:  map[string]*indexedNote{},
		names:  map[*Vault]*nameTable{},
		dupIDs: map[string]bool{},
	}
	ids := map[string]int{}
	for _, v := range s.vaults {
		table := &nameTable{
			stems:   map[string][]*indexedNote{},
			aliases: map[string][]*indexedNote{},
		}
		err := walkNotes(v.Root, func(path string, data []byte) {
			table.notes = append(table.notes, readIndexedNote(v, path, data))
		}, func(path string) {
			idx.linked = append(idx.linked, linkedNote(v, path))
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", v.Name, err)
		}
		sort.SliceStable(table.notes, func(i, j int) bool {
			a, b := table.notes[i].ref.Path, table.notes[j].ref.Path
			if ra, rb := tierRank(a), tierRank(b); ra != rb {
				return ra < rb
			}
			return a < b
		})
		for _, n := range table.notes {
			table.add(n)
			idx.byRef[n.ref.String()] = n
			if n.fm.ID != "" {
				ids[n.fm.ID]++
			}
		}
		idx.names[v] = table
		idx.notes = append(idx.notes, table.notes...)
	}
	for id, count := range ids {
		if count > 1 {
			idx.dupIDs[id] = true
		}
	}
	for _, n := range idx.notes {
		idx.resolveLinks(n)
	}
	return idx, nil
}

// linkedNote is the unread entry of a note path that is a symbolic link.
func linkedNote(v *Vault, path string) *indexedNote {
	return &indexedNote{
		ref:  Ref{Vault: v, Path: v.Rel(path)},
		stem: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		parseErr: fmt.Errorf("%w: %s is a symbolic link; brain reads no note through a link",
			ErrInvalidPath, v.Rel(path)),
	}
}

// readIndexedNote parses one note. A note whose frontmatter does not parse
// still links and is linked to by its stem.
func readIndexedNote(v *Vault, path string, data []byte) *indexedNote {
	n := &indexedNote{
		ref:  Ref{Vault: v, Path: v.Rel(path)},
		stem: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		data: data,
	}
	fm, body, err := markdown.ParseFrontmatter(data)
	if err == nil {
		n.fm = fm
	}
	n.parseErr, n.body = err, body
	n.links = extractLinks(data, body)
	return n
}

// extractLinks returns the wikilinks of body, a suffix of data, with lines
// and offsets counted in data. Links with an empty target ([[#heading]], a
// link into the note itself) are left out.
func extractLinks(data, body []byte) []noteLink {
	bodyOffset := len(data) - len(body)
	lineBase := bytes.Count(data[:bodyOffset], []byte("\n"))
	var links []noteLink
	for _, wl := range markdown.ExtractWikilinks(body) {
		if wl.Target == "" {
			continue
		}
		wl.Line += lineBase
		wl.Offset += bodyOffset
		links = append(links, noteLink{Wikilink: wl, excerpt: lineAt(data, wl.Offset)})
	}
	return links
}

// lineAt returns the line of data holding offset, capped at maxExcerpt bytes.
func lineAt(data []byte, offset int) string {
	start := bytes.LastIndexByte(data[:offset], '\n') + 1
	end := len(data)
	if nl := bytes.IndexByte(data[offset:], '\n'); nl >= 0 {
		end = offset + nl
	}
	line := strings.TrimRight(string(data[start:end]), "\r")
	if len(line) > maxExcerpt {
		line = line[:maxExcerpt]
	}
	return line
}

func (t *nameTable) add(n *indexedNote) {
	stem := strings.ToLower(n.stem)
	t.stems[stem] = append(t.stems[stem], n)
	seen := map[string]bool{}
	for _, name := range append([]string{n.fm.Title}, n.fm.Aliases...) {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		t.aliases[key] = append(t.aliases[key], n)
	}
}

// byPath returns the first note whose path without .md is key or ends in
// "/"+key, both lowercase.
func (t *nameTable) byPath(key string) *indexedNote {
	for _, n := range t.notes {
		p := strings.ToLower(strings.TrimSuffix(n.ref.Path, filepath.Ext(n.ref.Path)))
		if p == key || strings.HasSuffix(p, "/"+key) {
			return n
		}
	}
	return nil
}

// tierRank orders notes that share a name: notes, MOCs, outputs, inbox,
// then anything outside the tiers.
func tierRank(rel string) int {
	dir := tierDir(rel)
	for i, t := range []Tier{TierNotes, TierMOCs, TierOutputs, TierInbox} {
		if dir == t.Dir() {
			return i
		}
	}
	return len(AllTiers)
}

func (idx *linkIndex) resolveLinks(n *indexedNote) {
	for i := range n.links {
		n.links[i].to, n.links[i].match = idx.resolve(n.ref.Vault, n.links[i].Target)
	}
}

// resolve finds the note a wikilink target written in a note of from names.
func (idx *linkIndex) resolve(from *Vault, target string) (*indexedNote, linkMatch) {
	key := strings.ToLower(strings.TrimSpace(target))
	key = strings.TrimSuffix(key, ".md")
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return nil, matchNone
	}
	for _, v := range idx.searchOrder(from) {
		t := idx.names[v]
		if strings.Contains(key, "/") {
			if n := t.byPath(key); n != nil {
				return n, matchPath
			}
			continue
		}
		if ns := t.stems[key]; len(ns) > 0 {
			return ns[0], matchStem
		}
		if ns := t.aliases[key]; len(ns) > 0 {
			return ns[0], matchAlias
		}
	}
	return nil, matchNone
}

// searchOrder returns the vaults a link written in from resolves in: from
// itself, then the shared vaults in config order.
func (idx *linkIndex) searchOrder(from *Vault) []*Vault {
	order := []*Vault{from}
	for _, v := range idx.set.vaults {
		if v != from && v.Shared {
			order = append(order, v)
		}
	}
	return order
}

// note returns the indexed note at ref. A note the walk did not visit (a
// file under a skipped directory) is read on its own: its outbound links
// resolve, and nothing links to it.
func (idx *linkIndex) note(ref Ref) (*indexedNote, error) {
	if n, ok := idx.byRef[ref.String()]; ok {
		return n, nil
	}
	if err := ref.Vault.contained(ref.Abs()); err != nil {
		return &indexedNote{
			ref:      ref,
			stem:     strings.TrimSuffix(filepath.Base(ref.Path), ".md"),
			parseErr: err,
		}, nil
	}
	data, err := os.ReadFile(ref.Abs())
	if err != nil {
		return nil, err
	}
	n := readIndexedNote(ref.Vault, ref.Abs(), data)
	idx.resolveLinks(n)
	return n, nil
}

// inbound returns the links from other notes that resolve to n, in note
// order.
func (idx *linkIndex) inbound(n *indexedNote) []*linkFrom {
	var out []*linkFrom
	for _, src := range idx.notes {
		if src == n {
			continue
		}
		for i := range src.links {
			if src.links[i].to == n {
				out = append(out, &linkFrom{note: src, link: &src.links[i]})
			}
		}
	}
	return out
}

// linkFrom is one link and the note it is written in.
type linkFrom struct {
	note *indexedNote
	link *noteLink
}

// Inbound is a line in another note that links to the analyzed note.
type Inbound struct {
	Vault    string `json:"vault"              jsonschema:"vault of the linking note"`
	Path     string `json:"path"               jsonschema:"vault-relative path of the linking note"`
	Line     int    `json:"line"               jsonschema:"1-based line number in the linking note where the wikilink appears"`
	Excerpt  string `json:"excerpt"            jsonschema:"the matching line trimmed to ~200 chars"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the linking note, if set (A/B/C/D/mixed)"`
}

// Outbound is a wikilink in the analyzed note.
type Outbound struct {
	Target   string `json:"target"             jsonschema:"wikilink target as written, e.g. Target for [[Target|text]]"`
	Vault    string `json:"vault,omitempty"    jsonschema:"vault of the resolved target"`
	Path     string `json:"path,omitempty"     jsonschema:"vault-relative path of the resolved target"`
	Line     int    `json:"line"               jsonschema:"1-based line number in the note"`
	Evidence string `json:"evidence,omitempty" jsonschema:"trust tier of the target note, if resolved and set"`
	Dangling bool   `json:"dangling,omitempty" jsonschema:"true when no note matches the target"`
}

// Backlinks is the link neighbourhood of one note.
type Backlinks struct {
	Vault    string     `json:"vault"              jsonschema:"vault of the note whose links were analyzed"`
	NotePath string     `json:"note_path"          jsonschema:"vault-relative path of the note whose links were analyzed"`
	Evidence string     `json:"evidence,omitempty" jsonschema:"trust tier of the analyzed note, if set"`
	Inbound  []Inbound  `json:"inbound"`
	Outbound []Outbound `json:"outbound"`
}

// Backlinks returns the inbound and outbound wikilinks of the note at ref,
// resolved across the set. Inbound lists each linking line once.
func (s *Set) Backlinks(ref Ref) (Backlinks, error) {
	idx, err := buildLinkIndex(s)
	if err != nil {
		return Backlinks{}, err
	}
	n, err := idx.note(ref)
	if err != nil {
		return Backlinks{}, err
	}
	out := Backlinks{
		Vault:    ref.Vault.Name,
		NotePath: ref.Path,
		Evidence: markdown.EvidenceOf(n.fm),
		Inbound:  []Inbound{},
		Outbound: make([]Outbound, 0, len(n.links)),
	}
	type line struct {
		src  *indexedNote
		line int
	}
	seen := map[line]bool{}
	for _, in := range idx.inbound(n) {
		key := line{in.note, in.link.Line}
		if seen[key] {
			continue
		}
		seen[key] = true
		out.Inbound = append(out.Inbound, Inbound{
			Vault:    in.note.ref.Vault.Name,
			Path:     in.note.ref.Path,
			Line:     in.link.Line,
			Excerpt:  in.link.excerpt,
			Evidence: markdown.EvidenceOf(in.note.fm),
		})
	}
	for _, l := range n.links {
		o := Outbound{Target: l.Target, Line: l.Line, Dangling: l.to == nil}
		if l.to != nil {
			o.Vault, o.Path = l.to.ref.Vault.Name, l.to.ref.Path
			o.Evidence = markdown.EvidenceOf(l.to.fm)
		}
		out.Outbound = append(out.Outbound, o)
	}
	return out, nil
}

// Backlinks returns the links of the note at the vault-relative path rel,
// resolved within this vault alone.
func (v *Vault) Backlinks(rel string) (Backlinks, error) {
	ref, err := v.locate(rel)
	if err != nil {
		return Backlinks{}, err
	}
	return NewSet(v).Backlinks(ref)
}

// backlinkCount counts the other notes with a link that resolves to ref.
func (s *Set) backlinkCount(ref Ref) (int, error) {
	idx, err := buildLinkIndex(s)
	if err != nil {
		return 0, err
	}
	n, err := idx.note(ref)
	if err != nil {
		return 0, err
	}
	sources := map[*indexedNote]bool{}
	for _, in := range idx.inbound(n) {
		sources[in.note] = true
	}
	return len(sources), nil
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
		if isLink(d) || !strings.HasSuffix(strings.ToLower(path), ".md") {
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

// walkNotes calls visit with the path and content of every readable markdown
// note in the vault, skipping tool directories. Unreadable files are
// skipped. A markdown file that is a symbolic link is never read: linked
// gets its path instead.
func walkNotes(
	vaultRoot string,
	visit func(path string, data []byte),
	linked func(path string),
) error {
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
		if isLink(d) {
			linked(path)
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
