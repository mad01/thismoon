package hint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// maxConsultAssertions caps the session-start block. It is looser than the
// per-search cap because it fires once per session, but past a handful the
// block stops being read (docs/adr/0008).
const maxConsultAssertions = 5

// KeepConsult surfaces a repo's stored assertions when a session starts in
// that repo. The search hint (keep-assertions) covers code a session already
// went looking at; this one covers the consult that should happen before any
// searching — prior sessions' conclusions about the repo the session opened
// in.
type KeepConsult struct {
	cfg  config.Config
	base string // kof serve base URL; overridable for tests
	// originURL resolves a directory to its git origin remote URL;
	// overridable for tests.
	originURL func(dir string) string
}

func NewKeepConsult(cfg config.Config) *KeepConsult {
	return &KeepConsult{cfg: cfg, base: keepBaseURL(), originURL: gitOriginURL}
}

func (h *KeepConsult) ID() string    { return "keep-consult" }
func (h *KeepConsult) Event() string { return EventSessionStart }

func (h *KeepConsult) Check(in Input) *Advice {
	repo := repoFromOrigin(h.originURL(nearestDir(in.Cwd)))
	if repo == "" {
		return nil
	}
	found := matchRepo(repo, queryKeep(h.base, "repo:"+repo))
	if len(found) > maxConsultAssertions {
		found = found[:maxConsultAssertions]
	}
	fresh := seen.filter(in.SessionID, found)
	if len(fresh) == 0 {
		return nil
	}
	return &Advice{Hint: h.ID(), Text: render("repo:"+repo, fresh)}
}

// matchRepo keeps assertions whose subject is the repo itself or sits under
// it. The prefix query alone would also match sibling repos sharing the name
// as a prefix (`mad01/thismoon` vs `mad01/thismoon-arcade`), so the boundary
// after the repo name must be checked, not just the prefix.
func matchRepo(repo string, as []assertion) []assertion {
	exact := "repo:" + repo
	var out []assertion
	for _, a := range as {
		s := strings.ToLower(a.Subject)
		if s == exact || strings.HasPrefix(s, exact+"/") {
			out = append(out, a)
		}
	}
	return out
}

// repoFromOrigin reduces a git remote URL to the lowercased org/name kof
// subjects are keyed by: `git@github.com:mad01/thismoon.git` and
// `https://github.com/mad01/thismoon.git` both yield `mad01/thismoon`.
func repoFromOrigin(url string) string {
	url = strings.TrimSuffix(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(strings.ReplaceAll(url, ":", "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	org, name := parts[len(parts)-2], parts[len(parts)-1]
	// An org segment containing a dot or @ is a host, so the URL had no org
	// (e.g. https://host/name) and names nothing kof subjects are keyed by.
	if org == "" || name == "" || strings.ContainsAny(org, ".@") {
		return ""
	}
	return strings.ToLower(org + "/" + name)
}

// nearestDir walks up from the session cwd to the closest directory that
// exists, mirroring the write-internal-names guard: a stale cwd must degrade
// to silence, not an exec error.
func nearestDir(path string) string {
	dir := path
	for dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
	return dir
}

// gitOriginURL shells out to git; "" when dir is outside a repo or the repo
// has no origin remote.
func gitOriginURL(dir string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
