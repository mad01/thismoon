package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// linkRetarget says how promote points the links to a moved note at its new
// place.
type linkRetarget struct {
	// oldTitle and newTitle are the title before and after; newTitle is
	// empty when promote keeps the title.
	oldTitle, newTitle string
	// aliases are the moved note's aliases, which it keeps.
	aliases []string
	// newStem is the moved note's stem, newPath its vault-relative path
	// without .md.
	newStem, newPath string
}

// rewrite returns the link text that points l at the moved note, or false
// when l still resolves to it unchanged. A stem link takes the new stem and
// a path link the new path. A title link takes the new stem, with the old
// title as its display text, only when the title changes and is not also an
// alias; an alias link keeps resolving through the alias.
func (r linkRetarget) rewrite(l noteLink) (string, bool) {
	switch l.match {
	case matchStem:
		if strings.EqualFold(l.Target, r.newStem) {
			return "", false
		}
		return replaceLinkTarget(l.Raw, r.newStem, ""), true
	case matchPath:
		if strings.EqualFold(strings.TrimSuffix(l.Target, ".md"), r.newPath) {
			return "", false
		}
		return replaceLinkTarget(l.Raw, r.newPath, ""), true
	case matchAlias:
		renamed := r.newTitle != "" && !strings.EqualFold(r.newTitle, r.oldTitle)
		if !renamed || !strings.EqualFold(l.Target, r.oldTitle) ||
			hasAlias(markdown.Frontmatter{Aliases: r.aliases}, l.Target) {
			return "", false
		}
		return replaceLinkTarget(l.Raw, r.newStem, l.Target), true
	}
	return "", false
}

// replaceLinkTarget swaps the target of the raw [[target#heading|text]]
// link for target, keeping heading, block and display text. display, when
// set, becomes the display text of a link that has none.
func replaceLinkTarget(raw, target, display string) string {
	inner := raw[2 : len(raw)-2]
	cut := strings.IndexAny(inner, "|^#")
	if cut < 0 {
		cut = len(inner)
	}
	out := target + inner[cut:]
	if display != "" && !strings.Contains(inner, "|") {
		out += "|" + display
	}
	return "[[" + out + "]]"
}

// linkEdit replaces the raw link at offset with text.
type linkEdit struct {
	offset int
	raw    string
	text   string
}

// applyLinkEdits returns data with every edit applied. An edit whose raw
// link is not at its offset means the note changed since it was read.
func applyLinkEdits(data []byte, edits []linkEdit) ([]byte, error) {
	sorted := slices.Clone(edits)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].offset > sorted[j].offset })
	out := slices.Clone(data)
	for _, e := range sorted {
		end := e.offset + len(e.raw)
		if end > len(out) || string(out[e.offset:end]) != e.raw {
			return nil, fmt.Errorf("link %s moved while promoting", e.raw)
		}
		out = slices.Concat(out[:e.offset], []byte(e.text), out[end:])
	}
	return out, nil
}

// retargetLinks computes the edits that point every link to moved at its
// new place, by the note they are written in.
func retargetLinks(idx *linkIndex, moved *indexedNote, r linkRetarget) map[*indexedNote][]linkEdit {
	edits := map[*indexedNote][]linkEdit{}
	for _, n := range idx.notes {
		for _, l := range n.links {
			if l.to != moved {
				continue
			}
			if text, ok := r.rewrite(l); ok {
				edits[n] = append(edits[n], linkEdit{offset: l.Offset, raw: l.Raw, text: text})
			}
		}
	}
	return edits
}

// noteEdits is the link edits promote makes in one note.
type noteEdits struct {
	note  *indexedNote
	edits []linkEdit
}

// body returns the note's body with the edits applied.
func (e noteEdits) body() ([]byte, error) {
	data, err := applyLinkEdits(e.note.data, e.edits)
	if err != nil {
		return nil, err
	}
	_, body := markdown.Split(data)
	return body, nil
}

// promoteEdits resolves the vault's links and returns the edits that point
// the links to the note at path at its new place, keyed by the
// vault-relative path of the note they are written in. Only this vault is
// read: a private vault's notes are linked to from the vault alone, and
// promote refuses a shared vault.
func (v *Vault) promoteEdits(path string, r linkRetarget) (map[string]noteEdits, error) {
	idx, err := buildLinkIndex(NewSet(v))
	if err != nil {
		return nil, err
	}
	moved, err := idx.note(Ref{Vault: v, Path: v.Rel(path)})
	if err != nil {
		return nil, err
	}
	out := map[string]noteEdits{}
	for n, edits := range retargetLinks(idx, moved, r) {
		out[n.ref.Path] = noteEdits{note: n, edits: edits}
	}
	return out, nil
}

// batchIndexNamePattern matches MOC filenames that act as batch-provenance
// indexes (the kickoff registry of a research wave) rather than topical maps
// of content. Examples: "research-index-2026-134-reference-topics.md",
// "kickoff-q2-2026.md". Such files become graph hubs purely by batch
// membership and distort centrality, so Promote refuses to append to them.
var batchIndexNamePattern = regexp.MustCompile(`^(research-index|kickoff)-`)

// isBatchIndexMOC returns true when the target MOC is a batch-provenance
// index that should not be used as a moc-append target. Detection rules,
// either of which is sufficient:
//   - filename matches batchIndexNamePattern (case-insensitive)
//   - existing frontmatter carries the "meta/index" tag
//
// A non-existent file with a non-matching name returns (false, nil) — that's
// a legitimate "create a new topical MOC" case. A read error other than
// not-exist surfaces; malformed frontmatter is treated as not-a-batch-index
// so the existing best-effort behavior is preserved.
func isBatchIndexMOC(mocPath string) (bool, error) {
	base := strings.ToLower(filepath.Base(mocPath))
	if batchIndexNamePattern.MatchString(base) {
		return true, nil
	}
	data, err := os.ReadFile(mocPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	fm, _, parseErr := markdown.ParseFrontmatter(data)
	if parseErr != nil {
		return false, nil
	}
	return fm.HasTag("meta/index"), nil
}
