package guard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// WriteInternalNamesID identifies the write-time internal-name firewall.
const WriteInternalNamesID = "write-internal-names"

// WriteInternalNames blocks Write/Edit content that references internal
// org/repo names when the target file lives in a public (github.com) repo.
// Only github.com remotes count as public — other git hosts and files
// outside any repo are exempt. The name list comes from the suspenders guard
// config — the same source the pre-commit guard uses — plus the top-level
// directory names under each configured workspace dir.
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
	for _, excl := range g.cfg.Guards[WriteInternalNamesID].ExcludePaths {
		if excl != "" && strings.Contains(in.FilePath, excl) {
			return nil
		}
	}
	remote := g.remoteURL(nearestExistingDir(in.FilePath))
	if !isPublicRemote(remote) {
		return nil
	}
	// Repos allowlisted by canonical host/owner/repo may carry internal names
	// even though they live on github.com — a private companion repo whose
	// whole purpose is internal-only config. Matched by remote identity, not a
	// fragile path substring, so both the working checkout and any cached
	// clone (same origin) are covered.
	if repo := canonicalRepo(remote); repo != "" &&
		slices.Contains(g.cfg.Guards[WriteInternalNamesID].AllowRepos, repo) {
		return nil
	}
	names := g.blockedNames()
	var hits []string
	for _, name := range names {
		if matchWord(in.Content, name) {
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

// blockedNames merges configured blocked words with workspace-derived repo
// names, minus the allowlist.
func (g *WriteInternalNames) blockedNames() []string {
	allow := map[string]bool{}
	for _, a := range g.cfg.Suspenders.Allowlist {
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
	for _, w := range g.cfg.Suspenders.BlockedWords {
		add(w)
	}
	for _, dir := range g.cfg.Suspenders.WorkspaceDirs {
		for _, entry := range topLevelDirs(expandHome(dir)) {
			add(entry)
		}
	}
	return names
}

// isPublicRemote treats exactly github.com as public. All other hosts
// and missing remotes are exempt — the firewall only exists
// to keep internal names out of github.com repos.
func isPublicRemote(remote string) bool {
	return strings.Contains(remote, "github.com")
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

func topLevelDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// gitRemoteURL shells out to git; "" when dir is outside a repo or the repo
// has no origin remote.
func gitRemoteURL(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
