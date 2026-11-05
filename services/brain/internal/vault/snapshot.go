package vault

import (
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// NoteView is one note of a Snapshot. Frontmatter is zero when the note's
// frontmatter does not parse; ParseErr then says why.
type NoteView struct {
	Ref         Ref
	Stem        string
	Frontmatter markdown.Frontmatter
	ParseErr    error
	Body        []byte
	Data        []byte

	n *indexedNote
}

// Citation is one wikilink and the note it resolves to: From is the note it
// is written in, To the note it names, nil when the link is dangling.
type Citation struct {
	From   *NoteView
	Target string
	// Line is the 1-based line of the link in From's file.
	Line int
	To   *NoteView
}

// Snapshot is every note of a set read once, with every wikilink resolved by
// the one resolver behind backlinks and the graph. It is for checks over
// many notes: build it once, then ask about each.
type Snapshot struct {
	idx   *linkIndex
	views map[*indexedNote]*NoteView
	ids   map[string][]*NoteView
}

// Snapshot reads every markdown note of the set and resolves its links.
func (s *Set) Snapshot() (*Snapshot, error) {
	idx, err := buildLinkIndex(s)
	if err != nil {
		return nil, err
	}
	sn := &Snapshot{idx: idx, views: map[*indexedNote]*NoteView{}, ids: map[string][]*NoteView{}}
	for _, n := range idx.notes {
		v := sn.view(n)
		if id := n.fm.ID; id != "" {
			sn.ids[id] = append(sn.ids[id], v)
		}
	}
	return sn, nil
}

func (sn *Snapshot) view(n *indexedNote) *NoteView {
	if v, ok := sn.views[n]; ok {
		return v
	}
	v := &NoteView{
		Ref: n.ref, Stem: n.stem, Frontmatter: n.fm, ParseErr: n.parseErr,
		Body: n.body, Data: n.data, n: n,
	}
	sn.views[n] = v
	return v
}

// Notes returns every note the walk visited, vault by vault in config order.
// Files outside the tiers (a root README) are included; Tier tells them
// apart. A note path that is a symbolic link comes last, unread, with a
// ParseErr saying so.
func (sn *Snapshot) Notes() []*NoteView {
	out := make([]*NoteView, 0, len(sn.idx.notes)+len(sn.idx.linked))
	for _, n := range sn.idx.notes {
		out = append(out, sn.view(n))
	}
	for _, n := range sn.idx.linked {
		out = append(out, sn.view(n))
	}
	return out
}

// Note returns the note at ref. A note the walk skipped is read on its own.
func (sn *Snapshot) Note(ref Ref) (*NoteView, error) {
	n, err := sn.idx.note(ref)
	if err != nil {
		return nil, err
	}
	return sn.view(n), nil
}

// Citations returns the wikilinks written in note, in file order.
func (sn *Snapshot) Citations(note *NoteView) []Citation {
	out := make([]Citation, 0, len(note.n.links))
	for _, l := range note.n.links {
		c := Citation{From: note, Target: l.Target, Line: l.Line}
		if l.to != nil {
			c.To = sn.view(l.to)
		}
		out = append(out, c)
	}
	return out
}

// CitedBy returns the links in other notes that resolve to note.
func (sn *Snapshot) CitedBy(note *NoteView) []Citation {
	var out []Citation
	for _, in := range sn.idx.inbound(note.n) {
		out = append(out, Citation{
			From: sn.view(in.note), Target: in.link.Target, Line: in.link.Line, To: note,
		})
	}
	return out
}

// WithID returns the notes that carry id, in walk order.
func (sn *Snapshot) WithID(id string) []*NoteView { return sn.ids[id] }

// Resolve finds the note a stored note ref names, the way Set.Locate does:
// id:<ulid>, vault:path, or a vault-relative path found in exactly one
// vault. It reports false when no note, or more than one, matches.
func (sn *Snapshot) Resolve(ref string) (*NoteView, bool) {
	name, rel, qualified := splitRef(strings.TrimSpace(ref))
	if qualified && name == idRefPrefix {
		found := sn.ids[rel]
		return firstOnly(found)
	}
	var found []*NoteView
	for _, n := range sn.idx.notes {
		if qualified && n.ref.Vault.Name != name {
			continue
		}
		if strings.EqualFold(n.ref.Path, strings.TrimPrefix(rel, "/")) {
			found = append(found, sn.view(n))
		}
	}
	return firstOnly(found)
}

func firstOnly(found []*NoteView) (*NoteView, bool) {
	if len(found) != 1 {
		return nil, false
	}
	return found[0], true
}

// Tier returns the tier of the note, or "" for a file outside the tiers.
func (v *NoteView) Tier() Tier {
	return Tier(tierDir(v.Ref.Path))
}

// Status returns the note's effective status: its status field, else the
// default of its tier.
func (v *NoteView) Status() string { return StatusOf(v.Ref.Path, v.Frontmatter) }
