package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// discardRepos is a clone with an upstream: a bare origin seeded from a
// second clone, so the clone has @{u} and origin/HEAD the way a real one
// does. The seed pushes the upstream changes a pull would bring.
type discardRepos struct {
	t     *testing.T
	seed  string
	clone string
}

// newDiscardRepos builds the repos with no system or global git config, so
// the user's hooks and settings never reach the test.
func newDiscardRepos(t *testing.T) discardRepos {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if gitEnvRedirected() {
		t.Skip("a GIT_DIR-style variable is set; the proof refuses there by design")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "t")
		t.Setenv(k+"_EMAIL", "t@example.com")
	}
	root := t.TempDir()
	r := discardRepos{
		t:     t,
		seed:  filepath.Join(root, "seed"),
		clone: filepath.Join(root, "clone"),
	}
	up := filepath.Join(root, "up.git")
	r.git(root, "init", "-q", "--bare", "-b", "main", up)
	r.git(root, "init", "-q", "-b", "main", r.seed)
	r.write(r.seed, "a.txt", "a\n")
	r.write(r.seed, "b.txt", "b\n")
	r.write(r.seed, "d/f.txt", "f\n")
	r.git(r.seed, "add", "-A")
	r.git(r.seed, "commit", "-q", "-m", "init")
	r.git(r.seed, "remote", "add", "origin", up)
	r.git(r.seed, "push", "-q", "origin", "main")
	r.git(root, "clone", "-q", up, r.clone)
	return r
}

func (r discardRepos) git(dir string, args ...string) {
	r.t.Helper()
	if out, err := gitCmd(dir, args...).CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r discardRepos) write(dir, name, content string) {
	r.t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// upstream commits files in the seed, pushes them and fetches them into the
// clone: the state a pull starts from.
func (r discardRepos) upstream(files map[string]string) {
	r.t.Helper()
	for name, content := range files {
		r.write(r.seed, name, content)
	}
	r.git(r.seed, "add", "-A")
	r.git(r.seed, "commit", "-q", "-m", "upstream")
	r.git(r.seed, "push", "-q", "origin", "main")
	r.git(r.clone, "fetch", "-q")
}

// remove deletes a file or directory from the clone's worktree.
func (r discardRepos) remove(name string) {
	r.t.Helper()
	if err := os.RemoveAll(filepath.Join(r.clone, name)); err != nil {
		r.t.Fatal(err)
	}
}

// local writes files into the clone's worktree.
func (r discardRepos) local(files map[string]string) {
	r.t.Helper()
	for name, content := range files {
		r.write(r.clone, name, content)
	}
}

// submodule commits a submodule at sm, cloned from a bare upstream of its
// own and holding one file f, with `ignore = dirty` in .gitmodules so git
// status hides edits inside it unless told otherwise.
func (r discardRepos) submodule() {
	r.t.Helper()
	root := filepath.Dir(r.clone)
	seed, up := filepath.Join(root, "sm-seed"), filepath.Join(root, "sm.git")
	r.git(root, "init", "-q", "--bare", "-b", "main", up)
	r.git(root, "init", "-q", "-b", "main", seed)
	r.write(seed, "f", "f\n")
	r.git(seed, "add", "-A")
	r.git(seed, "commit", "-q", "-m", "init")
	r.git(seed, "push", "-q", up, "main")
	r.git(r.clone, "-c", "protocol.file.allow=always", "submodule", "add", "-q", up, "sm")
	r.git(r.clone, "config", "-f", ".gitmodules", "submodule.sm.ignore", "dirty")
	r.git(r.clone, "commit", "-q", "-am", "add sm")
}

// TestGitDiscardNothingLost runs the guard with its real git backends over a
// clone and its upstream. A discard passes when every file version it drops
// is committed in HEAD, the upstream or origin's default branch, and is
// refused with the first lossy path otherwise.
func TestGitDiscardNothingLost(t *testing.T) {
	pulled := map[string]string{"a.txt": "a2\n", "new.txt": "new\n", "d/f.txt": "f2\n"}
	tests := []struct {
		name     string
		setup    func(r discardRepos)
		maxPaths int
		commands []string
		wantDeny string // a part of the reason; "" means the command passes
	}{
		{
			name: "untracked files equal to upstream that block a pull",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"new.txt": "new\n"})
			},
			commands: []string{
				"git clean -f",
				"git clean -f -- new.txt",
				"git clean -fd",
				"git reset --hard origin/main",
				"git reset --hard @{u}",
			},
		},
		{
			name: "a modified file equal to upstream",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			commands: []string{
				"git checkout -- a.txt",
				"git checkout .",
				"git restore a.txt d",
				"git reset --hard",
				"git reset --hard origin/main",
				"cd d && git checkout -- f.txt",
				"git -C d restore .",
			},
		},
		{
			name:     "a modified file with unique content",
			setup:    func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{"git checkout -- a.txt", "git restore .", "git reset --hard"},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a mix names the unique file",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "b.txt": "mine\n", "new.txt": "new\n"})
			},
			commands: []string{"git checkout -- .", "git reset --hard origin/main"},
			wantDeny: "`b.txt` holds content no commit has",
		},
		{
			name: "an untracked file with unique content",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"new.txt": "new\n", "notes.txt": "mine\n"})
			},
			commands: []string{"git clean -f"},
			wantDeny: "`notes.txt` holds content no commit has",
		},
		{
			name: "an untracked file a reset writes over",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"new.txt": "mine\n", "notes.txt": "kept\n"})
			},
			commands: []string{"git reset --hard origin/main"},
			wantDeny: "`new.txt` holds content no commit has",
		},
		{
			name: "untracked files a reset leaves alone",
			setup: func(r discardRepos) {
				r.local(map[string]string{"notes.txt": "kept\n"})
			},
			commands: []string{"git reset --hard"},
		},
		{
			name: "a staged copy with unique content",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "mine\n"})
				r.git(r.clone, "add", "a.txt")
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{"git reset --hard origin/main", "git checkout origin/main -- a.txt"},
			wantDeny: "the staged copy of `a.txt` is in no commit",
		},
		{
			name: "a hard reset that drops a local commit",
			setup: func(r discardRepos) {
				r.local(map[string]string{"c.txt": "c\n"})
				r.git(r.clone, "add", "c.txt")
				r.git(r.clone, "commit", "-q", "-m", "local")
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{"git reset --hard origin/main"},
			wantDeny: "moves the branch off 1 commit(s)",
		},
		{
			name: "a path set belt cannot resolve",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{
				"git checkout -- '*.txt'",
				"git checkout -- ':(glob)a.txt'",
				`git restore "$F"`,
				"git checkout -p -- a.txt",
				"git restore --ours a.txt",
				"git clean -fi",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "more paths than the cap",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			maxPaths: 1,
			commands: []string{"git checkout -- ."},
			wantDeny: "more than 1 paths",
		},
		{
			name:     "a stash, which the proof does not read",
			setup:    func(r discardRepos) { r.local(map[string]string{"a.txt": "a\n", "x": "x\n"}) },
			commands: []string{"git stash -u"},
			wantDeny: "passes only on a clean tree",
		},
		{
			name: "a file where a restored path needs a directory",
			setup: func(r discardRepos) {
				r.remove("d")
				r.local(map[string]string{"d": "WORK\n"})
			},
			commands: []string{
				"git checkout -- d/f.txt",
				"git restore d/f.txt",
				"git reset --hard",
			},
			wantDeny: "`d` holds content no commit has",
		},
		{
			name: "a clean whose -e brings an ignored file in",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{".git/info/exclude": "secret.env\n"})
				r.local(map[string]string{"new.txt": "new\n", "secret.env": "SECRET\n"})
			},
			commands: []string{
				"git clean -f -e '!secret.env'",
				"git clean -f --exclude='!secret.env'",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a clean whose -e brings in the only ignored file",
			setup: func(r discardRepos) {
				r.local(map[string]string{".git/info/exclude": "secret.env\n"})
				r.local(map[string]string{"secret.env": "SECRET\n"})
			},
			commands: []string{"git clean -f -e '!secret.env'"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name:     "a clean -X whose -e brings an untracked file in",
			setup:    func(r discardRepos) { r.local(map[string]string{"keepme.txt": "mine\n"}) },
			commands: []string{"git clean -fX -e keepme.txt"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "an earlier step that changes what the discard drops",
			setup: func(r discardRepos) {
				r.local(map[string]string{"untracked": "u\n"})
			},
			commands: []string{
				"mv untracked a.txt && git checkout -- a.txt",
				"cp untracked a.txt; git restore a.txt",
				"echo x >a.txt && git reset --hard",
				"git checkout -- a.txt & mv untracked a.txt",
				"git checkout -- a.txt | mv untracked a.txt",
				"F=x; git checkout -- a.txt",
			},
			wantDeny: "may change what it drops",
		},
		{
			name: "an earlier git write that strands a commit",
			setup: func(r discardRepos) {
				r.local(map[string]string{"c.txt": "c\n"})
				r.git(r.clone, "add", "c.txt")
				r.git(r.clone, "commit", "-q", "-m", "local")
				r.local(map[string]string{"notes.txt": "kept\n"})
			},
			commands: []string{
				"git reset --soft HEAD~1 && git reset --hard",
				"git checkout -- . && git reset --hard",
			},
			wantDeny: "may change what it drops",
		},
		{
			name: "read-only steps around a discard",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "notes.txt": "kept\n"})
			},
			commands: []string{
				"git status --short && git checkout -- a.txt",
				"cat a.txt; git diff --stat && git checkout -- a.txt && git pull",
				"git reset --hard origin/main 2>&1 | tail -1",
				"git checkout -- a.txt >/dev/null",
			},
		},
		{
			name:  "a global pathspec option or environment",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{
				"git --icase-pathspecs checkout -- A.TXT",
				"GIT_ICASE_PATHSPECS=1 git checkout -- A.TXT",
				"git -C . --icase-pathspecs restore A.TXT",
				"env GIT_ICASE_PATHSPECS=1 git restore A.TXT",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "an assume-unchanged edit a reset drops",
			setup: func(r discardRepos) {
				r.git(r.clone, "update-index", "--assume-unchanged", "a.txt")
				r.local(map[string]string{"a.txt": "a\nWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard", "git checkout ."},
			wantDeny: "assume-unchanged or skip-worktree",
		},
		{
			name: "a skip-worktree edit a reset drops",
			setup: func(r discardRepos) {
				r.git(r.clone, "update-index", "--skip-worktree", "b.txt")
				r.local(map[string]string{"b.txt": "b\nWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard", "git restore b.txt"},
			wantDeny: "assume-unchanged or skip-worktree",
		},
		{
			name: "an origin/HEAD that points at a local branch",
			setup: func(r discardRepos) {
				r.git(r.clone, "checkout", "-q", "-b", "scratch")
				r.local(map[string]string{"a.txt": "mine\n"})
				r.git(r.clone, "commit", "-q", "-am", "wip")
				r.git(r.clone, "checkout", "-q", "main")
				r.git(r.clone, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/heads/scratch")
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{"git checkout -- a.txt"},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a submodule edit status hides that a recursing reset drops",
			setup: func(r discardRepos) {
				r.submodule()
				r.git(r.clone, "config", "submodule.recurse", "true")
				r.local(map[string]string{"sm/f": "f\nSUBWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a submodule edit status hides, in an otherwise clean tree",
			setup: func(r discardRepos) {
				r.submodule()
				r.git(r.clone, "config", "submodule.recurse", "true")
				r.local(map[string]string{"sm/f": "f\nSUBWORK\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a submodule edit status hides, without submodule.recurse",
			setup: func(r discardRepos) {
				r.submodule()
				r.local(map[string]string{"sm/f": "f\nSUBWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "a submodule at sm",
		},
		{
			name: "a clean submodule a recursing reset reaches",
			setup: func(r discardRepos) {
				r.submodule()
				r.git(r.clone, "config", "submodule.recurse", "true")
				r.local(map[string]string{"untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "submodule.recurse",
		},
		{
			name: "a clean submodule a plain reset leaves alone",
			setup: func(r discardRepos) {
				r.submodule()
				r.local(map[string]string{"untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newDiscardRepos(t)
			tt.setup(r)
			g := NewGitDiscard(config.Config{})
			g.emit = func(_, _, _, _ string, _ map[string]string) {}
			if tt.maxPaths > 0 {
				g.prove = prover{maxPaths: tt.maxPaths, budget: proofBudget}.prove
			}
			for _, command := range tt.commands {
				d := g.Check(Input{Event: EventBash, Command: command, Cwd: r.clone})
				switch {
				case tt.wantDeny == "" && d != nil:
					t.Errorf("Check(%q) = %q, want it to pass", command, d.Reason)
				case tt.wantDeny != "" && d == nil:
					t.Errorf("Check(%q) passed, want a deny naming %q", command, tt.wantDeny)
				case d != nil && !strings.Contains(d.Reason, tt.wantDeny):
					t.Errorf("Check(%q) reason = %q, want it to contain %q",
						command, d.Reason, tt.wantDeny)
				}
			}
		})
	}
}

// TestReadDiscardSpec pins how the proof reads each command's arguments.
func TestReadDiscardSpec(t *testing.T) {
	tests := []struct {
		sub, args string
		want      discardSpec
		wantErr   bool
	}{
		{sub: "checkout", args: "-- a b", want: discardSpec{
			kind: pathRestore, overlay: true, pathspecs: []string{"a", "b"},
		}},
		{sub: "checkout", args: "-q main -- a", want: discardSpec{
			kind: pathRestore, overlay: true, source: "main", staged: true,
			pathspecs: []string{"a"},
		}},
		{sub: "checkout", args: ".", want: discardSpec{
			kind: pathRestore, overlay: true, pathspecs: []string{"."},
		}},
		{sub: "checkout", args: "-f main", wantErr: true},
		{sub: "checkout", args: "main .", wantErr: true},
		{sub: "restore", args: "--source=HEAD~1 a", want: discardSpec{
			kind: pathRestore, source: "HEAD~1", pathspecs: []string{"a"},
		}},
		{sub: "restore", args: "-S -W a", want: discardSpec{
			kind: pathRestore, source: "HEAD", staged: true, pathspecs: []string{"a"},
		}},
		{sub: "restore", args: "-W", wantErr: true},
		{sub: "clean", args: "-fdx -- out", want: discardSpec{
			kind: untrackedClean, scope: cleanWithIgnored, pathspecs: []string{"out"},
		}},
		{sub: "clean", args: "-fdx -e keep -- out", wantErr: true},
		{sub: "clean", args: "-fekeep", wantErr: true},
		{sub: "clean", args: "-f --exclude=keep", wantErr: true},
		{sub: "clean", args: "-f --exclude keep", wantErr: true},
		{
			sub:  "clean",
			args: "-fX",
			want: discardSpec{kind: untrackedClean, scope: cleanOnlyIgnored},
		},
		{sub: "reset", args: "--hard", want: discardSpec{kind: hardReset, source: "HEAD"}},
		{sub: "reset", args: "--hard @{u}", want: discardSpec{kind: hardReset, source: "@{u}"}},
		{sub: "reset", args: "--hard -- a", wantErr: true},
		{sub: "reset", args: "--hard --recurse-submodules", wantErr: true},
		{sub: "checkout", args: "-- d/*.go", wantErr: true},
		{sub: "checkout", args: `-- a\b`, wantErr: true},
		{sub: "reset", args: "--hard -x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.sub+" "+tt.args, func(t *testing.T) {
			got, err := readDiscardSpec(gitInvocation{sub: tt.sub, args: strings.Fields(tt.args)})
			if (err != nil) != tt.wantErr {
				t.Fatalf("readDiscardSpec = %+v, %v; wantErr %v", got, err, tt.wantErr)
			}
			if !tt.wantErr && !specEqual(got, tt.want) {
				t.Errorf("readDiscardSpec = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func specEqual(a, b discardSpec) bool {
	return a.kind == b.kind && a.source == b.source && a.staged == b.staged &&
		a.overlay == b.overlay && a.scope == b.scope &&
		strings.Join(a.pathspecs, "\x00") == strings.Join(b.pathspecs, "\x00")
}
