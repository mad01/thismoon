package worktree

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/worktree/worktreetest"
)

// isolateState points csl's state directory into a temp HOME, so a test's
// worktree index never lands in the developer's state dir.
func isolateState(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	return home
}

// configFor is a loaded config whose dirs hold the fixture's main clone.
func configFor(f worktreetest.Fixture, hosts ...string) *config.Config {
	cfg := &config.Config{Dirs: []string{filepath.Dir(filepath.Dir(f.Main))}, Loaded: true}
	cfg.Index.Hosts = hosts
	return cfg
}

// excluding is configFor with hooks.post_merge.exclude set to exclude.
func excluding(f worktreetest.Fixture, exclude ...string) *config.Config {
	cfg := configFor(f)
	cfg.Hooks.PostMerge.Exclude = exclude
	return cfg
}

// TestRoot: a path anywhere inside a linked worktree resolves to its root;
// a main clone, and a path in no checkout, resolve to none.
func TestRoot(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "feat")
	worktreetest.Write(t, f.Worktree, "pkg/deep/x.go", "package deep\n")
	tests := []struct {
		name, path, want string
		ok               bool
	}{
		{"worktree root", f.Worktree, f.Worktree, true},
		{"nested dir", filepath.Join(f.Worktree, "pkg", "deep"), f.Worktree, true},
		{"missing file inside", filepath.Join(f.Worktree, "gone.go"), f.Worktree, true},
		{"main clone", f.Main, "", false},
		{"no checkout", t.TempDir(), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Root(tt.path)
			if got != tt.want || ok != tt.ok {
				t.Errorf("Root(%s) = %q, %v; want %q, %v", tt.path, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestResolve: a worktree resolves by a path inside it or by its
// org/repo@branch name; a main clone and an unknown name are refused with
// the fix, and so is a worktree whose host index.hosts leaves out.
func TestResolve(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "feat-x")
	cfg := configFor(f)

	for _, arg := range []string{f.Worktree, filepath.Join(f.Worktree, "main.go"), "org/repo@feat-x", "feat-x"} {
		wt, err := Resolve(cfg, arg)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", arg, err)
		}
		if wt.Name != "org/repo@feat-x" || wt.Path != f.Worktree {
			t.Errorf(
				"Resolve(%q) = %s at %s, want org/repo@feat-x at %s",
				arg,
				wt.Name,
				wt.Path,
				f.Worktree,
			)
		}
	}

	refusals := []struct {
		name string
		cfg  *config.Config
		arg  string
		want string
	}{
		{"main clone", cfg, f.Main, "not inside a linked git worktree"},
		{"unknown name", cfg, "nosuchbranch", "no repo matching"},
		{"empty", cfg, " ", "worktree is empty"},
		{
			"host not allowed",
			configFor(f, "gitlab.com"),
			f.Worktree,
			"host github.com not in index.hosts",
		},
		{
			"main excluded by name",
			excluding(f, "org/repo"),
			f.Worktree,
			"excluded by hooks.post_merge.exclude",
		},
		{
			"main excluded by path",
			excluding(f, f.Main),
			f.Worktree,
			"excluded by hooks.post_merge.exclude",
		},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.cfg, tt.arg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Resolve(%q) err = %v, want it to contain %q", tt.arg, err, tt.want)
			}
		})
	}
}

// TestFromWorkingDir: the CLI's --repo rule finds the worktree that holds
// the directory, and quietly falls back when the config drops its repo,
// since the caller did not name the worktree.
func TestFromWorkingDir(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "feat")
	if wt, ok := FromWorkingDir(configFor(f), f.Worktree); !ok || wt.Name != "org/repo@feat" {
		t.Errorf("FromWorkingDir(worktree) = %s, %v; want org/repo@feat", wt.Name, ok)
	}
	if _, ok := FromWorkingDir(configFor(f), f.Main); ok {
		t.Error("FromWorkingDir(main clone) found a worktree")
	}
	if _, ok := FromWorkingDir(configFor(f, "gitlab.com"), f.Worktree); ok {
		t.Error("FromWorkingDir with the host left out of index.hosts found a worktree")
	}
	if _, ok := FromWorkingDir(excluding(f, "org/repo"), f.Worktree); ok {
		t.Error("FromWorkingDir with the main checkout excluded found a worktree")
	}
}

// TestSelectsOnly: a repo filter picks the worktree only when it matches
// the worktree or its main checkout and no other discovered repo, and the
// walk runs only for a filter that matches the worktree at all.
func TestSelectsOnly(t *testing.T) {
	wt := finder.Repo{Name: "org/repo@feat", Path: "/wt/feat"}
	repos := []finder.Repo{
		{Name: "org/repo", Path: "/src/org/repo"},
		{Name: "org/repo-two", Path: "/src/org/repo-two"},
		{Name: "org/repo@feat", Path: "/wt/feat"},
	}
	tests := []struct {
		filter string
		want   bool
		walks  bool
	}{
		{"org/repo$", true, true},
		{"^ORG/REPO$", true, true},
		{"@feat", true, true},
		{"repo", false, true},       // repo-two matches too
		{"org/repo", false, true},   // a prefix of repo-two
		{"^nomatch$", false, false}, // not the worktree at all
		{"^org/repo-two$", false, false},
	}
	for _, tt := range tests {
		walked := false
		discover := func() ([]finder.Repo, error) {
			walked = true
			return repos, nil
		}
		got, err := SelectsOnly(tt.filter, wt, discover)
		if err != nil {
			t.Fatalf("SelectsOnly(%q): %v", tt.filter, err)
		}
		if got != tt.want || walked != tt.walks {
			t.Errorf(
				"SelectsOnly(%q) = %v (walked %v), want %v (walked %v)",
				tt.filter, got, walked, tt.want, tt.walks,
			)
		}
	}
	if _, err := SelectsOnly("repo(", wt, nil); err == nil {
		t.Error("SelectsOnly with a bad regex: no error")
	}
}

// TestEnsure_BuildsOnceAndFollowsEdits: the first call builds the index,
// the next reuses it, and a second edit of a file that is already modified
// rebuilds it, which git status alone would miss.
func TestEnsure_BuildsOnceAndFollowsEdits(t *testing.T) {
	isolateState(t)
	f := worktreetest.New(t, t.TempDir(), "feat")
	wt, err := Resolve(configFor(f), f.Worktree)
	if err != nil {
		t.Fatal(err)
	}

	worktreetest.Write(t, f.Worktree, "main.go", "package main\n\nfunc FirstEdit() {}\n")
	settle(t, f.Worktree) // an edit within changeSlack of the call would rebuild once more
	idx := mustEnsure(t, wt, true)
	root, err := search.WorktreeIndexRoot()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(idx.Dir)) != root {
		t.Errorf("index dir %s is not a generation under the worktree index root %s", idx.Dir, root)
	}
	mustEnsure(t, wt, false)

	// Same status line (" M main.go"), new content: size and mtime change.
	worktreetest.Write(t, f.Worktree, "main.go", "package main\n\nfunc SecondEditLonger() {}\n")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(f.Worktree, "main.go"), future, future); err != nil {
		t.Fatal(err)
	}
	mustEnsure(t, wt, true)
}

// TestEnsure_BranchSwitchDropsOldShards: after the worktree switches
// branch its name changes, and the rebuilt index holds only shards under
// the new name, so a search never returns both.
func TestEnsure_BranchSwitchDropsOldShards(t *testing.T) {
	isolateState(t)
	f := worktreetest.New(t, t.TempDir(), "first")
	cfg := configFor(f)
	wt, err := Resolve(cfg, f.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	mustEnsure(t, wt, true)

	worktreetest.Git(t, f.Worktree, "checkout", "-q", "-b", "second")
	wt, err = Resolve(cfg, f.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	idx := mustEnsure(t, wt, true)
	shards, err := filepath.Glob(filepath.Join(idx.Dir, "*.zoekt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 1 || !strings.Contains(shards[0], "second") {
		t.Errorf("shards after the switch = %v, want one under org/repo@second", shards)
	}
}

// TestEnsure_PrunesGoneWorktrees: an index whose worktree was removed is
// deleted on the next Ensure for any worktree.
func TestEnsure_PrunesGoneWorktrees(t *testing.T) {
	isolateState(t)
	f := worktreetest.New(t, t.TempDir(), "keep")
	cfg := configFor(f)
	gonePath := filepath.Join(filepath.Dir(f.Worktree), "gone")
	worktreetest.Git(t, f.Main, "worktree", "add", "-q", "-b", "gone", gonePath)
	gone, err := Resolve(cfg, gonePath)
	if err != nil {
		t.Fatal(err)
	}
	goneIdx := mustEnsure(t, gone, true)
	worktreetest.Git(t, f.Main, "worktree", "remove", gonePath)

	keep, err := Resolve(cfg, f.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	mustEnsure(t, keep, true)
	if _, err := os.Stat(goneIdx.Dir); !os.IsNotExist(err) {
		t.Errorf("index of the removed worktree still at %s (stat err %v)", goneIdx.Dir, err)
	}
}

// TestList: a clone's linked worktrees come back by name; a removed
// worktree's leftover entry does not.
func TestList(t *testing.T) {
	f := worktreetest.New(t, t.TempDir(), "one")
	other := filepath.Join(filepath.Dir(f.Worktree), "two")
	worktreetest.Git(t, f.Main, "worktree", "add", "-q", "-b", "two", other)
	if err := os.RemoveAll(other); err != nil { // leaves .git/worktrees/two behind
		t.Fatal(err)
	}
	main, err := finder.Inspect(f.Main)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, wt := range List([]finder.Repo{main}) {
		names = append(names, wt.Name)
	}
	if !slices.Equal(names, []string{"org/repo@one"}) {
		t.Errorf("List = %v, want [org/repo@one]", names)
	}
}

// TestStatusPaths: porcelain -z entries yield their paths, and a rename's
// source path is skipped.
func TestStatusPaths(t *testing.T) {
	out := []byte(" M a.go\x00?? new dir/b.go\x00R  c.go\x00old.go\x00D  d.go\x00")
	got := statusPaths(out)
	want := []string{"a.go", "new dir/b.go", "c.go", "d.go"}
	if !slices.Equal(got, want) {
		t.Errorf("statusPaths = %q, want %q", got, want)
	}
}

// mustEnsure runs Ensure, checks whether it built, and returns the index
// with its lease already ended, so a later prune in the test may take it.
func mustEnsure(t *testing.T, wt finder.Repo, wantBuilt bool) Index {
	t.Helper()
	idx, err := Ensure(wt, nil)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if idx.Built != wantBuilt {
		t.Fatalf("Ensure built = %v, want %v", idx.Built, wantBuilt)
	}
	idx.Close()
	idx.release = nil
	return idx
}
