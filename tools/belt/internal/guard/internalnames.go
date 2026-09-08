package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// nameMatcher is the resolved blocked-name set plus the sanctioned phrases
// neutralized before matching. write-internal-names and
// publish-internal-names build one from the same internal_names section, so
// the two firewalls agree on what an internal name is by construction.
type nameMatcher struct {
	names   []string
	phrases []string
}

// newNameMatcher derives the blocked-name set from the internal_names
// config. Deriving walks the workspace dirs, so callers build a matcher only
// once they know there is text headed for a public repo.
func newNameMatcher(s config.InternalNames) nameMatcher {
	return nameMatcher{names: BlockedNames(s), phrases: s.AllowPhrases}
}

// empty reports whether the matcher has nothing to match: no name source
// configured anywhere, the one way these guards fail open.
func (m nameMatcher) empty() bool { return len(m.names) == 0 }

// hits returns the blocked names found in content as whole words,
// case-insensitively, after the allow phrases are blanked out. Order follows
// the name set so a deny reason is stable across runs.
func (m nameMatcher) hits(content string) []string {
	if content == "" {
		return nil
	}
	content = stripAllowedPhrases(content, m.phrases)
	var hits []string
	for _, name := range m.names {
		if matchWord(content, name) {
			hits = append(hits, name)
		}
	}
	return hits
}

// maxListedHits caps how many names a deny reason spells out; the rest
// collapse into a count so a reason stays one readable line.
const maxListedHits = 5

// summarizeHits renders a hit list for a deny reason.
func summarizeHits(hits []string) string {
	if len(hits) > maxListedHits {
		hits = append(hits[:maxListedHits], fmt.Sprintf("+%d more", len(hits)-maxListedHits))
	}
	return strings.Join(hits, ", ")
}

// BlockedNames resolves the name set the internal-name guards match:
// configured blocked words plus, for every git repo under the workspace
// dirs, the org and repo segments of its origin remote (falling back to the
// filesystem path) and the checkout dir basename — the same derivation the
// suspenders pre-commit guard uses, so a nested checkout like
// ~/workspace/foo/bar contributes "foo" and "bar" as separate names. The
// result is lowercased and deduplicated, minus allowlisted names and
// anything shorter than three characters. Exported so `belt doctor` shows
// exactly the set the guards use.
func BlockedNames(s config.InternalNames) []string {
	allow := map[string]bool{}
	for _, a := range s.Allowlist {
		allow[strings.ToLower(a)] = true
		allow[strings.ToLower(filepath.Base(a))] = true
	}
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || len(name) < 3 || allow[name] || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, w := range s.BlockedWords {
		add(w)
	}
	// A hook must not break tool calls, so a partial walk (unreadable dir)
	// still contributes whatever repos it found.
	repos, _ := repofind.Find(s.WorkspaceDirs, nil)
	for _, r := range repos {
		for seg := range strings.SplitSeq(r.Name, "/") {
			add(seg)
		}
		add(filepath.Base(r.Path))
	}
	return names
}

// Which repos are public-bound is config.PublicBound's call: the
// public_repos list when the rendering carries one, else every github.com
// repo. Both guards ask it with a canonical host/owner/repo identity, so
// an org that is internal yet hosted on github.com is handled in one place.

// stripAllowedPhrases blanks case-insensitive occurrences of each allowed
// phrase before name matching, so a sanctioned compound that contains a
// blocked name passes while the bare name elsewhere in the same content
// still denies. The replacement is a space, keeping word boundaries intact
// for the surrounding text.
func stripAllowedPhrases(content string, phrases []string) string {
	for _, p := range phrases {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(p))
		content = re.ReplaceAllString(content, " ")
	}
	return content
}

// matchWord reports a case-insensitive whole-word match of name in content.
func matchWord(content, name string) bool {
	re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
	if err != nil {
		return false
	}
	return re.MatchString(content)
}
