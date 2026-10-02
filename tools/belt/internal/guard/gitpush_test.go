package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func newGitPushGuard(currentBranch string) *GitPushMain {
	g := NewGitPushMain(config.Config{})
	g.resolveBranch = func(dir string) string { return currentBranch }
	g.resolveRepo = func(dir string) string { return "" }
	return g
}

func TestGitPushMain(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		currentBranch string
		wantDeny      bool
	}{
		{"push explicit main", "git push origin main", "feature", true},
		{"push explicit master", "git push origin master", "feature", true},
		{"push feature branch", "git push origin my-feature", "my-feature", false},
		{"bare push on main", "git push", "main", true},
		{"bare push on feature", "git push", "feature-x", false},
		{"push -u origin HEAD on main", "git push -u origin HEAD", "main", true},
		{"push -u origin HEAD on feature", "git push -u origin HEAD", "fix-1", false},
		{"push refspec HEAD:main", "git push origin HEAD:main", "feature", true},
		{"push refs/heads/main", "git push origin refs/heads/main", "feature", true},
		{"force push main", "git push --force origin main", "feature", true},
		{"compound command", "cd /tmp && git push origin main", "feature", true},
		{"push after other git cmd", "git add . ; git push origin main", "feature", true},
		{"git -C push bare", "git -C /some/repo push", "main", true},
		{
			"git --git-dir space form",
			"git --git-dir /some/repo/.git push origin main",
			"feature",
			true,
		},
		{
			"git --git-dir equals form",
			"git --git-dir=/some/repo/.git push origin main",
			"feature",
			true,
		},
		{"push behind --no-pager", "git --no-pager push origin main", "feature", true},
		{"bare push behind -p", "git -p push", "main", true},
		{
			"push behind --no-optional-locks and -C",
			"git --no-optional-locks -C /some/repo push",
			"main",
			true,
		},
		{"non-push git", "git status", "main", false},
		{"quoted mention not a push", `echo "git push origin main"`, "feature", false},
		{"empty config fails closed", "git push origin main", "feature", true},
		{"delete main", "git push origin :main", "feature", true},
		{"push with push-option", "git push -o ci.skip origin main", "feature", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newGitPushGuard(tt.currentBranch)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil && !strings.Contains(d.Reason, GitPushMainID) {
				t.Errorf("reason missing guard id: %q", d.Reason)
			}
		})
	}
}

func TestGitPushUsesCwdForBarePush(t *testing.T) {
	g := NewGitPushMain(config.Config{})
	var gotDir string
	g.resolveBranch = func(dir string) string {
		gotDir = dir
		return "feature"
	}
	g.Check(Input{Event: EventBash, Command: "git push", Cwd: "/session/cwd"})
	if gotDir != "/session/cwd" {
		t.Errorf("resolver dir = %q, want /session/cwd", gotDir)
	}
}

// TestGitPushDashCDir pins that a `git -C` target reaches the resolvers as
// the same absolute path in every spelling the shell would expand (MAD-370):
// a push to a direct_main_repos entry must not be denied just because the
// path was written with ~ or $HOME.
func TestGitPushDashCDir(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	const worklog = "/Users/tester/code/worklog"
	tests := []struct {
		name, command, wantDir string
	}{
		{"absolute", "git -C /Users/tester/code/worklog push", worklog},
		{"tilde", "git -C ~/code/worklog push", worklog},
		{"HOME", "git -C $HOME/code/worklog push", worklog},
		{"braced HOME", "git -C ${HOME}/code/worklog push", worklog},
		{"quoted HOME", `git -C "$HOME/code/worklog" push`, worklog},
		{"bare tilde", "git -C ~ push", "/Users/tester"},
		{"relative to cwd", "git -C code/worklog push", "/session/cwd/code/worklog"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGitPushMain(config.Config{})
			var gotDir string
			g.resolveBranch = func(dir string) string {
				gotDir = dir
				return "feature"
			}
			g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/session/cwd"})
			if gotDir != tt.wantDir {
				t.Errorf("resolver dir = %q, want %q", gotDir, tt.wantDir)
			}
		})
	}
}

// unknownDirHint is the part of the fail-closed reason that tells an unknown
// push directory apart from a plain default-branch denial.
const unknownDirHint = "cannot tell which repo this push runs in"

// TestGitPushMainCdTracking pins that the guard follows `cd`/`pushd` across a
// compound command instead of trusting the session cwd: a bare or explicit
// default-branch push is evaluated against the directory the shell has cd'd
// into, so a `cd <non-exempt> && git push origin main` cannot ride an exempt
// session cwd. A leading ~ or $HOME expands like the shell would, in a cd and
// in a `git -C` target alike, and a target only a shell can resolve (a
// variable, `cd -`) fails closed without the resolvers ever being asked about
// an empty directory.
func TestGitPushMainCdTracking(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles" // exempt
	const other = "github.com/mad01/other-repo"  // not exempt
	repoAt := func(dir string) string {
		switch dir {
		case "/repos/dotfiles":
			return dotfiles
		case "/repos/other":
			return other
		}
		return ""
	}
	tests := []struct {
		name       string
		command    string
		cwd        string
		home       string // $HOME for the case; /repos when empty
		wantDeny   bool
		wantReason string // substring the denial must carry, if any
	}{
		{
			name:     "cd into non-exempt repo then push main denied",
			command:  "cd /repos/other && git push origin main",
			cwd:      "/repos/dotfiles",
			wantDeny: true,
		},
		{
			name:     "cd into non-exempt repo then bare push on main denied",
			command:  "cd /repos/other && git push",
			cwd:      "/repos/dotfiles",
			wantDeny: true,
		},
		{
			name:     "cd into exempt repo then push main allowed",
			command:  "cd /repos/dotfiles && git push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "cd relative into exempt repo then push main allowed",
			command:  "cd dotfiles && git push origin main",
			cwd:      "/repos",
			wantDeny: false,
		},
		{
			name:     "tilde cd into non-exempt repo then push main denied",
			command:  "cd ~/other && git push origin main",
			cwd:      "/repos/dotfiles",
			wantDeny: true,
		},
		{
			name:     "tilde cd into exempt repo then push main allowed",
			command:  "cd ~/dotfiles && git push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "HOME variable cd into exempt repo then push main allowed",
			command:  "cd $HOME/dotfiles && git push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "quoted HOME variable cd into exempt repo then push main allowed",
			command:  `cd "$HOME/dotfiles" && git push origin main`,
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "bare cd goes home, exempt home then push main allowed",
			command:  "cd && git push origin main",
			cwd:      "/repos/other",
			home:     "/repos/dotfiles",
			wantDeny: false,
		},
		{
			name:     "bare cd goes home, non-exempt home then push main denied",
			command:  "cd && git push origin main",
			cwd:      "/repos/dotfiles",
			home:     "/repos/other",
			wantDeny: true,
		},
		{
			name:       "unresolvable cd variable then push main fails closed",
			command:    "cd $TARGET && git push origin main",
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:       "unresolvable cd variable then bare push fails closed",
			command:    "cd $TARGET && git push",
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:       "cd dash previous dir then push main fails closed",
			command:    "cd - && git push origin main",
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:       "command substitution cd then push HEAD fails closed",
			command:    `cd "$(pwd)" && git push -u origin HEAD`,
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:     "unresolvable cd then push feature allowed",
			command:  "cd $TARGET && git push origin my-feature",
			cwd:      "/repos/dotfiles",
			wantDeny: false,
		},
		{
			name:     "cd into non-exempt repo then push feature allowed",
			command:  "cd /repos/other && git push origin my-feature",
			cwd:      "/repos/dotfiles",
			wantDeny: false,
		},
		{
			name:     "git -C overrides cd for exemption",
			command:  "cd /repos/other && git -C /repos/dotfiles push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "git -C overrides an unresolvable cd",
			command:  "cd $TARGET && git -C /repos/dotfiles push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "tilde git -C into exempt repo then push main allowed",
			command:  "git -C ~/dotfiles push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "HOME variable git -C into exempt repo then push main allowed",
			command:  "git -C $HOME/dotfiles push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:     "tilde git -C into non-exempt repo then push main denied",
			command:  "git -C ~/other push origin main",
			cwd:      "/repos/dotfiles",
			wantDeny: true,
		},
		{
			name:     "relative git -C resolves against the cd",
			command:  "cd /repos && git -C dotfiles push origin main",
			cwd:      "/repos/other",
			wantDeny: false,
		},
		{
			name:       "unresolvable git -C variable then push main fails closed",
			command:    "git -C $TARGET push origin main",
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:     "unresolvable git -C then push feature allowed",
			command:  "git -C $TARGET push origin my-feature",
			cwd:      "/repos/dotfiles",
			wantDeny: false,
		},
		{
			name:       "popd then git -C dot fails closed",
			command:    "pushd /repos/dotfiles && popd && git -C . push origin main",
			cwd:        "/repos/other",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:       "bare pushd then push main fails closed",
			command:    "pushd && git push origin main",
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:       "work-tree push fails closed even from an exempt cwd",
			command:    "git --work-tree=/repos/dotfiles push origin main",
			cwd:        "/repos/dotfiles",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
		{
			name:       "cd -P then push main fails closed",
			command:    "cd -P /repos/dotfiles && git push origin main",
			cwd:        "/repos/other",
			wantDeny:   true,
			wantReason: unknownDirHint,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := tt.home
			if home == "" {
				home = "/repos"
			}
			t.Setenv("HOME", home)
			g := NewGitPushMain(config.Config{DirectMainRepos: []string{dotfiles}})
			g.resolveBranch = func(dir string) string {
				if dir == "" {
					t.Error("resolveBranch asked about an empty directory")
				}
				return "main"
			}
			g.resolveRepo = func(dir string) string {
				if dir == "" {
					t.Error("resolveRepo asked about an empty directory")
				}
				return repoAt(dir)
			}
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if (d != nil) != tt.wantDeny {
				t.Fatalf(
					"Check(%q, cwd=%q) denial = %v, wantDeny %v",
					tt.command,
					tt.cwd,
					d,
					tt.wantDeny,
				)
			}
			if d != nil && !strings.Contains(d.Reason, tt.wantReason) {
				t.Errorf("reason %q does not contain %q", d.Reason, tt.wantReason)
			}
		})
	}
}

// initRepo creates a git repository at dir on branch main, with one commit and
// the given origin URL, and returns dir.
func initRepo(t *testing.T, dir, origin string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
		{"remote", "add", "origin", origin},
	} {
		cmd := gitCmd(dir, args...)
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// TestGitPushMainRealResolver runs the guard with its git-backed resolvers
// against real repositories, with belt's own process cwd parked inside the
// exempt one: the setup in which a cd the guard could not resolve once let a
// push into a non-exempt repo through, because git ran in the process cwd
// and reported the exempt repo. Every cd form below has to be judged against
// the repo it lands in, or denied when that cannot be told.
func TestGitPushMainRealResolver(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	exempt := initRepo(t, home, "git@github.com:mad01/dotfiles.git")
	other := initRepo(t, filepath.Join(home, "other"), "git@github.com:mad01/other.git")
	// A symlink whose target sits inside the non-exempt repo: git -C resolves
	// `link/..` physically into that repo, a shell's cd resolves it logically
	// back to home.
	if err := os.MkdirAll(filepath.Join(other, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(other, "sub"), filepath.Join(home, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(exempt)
	g := NewGitPushMain(config.Config{DirectMainRepos: []string{"github.com/mad01/dotfiles"}})

	tests := []struct {
		name       string
		command    string
		cwd        string
		wantDeny   bool
		wantReason string
	}{
		{"no cd from exempt cwd", "git push origin main", exempt, false, ""},
		{
			"absolute cd into non-exempt",
			"cd " + other + " && git push origin main",
			exempt,
			true,
			"",
		},
		{"tilde cd into non-exempt", "cd ~/other && git push origin main", exempt, true, ""},
		{"tilde cd then bare push on main", "cd ~/other && git push", exempt, true, ""},
		{"HOME cd into non-exempt", "cd $HOME/other && git push origin main", exempt, true, ""},
		{
			"quoted HOME cd into non-exempt",
			`cd "$HOME/other" && git push origin main`,
			exempt,
			true,
			"",
		},
		{"tilde cd home is exempt", "cd ~ && git push origin main", other, false, ""},
		{"bare cd home is exempt", "cd && git push origin main", other, false, ""},
		{
			"variable cd then push main",
			"cd $TARGET && git push origin main",
			exempt,
			true,
			unknownDirHint,
		},
		{"variable cd then bare push", "cd $TARGET && git push", exempt, true, unknownDirHint},
		{"cd dash then push main", "cd - && git push origin main", exempt, true, unknownDirHint},
		{
			"substitution cd then push main",
			`cd "$(pwd)" && git push origin main`,
			exempt,
			true,
			unknownDirHint,
		},
		{"variable cd then push feature", "cd $TARGET && git push origin feat", exempt, false, ""},
		{
			"git -C exempt after variable cd",
			"cd $TARGET && git -C " + exempt + " push origin main",
			other,
			false,
			"",
		},
		{"tilde git -C into non-exempt", "git -C ~/other push origin main", exempt, true, ""},
		{"HOME git -C into non-exempt", "git -C $HOME/other push origin main", exempt, true, ""},
		{"tilde git -C home is exempt", "git -C ~ push origin main", other, false, ""},
		{
			"variable git -C then push main",
			"git -C $TARGET push origin main",
			exempt,
			true,
			unknownDirHint,
		},
		{
			"git -C through a symlink parent lands in non-exempt",
			"git -C ~/link/.. push origin main",
			exempt,
			true,
			"",
		},
		{
			"cd through a symlink parent is logical and exempt",
			"cd ~/link/.. && git push origin main",
			other,
			false,
			"",
		},
		{
			"popd then git -C dot fails closed",
			"pushd " + other + " && popd && git -C . push origin main",
			exempt,
			true,
			unknownDirHint,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if (d != nil) != tt.wantDeny {
				t.Fatalf(
					"Check(%q, cwd=%q) denial = %v, wantDeny %v",
					tt.command,
					tt.cwd,
					d,
					tt.wantDeny,
				)
			}
			if d != nil && !strings.Contains(d.Reason, tt.wantReason) {
				t.Errorf("reason %q does not contain %q", d.Reason, tt.wantReason)
			}
		})
	}
}

// TestGitOutputUnknownDirNeverRunsGit pins the choke point every git-backed
// resolver goes through: an empty dir is unknown, not the process cwd, even
// when that cwd is a repo git would happily answer for.
func TestGitOutputUnknownDirNeverRunsGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := initRepo(t, t.TempDir(), "git@github.com:mad01/example.git")
	t.Chdir(repo)
	if got := gitRemoteURL(""); got != "" {
		t.Errorf("gitRemoteURL(\"\") = %q, want empty", got)
	}
	if got := gitCurrentBranch(""); got != "" {
		t.Errorf("gitCurrentBranch(\"\") = %q, want empty", got)
	}
	if got := CanonicalRepoAt(""); got != "" {
		t.Errorf("CanonicalRepoAt(\"\") = %q, want empty", got)
	}
}

func TestGitPushMainAllowRepos(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles"
	allow := []string{dotfiles}
	tests := []struct {
		name          string
		command       string
		currentBranch string
		repo          string
		allow         []string
		wantDeny      bool
	}{
		{"dotfiles bare push on main allowed", "git push", "main", dotfiles, allow, false},
		{
			"dotfiles explicit origin main allowed",
			"git push origin main",
			"feature",
			dotfiles,
			allow,
			false,
		},
		{
			"dotfiles HEAD:main allowed",
			"git push origin HEAD:main",
			"feature",
			dotfiles,
			allow,
			false,
		},
		{"dotfiles git -C push allowed", "git -C /repo push", "main", dotfiles, allow, false},
		{
			"non-allowlisted repo denied",
			"git push origin main",
			"feature",
			"github.com/mad01/thismoon",
			allow,
			true,
		},
		{"unresolved repo denied", "git push origin main", "feature", "", allow, true},
		{"empty allowlist denied", "git push origin main", "feature", dotfiles, nil, true},
		// allow_repos takes the same patterns as git_identity[].repos and
		// commit_guards[].repos, so one spelling works file-wide.
		{
			"org wildcard allowed",
			"git push origin main",
			"feature",
			dotfiles,
			[]string{"github.com/mad01/*"},
			false,
		},
		{
			"org wildcard denies other orgs",
			"git push origin main",
			"feature",
			dotfiles,
			[]string{"github.com/other/*"},
			true,
		},
		{
			"org wildcard needs a repo under it",
			"git push origin main",
			"feature",
			"github.com/mad01",
			[]string{"github.com/mad01/*"},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGitPushMain(config.Config{
				Guards: map[string]config.Toggle{GitPushMainID: {AllowRepos: tt.allow}},
			})
			g.resolveBranch = func(string) string { return tt.currentBranch }
			g.resolveRepo = func(string) string { return tt.repo }
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
		})
	}
}

// TestGitPushMainDirectMainRepos pins that the shared direct_main_repos
// list exempts this guard (one of its two readers, docs/adr/0013), with the
// same fail-closed rules as allow_repos for unresolved repos.
func TestGitPushMainDirectMainRepos(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles"
	tests := []struct {
		name     string
		repo     string
		global   []string
		wantDeny bool
	}{
		{"direct-main repo allowed", dotfiles, []string{dotfiles}, false},
		{"wildcard allowed", dotfiles, []string{"github.com/mad01/*"}, false},
		{"repo off the list denied", "github.com/mad01/thismoon", []string{dotfiles}, true},
		{"unresolved repo still denied", "", []string{dotfiles}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGitPushMain(config.Config{DirectMainRepos: tt.global})
			g.resolveBranch = func(string) string { return "main" }
			g.resolveRepo = func(string) string { return tt.repo }
			d := g.Check(Input{Event: EventBash, Command: "git push", Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("denial = %v, wantDeny %v", d, tt.wantDeny)
			}
		})
	}
}

func TestCanonicalRepo(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"git@github.com:mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"https://github.com/mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"https://github.com/mad01/dotfiles", "github.com/mad01/dotfiles"},
		{"ssh://git@github.com/mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"ssh://git@github.com:22/mad01/dotfiles.git", "github.com/mad01/dotfiles"},
		{"/local/path/repo", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := canonicalRepo(tt.in); got != tt.want {
			t.Errorf("canonicalRepo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
