// Package vault knows the layout of a brain vault: the three tiers
// (inbox/notes/outputs) plus MOCs, how to resolve a path to a tier, how to
// generate safe filenames, and how to promote a note from inbox to notes
// (atomic move + frontmatter update + backlink rewrite).
package vault

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Tier enumerates the directory tiers in a brain vault.
type Tier string

const (
	TierInbox   Tier = "inbox"
	TierNotes   Tier = "notes"
	TierOutputs Tier = "outputs"
	TierMOCs    Tier = "MOCs"
)

// AllTiers is the canonical iteration order (raw → validated → output → maps).
var AllTiers = []Tier{TierInbox, TierNotes, TierOutputs, TierMOCs}

// ParseTier normalizes a tier name. Case-insensitive. Accepts "moc" / "mocs"
// as aliases for MOCs. Returns an error wrapping ErrUnknownTier for unknown
// tiers.
func ParseTier(name string) (Tier, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "inbox":
		return TierInbox, nil
	case "notes":
		return TierNotes, nil
	case "outputs", "output":
		return TierOutputs, nil
	case "mocs", "moc":
		return TierMOCs, nil
	}
	return "", fmt.Errorf("%w %q (expected inbox, notes, outputs, or MOCs)", ErrUnknownTier, name)
}

// Dir returns the tier's directory name relative to the vault root.
func (t Tier) Dir() string { return string(t) }

// TierOf returns the tier a note belongs to based on its path relative to the
// vault root. Returns an error if the path is outside the known tiers.
func TierOf(vaultRoot, notePath string) (Tier, error) {
	rel, err := relativeToVault(vaultRoot, notePath)
	if err != nil {
		return "", err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return "", fmt.Errorf("path %q has no tier component", notePath)
	}
	for _, t := range AllTiers {
		if parts[0] == t.Dir() {
			return t, nil
		}
	}
	return "", fmt.Errorf("path %q is not under a known tier", rel)
}

// relativeToVault returns the note path relative to the vault root, resolving
// symlinks via filepath.Clean. Returns an error if the note is outside the vault.
func relativeToVault(vaultRoot, notePath string) (string, error) {
	absVault, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", fmt.Errorf("abs vault: %w", err)
	}
	absNote, err := filepath.Abs(notePath)
	if err != nil {
		return "", fmt.Errorf("abs note: %w", err)
	}
	rel, err := filepath.Rel(absVault, absNote)
	if err != nil {
		return "", fmt.Errorf("rel: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("note %q is outside vault %q", notePath, vaultRoot)
	}
	return rel, nil
}

// Slug converts a title into a safe filename stem: lowercased, with
// letters (any script), digits and the marks that combine with a letter
// kept, and every run of other characters collapsed into one hyphen. A
// title with no letter or digit slugs to "", and the caller names the file
// by the note's id instead.
func Slug(title string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r) && !lastDash:
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// stemFor is the stem a note titled title gets: its slug, else its id
// lowercased.
func stemFor(title, id string) string {
	if s := Slug(title); s != "" {
		return s
	}
	return strings.ToLower(id)
}
