package hint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// kof-consult's caps. The first block a session gets carries more because
// the session has no kof context yet; later prompts are follow-ups inside a
// task, so they get the tighter cap kof-assertions uses. Past a handful a
// block stops being read (docs/adr/0008).
const (
	maxConsultFirst = 5
	maxConsultLater = 3
)

// minConsultPrompt is the shortest prompt, in characters, worth ranking
// against. Shorter ones ("yes", "continue", "go ahead") carry no topic.
const minConsultPrompt = 12

// consultGitBudget bounds kof-consult's one git exec. The hint runs on every
// prompt, so a stalled git must cost no more than the kof query's budget.
const consultGitBudget = 300 * time.Millisecond

// consultShownMarker records in the seen file that kof-consult has spoken
// this session, which moves it from the first cap to the later one.
const consultShownMarker = "kof-consult-shown"

// KofConsult surfaces the stored assertions about the repo a session works
// in that the prompt just submitted is about. The search hint
// (kof-assertions) covers code a session already went looking at; this one
// covers the consult that should happen as a task is stated, before any
// searching: prior sessions' conclusions about what the prompt names.
type KofConsult struct {
	cfg  config.Config
	base string // kof serve base URL; overridable for tests
	// originURL resolves a directory to its git origin remote URL, the one
	// git exec a check makes: the kof org/name and the canonical identity
	// exclude_repos matches are both read from it. Overridable for tests.
	originURL func(dir string) string
}

func NewKofConsult(cfg config.Config) *KofConsult {
	return &KofConsult{cfg: cfg, base: kofBaseURL(), originURL: boundedOriginURL}
}

func (h *KofConsult) ID() string    { return "kof-consult" }
func (h *KofConsult) Event() string { return EventPrompt }

func (h *KofConsult) Check(in Input) *Advice {
	prompt := strings.TrimSpace(in.Prompt)
	if utf8.RuneCountInString(prompt) < minConsultPrompt {
		return nil
	}
	origin := h.originURL(nearestDir(in.Cwd))
	repo := repoFromOrigin(origin)
	if repo == "" {
		return nil
	}
	if h.cfg.HintRepoExcluded(h.ID(), gitloc.CanonicalRepo(origin)) {
		return nil
	}
	ranked := rankByPrompt(prompt, repo, matchRepo(repo, queryKof(h.base, "repo:"+repo)))
	if len(ranked) == 0 {
		return nil
	}
	path := seen.path(in.SessionID)
	spoken := path != "" && seen.load(path)[consultShownMarker]
	limit := maxConsultFirst
	if spoken {
		limit = maxConsultLater
	}
	shown := seen.take(in.SessionID, ranked, limit)
	if len(shown) == 0 {
		return nil
	}
	if path != "" && !spoken {
		seen.record(path, []string{consultShownMarker})
	}
	return &Advice{Hint: h.ID(), Text: render("repo:"+repo, shown), Tags: kofTags(shown)}
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
	return gitLine(dir, "remote", "get-url", "origin")
}

// boundedOriginURL is gitOriginURL under consultGitBudget: "" when git does
// not answer in time, which the hint reads as "not in a repo".
func boundedOriginURL(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), consultGitBudget)
	defer cancel()
	return gitloc.Output(ctx, dir, "remote", "get-url", "origin")
}

// gitTopLevel shells out to git; "" when dir is outside a repo working tree.
func gitTopLevel(dir string) string {
	return gitLine(dir, "rev-parse", "--show-toplevel")
}

func gitLine(dir string, args ...string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
