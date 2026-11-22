package guard

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestReadSubmoduleCall pins how a git submodule call is read: as
// git-submodule.sh reads its words, exact options only, the first other
// word starting the pathspecs, and a word belt cannot read touching every
// submodule.
func TestReadSubmoduleCall(t *testing.T) {
	tests := []struct {
		sub, args string
		fed       bool
		wantOK    bool
		wantAll   bool
		wantPaths []string
	}{
		{sub: "submodule", args: "deinit -f --all", wantOK: true, wantAll: true},
		{sub: "submodule", args: "deinit --force sm", wantOK: true, wantPaths: []string{"sm"}},
		{sub: "submodule", args: "deinit -f -- sm lib", wantOK: true, wantPaths: []string{"sm", "lib"}},
		{sub: "submodule", args: "deinit -f -- -q", wantOK: true, wantPaths: []string{"-q"}},
		{sub: "submodule", args: "deinit -f -- 'sm'", wantOK: true, wantPaths: []string{"sm"}},
		{sub: "submodule", args: "deinit -f", wantOK: true, wantAll: true},
		{sub: "submodule", args: "-q --quiet deinit -q -f sm", wantOK: true, wantPaths: []string{"sm"}},
		{sub: "submodule", args: "deinit sm -f"},  // -f is a path after the first one
		{sub: "submodule", args: "deinit -- -f"},  // so is one after --
		{sub: "submodule", args: "deinit -fq sm"}, // no bundles: git stops with its usage
		{sub: "submodule", args: "deinit --forc sm"},
		{sub: "submodule", args: "deinit sm"},
		{sub: "submodule", args: "deinit -f $P", wantOK: true, wantAll: true},
		{sub: "submodule", args: "deinit $F sm", wantOK: true, wantAll: true},
		{sub: "submodule", args: "deinit sm", fed: true, wantOK: true, wantAll: true},
		{sub: "submodule", args: "update --force", wantOK: true, wantAll: true},
		{sub: "submodule", args: "update -f sm", wantOK: true, wantPaths: []string{"sm"}},
		{sub: "submodule", args: "update --init --recursive -f -j 4 --depth=1 sm", wantOK: true,
			wantPaths: []string{"sm"}},
		{sub: "submodule", args: "update -j4 --jobs=2 --filter blob:none -f", wantOK: true, wantAll: true},
		{sub: "submodule", args: "update --reference -f sm"}, // -f is the reference
		{sub: "submodule", args: "update --reference $R -f", wantOK: true, wantAll: true},
		{sub: "submodule", args: "update --reference"},
		{sub: "submodule", args: "update --all -f"},
		{sub: "submodule", args: "update --init --recursive"},
		{sub: "submodule", args: "update"},
		{sub: "submodule", args: "update $X", wantOK: true, wantAll: true},
		{sub: "submodule", args: "$CMD -f", wantOK: true, wantAll: true},
		{sub: "submodule", args: "status"},
		{sub: "submodule", args: "foreach git status"},
		{sub: "submodule", args: "add -f ../up sm"},
		{sub: "submodule", args: "--frob deinit -f sm"},
		{sub: "submodule", args: ""},
		{sub: "submodule", args: "", fed: true, wantOK: true, wantAll: true},
		{sub: "submodule", args: "status", fed: true},
		{sub: "submodule--helper", args: "update sm", wantOK: true, wantAll: true},
		{sub: "submodule--helper", args: "deinit sm", wantOK: true, wantAll: true},
		{sub: "submodule--helper", args: "$X", wantOK: true, wantAll: true},
		{sub: "submodule--helper", args: "absorbgitdirs"},
		{sub: "submodule--helper", args: "", fed: true, wantOK: true, wantAll: true},
		{sub: "status", args: "deinit -f"},
	}
	for _, tt := range tests {
		t.Run(tt.sub+" "+tt.args, func(t *testing.T) {
			c := gitInvocation{sub: tt.sub, args: strings.Fields(tt.args)}
			got, ok := readSubmoduleCall(c, tt.fed)
			if ok != tt.wantOK || ok && (got.all != tt.wantAll || !slices.Equal(got.paths, tt.wantPaths)) {
				t.Errorf("readSubmoduleCall = %+v, %v; want ok %v, all %v, paths %q",
					got, ok, tt.wantOK, tt.wantAll, tt.wantPaths)
			}
		})
	}
}

// TestGitDiscardSubmoduleCommands runs the guard with a fake submodule
// check: a submodule named dirty holds work, and a call that touches every
// submodule touches it too. A global option makes a call touch every one.
func TestGitDiscardSubmoduleCommands(t *testing.T) {
	tests := []struct {
		command  string
		wantDeny bool
	}{
		{"git submodule deinit -f --all", true},
		{"git submodule deinit -f dirty", true},
		{"git submodule deinit -f clean", false},
		{"git submodule update --force", true},
		{"git submodule update -f -- clean", false},
		{"git submodule update -f clean dirty", true},
		{"git -c x.y=z submodule update -f clean", true},
		{"git -C /repo submodule update -f clean", false},
		{"echo dirty | xargs git submodule deinit -f", true},
		{"git submodule deinit dirty", false},
		{"git submodule update dirty", false},
		{"git submodule update --init --recursive", false},
		{"git submodule status", false},
		{"git submodule--helper update dirty", true},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, discardFixture{repo: "github.com/mad01/thismoon"}, &warned)
			g.submoduleCheck = func(_ context.Context, dir string, touch submoduleTouch) error {
				if dir != "/repo" {
					return submoduleUnread("wrong dir " + dir)
				}
				if touch.all || slices.Contains(touch.paths, "dirty") {
					return submoduleLoss{"the submodule at /repo/dirty holds uncommitted work"}
				}
				return nil
			}
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil && !strings.Contains(d.Reason, submoduleRule) {
				t.Errorf("reason = %q, want the submodule rule", d.Reason)
			}
		})
	}
}

// TestGitDiscardWorkTreeSettings pins that a discard passes no clean-tree
// check when a setting it runs with may set core.worktree: belt reads git
// status before the command runs, in the work tree the setting replaces.
func TestGitDiscardWorkTreeSettings(t *testing.T) {
	clean := discardFixture{repo: "github.com/mad01/thismoon"}
	tests := []struct {
		command  string
		wantDeny bool
	}{
		{"git -c core.worktree=/elsewhere checkout -- f", true},
		{"git -c CORE.WORKTREE=/elsewhere checkout -- f", true},
		{"git --config-env=core.worktree=WT checkout -- f", true},
		{"git -c include.path=/x/cfg reset --hard", true},
		{"git -c includeIf.onbranch:main.path=/x/cfg reset --hard", true},
		{"git -c $K=/elsewhere $SUB -- f", true},
		{"git config core.worktree /elsewhere && git checkout -- f", true},
		{"git config --local core.worktree /elsewhere; git reset --hard", true},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.worktree GIT_CONFIG_VALUE_0=/x git checkout -- f", true},
		{"git -c core.worktree=/elsewhere submodule update --force", true},
		{"git -c core.worktree=/elsewhere $SUB -- f", true},
		{"git config --unset core.worktree && git checkout -- .", true},
		{"git config unset core.worktree && git checkout -- f", true},
		{"git config --unset-all core.worktree; git reset --hard", true},
		{"git config unset --all CORE.worktree && git checkout -- f", true},
		{"git config --local --unset core.worktree && git checkout -- f", true},
		{"git config --remove-section core && git checkout -- f", true},
		{"git config remove-section CORE && git checkout -- f", true},
		{"git config --rename-section core old && git checkout -- f", true},
		{"git config rename-section old core && git checkout -- f", true},
		{"git config --edit && git checkout -- f", true},
		{"git config -e; git checkout -- f", true},
		{"git config edit && git checkout -- f", true},
		{"git config --unset $K && git checkout -- f", true},
		{"git config $OP && git checkout -- f", true},
		{"printf '[core]\\n' >> .git/config && git checkout -- f", true},
		{"sed -i '' /worktree/d .git/config && git checkout -- f", true},
		{"printf x | tee .git/config && git checkout -- f", true},
		{"git checkout -- f & git config --unset core.worktree", true},
		{"git config --unset core.worktree && git submodule update --force", true},
		{"git -c color.ui=never checkout -- f", false},
		{"git config user.name t && git checkout -- f", false},
		{"git config --unset user.name && git checkout -- f", false},
		{"git config --remove-section user && git checkout -- f", false},
		{"git config --unset user.name core.worktree && git checkout -- f", false},
		{"git config core.worktree; git checkout -- f", false},
		{"git config --get core.worktree && git checkout -- f", false},
		{"git config --get $K && git checkout -- f", false},
		{"git config list --show-origin && git checkout -- f", false},
		{"cat .git/config && git checkout -- f", false},
		{"git checkout -- f; git config --unset core.worktree", false},
		{"git -c core.worktree=/elsewhere status", false},
		{"git checkout -- f", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, clean, &warned)
			g.submoduleCheck = func(context.Context, string, submoduleTouch) error { return nil }
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil && !strings.Contains(d.Reason, "belt cannot tell which repository it runs in") {
				t.Errorf("reason = %q, want the unknown-tree reason", d.Reason)
			}
		})
	}
}

// addSubmodule commits nothing but adds a submodule at path to the clone,
// cloned from a bare upstream of its own that holds one file f.
func (r discardRepos) addSubmodule(path string) {
	r.t.Helper()
	r.addSubmoduleIn(r.clone, path)
}

// addSubmoduleIn adds a submodule at path to the checkout at dir, under
// the clone, as addSubmodule does.
func (r discardRepos) addSubmoduleIn(dir, path string) {
	r.t.Helper()
	root := filepath.Dir(r.clone)
	name := strings.ReplaceAll(strings.TrimPrefix(filepath.Join(dir, path), r.clone+"/"), "/", "-")
	seed, up := filepath.Join(root, name+"-seed"), filepath.Join(root, name+".git")
	r.git(root, "init", "-q", "--bare", "-b", "main", up)
	r.git(root, "init", "-q", "-b", "main", seed)
	r.write(seed, "f", "f\n")
	r.git(seed, "add", "-A")
	r.git(seed, "commit", "-q", "-m", "init")
	r.git(seed, "push", "-q", up, "main")
	r.git(dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", up, path)
}

// nestSubmodule adds a submodule n to the checkout at dir and commits it
// there, and returns the path of n.
func (r discardRepos) nestSubmodule(dir string) string {
	r.t.Helper()
	r.addSubmoduleIn(dir, "n")
	r.git(dir, "commit", "-q", "-m", "nest n")
	return filepath.Join(dir, "n")
}

// strandSubmodule makes the submodule at dir a folder that is not checked
// out and holds one file, stray.
func (r discardRepos) strandSubmodule(dir string) {
	r.t.Helper()
	r.removeAll(dir)
	r.write(dir, "stray", "stray\n")
}

// TestGitDiscardSubmoduleProof runs the submodule check with its real git
// backend over a clone with two submodules, sm and lib/other. A forced
// deinit or update passes when every submodule its pathspecs match is
// clean, and denies naming the first one that holds work.
func TestGitDiscardSubmoduleProof(t *testing.T) {
	tests := []struct {
		name  string
		setup func(r discardRepos)
		// cwd is where the commands run, relative to the clone.
		cwd    string
		pass   []string
		deny   []string
		reason string
	}{
		{
			name: "clean submodules",
			pass: []string{
				"git submodule deinit -f --all", "git submodule deinit -f sm",
				"git submodule update --force", "git submodule update -f -- lib",
				"git submodule--helper update --force",
			},
		},
		{
			name:  "a submodule ahead of the commit the superproject records",
			setup: func(r discardRepos) { r.commitIn(filepath.Join(r.clone, "sm"), "g") },
			pass:  []string{"git submodule update --force", "git submodule deinit -f --all"},
		},
		{
			name:  "a dirty superproject file and clean submodules",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			pass:  []string{"git submodule update --force", "git submodule deinit -f --all"},
		},
		{
			name: "a modified file in sm",
			setup: func(r discardRepos) {
				r.write(filepath.Join(r.clone, "sm"), "f", "mine\n")
			},
			deny: []string{
				"git submodule deinit -f --all", "git submodule deinit -f sm",
				"git submodule deinit -f sm/", "git submodule deinit -f .",
				"git submodule deinit -f 's*'", "git submodule update --force",
				"git submodule update -f sm", "git -c x.y=z submodule deinit -f lib/other",
				"git submodule--helper deinit lib/other",
			},
			pass: []string{
				"git submodule deinit -f lib/other", "git submodule deinit -f lib",
				"git submodule update --force lib/other", "git submodule deinit sm",
				"git submodule deinit sm -f", "git submodule update sm",
			},
			reason: "holds uncommitted work (`M f`)",
		},
		{
			name:   "a modified file in sm, from a subdirectory",
			setup:  func(r discardRepos) { r.write(filepath.Join(r.clone, "sm"), "f", "mine\n") },
			cwd:    "d",
			deny:   []string{"git submodule update --force", "git submodule deinit -f ../sm"},
			pass:   []string{"git submodule deinit -f ../lib"},
			reason: "holds uncommitted work",
		},
		{
			name: "an untracked file in sm that .gitmodules ignores",
			setup: func(r discardRepos) {
				r.git(r.clone, "config", "-f", ".gitmodules", "submodule.sm.ignore", "all")
				r.git(r.clone, "commit", "-q", "-am", "ignore sm")
				r.write(filepath.Join(r.clone, "sm"), "new", "new\n")
			},
			deny:   []string{"git submodule deinit -f sm", "git submodule update --force"},
			reason: "holds uncommitted work (`?? new`)",
		},
		{
			name: "a staged change in sm",
			setup: func(r discardRepos) {
				sm := filepath.Join(r.clone, "sm")
				r.write(sm, "f", "mine\n")
				r.git(sm, "add", "f")
			},
			deny:   []string{"git submodule deinit -f sm", "git submodule update -f"},
			reason: "holds uncommitted work (`M  f`)",
		},
		{
			name: "a submodule folder not checked out that holds a file",
			setup: func(r discardRepos) {
				r.remove("sm")
				r.local(map[string]string{"sm/stray": "stray\n"})
			},
			deny:   []string{"git submodule deinit -f sm"},
			reason: "is not checked out and holds files no repository tracks (`stray`)",
		},
		{
			name: "a submodule folder not checked out and empty, and one missing",
			setup: func(r discardRepos) {
				r.remove("sm")
				r.remove("lib/other")
				if err := os.Mkdir(filepath.Join(r.clone, "sm"), 0o755); err != nil {
					r.t.Fatal(err)
				}
			},
			pass: []string{"git submodule deinit -f --all"},
		},
		{
			name: "a submodule nested in sm not checked out that holds a file",
			setup: func(r discardRepos) {
				r.strandSubmodule(r.nestSubmodule(filepath.Join(r.clone, "sm")))
			},
			deny: []string{
				"git submodule deinit -f sm", "git submodule deinit -f --all",
				"git submodule update --force sm",
			},
			pass:   []string{"git submodule deinit -f lib/other", "git submodule deinit sm"},
			reason: "sm/n is not checked out and holds files no repository tracks (`stray`)",
		},
		{
			name: "a submodule two levels down in sm not checked out that holds a file",
			setup: func(r discardRepos) {
				sm := filepath.Join(r.clone, "sm")
				r.strandSubmodule(r.nestSubmodule(r.nestSubmodule(sm)))
				r.git(sm, "commit", "-q", "-am", "move n")
			},
			deny:   []string{"git submodule deinit -f sm"},
			reason: "sm/n/n is not checked out and holds files no repository tracks (`stray`)",
		},
		{
			name: "clean submodules nested two levels down in sm, one not checked out",
			setup: func(r discardRepos) {
				sm := filepath.Join(r.clone, "sm")
				nn := r.nestSubmodule(r.nestSubmodule(sm))
				r.removeAll(nn)
				if err := os.Mkdir(nn, 0o755); err != nil {
					r.t.Fatal(err)
				}
				r.git(sm, "commit", "-q", "-am", "move n")
			},
			pass: []string{
				"git submodule deinit -f --all", "git submodule update --force --recursive",
				"git submodule deinit -f sm",
			},
		},
		{
			name: "a submodule whose git directory names another work tree",
			setup: func(r discardRepos) {
				sm, copied := filepath.Join(r.clone, "sm"), filepath.Join(filepath.Dir(r.clone), "copied")
				r.write(copied, "f", "f\n")
				r.git(sm, "config", "core.worktree", copied)
				r.write(sm, "mine", "mine\n")
			},
			deny:   []string{"git submodule deinit -f sm", "git submodule update --force"},
			pass:   []string{"git submodule deinit -f lib/other"},
			reason: "git reads the work tree of the submodule at",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newDiscardRepos(t)
			r.addSubmodule("sm")
			r.addSubmodule("lib/other")
			r.git(r.clone, "commit", "-q", "-m", "add submodules")
			if tt.setup != nil {
				tt.setup(r)
			}
			g := NewGitDiscard(config.Config{})
			g.emit = func(_, _, _, _ string, _ map[string]string) {}
			cwd := filepath.Join(r.clone, tt.cwd)
			for _, command := range tt.pass {
				if d := g.Check(Input{Event: EventBash, Command: command, Cwd: cwd}); d != nil {
					t.Errorf("Check(%q) = %q, want it to pass", command, d.Reason)
				}
			}
			for _, command := range tt.deny {
				d := g.Check(Input{Event: EventBash, Command: command, Cwd: cwd})
				switch {
				case d == nil:
					t.Errorf("Check(%q) passed, want a deny naming %q", command, tt.reason)
				case !strings.Contains(d.Reason, tt.reason) || !strings.Contains(d.Reason, submoduleRule):
					t.Errorf("Check(%q) reason = %q, want %q and the submodule rule",
						command, d.Reason, tt.reason)
				}
			}
		})
	}
}

// TestGitDiscardMovedWorkTree pins that the clean-tree check reads the work
// tree a core.worktree already in the repository's config names: git
// status runs where the command runs, and git reads that config there too.
// A command that drops the key first moves the discard back to the folder
// with the edit, which that status did not read.
func TestGitDiscardMovedWorkTree(t *testing.T) {
	for _, tt := range []struct {
		name, moved, command string
		wantDeny             bool
	}{
		{name: "a moved tree with an edit", moved: "mine\n", wantDeny: true},
		{name: "a moved tree equal to HEAD", moved: "a\n"},
		{
			name: "a moved tree equal to HEAD, the key unset first", moved: "a\n",
			command: "git config --unset core.worktree && git checkout -- .", wantDeny: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := newDiscardRepos(t)
			moved := filepath.Join(filepath.Dir(r.clone), "moved")
			for name, content := range map[string]string{"a.txt": tt.moved, "b.txt": "b\n", "d/f.txt": "f\n"} {
				r.write(moved, name, content)
			}
			r.local(map[string]string{"a.txt": "left behind\n"})
			r.git(r.clone, "config", "core.worktree", moved)
			g := NewGitDiscard(config.Config{})
			g.emit = func(_, _, _, _ string, _ map[string]string) {}
			command := cmp.Or(tt.command, "git checkout -- .")
			d := g.Check(Input{Event: EventBash, Command: command, Cwd: r.clone})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) = %v, wantDeny %v", command, d, tt.wantDeny)
			}
		})
	}
}

// TestSubmoduleDirtSkipsFSMonitor: the submodule check's git calls run no
// core.fsmonitor program the superproject or a submodule names.
func TestSubmoduleDirtSkipsFSMonitor(t *testing.T) {
	r := newDiscardRepos(t)
	r.addSubmodule("sm")
	r.git(r.clone, "commit", "-q", "-m", "add sm")
	sm := filepath.Join(r.clone, "sm")
	marker := filepath.Join(t.TempDir(), "ran")
	monitor := filepath.Join(t.TempDir(), "monitor.sh")
	r.write(filepath.Dir(monitor), "monitor.sh", "#!/bin/sh\ntouch '"+marker+"'\n")
	if err := os.Chmod(monitor, 0o755); err != nil {
		t.Fatal(err)
	}
	r.git(r.clone, "config", "core.fsmonitor", monitor)
	r.git(sm, "config", "core.fsmonitor", monitor)
	r.git(sm, "status", "--porcelain")
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git runs no core.fsmonitor hook on status")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := submoduleDirt(context.Background(), r.clone, everySubmodule); err != nil {
		t.Fatalf("submoduleDirt = %v, want a clean pass", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the submodule check ran the core.fsmonitor program")
	}
}

// TestSubmoduleTreeDirtCeiling: a submodule folder whose .git git cannot
// read fails the check, rather than letting git status find the
// superproject above it.
func TestSubmoduleTreeDirtCeiling(t *testing.T) {
	r := newDiscardRepos(t)
	tree := filepath.Join(r.clone, "sm")
	if err := os.MkdirAll(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := submoduleTreeDirt(context.Background(), tree)
	if !errors.As(err, new(submoduleLoss)) || !strings.Contains(err.Error(), "could not read") {
		t.Errorf("submoduleTreeDirt = %v, want a read failure", err)
	}
}
