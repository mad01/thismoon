package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

const (
	publicOrigin   = "git@github.com:acme/public.git"
	internalOrigin = "git@git.internal.example:org/repo.git"
)

// fakeGit answers git calls from a map keyed "<dir>|<args>" or, for any
// dir, "*|<args>". Unknown calls return "" like a failing git would.
func fakeGit(answers map[string]string) func(dir string, args ...string) string {
	return func(dir string, args ...string) string {
		key := strings.Join(args, " ")
		if v, ok := answers[dir+"|"+key]; ok {
			return v
		}
		return answers["*|"+key]
	}
}

type publishFixture struct {
	git   map[string]string
	files map[string]string
	cfg   config.Config
}

func newPublishGuard(event string, f publishFixture) *PublishInternalNames {
	cfg := f.cfg
	if cfg.Names.BlockedWords == nil {
		cfg.Names.BlockedWords = []string{"internalco"}
	}
	g := NewPublishInternalNames(cfg, event)
	g.git = fakeGit(f.git)
	g.readFile = func(path string) (string, error) {
		if s, ok := f.files[path]; ok {
			return s, nil
		}
		return "", os.ErrNotExist
	}
	g.emit = func(string, string, string, string, map[string]string) {}
	return g
}

func TestPublishInternalNamesBash(t *testing.T) {
	publicRepo := map[string]string{"*|remote get-url origin": publicOrigin}
	internalRepo := map[string]string{"*|remote get-url origin": internalOrigin}
	tests := []struct {
		name       string
		command    string
		cwd        string
		git        map[string]string
		files      map[string]string
		wantDeny   bool
		wantReason string
	}{
		// gh
		{
			name: "gh pr create body", command: `gh pr create --title "t" --body "uses internalco"`,
			git: publicRepo, wantDeny: true, wantReason: "gh pr create",
		},
		{name: "gh pr view reads", command: "gh pr view 12", git: publicRepo},
		{
			name:    "gh pr create -R internal host",
			command: "gh pr create -R git.internal.example/org/repo --body internalco",
			git:     publicRepo,
		},
		{
			name:    "gh pr create -R other public repo",
			command: "gh pr create -R acme/other --body internalco",
			git:     internalRepo, wantDeny: true, wantReason: "github.com/acme/other",
		},
		{
			name: "gh pr create body file", command: "gh pr create --title t --body-file body.md",
			cwd: "/repo", git: publicRepo, files: map[string]string{"/repo/body.md": "internalco inside"},
			wantDeny: true, wantReason: "file body.md",
		},
		{
			name: "gh api field", command: "gh api repos/acme/public/pulls -f body=internalco",
			git: publicRepo, wantDeny: true, wantReason: "gh api",
		},
		{
			name:    "gh api internal hostname",
			command: "gh api --hostname git.internal.example repos/x/y -f body=internalco",
			git:     publicRepo,
		},
		{
			name: "gh gist create outside a repo", command: "gh gist create notes.md",
			cwd: "/tmp", files: map[string]string{"/tmp/notes.md": "internalco notes"},
			wantDeny: true, wantReason: "gh gist create",
		},
		{
			name: "gh release create tag", command: `gh release create v1-internalco --notes "x"`,
			git: publicRepo, wantDeny: true, wantReason: "gh release create",
		},
		{
			name: "gh heredoc body",
			command: "gh pr create --title t --body \"$(cat <<'EOF'\n" +
				"mentions internalco here\nEOF\n)\"",
			git: publicRepo, wantDeny: true, wantReason: "command text",
		},
		{
			name: "gh in internal repo", command: "gh pr create --body internalco",
			git: internalRepo,
		},
		{
			name: "cd tracking", command: "cd /internal && gh pr create --body internalco",
			cwd: "/public",
			git: map[string]string{
				"/public|remote get-url origin":   publicOrigin,
				"/internal|remote get-url origin": internalOrigin,
			},
		},
		{
			name:    "quoted gh mention",
			command: `echo "gh pr create --body internalco"`,
			git:     publicRepo,
		},
		// git commit
		{
			name: "commit -m", command: `git commit -m "add internalco support"`,
			git: publicRepo, wantDeny: true, wantReason: "git commit",
		},
		{name: "commit -m clean", command: `git commit -m "fix typo"`, git: publicRepo},
		{name: "commit editor", command: "git commit", git: publicRepo},
		{
			name: "commit -F", command: "git commit -F /tmp/msg", git: publicRepo,
			files: map[string]string{"/tmp/msg": "internalco"}, wantDeny: true,
			wantReason: "message file /tmp/msg",
		},
		{name: "commit in internal repo", command: `git commit -m "internalco"`, git: internalRepo},
		{
			name: "commit -C dir", command: `git -C /internal commit -m "internalco"`, cwd: "/public",
			git: map[string]string{
				"/public|remote get-url origin":   publicOrigin,
				"/internal|remote get-url origin": internalOrigin,
			},
		},
		// branch and tag creation
		{
			name: "checkout -b", command: "git checkout -b feat/internalco-fix", git: publicRepo,
			wantDeny: true, wantReason: "branch name feat/internalco-fix",
		},
		{name: "checkout existing", command: "git checkout main", git: publicRepo},
		{
			name: "switch -c", command: "git switch -c internalco", git: publicRepo,
			wantDeny: true, wantReason: "git switch",
		},
		{
			name: "branch name", command: "git branch internalco", git: publicRepo,
			wantDeny: true, wantReason: "branch name internalco",
		},
		{name: "branch delete", command: "git branch -d internalco", git: publicRepo},
		{name: "branch list", command: "git branch", git: publicRepo},
		{
			name: "tag name", command: "git tag v1-internalco", git: publicRepo,
			wantDeny: true, wantReason: "git tag",
		},
		{name: "tag delete", command: "git tag -d v1-internalco", git: publicRepo},
		{
			name: "tag message", command: `git tag -a v2 -m "for internalco"`, git: publicRepo,
			wantDeny: true, wantReason: "git tag",
		},
		{
			name: "worktree add -b", command: "git worktree add -b internalco ../wt", git: publicRepo,
			wantDeny: true, wantReason: "branch name internalco",
		},
		// git push: ref names
		{
			name: "push branch name", command: "git push origin internalco-branch", git: publicRepo,
			wantDeny: true, wantReason: "branch name internalco-branch",
		},
		{
			name:    "push delete refspec",
			command: "git push origin :internalco-branch",
			git:     publicRepo,
		},
		{
			name:    "push --delete",
			command: "git push --delete origin internalco-branch",
			git:     publicRepo,
		},
		{
			name: "push HEAD resolves branch", command: "git push -u origin HEAD",
			git: map[string]string{
				"*|remote get-url origin":       publicOrigin,
				"*|rev-parse --abbrev-ref HEAD": "feat/internalco",
			},
			wantDeny: true, wantReason: "branch name feat/internalco",
		},
		{
			name: "push --tags", command: "git push --tags", git: map[string]string{
				"*|remote get-url origin":                            publicOrigin,
				"*|for-each-ref --format=%(refname:short) refs/tags": "v1\nv2-internalco",
			},
			wantDeny: true, wantReason: "tag name v2-internalco",
		},
		{
			name: "push --all", command: "git push --all origin", git: map[string]string{
				"*|remote get-url origin":                             publicOrigin,
				"*|for-each-ref --format=%(refname:short) refs/heads": "main\ninternalco",
			},
			wantDeny: true, wantReason: "branch name internalco",
		},
		{
			name: "push to internal remote", command: "git push upstream feat",
			git: map[string]string{"*|remote get-url upstream": internalOrigin},
		},
		// git push: outgoing commit messages
		{
			name: "push new branch scans commits since remote HEAD", command: "git push origin feat",
			git: map[string]string{
				"*|remote get-url origin":                               publicOrigin,
				"*|rev-parse --verify --quiet refs/remotes/origin/HEAD": "abc",
				"*|log -z --format=%h%n%B -n 200 refs/remotes/origin/HEAD..feat": "1a2b3c4\n" +
					"add internalco support\n\nlonger body\n\x00",
			},
			wantDeny: true, wantReason: "commit 1a2b3c4 message",
		},
		{
			name: "push clean commits", command: "git push origin feat",
			git: map[string]string{
				"*|remote get-url origin":                               publicOrigin,
				"*|rev-parse --verify --quiet refs/remotes/origin/HEAD": "abc",
				"*|log -z --format=%h%n%B -n 200 refs/remotes/origin/HEAD..feat": "1a2b3c4\n" +
					"fix typo\n\x00",
			},
		},
		{
			name: "bare push uses push upstream", command: "git push",
			git: map[string]string{
				"*|remote get-url origin":                               publicOrigin,
				"*|rev-parse --abbrev-ref --symbolic-full-name @{push}": "origin/feat",
				"*|rev-parse --verify --quiet refs/remotes/origin/feat": "def",
				"*|log -z --format=%h%n%B -n 200 refs/remotes/origin/feat..HEAD": "9f9f9f9\n" +
					"mention internalco\n\x00",
			},
			wantDeny: true, wantReason: "commit 9f9f9f9 message",
		},
		{
			name: "bare push without upstream checks branch name", command: "git push",
			git: map[string]string{
				"*|remote get-url origin":       publicOrigin,
				"*|rev-parse --abbrev-ref HEAD": "internalco-wip",
			},
			wantDeny: true, wantReason: "branch name internalco-wip",
		},
		{
			name: "push with no known base checks names only", command: "git push origin feat",
			git: publicRepo,
		},
		{name: "non-git command", command: "ls -la", git: publicRepo},
		{name: "empty command", command: "", git: publicRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newPublishGuard(EventBash, publishFixture{git: tt.git, files: tt.files})
			cwd := tt.cwd
			if cwd == "" {
				cwd = "/repo"
			}
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d == nil {
				return
			}
			if !strings.Contains(d.Reason, PublishInternalNamesID) {
				t.Errorf("reason missing guard id: %q", d.Reason)
			}
			if tt.wantReason != "" && !strings.Contains(d.Reason, tt.wantReason) {
				t.Errorf("reason %q does not mention %q", d.Reason, tt.wantReason)
			}
		})
	}
}

func TestPublishInternalNamesBashAllowRepos(t *testing.T) {
	cfg := config.Config{Guards: map[string]config.Toggle{
		PublishInternalNamesID: {AllowRepos: []string{"github.com/acme/companion"}},
	}}
	git := map[string]string{"*|remote get-url origin": "git@github.com:acme/companion.git"}
	g := newPublishGuard(EventBash, publishFixture{git: git, cfg: cfg})
	for _, command := range []string{
		`git commit -m "internalco"`,
		"git push origin internalco",
		"gh pr create --body internalco",
		"gh pr create -R acme/companion --body internalco",
	} {
		if d := g.Check(Input{Event: EventBash, Command: command, Cwd: "/repo"}); d != nil {
			t.Errorf("Check(%q) in an allowlisted repo denied: %s", command, d.Reason)
		}
	}
}

// TestPublishInternalNamesInternalOrgOnGitHub covers an org that is internal
// yet hosted on github.com: an allow_repos org wildcard must exempt every
// way of addressing its repos, including calls made outside any checkout.
// The org name itself is a derived blocked name, so every command below
// carries a hit that only the wildcard can excuse.
func TestPublishInternalNamesInternalOrgOnGitHub(t *testing.T) {
	cfg := config.Config{
		Names: config.InternalNames{BlockedWords: []string{"internalco", "internal-org"}},
		Guards: map[string]config.Toggle{
			PublishInternalNamesID: {AllowRepos: []string{"github.com/internal-org/*"}},
		},
	}
	inOrgRepo := map[string]string{"*|remote get-url origin": "git@github.com:internal-org/svc.git"}
	allowed := []struct {
		name    string
		command string
		git     map[string]string
	}{
		{"push in an org repo", "git push origin feat/internalco", inOrgRepo},
		{"commit in an org repo", `git commit -m "bump internalco"`, inOrgRepo},
		{"pr create in an org repo", "gh pr create --body internalco", inOrgRepo},
		{"pr create -R org repo", "gh pr create -R internal-org/svc --body internalco", nil},
		{
			"api endpoint outside a checkout",
			"gh api repos/internal-org/svc/pulls -f body=internalco",
			nil,
		},
		{
			"api full url",
			"gh api https://api.github.com/repos/internal-org/svc/issues -f title=internalco",
			nil,
		},
		{"repo create in the org", "gh repo create internal-org/internalco-tools --private", nil},
	}
	for _, tt := range allowed {
		t.Run(tt.name, func(t *testing.T) {
			g := newPublishGuard(EventBash, publishFixture{git: tt.git, cfg: cfg})
			if d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"}); d != nil {
				t.Errorf("wildcard did not exempt %q: %s", tt.command, d.Reason)
			}
		})
	}
	// The same calls against a repo outside the org still deny.
	g := newPublishGuard(EventBash, publishFixture{cfg: cfg})
	for _, command := range []string{
		"gh api repos/acme/public/pulls -f body=internalco",
		"gh repo create acme/internalco-tools --public",
	} {
		if d := g.Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"}); d == nil {
			t.Errorf("%q outside the org must deny", command)
		}
	}
	// And the MCP half honours the same wildcard.
	m := newPublishGuard(EventExternalText, publishFixture{cfg: cfg})
	if d := m.Check(Input{
		Event: EventExternalText, ToolName: "mcp__gh_com__create_pull_request",
		ToolInput: map[string]any{"owner": "internal-org", "repo": "svc", "body": "internalco"},
	}); d != nil {
		t.Errorf("MCP call into the org denied: %s", d.Reason)
	}
}

func TestGhNamedRepo(t *testing.T) {
	tests := map[string]string{
		"api repos/acme/public/pulls":                    "github.com/acme/public",
		"api /repos/acme/public":                         "github.com/acme/public",
		"api https://api.github.com/repos/acme/public/x": "github.com/acme/public",
		"api user":                       "",
		"api orgs/acme/repos":            "",
		"repo create acme/new --private": "github.com/acme/new",
		"repo create new --private":      "",
		"pr create --body x":             "",
	}
	for in, want := range tests {
		c, _ := parseGh(strings.Fields(in))
		if got := c.namedRepo(); got != want {
			t.Errorf("namedRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPublishInternalNamesBashAllowPhrases(t *testing.T) {
	cfg := config.Config{Names: config.InternalNames{
		BlockedWords: []string{"internalco"},
		AllowPhrases: []string{"internalco-companion"},
	}}
	git := map[string]string{"*|remote get-url origin": publicOrigin}
	g := newPublishGuard(EventBash, publishFixture{git: git, cfg: cfg})
	if d := g.Check(Input{
		Event: EventBash, Command: `git commit -m "sync from internalco-companion"`, Cwd: "/repo",
	}); d != nil {
		t.Errorf("sanctioned phrase denied: %s", d.Reason)
	}
	if d := g.Check(Input{
		Event: EventBash, Command: `git commit -m "internalco-companion and internalco"`, Cwd: "/repo",
	}); d == nil {
		t.Error("bare name beside the sanctioned phrase must still deny")
	}
}

func TestPublishInternalNamesSoftMode(t *testing.T) {
	soft := config.Config{Guards: map[string]config.Toggle{
		PublishInternalNamesID: {Mode: config.ModeSoft},
	}}
	git := map[string]string{"*|remote get-url origin": publicOrigin}
	g := newPublishGuard(EventBash, publishFixture{git: git, cfg: soft})
	var warned []string
	g.emit = func(_, level, _, message string, _ map[string]string) {
		warned = append(warned, level+": "+message)
	}
	d := g.Check(Input{Event: EventBash, Command: `git commit -m "internalco"`, Cwd: "/repo"})
	if d != nil {
		t.Fatalf("soft mode must allow, got %s", d.Reason)
	}
	if len(warned) != 1 || !strings.HasPrefix(warned[0], "warn: ") ||
		!strings.Contains(warned[0], "soft mode") {
		t.Errorf("soft mode must emit one warn event, got %v", warned)
	}
}

func TestPublishInternalNamesEmptyNameSetAllows(t *testing.T) {
	git := map[string]string{"*|remote get-url origin": publicOrigin}
	g := NewPublishInternalNames(config.Config{}, EventBash)
	g.git = fakeGit(git)
	if d := g.Check(Input{Event: EventBash, Command: `git commit -m "anything"`, Cwd: "/repo"}); d != nil {
		t.Errorf("no configured names must allow, got %s", d.Reason)
	}
}

func TestPublishInternalNamesTool(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		input      map[string]any
		cfg        config.Config
		wantDeny   bool
		wantReason string
	}{
		{
			name: "pull request body", tool: "mcp__gh_com__create_pull_request",
			input: map[string]any{
				"owner": "acme", "repo": "public", "title": "t", "body": "uses internalco",
				"head": "feat", "base": "main",
			},
			wantDeny: true, wantReason: "field body: internalco",
		},
		{
			name: "pull request head branch", tool: "mcp__gh_com__create_pull_request",
			input: map[string]any{
				"owner": "acme", "repo": "public", "title": "t", "head": "feat/internalco",
			},
			wantDeny: true, wantReason: "field head",
		},
		{
			name: "target repo named in reason", tool: "mcp__gh_com__add_issue_comment",
			input:    map[string]any{"owner": "acme", "repo": "public", "body": "internalco"},
			wantDeny: true, wantReason: "public repo github.com/acme/public",
		},
		{
			name: "read-only operation", tool: "mcp__gh_com__get_file_contents",
			input: map[string]any{"owner": "acme", "repo": "public", "path": "internalco.md"},
		},
		{
			name: "search operation", tool: "mcp__gh_com__search_code",
			input: map[string]any{"query": "internalco"},
		},
		{
			name: "allowlisted repo", tool: "mcp__gh_com__create_pull_request",
			input: map[string]any{"owner": "acme", "repo": "companion", "body": "internalco"},
			cfg: config.Config{Guards: map[string]config.Toggle{
				PublishInternalNamesID: {AllowRepos: []string{"github.com/acme/companion"}},
			}},
		},
		{
			name: "allow phrase", tool: "mcp__gh_com__create_pull_request",
			input: map[string]any{
				"owner": "acme",
				"repo":  "public",
				"body":  "see internalco-companion",
			},
			cfg: config.Config{Names: config.InternalNames{
				BlockedWords: []string{"internalco"},
				AllowPhrases: []string{"internalco-companion"},
			}},
		},
		{
			name: "labels array", tool: "mcp__gh_com__issue_write",
			input: map[string]any{
				"owner": "acme", "repo": "public", "method": "create", "title": "t",
				"labels": []any{"bug", "internalco"},
			},
			wantDeny: true, wantReason: "field labels",
		},
		{
			name: "nested push_files content", tool: "mcp__gh_com__push_files",
			input: map[string]any{
				"owner": "acme", "repo": "public", "branch": "feat", "message": "m",
				"files": []any{map[string]any{"path": "a.md", "content": "internalco"}},
			},
			wantDeny: true, wantReason: "field files",
		},
		{
			name: "new repository without owner", tool: "mcp__gh_com__create_repository",
			input:    map[string]any{"name": "internalco-tools", "private": false},
			wantDeny: true, wantReason: "to a public repo",
		},
		{
			name: "clean call", tool: "mcp__gh_com__create_branch",
			input: map[string]any{"owner": "acme", "repo": "public", "branch": "feat/clean"},
		},
		{name: "non-mcp tool", tool: "Bash", input: map[string]any{"command": "internalco"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newPublishGuard(EventExternalText, publishFixture{cfg: tt.cfg})
			d := g.Check(Input{Event: EventExternalText, ToolName: tt.tool, ToolInput: tt.input})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%s) denial = %v, wantDeny %v", tt.tool, d, tt.wantDeny)
			}
			if d != nil && tt.wantReason != "" && !strings.Contains(d.Reason, tt.wantReason) {
				t.Errorf("reason %q does not mention %q", d.Reason, tt.wantReason)
			}
		})
	}
}

// TestPublishInternalNamesRealGitPush exercises the push base and outgoing
// log resolution against a real repository: one commit on main mirrored as
// the remote HEAD, one new commit on a feature branch whose message carries
// the blocked name.
func TestPublishInternalNamesRealGitPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "init")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	run("checkout", "-q", "-b", "feat")
	run("commit", "-q", "--allow-empty", "-m", "add internalco support")
	sha := run("rev-parse", "--short", "HEAD")
	run("remote", "add", "origin", "https://github.com/acme/public.git")

	cfg := config.Config{Names: config.InternalNames{BlockedWords: []string{"internalco"}}}
	g := NewPublishInternalNames(cfg, EventBash)
	g.emit = func(string, string, string, string, map[string]string) {}

	d := g.Check(Input{Event: EventBash, Command: "git push -u origin feat", Cwd: repo})
	if d == nil {
		t.Fatal("push of a commit whose message carries the name must deny")
	}
	if !strings.Contains(d.Reason, "commit "+sha+" message") {
		t.Errorf("reason %q does not name commit %s", d.Reason, sha)
	}
	if d := g.Check(Input{Event: EventBash, Command: "git push origin main", Cwd: repo}); d != nil {
		t.Errorf("push of commits the remote already has must allow, got %s", d.Reason)
	}
	// The same check from a `git -C` invocation run elsewhere.
	other := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if d := g.Check(Input{
		Event: EventBash, Command: "git -C " + repo + " push origin feat", Cwd: other,
	}); d == nil {
		t.Error("git -C form must resolve the repo the push runs in")
	}
}

func TestParseGh(t *testing.T) {
	tests := []struct {
		tokens   []string
		group    string
		sub      string
		publish  bool
		wantSkip bool
	}{
		{[]string{"pr", "create", "--body", "x"}, "pr", "create", true, false},
		{[]string{"pr", "view", "12"}, "pr", "view", false, false},
		{[]string{"-R", "o/r", "issue", "comment", "1"}, "o/r", "issue", false, false},
		{[]string{"api", "repos/o/r/pulls"}, "api", "repos/o/r/pulls", true, false},
		{[]string{"repo", "clone", "o/r"}, "repo", "clone", false, false},
		{[]string{"--help"}, "", "", false, true},
	}
	for _, tt := range tests {
		c, ok := parseGh(tt.tokens)
		if ok == tt.wantSkip {
			t.Errorf("parseGh(%v) ok = %v", tt.tokens, ok)
			continue
		}
		if tt.wantSkip {
			continue
		}
		if c.group != tt.group || c.sub != tt.sub || c.publishes() != tt.publish {
			t.Errorf("parseGh(%v) = %+v publishes %v, want %s/%s publishes %v",
				tt.tokens, c, c.publishes(), tt.group, tt.sub, tt.publish)
		}
	}
}

func TestGhRepoArg(t *testing.T) {
	tests := map[string]string{
		"acme/public":                        "github.com/acme/public",
		"acme/public.git":                    "github.com/acme/public",
		"git.internal.example/org/repo":      "git.internal.example/org/repo",
		"https://github.com/acme/public":     "github.com/acme/public",
		"https://github.com/acme/public.git": "github.com/acme/public",
		"public":                             "",
	}
	for in, want := range tests {
		if got := ghRepoArg(in); got != want {
			t.Errorf("ghRepoArg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGhFiles(t *testing.T) {
	c, _ := parseGh(strings.Fields(
		"pr create --body-file body.md -F extra.md --notes-file=notes.md " +
			"--input in.json --field body=@raw.md",
	))
	want := []string{"body.md", "extra.md", "notes.md", "in.json", "raw.md"}
	if got := c.files(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("files = %v, want %v", got, want)
	}
	gist, _ := parseGh(strings.Fields("gist create --public a.md b.md"))
	if got := gist.files(); strings.Join(got, ",") != "a.md,b.md" {
		t.Errorf("gist files = %v", got)
	}
}

func TestCollectStrings(t *testing.T) {
	got := collectStrings(map[string]any{
		"z": "last", "a": []any{"first", map[string]any{"k": "nested"}}, "n": 3, "b": true,
	})
	if got != "first\nnested\nlast" {
		t.Errorf("collectStrings = %q", got)
	}
}

// TestPublishInternalNamesPublicReposList covers the enumerated mode on the
// publish side: only listed repos are guarded, an internal org on
// github.com passes with no exemption at all, and targets that cannot be
// named (gists, a raw api call, an MCP call without owner/repo) pass too.
func TestPublishInternalNamesPublicReposList(t *testing.T) {
	cfg := config.Config{
		PublicRepos: []string{"github.com/you/tool", "github.com/oss-org/*"},
		Names:       config.InternalNames{BlockedWords: []string{"internalco", "work-org"}},
	}
	remote := func(url string) map[string]string {
		return map[string]string{"*|remote get-url origin": url}
	}
	bash := []struct {
		name     string
		command  string
		git      map[string]string
		wantDeny bool
	}{
		{
			"push in a listed repo",
			"git push origin internalco",
			remote("git@github.com:you/tool.git"),
			true,
		},
		{
			"commit in a listed org",
			`git commit -m "internalco"`,
			remote("git@github.com:oss-org/lib.git"),
			true,
		},
		{
			"push in the work org",
			"git push origin internalco",
			remote("git@github.com:work-org/svc.git"),
			false,
		},
		{
			"commit in an unlisted private repo",
			`git commit -m "internalco"`,
			remote("git@github.com:you/private.git"),
			false,
		},
		{"pr create -R work org", "gh pr create -R work-org/svc --body internalco", nil, false},
		{"pr create -R listed", "gh pr create -R you/tool --body internalco", nil, true},
		{
			"api endpoint in the work org",
			"gh api repos/work-org/svc/pulls -f body=internalco",
			nil,
			false,
		},
		{"gist outside any list", "gh gist create notes.md", nil, false},
		{"raw api call", "gh api user -f bio=internalco", nil, false},
		{"repo create in the work org", "gh repo create work-org/internalco --private", nil, false},
	}
	for _, tt := range bash {
		t.Run(tt.name, func(t *testing.T) {
			g := newPublishGuard(EventBash, publishFixture{git: tt.git, cfg: cfg})
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
		})
	}
	mcp := []struct {
		name     string
		input    map[string]any
		wantDeny bool
	}{
		{"listed repo", map[string]any{"owner": "you", "repo": "tool", "body": "internalco"}, true},
		{
			"work org",
			map[string]any{"owner": "work-org", "repo": "svc", "body": "internalco"},
			false,
		},
		{"no owner/repo", map[string]any{"name": "internalco-tools"}, false},
	}
	for _, tt := range mcp {
		t.Run("mcp "+tt.name, func(t *testing.T) {
			g := newPublishGuard(EventExternalText, publishFixture{cfg: cfg})
			d := g.Check(Input{
				Event: EventExternalText, ToolName: "mcp__gh_com__create_pull_request", ToolInput: tt.input,
			})
			if (d != nil) != tt.wantDeny {
				t.Errorf("%s: denial = %v, wantDeny %v", tt.name, d, tt.wantDeny)
			}
		})
	}
}
