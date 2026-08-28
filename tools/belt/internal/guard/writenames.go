package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// WriteInternalNamesID identifies the write-time internal-name firewall.
const WriteInternalNamesID = "write-internal-names"

// WriteInternalNames blocks Write/Edit content that references internal
// org/repo names when the target file lives in a public (github.com) repo.
// Only github.com remotes count as public — other git hosts and files
// outside any repo are exempt. The name list comes from the internal_names
// section of the belt config, falling back to the guard: section of the
// suspenders config when belt does not set one.
type WriteInternalNames struct {
	cfg config.Config
	// remoteURL returns the origin URL for the repo containing dir, or ""
	// when there is no repo or no remote. Injectable for tests.
	remoteURL func(dir string) string
}

// NewWriteInternalNames builds the guard with the real git-backed remote lookup.
func NewWriteInternalNames(cfg config.Config) *WriteInternalNames {
	return &WriteInternalNames{cfg: cfg, remoteURL: gitRemoteURL}
}

func (g *WriteInternalNames) ID() string    { return WriteInternalNamesID }
func (g *WriteInternalNames) Event() string { return EventWrite }

// Check denies when content headed for a public-repo file mentions an
// internal name.
func (g *WriteInternalNames) Check(in Input) *Denial {
	if in.FilePath == "" || in.Content == "" {
		return nil
	}
	if g.cfg.Guards[WriteInternalNamesID].ExcludesPath(in.FilePath) {
		return nil
	}
	remote := g.remoteURL(nearestExistingDir(in.FilePath))
	if !isPublicRemote(remote) {
		return nil
	}
	// Repos allowlisted by canonical host/owner/repo may carry internal names
	// even though they live on github.com — a private companion repo whose
	// whole purpose is internal-only config. Matched by remote identity, not a
	// fragile path substring, so both the working checkout and any cached
	// clone (same origin) are covered. allow_repos_by_profile entries apply
	// only on machines carrying that profile.
	if g.cfg.RepoAllowed(WriteInternalNamesID, canonicalRepo(remote)) {
		return nil
	}
	names := BlockedNames(g.cfg.Names)
	content := stripAllowedPhrases(in.Content, g.cfg.Names.AllowPhrases)
	var hits []string
	for _, name := range names {
		if matchWord(content, name) {
			hits = append(hits, name)
		}
	}
	if len(hits) == 0 {
		return nil
	}
	if len(hits) > 5 {
		hits = append(hits[:5], fmt.Sprintf("+%d more", len(hits)-5))
	}
	return Reasonf(WriteInternalNamesID,
		"content for %s references internal names (%s) but the file is in a public repo (%s). "+
			"Internal org/repo/tool names must never land in public repos — rephrase or drop them before writing.",
		in.FilePath, strings.Join(hits, ", "), remote)
}

// BlockedNames resolves the name set the write-internal-names guard matches:
// configured blocked words plus, for every git repo under the workspace
// dirs, the org and repo segments of its origin remote (falling back to the
// filesystem path) and the checkout dir basename — the same derivation the
// suspenders pre-commit guard uses, so a nested checkout like
// ~/workspace/foo/bar contributes "foo" and "bar" as separate names. The
// result is lowercased and deduplicated, minus allowlisted names and
// anything shorter than three characters. Exported so `belt doctor` shows
// exactly the set the guard uses.
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

// isPublicRemote treats exactly github.com as public. All other hosts
// and missing remotes are exempt — the firewall only exists
// to keep internal names out of github.com repos.
func isPublicRemote(remote string) bool {
	return strings.Contains(remote, "github.com")
}

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

// nearestExistingDir walks up from the target file to the closest directory
// that exists. A Write may create its parent directories, so the file's own
// dir can be absent — running git there would fail and silently exempt the
// write from the public-repo check.
func nearestExistingDir(path string) string {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}
