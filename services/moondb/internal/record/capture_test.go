package record

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/ingest"
	"github.com/mad01/thismoon/services/moondb/internal/ingest/faketest"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// demoGo is a tab-indented file the Bash fixture's sed edits.
const (
	demoGo    = "package demo\n\nfunc Greet() string {\n\treturn \"hello\"\n}\n"
	demoGoSed = "package demo\n\nfunc Greet() string {\n\treturn \"hello, world\"\n}\n"
)

// bashRepo is newRepo with the files the Bash fixture's command changes
// committed, and the command's changes made on disk.
func bashRepo(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	writeFile(t, filepath.Join(repo, "demo.go"), demoGo)
	writeFile(t, filepath.Join(repo, "notes.txt"), "one\ntwo\nthree\n")
	writeFile(t, filepath.Join(repo, "gone.txt"), "bye\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "files")
	writeFile(t, filepath.Join(repo, "demo.go"), demoGoSed)
	writeFile(t, filepath.Join(repo, "notes.txt"), "one\ntwo\nthree\nfour\n")
	writeFile(t, filepath.Join(repo, "fresh.txt"), "new\n")
	if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestBashEditDiff(t *testing.T) {
	repo := bashRepo(t)
	s := newSession(t, repo)
	s.mustRun("bash_diff.json", "$DIR", repo)

	envs := s.take()
	if len(envs) != 4 {
		t.Fatalf("spool holds %v, want four edits", kinds(envs))
	}
	type edit struct {
		id, path  string
		kind      store.EditKind
		hunks     bool
		observed  string
		head      string
		headBlob  bool
		noContent bool
	}
	want := []edit{
		{"toolu_bash_diff_1:0", "demo.go", store.EditUpdate, true, demoGoSed, demoGo, true, false},
		{"toolu_bash_diff_1:1", "fresh.txt", store.EditCreate, true, "new\n", "", false, false},
		{"toolu_bash_diff_1:2", "gone.txt", store.EditDelete, true, "", "bye\n", true, true},
		{
			"toolu_bash_diff_1:3", "notes.txt", store.EditUpdate, false, "one\ntwo\nthree\nfour\n",
			"one\ntwo\nthree\n", true, false,
		},
	}
	for i, w := range want {
		env, ep := envs[i], envs[i].Edit
		if env.Kind != spool.KindEdit || env.ID != w.id || ep.Tool != "Bash" || ep.Path != w.path ||
			ep.Kind != w.kind || ep.Input != nil || ep.UserModified {
			t.Errorf("edit %d = %s %+v, want %s of %s, a Bash %s with no input",
				i, env.ID, ep, w.id, w.path, w.kind)
			continue
		}
		if (ep.PatchChunk != "") != w.hunks || (ep.HeadBlob != "") != w.headBlob ||
			(w.noContent && (ep.ObservedChunk != "" || ep.ObservedBlob != "")) {
			t.Errorf("%s: patch %q head blob %q observed %q/%q", w.path,
				ep.PatchChunk, ep.HeadBlob, ep.ObservedChunk, ep.ObservedBlob)
		}
		checkFields(t, []field{
			{w.path + " observed", s.chunk(ep.ObservedChunk), w.observed},
			{w.path + " head", s.chunk(ep.HeadChunk), w.head},
		})
	}
	var hunks []struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(s.chunk(envs[0].Edit.PatchChunk)), &hunks); err != nil ||
		len(hunks) != 1 || !slices.Contains(hunks[0].Lines, "+\treturn \"hello, world\"") {
		t.Errorf("demo.go patch = %+v (%v), want the diff's one hunk, tab kept", hunks, err)
	}
	wantPaths := strings.Join([]string{
		repo + "\tdemo.go", repo + "\tfresh.txt", repo + "\tgone.txt", repo + "\tnotes.txt", "",
	}, "\n")
	checkFields(t, []field{{".paths", s.state(pathsExt), wantPaths}})
}

func TestBashEditStampedAtHookStart(t *testing.T) {
	// The Bash hook is async, so an Edit of the same file can land between
	// the command and the hook's read. Stamped at its start, before any
	// read, the Bash edit sorts before such an Edit.
	repo := bashRepo(t)
	s := newSession(t, repo)
	s.now = stampingClock(t, filepath.Join(repo, "demo.go"))
	s.mustRun("bash_diff.json", "$DIR", repo)

	envs := s.take()
	if len(envs) != 4 {
		t.Fatalf("spool holds %v, want four edits", kinds(envs))
	}
	for _, env := range envs {
		if reading(env.At) != 1 {
			t.Errorf("%s stamped at reading %d, want 1, the hook's start", env.ID, reading(env.At))
		}
	}
	if read := readingIn(s.chunk(envs[0].Edit.ObservedChunk)); read < reading(envs[0].At) {
		t.Errorf("demo.go read holds reading %d, want one at or after its stamp %d",
			read, reading(envs[0].At))
	}
}

func TestHeadContentAfterCommit(t *testing.T) {
	// The session edited a.txt, then committed it. A Bash change after the
	// commit ships the new HEAD content, or its before has nothing to go on.
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(repo, "a.txt")
	writeFile(t, file, "one\ndeux\n")
	s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_edit_1")
	s.take()
	git(t, repo, "commit", "-q", "-am", "deux")
	writeFile(t, file, "one\ndeux\ntrois\n")
	body := `{"session_id":"sess-1","prompt_id":"prompt-1","cwd":"$DIR",` +
		`"hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"toolu_b",` +
		`"tool_input":{"command":"echo trois >> a.txt"},` +
		`"tool_response":{"bashEditDiff":{"changedFiles":["$DIR/a.txt"]}}}`
	if err := s.runRaw(strings.ReplaceAll(body, "$DIR", repo)); err != nil {
		t.Fatal(err)
	}
	ep := s.takeOne(spool.KindEdit).Edit
	checkFields(t, []field{
		{"HeadBlob", ep.HeadBlob, git(t, repo, "rev-parse", "HEAD:a.txt")},
		{"head", s.chunk(ep.HeadChunk), "one\ndeux\n"},
	})
}

func TestBashEditDiffExcludesAndShapes(t *testing.T) {
	repo := bashRepo(t)

	t.Run("excluded path ships no content", func(t *testing.T) {
		s := newSession(t, repo)
		s.excludes = []string{"notes.txt", "demo.go"}
		s.mustRun("bash_diff.json", "$DIR", repo)
		for _, env := range s.take() {
			ep := env.Edit
			excluded := ep.Path == "notes.txt" || ep.Path == "demo.go"
			if ep.Excluded != excluded ||
				(excluded && (ep.PatchChunk != "" || ep.ObservedChunk != "" || ep.HeadBlob != "")) {
				t.Errorf("%s: excluded %v with patch %q observed %q head %q",
					ep.Path, ep.Excluded, ep.PatchChunk, ep.ObservedChunk, ep.HeadBlob)
			}
		}
	})

	cases := []struct {
		name string
		diff string
		want []string
	}{
		{"no diff", `null`, nil},
		{"diff of the wrong type", `"skipped"`, nil},
		{"skipped command", `{"skipped": true, "changedFiles": []}`, nil},
		{
			"paths only", `{"changedFiles": ["$DIR/notes.txt", 7, "$DIR/notes.txt"]}`,
			[]string{"notes.txt"},
		},
		{"file entry without a path", `{"files": [{"hunks": []}, "x"],` +
			` "changedFiles": ["$DIR/demo.go"]}`, []string{"demo.go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSession(t, repo)
			body := `{"session_id":"sess-1","prompt_id":"prompt-1","cwd":"$DIR",` +
				`"hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"toolu_b",` +
				`"tool_input":{"command":"true"},"tool_response":{"bashEditDiff":` + tc.diff + `}}`
			if err := s.runRaw(strings.ReplaceAll(body, "$DIR", repo)); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, env := range s.take() {
				got = append(got, env.Edit.Path)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("edits of %v, want %v", got, tc.want)
			}
		})
	}
}

// pulledRepo is a repo whose main took a fast-forward merge of a branch
// that changed demo.go and notes.txt, added fresh.txt, and removed gone.txt
// and the whole old/ directory: what a pull leaves behind, every file clean.
func pulledRepo(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	writeFile(t, filepath.Join(repo, "demo.go"), demoGo)
	writeFile(t, filepath.Join(repo, "notes.txt"), "one\ntwo\nthree\n")
	writeFile(t, filepath.Join(repo, "gone.txt"), "bye\n")
	writeFile(t, filepath.Join(repo, "old", "deep", "x.txt"), "x\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "files")
	git(t, repo, "checkout", "-q", "-b", "upstream")
	writeFile(t, filepath.Join(repo, "demo.go"), demoGoSed)
	writeFile(t, filepath.Join(repo, "notes.txt"), "one\ntwo\nthree\nfour\n")
	writeFile(t, filepath.Join(repo, "fresh.txt"), "new\n")
	git(t, repo, "rm", "-q", "-r", "gone.txt", "old")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "upstream")
	git(t, repo, "checkout", "-q", "main")
	git(t, repo, "merge", "-q", "--ff-only", "upstream")
	return repo
}

func TestBashPullRecordsNoEdits(t *testing.T) {
	// A pull rewrites files to their committed state. The harness lists
	// them all, but the command authored none of them, so the hook writes
	// no edit and only counts them, the file in the removed directory too.
	// Reached through a symlink, the removed directory's file still
	// resolves into the worktree (/tmp and /var are symlinks on macOS).
	for _, viaLink := range []bool{false, true} {
		t.Run(fmt.Sprintf("via symlink %v", viaLink), func(t *testing.T) {
			repo := pulledRepo(t)
			if _, err := os.Stat(filepath.Join(repo, "old")); err == nil {
				t.Fatal("the merge left old/ in place; the test needs it gone")
			}
			dir := repo
			if viaLink {
				dir = symlinkTo(t, repo)
			}
			s := newSession(t, dir)
			s.mustRun("bash_pull.json", "$DIR", dir)
			checkPullSkipped(t, s)
		})
	}
}

// symlinkTo returns a new symlink to target.
func symlinkTo(t *testing.T, target string) string {
	t.Helper()
	link := filepath.Join(physicalTempDir(t), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// checkPullSkipped checks that the bash_pull.json call wrote only its
// skipped count, five clean files, and logged it.
func checkPullSkipped(t *testing.T, s *testSession) {
	t.Helper()
	env := s.takeOne(spool.KindSkipped)
	want := store.SkippedFiles{Tool: "Bash", ToolUseID: "toolu_bash_pull_1", Clean: 5}
	if env.ID != "toolu_bash_pull_1:skipped" || env.Turn.ID != "prompt-1" || *env.Skipped != want {
		t.Errorf("skipped envelope %s of turn %s = %+v, want %+v",
			env.ID, env.Turn.ID, *env.Skipped, want)
	}
	if paths := s.state(pathsExt); paths != "" {
		t.Errorf(".paths = %q, want nothing noted", paths)
	}
	wantLogs := []string{
		"Bash toolu_bash_pull_1: skipped 5 changed files: " +
			"5 clean after the command, 0 past the budget, 0 status failed",
	}
	if !slices.Equal(s.logs, wantLogs) {
		t.Errorf("logged %q, want %q", s.logs, wantLogs)
	}
}

// bashPayload is a Bash PostToolUse payload run in cwd whose diff lists
// changed by path alone.
func bashPayload(cwd, toolUseID string, changed ...string) string {
	dir, _ := json.Marshal(cwd)
	names, _ := json.Marshal(changed)
	return `{"session_id":"sess-1","prompt_id":"prompt-1","cwd":` + string(dir) + `,` +
		`"hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"` + toolUseID + `",` +
		`"tool_input":{"command":"true"},` +
		`"tool_response":{"bashEditDiff":{"changedFiles":` + string(names) + `}}}`
}

// bashEdit is what a test expects of one edit envelope a Bash call wrote.
type bashEdit struct {
	id, path string
	kind     store.EditKind
	placed   bool
}

// checkBashEdits compares the edit envelopes in envs with want, in order,
// and returns the envelopes left after them.
func checkBashEdits(t *testing.T, envs []spool.Envelope, want []bashEdit) []spool.Envelope {
	t.Helper()
	if len(envs) < len(want) {
		t.Fatalf("spool holds %v, want %d edits first", kinds(envs), len(want))
	}
	for i, w := range want {
		env, ep := envs[i], envs[i].Edit
		if env.Kind != spool.KindEdit || env.ID != w.id || ep.Path != w.path || ep.Kind != w.kind ||
			(ep.Location.Worktree != "") != w.placed {
			t.Errorf("envelope %d = %s %s %+v, want edit %s of %s, a %s, placed %v",
				i, env.Kind, env.ID, ep, w.id, w.path, w.kind, w.placed)
		}
	}
	return envs[len(want):]
}

func TestBashKeepsOnlyWhatStaysChanged(t *testing.T) {
	repo := newRepo(t)
	for _, name := range []string{"clean.txt", "dirty.txt", "gone.txt"} {
		writeFile(t, filepath.Join(repo, name), name+"\n")
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "files")
	writeFile(t, filepath.Join(repo, "dirty.txt"), "dirty, then edited\n")
	writeFile(t, filepath.Join(repo, "new.txt"), "untracked\n")
	if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	s := newSession(t, repo)
	var paths []string
	for _, name := range []string{"clean.txt", "dirty.txt", "new.txt", "gone.txt"} {
		paths = append(paths, filepath.Join(repo, name))
	}
	if err := s.runRaw(bashPayload(repo, "toolu_mixed", paths...)); err != nil {
		t.Fatal(err)
	}

	// Each edit keeps the file's position in the payload's list.
	rest := checkBashEdits(t, s.take(), []bashEdit{
		{"toolu_mixed:1", "dirty.txt", store.EditUpdate, true},
		{"toolu_mixed:2", "new.txt", store.EditUpdate, true},
		{"toolu_mixed:3", "gone.txt", store.EditDelete, true},
	})
	want := store.SkippedFiles{Tool: "Bash", ToolUseID: "toolu_mixed", Clean: 1}
	if len(rest) != 1 || rest[0].Kind != spool.KindSkipped || *rest[0].Skipped != want {
		t.Fatalf("after the edits the spool holds %v, want one skipped %+v", kinds(rest), want)
	}
}

func TestBashKeepsIgnoredRenamedAndOutsideFiles(t *testing.T) {
	// git status leaves out none of these: an ignored build output, both
	// sides of a staged rename, and a file outside any repository, which
	// git cannot check at all.
	repo := newRepo(t)
	writeFile(t, filepath.Join(repo, ".gitignore"), "build/\n")
	writeFile(t, filepath.Join(repo, "old.txt"), "moved\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "files")
	git(t, repo, "mv", "old.txt", "renamed.txt")
	writeFile(t, filepath.Join(repo, "build", "out.bin"), "built\n")
	outside := filepath.Join(physicalTempDir(t), "free.txt")
	writeFile(t, outside, "free\n")
	s := newSession(t, repo)
	paths := []string{
		filepath.Join(repo, "old.txt"), filepath.Join(repo, "renamed.txt"),
		filepath.Join(repo, "build", "out.bin"), outside,
	}
	if err := s.runRaw(bashPayload(repo, "toolu_edges", paths...)); err != nil {
		t.Fatal(err)
	}

	rest := checkBashEdits(t, s.take(), []bashEdit{
		{"toolu_edges:0", "old.txt", store.EditDelete, true},
		{"toolu_edges:1", "renamed.txt", store.EditUpdate, true},
		{"toolu_edges:2", "build/out.bin", store.EditUpdate, true},
		{"toolu_edges:3", outside, store.EditUpdate, false},
	})
	if len(rest) != 0 || len(s.logs) != 0 {
		t.Errorf(
			"after the edits the spool holds %v and the log %q, want neither",
			kinds(rest),
			s.logs,
		)
	}
}

func TestBashGoneFileThroughSymlinkStaysPlaced(t *testing.T) {
	// The command removed a whole directory and the session reaches the
	// repo through a symlink: the file still lands in its worktree, as a
	// delete, not under an absolute path in none.
	repo := newRepo(t)
	writeFile(t, filepath.Join(repo, "old", "deep", "x.txt"), "x\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "files")
	if err := os.RemoveAll(filepath.Join(repo, "old")); err != nil {
		t.Fatal(err)
	}
	link := symlinkTo(t, repo)
	s := newSession(t, link)
	body := bashPayload(link, "toolu_rm", filepath.Join(link, "old", "deep", "x.txt"))
	if err := s.runRaw(body); err != nil {
		t.Fatal(err)
	}

	rest := checkBashEdits(t, s.take(), []bashEdit{
		{"toolu_rm:0", "old/deep/x.txt", store.EditDelete, true},
	})
	if len(rest) != 0 {
		t.Errorf("after the edit the spool holds %v, want nothing", kinds(rest))
	}
}

func TestBashGoneFileInNoWorktreeIsFailed(t *testing.T) {
	// A removed linked worktree whose parent is no repository, the
	// .worktrees layout, and a loose file the command deleted: git can
	// check neither, and all that is left of them is an absolute path, so
	// the hook counts them and writes no edit.
	repo := newRepo(t)
	wt := filepath.Join(physicalTempDir(t), ".worktrees", "demo", "feat")
	git(t, repo, "worktree", "add", "-q", "-b", "feat", wt)
	writeFile(t, filepath.Join(wt, "sub", "new.txt"), "new\n")
	git(t, repo, "worktree", "remove", "--force", wt)
	loose := filepath.Join(physicalTempDir(t), "loose.txt")
	s := newSession(t, repo)
	paths := []string{
		filepath.Join(wt, "a.txt"), filepath.Join(wt, "sub", "new.txt"), loose,
	}
	if err := s.runRaw(bashPayload(wt, "toolu_wt_rm", paths...)); err != nil {
		t.Fatal(err)
	}

	env := s.takeOne(spool.KindSkipped)
	want := store.SkippedFiles{Tool: "Bash", ToolUseID: "toolu_wt_rm", Failed: 3}
	if *env.Skipped != want {
		t.Errorf("skipped = %+v, want %+v", *env.Skipped, want)
	}
	if paths := s.state(pathsExt); paths != "" {
		t.Errorf(".paths = %q, want nothing noted", paths)
	}
}

// gitCounter puts a git on PATH that logs the first two arguments of every
// call to the returned file before it runs the real git.
func gitCounter(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	bin := physicalTempDir(t)
	log := filepath.Join(bin, "calls.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$1 $2\" >> %q\nexec %q \"$@\"\n", log, gitPath)
	writeFile(t, filepath.Join(bin, "git"), script)
	if err := os.Chmod(filepath.Join(bin, "git"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A new executable's first launch can wait on the system's check of
	// it; take that wait here, not under the budget.
	if err := exec.Command(filepath.Join(bin, "git"), "version").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// locateCalls counts the location lookups in a gitCounter log: each runs
// one rev-parse with --path-format.
func locateCalls(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "rev-parse --path-format=absolute\n")
}

func TestBashLocatesEachWorktreeOnce(t *testing.T) {
	// A command that changes files across many directories costs one git
	// location lookup per worktree, not one per directory: once a
	// worktree is located, its files are placed by path. A nested
	// repository is located on its own. A directory outside any
	// repository is located once for all its files, and placement and
	// capture share that answer.
	repo := newRepo(t)
	var paths []string
	for i := range 12 {
		name := filepath.Join(repo, fmt.Sprintf("dir%d", i), "sub", "f.txt")
		writeFile(t, name, "changed\n")
		paths = append(paths, name)
	}
	inner := filepath.Join(repo, "vendor", "inner")
	git(t, repo, "init", "-q", "-b", "main", inner)
	git(t, inner, "commit", "-q", "--allow-empty", "-m", "init")
	writeFile(t, filepath.Join(inner, "deep", "g.txt"), "nested\n")
	paths = append(paths, filepath.Join(inner, "deep", "g.txt"))
	free := physicalTempDir(t)
	for _, name := range []string{"one.txt", "two.txt"} {
		writeFile(t, filepath.Join(free, name), "free\n")
		paths = append(paths, filepath.Join(free, name))
	}
	log := gitCounter(t)
	s := newSession(t, repo)
	if err := s.runRaw(bashPayload(repo, "toolu_wide", paths...)); err != nil {
		t.Fatal(err)
	}

	envs := s.take()
	if len(envs) != len(paths) || slices.Contains(kinds(envs), spool.KindSkipped) {
		t.Fatalf("spool holds %v, want %d edits", kinds(envs), len(paths))
	}
	nestedID := fmt.Sprintf("toolu_wide:%d", len(paths)-3)
	i := slices.IndexFunc(envs, func(e spool.Envelope) bool { return e.ID == nestedID })
	if i < 0 || envs[i].Edit.Location.Worktree != inner || envs[i].Edit.Path != "deep/g.txt" {
		t.Errorf("edit %s = %+v, want deep/g.txt in %q", nestedID, envs[max(i, 0)].Edit, inner)
	}
	if n := locateCalls(t, log); n != 3 {
		t.Errorf("%d location lookups, want 3: the repo, the nested repo, the outside dir", n)
	}
}

func TestCaptureReportsCutBeforeRead(t *testing.T) {
	// A budget spent before the disk read is reported, so the Bash hook
	// can count the file instead of writing an edit without its read.
	dir := physicalTempDir(t)
	file := filepath.Join(dir, "f.txt")
	writeFile(t, file, "body\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := &recorder{
		ctx:  ctx,
		p:    payload{tool: toolBash, toolUseID: "toolu_cut"},
		opts: Options{}.withDefaults(),
		locs: map[string]store.Location{},
	}
	where := placement{abs: file, rel: file}
	env, _, cut := rec.captureEdit(target{name: file, where: &where})
	if !cut || env.Edit.ObservedChunk != "" {
		t.Errorf("cut = %v, observed %q; want a cut before any read", cut, env.Edit.ObservedChunk)
	}
}

func TestBashBudgetSpentWritesNoUnplacedEdit(t *testing.T) {
	// With the budget gone before git places anything, an edit would land
	// under an absolute path in no worktree. The hook counts the files
	// instead.
	repo := bashRepo(t)
	s := newSession(t, repo)
	s.budget = time.Nanosecond
	s.mustRun("bash_diff.json", "$DIR", repo)

	env := s.takeOne(spool.KindSkipped)
	want := store.SkippedFiles{Tool: "Bash", ToolUseID: "toolu_bash_diff_1", Timeout: 4}
	if *env.Skipped != want {
		t.Errorf("skipped = %+v, want %+v", *env.Skipped, want)
	}
	if paths := s.state(pathsExt); paths != "" {
		t.Errorf(".paths = %q, want nothing noted", paths)
	}
}

func TestBashBudgetIsItsOwn(t *testing.T) {
	// The Bash hook nobody waits on gets BashBudget even when the Edit
	// hook's budget is gone.
	repo := bashRepo(t)
	s := newSession(t, repo)
	body := readFile(t, filepath.Join("testdata", "bash_diff.json"))
	body = strings.NewReplacer("$DIR", repo, "$CWD", repo, "$TRANSCRIPT", s.transcript).
		Replace(body)
	err := Run(context.Background(), strings.NewReader(body), Options{
		Workdir: s.workdir, Budget: time.Nanosecond, BashBudget: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(s.take()); len(got) != 4 || slices.Contains(got, spool.KindSkipped) {
		t.Errorf("spool holds %v, want four edits and no skipped count", got)
	}
}

func TestSnapshots(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, filepath.Join(repo, "b.txt"), "bee\n")
	git(t, repo, "add", "b.txt")
	git(t, repo, "commit", "-q", "-m", "b")
	head := git(t, repo, "rev-parse", "HEAD")
	s := newSession(t, repo)

	s.mustRun("prompt.json")
	if got := kinds(s.take()); !slices.Equal(got, []spool.Kind{spool.KindTurnOpen}) {
		t.Fatalf("a clean worktree the session never touched wrote %v, want only turn-open", got)
	}

	writeFile(t, filepath.Join(repo, "a.txt"), "one\nzwei\n")
	writeFile(t, filepath.Join(repo, "u.txt"), "untracked\n")
	writeFile(t, filepath.Join(repo, ".env"), "SECRET=1\n")
	writeFile(t, filepath.Join(repo, "big.bin"), strings.Repeat("x", moondb.MaxChunkBytes+1))
	if err := os.Remove(filepath.Join(repo, "b.txt")); err != nil {
		t.Fatal(err)
	}
	s.mustRun("prompt.json", "prompt-1", "prompt-2")
	envs := s.take()
	if !slices.Equal(kinds(envs), []spool.Kind{spool.KindTurnOpen, spool.KindSnapshot}) {
		t.Fatalf("prompt wrote %v, want turn-open then snapshot", kinds(envs))
	}
	env := envs[1]
	snap := env.Snapshot
	if env.ID != snapshotID("prompt-2", store.SnapshotPre, snap) || env.Turn.ID != "prompt-2" ||
		snap.Phase != store.SnapshotPre || snap.Location.Worktree != repo ||
		snap.Location.Head != head || snap.Scope != store.SnapshotScope ||
		snap.StateHash == "" || snap.Truncated || !env.At.Equal(testNow) ||
		!snap.Started.Equal(testNow) {
		t.Errorf("snapshot %s = %+v", env.ID, snap)
	}
	wantSkipped := store.SnapshotSkipped{Excluded: 1, TooLarge: 1}
	if snap.Skipped != wantSkipped {
		t.Errorf("Skipped = %+v, want %+v", snap.Skipped, wantSkipped)
	}
	checkSnapshotFiles(t, s, snap.Files, []snapFile{
		{path: ".env", status: "??", reason: store.ReasonExcluded},
		{path: "a.txt", status: ".M", content: "one\nzwei\n", head: "one\ntwo\n"},
		{path: "b.txt", status: ".D", deleted: true, head: "bee\n"},
		{path: "big.bin", status: "??", reason: store.ReasonTooLarge},
		{path: "u.txt", status: "??", content: "untracked\n"},
	})

	s.mustRun("stop.json", "prompt-1", "prompt-2")
	if got := kinds(s.take()); !slices.Equal(got, []spool.Kind{spool.KindTurnClose}) {
		t.Errorf("a stop with nothing changed wrote %v, want only turn-close", got)
	}

	// u.txt changes again but stays untracked, which the status alone would
	// not show. a.txt is unchanged and its chunk is in the store, so it is
	// not read again: making it unreadable shows that.
	aChunk := snap.Files[1].Chunk
	storeChunk(t, s.workdir, aChunk)
	if err := os.Chmod(filepath.Join(repo, "a.txt"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(repo, "a.txt"), 0o600) })
	writeFile(t, filepath.Join(repo, "u.txt"), "untracked, again\n")
	s.mustRun("stop.json", "prompt-1", "prompt-3")
	envs = s.take()
	if !slices.Equal(kinds(envs), []spool.Kind{spool.KindTurnClose, spool.KindSnapshot}) {
		t.Fatalf("stop wrote %v, want turn-close then snapshot", kinds(envs))
	}
	if envs[1].Snapshot.Phase != store.SnapshotPost {
		t.Errorf("phase = %q, want post", envs[1].Snapshot.Phase)
	}
	files := envs[1].Snapshot.Files
	if len(files) != 5 || files[1].Chunk != aChunk ||
		files[1].HeadChunk != snap.Files[1].HeadChunk ||
		s.chunk(files[4].Chunk) != "untracked, again\n" {
		t.Errorf("files = %+v, want a.txt from its stamp and u.txt read again", files)
	}
}

func TestSnapshotStampedAfterItsReads(t *testing.T) {
	// An Edit stamped before a snapshot's At is one the snapshot may have
	// seen, so At must come after the reads.
	repo := newRepo(t)
	s := newSession(t, repo)
	s.now = stampingClock(t, filepath.Join(repo, "u.txt"))
	s.mustRun("prompt.json")
	envs := s.take()
	if !slices.Equal(kinds(envs), []spool.Kind{spool.KindTurnOpen, spool.KindSnapshot}) {
		t.Fatalf("prompt wrote %v, want turn-open then snapshot", kinds(envs))
	}
	env := envs[1]
	files := env.Snapshot.Files
	if len(files) != 1 || files[0].Path != "u.txt" {
		t.Fatalf("files = %+v, want u.txt alone", files)
	}
	started, read, at := reading(env.Snapshot.Started), readingIn(s.chunk(files[0].Chunk)),
		reading(env.At)
	if started > read || read >= at {
		t.Errorf("started at reading %d, read reading %d, stamped at reading %d; "+
			"want the read between the start and the stamp", started, read, at)
	}
}

func TestSecondStopKeepsBothSnapshots(t *testing.T) {
	// A Stop hook that blocks lets the turn go on, and its next Stop comes
	// with the same prompt id: both post snapshots must be stored.
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(repo, "a.txt")
	writeFile(t, file, "one\nzwei\n")
	s.mustRun("prompt.json")
	writeFile(t, file, "one\ndrei\n")
	s.mustRun("stop.json")
	writeFile(t, file, "one\nvier\n")
	s.mustRun("stop.json")
	envs := s.take()

	st := faketest.New()
	in := &ingest.Ingester{Store: st, Spool: s.reader()}
	ctx := context.Background()
	for _, env := range envs {
		if err := in.Ingest(ctx, env); err != nil {
			t.Fatalf("ingest %s: %v", env.ID, err)
		}
	}
	td, err := st.Turn(ctx, "prompt-1")
	if err != nil {
		t.Fatal(err)
	}
	var phases []store.SnapshotPhase
	for _, snap := range td.Snapshots {
		phases = append(phases, snap.Phase)
	}
	slices.Sort(phases)
	want := []store.SnapshotPhase{store.SnapshotPost, store.SnapshotPost, store.SnapshotPre}
	if !slices.Equal(phases, want) {
		t.Errorf("stored snapshots %v, want %v", phases, want)
	}

	writes := st.Writes()
	if err := in.Ingest(ctx, envs[len(envs)-1]); err != nil {
		t.Fatal(err)
	}
	if got := st.Writes(); got != writes {
		t.Errorf("replaying the last snapshot wrote %d records, want none", got-writes)
	}
}

func TestSnapshotListsWhatItCannotRead(t *testing.T) {
	// A path the snapshot leaves out was clean, so one it cannot read must
	// still be listed, without content and with the reason.
	repo := newRepo(t)
	s := newSession(t, repo)
	if err := os.Symlink("a.txt", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(repo, "locked.txt")
	writeFile(t, locked, "secret\n")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })
	// A touched path that is a directory now: git status shows no empty
	// directory, so only the session's own list puts it in scope.
	if err := os.Mkdir(filepath.Join(repo, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(s.workdir, "sessions", "sess-1"+pathsExt), repo+"\tsub\n")

	s.mustRun("prompt.json")
	envs := s.take()
	if !slices.Equal(kinds(envs), []spool.Kind{spool.KindTurnOpen, spool.KindSnapshot}) {
		t.Fatalf("prompt wrote %v, want turn-open then snapshot", kinds(envs))
	}
	snap := envs[1].Snapshot
	if snap.Truncated {
		t.Errorf("snapshot truncated, want whole: %+v", snap)
	}
	checkSnapshotFiles(t, s, snap.Files, []snapFile{
		{path: "link", status: "??", reason: store.ReasonSymlink},
		{path: "locked.txt", status: "??", reason: store.ReasonUnreadable},
		{path: "sub", reason: store.ReasonNotRegular},
	})
}

func TestSnapshotOfCleanedWorktree(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	writeFile(t, filepath.Join(repo, "u.txt"), "untracked\n")
	s.mustRun("prompt.json")
	if got := kinds(s.take()); !slices.Equal(
		got,
		[]spool.Kind{spool.KindTurnOpen, spool.KindSnapshot},
	) {
		t.Fatalf("prompt wrote %v, want turn-open then snapshot", got)
	}
	git(t, repo, "add", "u.txt")
	git(t, repo, "commit", "-q", "-m", "u")

	// The worktree is clean now. The session snapshotted it before, so an
	// empty snapshot records that every path it held is clean.
	s.mustRun("stop.json")
	envs := s.take()
	if len(envs) != 2 || envs[1].Kind != spool.KindSnapshot || len(envs[1].Snapshot.Files) != 0 {
		t.Fatalf("stop wrote %v, want turn-close and an empty snapshot", kinds(envs))
	}
}

func TestSnapshotFileCapAndBudget(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	rec := &recorder{
		base: context.Background(), ctx: context.Background(), opts: Options{}.withDefaults(),
		state: sessionState{dir: filepath.Join(s.workdir, "sessions"), id: "sess-1"},
		spool: &spool.Writer{Dir: filepath.Join(s.workdir, "spool")},
		locs:  map[string]store.Location{},
	}
	var entries []statusEntry
	for i := range moondb.MaxSnapshotFiles + 2 {
		entries = append(entries, statusEntry{path: fmt.Sprintf("f%04d", i), xy: "??"})
	}
	scope, tooMany := rec.scope(repo, entries)
	if len(scope) != moondb.MaxSnapshotFiles || tooMany != 2 {
		t.Errorf("scope of %d paths, %d past the cap; want %d and 2",
			len(scope), tooMany, moondb.MaxSnapshotFiles)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := rec.within(ctx).read(repo, scope[:3], stampAll(repo, scope[:3]), snapState{})
	if len(b.files) != 0 || b.skipped.Timeout != 3 {
		t.Errorf("read with the budget spent = %d files, timeout %d; want none and 3",
			len(b.files), b.skipped.Timeout)
	}
}

func TestParseStatus(t *testing.T) {
	hex := strings.Repeat("a", 40)
	ord := "1 .M N... 100644 100644 100644 " + hex + " " + hex + " dir/a file.txt"
	renamed := "2 R. N... 100644 100644 100644 " + hex + " " + hex + " R100 new.txt"
	copied := "2 C. N... 100644 100644 100644 " + hex + " " + hex + " C75 copy.txt"
	unmerged := "u UU N... 100644 100644 100644 100644 " + hex + " " + hex + " " + hex + " both.txt"
	sub := "1 .M S.M. 160000 160000 160000 " + hex + " " + hex + " vendor/sub"
	out := strings.Join([]string{
		ord, renamed, "old.txt", copied, "src.txt", unmerged, "? untracked.txt", sub,
		"! ignored", "x junk", "",
	}, "\x00")
	got := parseStatus([]byte(out))
	want := []statusEntry{
		{path: "dir/a file.txt", xy: ".M"},
		{path: "new.txt", xy: "R.", orig: "old.txt"},
		{path: "copy.txt", xy: "C."},
		{path: "both.txt", xy: "UU"},
		{path: "untracked.txt", xy: "??"},
		{path: "ignored", xy: "!!"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseStatus = %+v\nwant %+v", got, want)
	}
	if got := parseStatus(nil); len(got) != 0 {
		t.Errorf("parseStatus(nil) = %+v, want none", got)
	}
}

func TestParseLsTree(t *testing.T) {
	oid := strings.Repeat("b", 40)
	out := strings.Join([]string{
		"100644 blob " + oid + "      12\tdir/a file.txt",
		"100755 blob " + oid + "       0\tst*r.sh",
		"160000 commit " + oid + "       -\tvendor/sub",
		"040000 tree " + oid + "       -\tdir",
		"junk",
		"",
	}, "\x00")
	got := parseLsTree([]byte(out))
	want := map[string]treeBlob{
		"dir/a file.txt": {blob: oid, size: 12},
		"st*r.sh":        {blob: oid, size: 0},
	}
	if !maps.Equal(got, want) {
		t.Errorf("parseLsTree = %+v\nwant %+v", got, want)
	}
}

func TestReadBatch(t *testing.T) {
	one, two := "one\n", ""
	idOne, idTwo := blobID([]byte(one)), blobID([]byte(two))
	out := idOne + " blob 4\n" + one + "\n" +
		strings.Repeat("c", 40) + " missing\n" +
		idOne + " blob 4\nbad!\n" + // content that does not match its id
		idTwo + " blob 0\n\n"
	got := readBatch(bufio.NewReader(strings.NewReader(out)), 4)
	if len(got) != 2 || string(got[idOne]) != one || string(got[idTwo]) != two {
		t.Errorf("readBatch = %q, want both verified blobs", got)
	}
	cut := idOne + " blob 4\non"
	if got := readBatch(bufio.NewReader(strings.NewReader(cut)), 1); len(got) != 0 {
		t.Errorf("readBatch of a cut answer = %q, want none", got)
	}
}

// snapFile is one expected entry of a snapshot's file list. An entry with
// a reason or deleted holds no content; head is the HEAD content it ships,
// "" for a path HEAD does not track.
type snapFile struct {
	path, status, content string
	deleted               bool
	reason                store.SnapshotReason
	head                  string
}

func checkSnapshotFiles(t *testing.T, s *testSession, got []store.SnapshotFile, want []snapFile) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("files = %+v, want %d", got, len(want))
	}
	for i, w := range want {
		f := got[i]
		if f.Path != w.path || f.Status != w.status || f.Deleted != w.deleted ||
			f.Reason != w.reason {
			t.Errorf("file %d = %+v, want %+v", i, f, w)
			continue
		}
		switch {
		case w.deleted || w.reason != "":
			if f.Chunk != "" || f.Blob != "" {
				t.Errorf("%s: chunk %q blob %q, want neither", f.Path, f.Chunk, f.Blob)
			}
		case s.chunk(f.Chunk) != w.content || f.Blob != blobID([]byte(w.content)) ||
			f.Size != int64(len(w.content)):
			t.Errorf("%s = %+v holding %q, want %q", f.Path, f, s.chunk(f.Chunk), w.content)
		}
		wantBlob := ""
		if w.head != "" {
			wantBlob = blobID([]byte(w.head))
		}
		if f.HeadBlob != wantBlob || s.chunk(f.HeadChunk) != w.head {
			t.Errorf("%s: head blob %q holding %q, want %q", f.Path, f.HeadBlob,
				s.chunk(f.HeadChunk), w.head)
		}
	}
}

// storeChunk puts an object file for key where the store keeps it, which
// is all the spool writer checks.
func storeChunk(t *testing.T, workdir, key string) {
	t.Helper()
	writeFile(t, filepath.Join(workdir, "objects", key[:2], key+".gz"), "")
}
