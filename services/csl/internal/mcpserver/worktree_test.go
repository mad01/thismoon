package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/outline"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/worktree/worktreetest"
)

// wtName is the name the fixture worktree's results carry.
const wtName = "org/repo@feat"

// setupWorktree builds a clone org/repo with a linked worktree on branch
// feat, configures csl to discover the clone, and isolates every csl path.
// The worktree holds an uncommitted feat.go defining BranchOnly, which the
// main clone does not have.
func setupWorktree(t *testing.T) worktreetest.Fixture {
	t.Helper()
	home := t.TempDir()
	f := worktreetest.New(t, home, "feat")
	worktreetest.Write(t, f.Worktree, "feat.go", "package main\n\nfunc BranchOnly() {}\n")
	worktreetest.Write(t, home, ".config/csl/config.yaml",
		"dirs:\n  - "+filepath.Dir(filepath.Dir(f.Main))+"\n")
	isolateConfigEnv(t, home)
	return f
}

// TestHandleSearch_Worktree: with worktree set, csl_search answers from the
// worktree's own index, uncommitted files included, names the repo
// org/repo@feat, and ignores a repo filter.
func TestHandleSearch_Worktree(t *testing.T) {
	f := setupWorktree(t)
	for _, arg := range []string{f.Worktree, "feat"} {
		in := searchInput{Query: "BranchOnly|MainOnly", Repo: "^nomatch$"}
		in.Worktree = arg
		_, out, err := handleSearch(context.Background(), nil, in)
		if err != nil {
			t.Fatalf("worktree %q: %v", arg, err)
		}
		var got []string
		for _, file := range out.Files {
			got = append(got, file.Repo+"/"+file.Path)
		}
		joined := strings.Join(got, " ")
		for _, want := range []string{wtName + "/feat.go", wtName + "/main.go"} {
			if !strings.Contains(joined, want) {
				t.Errorf("worktree %q: files %v lack %s", arg, got, want)
			}
		}
	}
}

// TestNewSearchEnvFor_MainIndexHasNoWorktree: without worktree the setup
// covers the main checkout alone, so a fleet search returns one entry per
// repo however many worktrees it has, and no worktree index gets built.
func TestNewSearchEnvFor_MainIndexHasNoWorktree(t *testing.T) {
	setupWorktree(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	env, err := newSearchEnvFor(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(env.repos) != 1 || env.repos[0].Name != "org/repo" || env.worktree {
		t.Errorf(
			"main setup repos = %+v (worktree %v), want only org/repo",
			env.repos,
			env.worktree,
		)
	}
	root, err := search.WorktreeIndexRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("a main-index setup created %s (stat err %v)", root, err)
	}
}

// TestHandleRead_Worktree: worktree reads the branch's copy of a file, and
// a call with neither repo nor worktree says what to pass.
func TestHandleRead_Worktree(t *testing.T) {
	f := setupWorktree(t)
	in := readInput{File: "feat.go"}
	in.Worktree = f.Worktree
	_, out, err := handleRead(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if out.Repo != wtName || out.TotalLines != 3 ||
		!strings.Contains(out.Lines[2].Text, "BranchOnly") {
		t.Errorf(
			"read = %s, %d lines %+v; want feat.go from %s",
			out.Repo,
			out.TotalLines,
			out.Lines,
			wtName,
		)
	}

	_, _, err = handleRead(context.Background(), nil, readInput{File: "feat.go"})
	if err == nil || !strings.Contains(err.Error(), "repo is required (or worktree") {
		t.Errorf("no repo, no worktree: err = %v, want the repo-or-worktree refusal", err)
	}
}

// TestHandleCount_Worktree: worktree counts in the worktree's own index.
func TestHandleCount_Worktree(t *testing.T) {
	f := setupWorktree(t)
	in := countInput{Query: "BranchOnly", GroupBy: "repo"}
	in.Worktree = f.Worktree
	_, out, err := handleCount(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("handleCount: %v", err)
	}
	if out.Total != 1 || len(out.Groups) != 1 || out.Groups[0].Group != wtName {
		t.Errorf("count = %+v, want 1 under %s", out, wtName)
	}
}

// TestHandleOutline_Worktree: worktree outlines the worktree's files.
func TestHandleOutline_Worktree(t *testing.T) {
	f := setupWorktree(t)
	in := outlineInput{}
	in.Worktree = f.Worktree
	_, res, err := handleOutline(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("handleOutline: %v", err)
	}
	if res.Repo != wtName || !strings.Contains(outlineNames(res.Symbols), "BranchOnly") {
		t.Errorf("outline of %s = %+v, want BranchOnly from %s", res.Repo, res.Symbols, wtName)
	}
}

// TestAsk_Worktree: csl_ask with worktree selects and reads the worktree's
// files, and a file list may name them beside the main checkout's.
func TestAsk_Worktree(t *testing.T) {
	f := setupWorktree(t)
	countingWorker(t, nil)

	res, err := Ask(context.Background(), AskRequest{
		Question: "q", Queries: []string{"BranchOnly"}, Worktree: f.Worktree,
	})
	if err != nil {
		t.Fatalf("Ask with queries: %v", err)
	}
	if len(res.Corpus.Files) != 1 || res.Corpus.Files[0].Repo != wtName {
		t.Errorf("corpus = %+v, want feat.go from %s", res.Corpus.Files, wtName)
	}

	res, err = Ask(context.Background(), AskRequest{
		Question: "q",
		FileList: []string{wtName + "/feat.go", "org/repo/main.go"},
		Worktree: f.Worktree,
	})
	if err != nil {
		t.Fatalf("Ask with a file list: %v", err)
	}
	if len(res.Corpus.Files) != 2 {
		t.Errorf(
			"corpus = %+v, want the worktree file and the main checkout file",
			res.Corpus.Files,
		)
	}
}

// TestWorktreeResults_SayWhenStale: while another call rebuilds the
// worktree's index, csl_search, csl_search_multi and csl_count answer from
// the previous build and say so in a note, and the text form leads with it.
func TestWorktreeResults_SayWhenStale(t *testing.T) {
	f := setupWorktree(t)
	in := searchInput{Query: "BranchOnly"}
	in.Worktree = f.Worktree
	if _, _, err := handleSearch(context.Background(), nil, in); err != nil {
		t.Fatalf("first search: %v", err)
	}
	worktreetest.Write(
		t,
		f.Worktree,
		"feat.go",
		"package main\n\nfunc BranchOnly() {}\n\nfunc Later() {}\n",
	)
	root, err := search.WorktreeIndexRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := search.WorktreeIndexDir(root, f.Worktree)
	release, err := search.LeaseWorktreeIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	unlock, err := search.LockWorktreeBuild(dir) // another call's build
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	_, out, err := handleSearch(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("search during a rebuild: %v", err)
	}
	if out.Total != 1 || !strings.Contains(out.Note, "previous build") {
		t.Errorf("search = %d files, note %q; want feat.go and the stale note", out.Total, out.Note)
	}
	if text := renderSearchText(out); !strings.HasPrefix(text, "note: ") {
		t.Errorf("text form = %q, want it to lead with the note", text)
	}

	multi := searchMultiInput{Queries: []searchMultiQuery{{Query: "BranchOnly"}}}
	multi.Worktree = f.Worktree
	_, mout, err := handleSearchMulti(context.Background(), nil, multi)
	if err != nil || !strings.Contains(mout.Note, "previous build") {
		t.Errorf("multi note = %q, err %v; want the stale note", mout.Note, err)
	}

	count := countInput{Query: "BranchOnly"}
	count.Worktree = f.Worktree
	_, cout, err := handleCount(context.Background(), nil, count)
	if err != nil || !strings.Contains(cout.Note, "previous build") {
		t.Errorf("count note = %q, err %v; want the stale note", cout.Note, err)
	}
}

// outlineNames joins the symbol names of an outline.
func outlineNames(symbols []outline.Entry) string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	return strings.Join(names, " ")
}
