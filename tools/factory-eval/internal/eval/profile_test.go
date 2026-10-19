package eval

import (
	"slices"
	"strings"
	"testing"
)

// ruleMatches reports whether a Bash permission rule matches cmd, following
// Claude Code's documented glob semantics: a rule with no `*` matches the
// whole command exactly; otherwise the rule is a whole-command glob where
// each `*` matches any text, spaces included. A trailing `:*` also matches
// the bare command, and so does a trailing ` *` when it is the rule's only
// wildcard. Rules for other tools never match.
func ruleMatches(rule, cmd string) bool {
	body, ok := strings.CutPrefix(rule, "Bash(")
	if !ok {
		return false
	}
	body = strings.TrimSuffix(body, ")")
	if bare, ok := strings.CutSuffix(body, ":*"); ok {
		return globMatches(bare, cmd) || globMatches(bare+" *", cmd)
	}
	if bare, ok := strings.CutSuffix(body, " *"); ok && !strings.Contains(bare, "*") &&
		cmd == bare {
		return true
	}
	return globMatches(body, cmd)
}

// globMatches matches s against pattern, where `*` is any text.
func globMatches(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	rest, ok := strings.CutPrefix(s, parts[0])
	if !ok {
		return false
	}
	last := parts[len(parts)-1]
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	return strings.HasSuffix(rest, last)
}

func TestRuleMatchesFollowsTheDocumentedGlob(t *testing.T) {
	tests := []struct {
		rule, cmd string
		want      bool
	}{
		{"Bash(git status)", "git status", true},
		{"Bash(git status)", "git status -s", false},
		{"Bash(git:*)", "git", true},
		{"Bash(git:*)", "git log", true},
		{"Bash(git:*)", "gitk", false},
		{"Bash(git *)", "git", true},
		{"Bash(git -C * push *)", "git -C /wt push", false},
		{"Bash(git -C * push)", "git -C /wt push", true},
		{"Bash(git push*)", "git pushx", true},
		{"Bash(make -C * test*)", "make -C /wt test", true},
		{"Read", "git status", false},
	}
	for _, tt := range tests {
		if got := ruleMatches(tt.rule, tt.cmd); got != tt.want {
			t.Errorf("ruleMatches(%q, %q) = %v, want %v", tt.rule, tt.cmd, got, tt.want)
		}
	}
}

// TestDenyRulesLeaveWorkerCommitsAlone: the deny list must stop the remote,
// config and --output forms, with or without -C, and must not deny a
// worker's commit whose message only contains such a word inside another
// one. The factory arm's workers run `git -C <worktree> <subcommand>` and
// never retry a denied call, so a false match costs the factory arm a case.
func TestDenyRulesLeaveWorkerCommitsAlone(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/src/github.com/mad01/thismoon"}
	perms := armPermissions(ArmFactory, iso, testExclusions)
	const gitC = "git -C /private/var/folders/xy/T/factory-eval-1/clone "
	const suspenders = `"docs(suspenders): fold the configuration section into How it works"`
	allowed := []string{
		gitC + "commit -m " + suspenders,
		gitC + `commit -m "docs(speak): say how it fetches voices"`,
		gitC + `commit -m "docs(belt): say which hooks pushed the hint"`,
		gitC + `commit -m "docs(csl): explain pulling stale repos"`,
		gitC + `commit -m "docs(ralph): list the remotes a recipe source uses"`,
		gitC + `add "tools/belt/config/defaults.go"`,
		gitC + `commit -m "docs: move tools/belt/config/README.md"`,
		gitC + "diff --stat",
		"git commit -m " + suspenders,
	}
	for _, cmd := range allowed {
		matches := func(r string) bool { return ruleMatches(r, cmd) }
		if i := slices.IndexFunc(perms.Deny, matches); i >= 0 {
			t.Errorf("%s\nis denied by %s", cmd, perms.Deny[i])
		}
		if !slices.ContainsFunc(perms.Allow, matches) {
			t.Errorf("%s\nmatches no allow rule, so the matcher or the allow list is wrong", cmd)
		}
	}
	deniedCmds := []string{
		"git -C /wt push", "git -C /wt push origin x", "git -C /wt fetch",
		"git -C /wt -c k=v push", "git -C /wt pull --rebase", "git -C /wt clone /src /dst",
		"git -C /wt ls-remote", "git -C /wt submodule update", "git -C /wt remote -v",
		"git -C /wt config user.name", "git -C /wt diff --output=/tmp/x",
		"git -C /wt log --output /tmp/x", "git push origin main", "git fetch",
		"git config --get user.email",
	}
	for _, cmd := range deniedCmds {
		if !slices.ContainsFunc(perms.Deny, func(r string) bool { return ruleMatches(r, cmd) }) {
			t.Errorf("%s matches no deny rule", cmd)
		}
	}
}
