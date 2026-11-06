package vault

import (
	"sort"
	"strings"
	"unicode"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// NormalizeTitle returns the form two titles are compared in for the
// duplicate check: lowercased, with every run of characters other than
// letters, digits, + and # turned into one space and the ends trimmed. So
// "Kubernetes: Upgrades" and "kubernetes-upgrades" match, while "C++" and
// "C#" stay apart.
func NormalizeTitle(title string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '#' {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
			continue
		}
		space = true
	}
	return b.String()
}

// TitleMatch is a note whose title or one of whose aliases reads the same
// as another title once normalized.
type TitleMatch struct {
	Ref Ref
	// Title is the note's title.
	Title string
	// Alias is the alias that matched, empty when the title did.
	Alias string
}

// SameTitle returns the tier notes of the set whose normalized title or
// alias equals title's, vault by vault in set order and within a vault in
// the resolver's tier order (notes, MOCs, outputs, inbox). A revoked note is
// left out: writing its replacement is no duplicate. A title that
// normalizes to nothing matches no note.
func (s *Set) SameTitle(title string) ([]TitleMatch, error) {
	want := NormalizeTitle(title)
	if want == "" {
		return nil, nil
	}
	var out []TitleMatch
	for _, v := range s.vaults {
		var found []TitleMatch
		err := v.walkTierNotes(func(path string, data []byte) error {
			fm, _, err := markdown.ParseFrontmatter(data)
			if err != nil || lifecycle.Revoked(fm.Status) {
				return nil //nolint:nilerr // a note that does not parse matches nothing
			}
			if m, ok := titleMatch(fm, want); ok {
				m.Ref = Ref{Vault: v, Path: v.Rel(path)}
				found = append(found, m)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.SliceStable(found, func(i, j int) bool {
			return tierRank(found[i].Ref.Path) < tierRank(found[j].Ref.Path)
		})
		out = append(out, found...)
	}
	return out, nil
}

// titleMatch reports whether fm's title or an alias normalizes to want.
func titleMatch(fm markdown.Frontmatter, want string) (TitleMatch, bool) {
	if NormalizeTitle(fm.Title) == want {
		return TitleMatch{Title: fm.Title}, true
	}
	for _, a := range fm.Aliases {
		if NormalizeTitle(a) == want {
			return TitleMatch{Title: fm.Title, Alias: a}, true
		}
	}
	return TitleMatch{}, false
}
