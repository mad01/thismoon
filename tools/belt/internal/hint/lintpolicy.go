package hint

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// LintPolicyID identifies the lint-policy hint.
const LintPolicyID = "lint-policy"

// LintPolicy relays a repo's own lint/format policy right after a `git
// commit` there. The hint carries no language table and runs no linter: a
// repo opts in by declaring a hints.lint-policy.message line in its root
// .belt.yaml (docs/adr/0012), and that message is the whole advice — the
// repo knows its toolchain, belt only knows the moment. Repos without a
// declared message stay silent, so the hint never speaks without a policy
// behind it.
type LintPolicy struct {
	cfg config.Config
	// resolveRepo returns the canonical host/owner/repo identity of the
	// repo at dir, or "" when it has no resolvable origin. Injectable for
	// tests.
	resolveRepo func(dir string) string
	// resolveRoot returns the working-tree root of the repo at dir, where
	// the .belt.yaml overlay lives, or "" outside a repo. Injectable for
	// tests.
	resolveRoot func(dir string) string
}

// NewLintPolicy builds the hint with the real git-backed resolvers.
func NewLintPolicy(cfg config.Config) *LintPolicy {
	return &LintPolicy{
		cfg:         cfg,
		resolveRepo: guard.CanonicalRepoAt,
		resolveRoot: gitTopLevel,
	}
}

func (h *LintPolicy) ID() string    { return LintPolicyID }
func (h *LintPolicy) Event() string { return EventBash }

// Check fires at most once per session per repo, when a `git commit` runs in
// a repo whose overlay declares a lint-policy message. A trigger command that
// itself names the toolchain (`make lint && git commit`) is skipped without
// spending the session's nudge, so a later bare commit still draws it.
func (h *LintPolicy) Check(in Input) *Advice {
	if in.Command == "" {
		return nil
	}
	if mentionsLintTooling(in.Command) {
		return nil
	}
	// Without a session id the once-per-session cap is impossible, and
	// advice on every commit is worse than none.
	path := seen.path(in.SessionID)
	if path == "" {
		return nil
	}
	for _, dir := range guard.GitCommitDirs(in.Command) {
		if dir == "" {
			dir = in.Cwd
		}
		overlay, err := loadRepoOverlay(h.resolveRoot(dir))
		if err != nil {
			// Fail visible, never deny: a broken repo file draws one
			// line on every commit there until it is fixed.
			return &Advice{Hint: LintPolicyID, Text: fmt.Sprintf(
				"could not evaluate this repo's lint policy: %v — fix or remove the file.", err)}
		}
		local := overlay.Hints[LintPolicyID]
		if local.Message == "" {
			continue // no declared policy, nothing to relay
		}
		repo := h.resolveRepo(dir)
		if repo == "" {
			continue
		}
		if local.Exclude != nil {
			if *local.Exclude {
				continue
			}
		} else if h.cfg.HintRepoExcluded(LintPolicyID, repo) {
			continue
		}
		marker := LintPolicyID + ":" + repo
		if seen.load(path)[marker] {
			continue
		}
		seen.record(path, []string{marker})
		return &Advice{Hint: LintPolicyID, Text: fmt.Sprintf(
			"this commit landed in %s, which declares a lint/format policy: %s "+
				"If that has not run against these changes, run it before pushing. "+
				"This reminder fires once per session.", repo, local.Message)}
	}
	return nil
}

// mentionsLintTooling reports whether the command line itself runs or names
// the fmt/lint toolchain outside quoted strings — `make lint && git commit`
// needs no reminder about what it just did. Quoted spans are dropped first
// so a commit message mentioning "lint" does not count as having run it.
func mentionsLintTooling(command string) bool {
	bare := strings.ToLower(stripQuoted(command))
	return strings.Contains(bare, "lint") || strings.Contains(bare, "fmt")
}

// stripQuoted removes single- and double-quoted spans from a shell command,
// keeping only the bare tokens. An unterminated quote drops the rest of the
// string, which errs toward suppression — the safe direction for a hint.
func stripQuoted(s string) string {
	var b strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
