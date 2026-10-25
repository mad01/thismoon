package record

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
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
	if env.ID != snapshotID("prompt-2", store.SnapshotPre, repo) || env.Turn.ID != "prompt-2" ||
		snap.Phase != store.SnapshotPre || snap.Location.Worktree != repo ||
		snap.Location.Head != head || snap.Scope != store.SnapshotScope ||
		snap.StateHash == "" || snap.Truncated || !env.At.Equal(testNow) {
		t.Errorf("snapshot %s = %+v", env.ID, snap)
	}
	wantSkipped := store.SnapshotSkipped{Excluded: 1, TooLarge: 1}
	if snap.Skipped != wantSkipped {
		t.Errorf("Skipped = %+v, want %+v", snap.Skipped, wantSkipped)
	}
	checkSnapshotFiles(t, s, snap.Files, []snapFile{
		{path: "a.txt", status: ".M", content: "one\nzwei\n"},
		{path: "b.txt", status: ".D", deleted: true},
		{path: "big.bin", status: "??", noContent: true},
		{path: "u.txt", status: "??", content: "untracked\n"},
	})

	s.mustRun("stop.json", "prompt-1", "prompt-2")
	if got := kinds(s.take()); !slices.Equal(got, []spool.Kind{spool.KindTurnClose}) {
		t.Errorf("a stop with nothing changed wrote %v, want only turn-close", got)
	}

	// u.txt changes again but stays untracked, which the status alone would
	// not show. a.txt is unchanged and its chunk is in the store, so it is
	// not read again: making it unreadable shows that.
	aChunk := snap.Files[0].Chunk
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
	if len(files) != 4 || files[0].Chunk != aChunk ||
		s.chunk(files[3].Chunk) != "untracked, again\n" {
		t.Errorf("files = %+v, want a.txt from its stamp and u.txt read again", files)
	}
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
	scope, skipped := rec.scope(repo, entries)
	if len(scope) != moondb.MaxSnapshotFiles || skipped.TooMany != 2 {
		t.Errorf("scope of %d paths, %d past the cap; want %d and 2",
			len(scope), skipped.TooMany, moondb.MaxSnapshotFiles)
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
	unmerged := "u UU N... 100644 100644 100644 100644 " + hex + " " + hex + " " + hex + " both.txt"
	sub := "1 .M S.M. 160000 160000 160000 " + hex + " " + hex + " vendor/sub"
	out := strings.Join([]string{
		ord, renamed, "old.txt", unmerged, "? untracked.txt", sub, "! ignored", "x junk", "",
	}, "\x00")
	got := parseStatus([]byte(out))
	want := []statusEntry{
		{path: "dir/a file.txt", xy: ".M"},
		{path: "new.txt", xy: "R."},
		{path: "both.txt", xy: "UU"},
		{path: "untracked.txt", xy: "??"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseStatus = %+v\nwant %+v", got, want)
	}
	if got := parseStatus(nil); len(got) != 0 {
		t.Errorf("parseStatus(nil) = %+v, want none", got)
	}
}

// snapFile is one expected entry of a snapshot's file list.
type snapFile struct {
	path, status, content string
	deleted, noContent    bool
}

func checkSnapshotFiles(t *testing.T, s *testSession, got []store.SnapshotFile, want []snapFile) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("files = %+v, want %d", got, len(want))
	}
	for i, w := range want {
		f := got[i]
		if f.Path != w.path || f.Status != w.status || f.Deleted != w.deleted {
			t.Errorf("file %d = %+v, want %+v", i, f, w)
			continue
		}
		switch {
		case w.deleted || w.noContent:
			if f.Chunk != "" || f.Blob != "" {
				t.Errorf("%s: chunk %q blob %q, want neither", f.Path, f.Chunk, f.Blob)
			}
		case s.chunk(f.Chunk) != w.content || f.Blob != blobID([]byte(w.content)) ||
			f.Size != int64(len(w.content)):
			t.Errorf("%s = %+v holding %q, want %q", f.Path, f, s.chunk(f.Chunk), w.content)
		}
	}
}

// storeChunk puts an object file for key where the store keeps it, which
// is all the spool writer checks.
func storeChunk(t *testing.T, workdir, key string) {
	t.Helper()
	writeFile(t, filepath.Join(workdir, "objects", key[:2], key+".gz"), "")
}
