package guard

import (
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// WriteInternalNamesID identifies the write-time internal-name firewall.
const WriteInternalNamesID = "write-internal-names"

// WriteInternalNames blocks Write/Edit content that references internal
// org/repo names when the target file lives in a public-bound repo: one on
// the config's public_repos list, or, when the rendering has no such list,
// any github.com repo. Files outside any repo are exempt. The name list
// comes from the internal_names section of the belt config (see
// BlockedNames); the publish-internal-names guard matches the same set on
// the way out to a public remote.
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
	repo := canonicalRepo(remote)
	if !g.cfg.PublicBound(repo) {
		return nil
	}
	// Repos allowlisted by canonical host/owner/repo may carry internal names
	// even though they are public-bound by list or host — a private
	// companion repo inside a listed org, say. Matched by remote identity,
	// not a fragile path substring, so both the working checkout and any
	// cached clone (same origin) are covered.
	if g.cfg.RepoAllowed(WriteInternalNamesID, repo) {
		return nil
	}
	hits := newNameMatcher(g.cfg.Names).hits(in.Content)
	if len(hits) == 0 {
		return nil
	}
	return Reasonf(WriteInternalNamesID,
		"content for %s references internal names (%s) but the file is in a public repo (%s). "+
			"Internal org/repo/tool names must never land in public repos — rephrase or drop them before writing.",
		in.FilePath, summarizeHits(hits), remote)
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
