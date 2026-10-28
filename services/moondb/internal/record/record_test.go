package record

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

var testNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

// git runs one git command for test setup, with an identity and no
// signing whatever the developer's config says.
func git(tb testing.TB, dir string, args ...string) string {
	tb.Helper()
	base := []string{
		"-c", "user.name=moondb", "-c", "user.email=moondb@example.com", "-c", "commit.gpgsign=false",
	}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// physicalTempDir is a temp dir with symlinks resolved, the form git
// reports paths in.
func physicalTempDir(tb testing.TB) string {
	tb.Helper()
	dir, err := filepath.EvalSymlinks(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	return dir
}

// newRepo makes a repo on main with a.txt committed and an origin remote.
// The developer's git config stays out of it, for setup and for Run.
func newRepo(tb testing.TB) string {
	tb.Helper()
	tb.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	tb.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := physicalTempDir(tb)
	git(tb, dir, "init", "-q", "-b", "main")
	git(tb, dir, "remote", "add", "origin", "https://github.com/example/demo.git")
	writeFile(tb, filepath.Join(dir, "a.txt"), "one\ntwo\n")
	git(tb, dir, "add", "a.txt")
	git(tb, dir, "commit", "-q", "-m", "init")
	return dir
}

func writeFile(tb testing.TB, path, body string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		tb.Fatal(err)
	}
}

func readFile(tb testing.TB, path string) string {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return string(data)
}

// testSession feeds fixtures for one session through Run against its own
// workdir.
type testSession struct {
	tb         testing.TB
	workdir    string
	cwd        string
	transcript string
	excludes   []string
	budget     time.Duration
	// env is the environment Run reads, so a test never sees the one it
	// runs in; empty unless a test sets it.
	env map[string]string
	// now is the clock Run reads; testNow throughout when nil.
	now func() time.Time
	// logs holds the lines Run logged, in order.
	logs []string
}

func newSession(tb testing.TB, cwd string) *testSession {
	tb.Helper()
	root := physicalTempDir(tb)
	return &testSession{
		tb:         tb,
		workdir:    filepath.Join(root, "moondb"),
		cwd:        cwd,
		transcript: filepath.Join(root, "transcript.jsonl"),
		budget:     10 * time.Second,
	}
}

// run reads testdata/<fixture>, replaces $CWD, $TRANSCRIPT and the given
// name/value pairs, and records it.
func (s *testSession) run(fixture string, vars ...string) error {
	s.tb.Helper()
	raw := readFile(s.tb, filepath.Join("testdata", fixture))
	pairs := append([]string{"$CWD", s.cwd, "$TRANSCRIPT", s.transcript}, vars...)
	for i := 1; i < len(pairs); i += 2 {
		quoted, _ := json.Marshal(pairs[i])
		pairs[i] = string(quoted[1 : len(quoted)-1])
	}
	return s.runRaw(strings.NewReplacer(pairs...).Replace(raw))
}

func (s *testSession) runRaw(body string) error {
	now := s.now
	if now == nil {
		now = func() time.Time { return testNow }
	}
	return Run(context.Background(), strings.NewReader(body), Options{
		Workdir:    s.workdir,
		Now:        now,
		Excludes:   s.excludes,
		Budget:     s.budget,
		BashBudget: s.budget,
		Logf: func(format string, args ...any) {
			s.logs = append(s.logs, fmt.Sprintf(format, args...))
		},
		Getenv: func(key string) string { return s.env[key] },
	})
}

// stampingClock is a clock whose every reading also rewrites file, so a
// test can tell which readings came before a read of it: reading n writes
// "stamp n" and returns testNow plus n seconds.
func stampingClock(tb testing.TB, file string) func() time.Time {
	n := 0
	return func() time.Time {
		n++
		writeFile(tb, file, "stamp "+strconv.Itoa(n)+"\n")
		return testNow.Add(time.Duration(n) * time.Second)
	}
}

// reading is the stampingClock reading behind t, and readingIn the one
// whose write content holds; -1 for content no reading wrote.
func reading(t time.Time) int { return int(t.Sub(testNow) / time.Second) }

func readingIn(content string) int {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(content, "stamp "), "\n"))
	if err != nil {
		return -1
	}
	return n
}

func (s *testSession) mustRun(fixture string, vars ...string) {
	s.tb.Helper()
	if err := s.run(fixture, vars...); err != nil {
		s.tb.Fatalf("Run(%s): %v", fixture, err)
	}
}

func (s *testSession) reader() *spool.Reader {
	return &spool.Reader{Dir: filepath.Join(s.workdir, "spool")}
}

// take returns the pending envelopes in spool order and removes them, so
// the next step sees only what it wrote.
func (s *testSession) take() []spool.Envelope {
	s.tb.Helper()
	r := s.reader()
	paths, err := r.List()
	if err != nil {
		s.tb.Fatal(err)
	}
	var envs []spool.Envelope
	for _, p := range paths {
		env, err := r.Read(p)
		if err != nil {
			s.tb.Fatal(err)
		}
		envs = append(envs, env)
		if err := r.Remove(p); err != nil {
			s.tb.Fatal(err)
		}
	}
	return envs
}

// takeOne is take when exactly one envelope of kind is expected.
func (s *testSession) takeOne(kind spool.Kind) spool.Envelope {
	s.tb.Helper()
	envs := s.take()
	if len(envs) != 1 || envs[0].Kind != kind {
		s.tb.Fatalf("spool holds %d envelopes %v, want one %s", len(envs), kinds(envs), kind)
	}
	return envs[0]
}

func kinds(envs []spool.Envelope) []spool.Kind {
	var out []spool.Kind
	for _, e := range envs {
		out = append(out, e.Kind)
	}
	return out
}

// chunk returns the spooled bytes of key, "" for no key.
func (s *testSession) chunk(key string) string {
	s.tb.Helper()
	if key == "" {
		return ""
	}
	data, err := s.reader().Chunk(key)
	if err != nil {
		s.tb.Fatal(err)
	}
	return string(data)
}

func (s *testSession) state(ext string) string {
	data, _ := os.ReadFile(filepath.Join(s.workdir, "sessions", "sess-1"+ext))
	return string(data)
}

// field is one expectation on a string a test pulled out of an envelope.
type field struct {
	name, got, want string
}

func checkFields(t *testing.T, fields []field) {
	t.Helper()
	for _, f := range fields {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
}

func TestEditInRepo(t *testing.T) {
	repo := newRepo(t)
	head := git(t, repo, "rev-parse", "HEAD")
	s := newSession(t, repo)
	file := filepath.Join(repo, "a.txt")
	writeFile(t, file, "one\ndeux\n")
	s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_edit_1")

	env := s.takeOne(spool.KindEdit)
	ep := env.Edit
	wantLoc := store.Location{
		Repo: "github.com/example/demo", Worktree: repo, Canonical: repo, Branch: "main", Head: head,
	}
	if ep.Location != wantLoc {
		t.Errorf("Location = %+v, want %+v", ep.Location, wantLoc)
	}
	wantSession := spool.SessionRef{ID: "sess-1", Cwd: repo, TranscriptPath: s.transcript}
	if !reflect.DeepEqual(env.Session, wantSession) || !env.At.Equal(testNow) {
		t.Errorf(
			"envelope session %+v at %v, want %+v at %v",
			env.Session,
			env.At,
			wantSession,
			testNow,
		)
	}
	if ep.Index != 0 || ep.ObservedSize != 9 || ep.Kind != store.EditUpdate || ep.Excluded {
		t.Errorf("Edit = %+v, want index 0, observed size 9, an update, not excluded", ep)
	}
	checkFields(t, []field{
		{"ID", env.ID, "toolu_edit_1:0"},
		{"Harness", env.Harness, "claude-code"},
		{"Turn.ID", env.Turn.ID, "prompt-1"},
		{"Tool", ep.Tool, "Edit"},
		{"AbsPath", ep.AbsPath, file},
		{"Path", ep.Path, "a.txt"},
		{"EOL", ep.EOL, "lf"},
		{"ObservedBlob", ep.ObservedBlob, git(t, repo, "hash-object", "a.txt")},
		{"HeadBlob", ep.HeadBlob, git(t, repo, "rev-parse", "HEAD:a.txt")},
		{"original", s.chunk(ep.OriginalChunk), "one\ntwo\n"},
		{"observed", s.chunk(ep.ObservedChunk), "one\ndeux\n"},
		{"head", s.chunk(ep.HeadChunk), "one\ntwo\n"},
		{"content", s.chunk(ep.ContentChunk), ""},
	})
	var patch []struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(s.chunk(ep.PatchChunk)), &patch); err != nil ||
		len(patch) != 1 || !slices.Equal(patch[0].Lines, []string{" one", "-two", "+deux"}) {
		t.Errorf("patch chunk = %+v (%v), want the fixture's one hunk", patch, err)
	}
	var input map[string]any
	if err := json.Unmarshal(ep.Input, &input); err != nil || input["new_string"] != "deux" {
		t.Errorf("Input = %s, want the tool_input verbatim", ep.Input)
	}

	// The second edit of the path ships the HEAD content too: the store
	// may not hold it, and the spool writer skips a chunk the store has.
	writeFile(t, file, "one\ndos\n")
	s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_edit_2")
	second := s.takeOne(spool.KindEdit).Edit
	if second.HeadBlob != ep.HeadBlob || second.HeadChunk != ep.HeadChunk {
		t.Errorf("second edit HeadBlob %q HeadChunk %q, want %q and %q",
			second.HeadBlob, second.HeadChunk, ep.HeadBlob, ep.HeadChunk)
	}
	checkFields(t, []field{
		{".paths", s.state(pathsExt), repo + "\ta.txt\n"},
		{".touched", s.state(touchedExt), repo + "\n"},
		{".heads", s.state(headsExt), repo + "\t" + head + "\n"},
	})
}

func TestWriteCreate(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(repo, "new", "b.txt")
	writeFile(t, file, "fresh\r\nfile\r\n")
	s.mustRun("write_create.json", "$FILE", file)

	ep := s.takeOne(spool.KindEdit).Edit
	if ep.Kind != store.EditCreate {
		t.Errorf("Kind = %q, want %q", ep.Kind, store.EditCreate)
	}
	checkFields(t, []field{
		{"Path", ep.Path, "new/b.txt"},
		{"EOL", ep.EOL, "crlf"},
		{"OriginalChunk (originalFile null)", ep.OriginalChunk, ""},
		{"HeadBlob (untracked)", ep.HeadBlob, ""},
		{"content", s.chunk(ep.ContentChunk), "fresh\r\nfile\r\n"},
		{"patch", s.chunk(ep.PatchChunk), "[]"},
		{"observed", s.chunk(ep.ObservedChunk), "fresh\r\nfile\r\n"},
	})
}

func TestMultiEditFromSubagent(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(repo, "a.txt")
	writeFile(t, file, "uno\ndos\n")
	s.mustRun("multiedit.json", "$FILE", file)

	env := s.takeOne(spool.KindEdit)
	ep := env.Edit
	if env.Agent != (spool.AgentRef{ID: "agent-7", Type: "worker"}) {
		t.Errorf("Agent = %+v, want agent-7/worker", env.Agent)
	}
	if !ep.UserModified || ep.Kind != store.EditUpdate {
		t.Errorf("UserModified %v Kind %q, want true and update", ep.UserModified, ep.Kind)
	}
	checkFields(t, []field{
		{"ID", env.ID, "toolu_multi_1:0"},
		{"Tool", ep.Tool, "MultiEdit"},
		{"original", s.chunk(ep.OriginalChunk), "one\ntwo\n"},
		{"observed", s.chunk(ep.ObservedChunk), "uno\ndos\n"},
		{"head", s.chunk(ep.HeadChunk), "one\ntwo\n"},
	})
}

func TestNotebookEdit(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(repo, "nb.ipynb")
	writeFile(t, file, "{}\n")
	s.mustRun("notebook.json", "$FILE", file)

	ep := s.takeOne(spool.KindEdit).Edit
	if ep.Kind != store.EditUnknown {
		t.Errorf("Kind = %q, want %q", ep.Kind, store.EditUnknown)
	}
	checkFields(t, []field{
		{"Tool", ep.Tool, "NotebookEdit"},
		{"Path", ep.Path, "nb.ipynb"},
		{"PatchChunk", ep.PatchChunk, ""},
		{"observed", s.chunk(ep.ObservedChunk), "{}\n"},
	})
}

func TestExcludedFileShipsNoContent(t *testing.T) {
	cases := []struct {
		name     string
		rel      string
		excludes []string
	}{
		{"default glob on the base name", ".env", nil},
		{"default glob in a subdirectory", "deploy/prod.pem", nil},
		{"custom glob on the relative path", "secrets/token.txt", []string{"secrets/*"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepo(t)
			s := newSession(t, repo)
			s.excludes = tc.excludes
			file := filepath.Join(repo, tc.rel)
			writeFile(t, file, "SECRET=1\n")
			s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_secret")

			ep := s.takeOne(spool.KindEdit).Edit
			if !ep.Excluded || ep.Path != tc.rel || ep.Location.Worktree != repo {
				t.Errorf("Edit = %+v, want excluded %s in %s", ep, tc.rel, repo)
			}
			leaked := ep.PatchChunk + ep.OriginalChunk + ep.ContentChunk + ep.ObservedChunk +
				ep.ObservedBlob + ep.HeadBlob + ep.HeadChunk + string(ep.Input)
			if leaked != "" {
				t.Errorf("excluded edit carries content fields: %+v", ep)
			}
			chunks, _ := filepath.Glob(filepath.Join(s.workdir, "spool", "*.chunk"))
			if len(chunks) != 0 {
				t.Errorf("spool holds chunks %v for an excluded file", chunks)
			}
		})
	}
}

func TestFileOutsideAnyRepo(t *testing.T) {
	dir := physicalTempDir(t)
	s := newSession(t, dir)
	file := filepath.Join(dir, "notes.txt")
	writeFile(t, file, "loose\n")
	s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_loose")

	ep := s.takeOne(spool.KindEdit).Edit
	if ep.Location != (store.Location{}) {
		t.Errorf("Location = %+v, want none", ep.Location)
	}
	checkFields(t, []field{
		{"Path", ep.Path, file},
		{"AbsPath", ep.AbsPath, file},
		{"HeadBlob", ep.HeadBlob, ""},
		{"observed", s.chunk(ep.ObservedChunk), "loose\n"},
		{".touched", s.state(touchedExt), ""},
		{".paths", s.state(pathsExt), "\t" + file + "\n"},
	})
}

func TestSymlinkedPathResolves(t *testing.T) {
	repo := newRepo(t)
	link := filepath.Join(physicalTempDir(t), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	s := newSession(t, link)
	writeFile(t, filepath.Join(repo, "a.txt"), "one\ndeux\n")
	s.mustRun("edit.json", "$FILE", filepath.Join(link, "a.txt"), "$TOOL_USE_ID", "toolu_link")

	ep := s.takeOne(spool.KindEdit).Edit
	checkFields(t, []field{
		{"AbsPath", ep.AbsPath, filepath.Join(repo, "a.txt")},
		{"Path", ep.Path, "a.txt"},
		{"Worktree", ep.Location.Worktree, repo},
	})
}

func TestOverCapFileShipsBlobsOnly(t *testing.T) {
	repo := newRepo(t)
	s := newSession(t, repo)
	file := filepath.Join(repo, "big.txt")
	big := strings.Repeat("x\n", moondb.MaxChunkBytes/2+1)
	writeFile(t, file, big)
	git(t, repo, "add", "big.txt")
	git(t, repo, "commit", "-q", "-m", "big")
	writeFile(t, file, big+"y\n")
	s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_big")

	ep := s.takeOne(spool.KindEdit).Edit
	if ep.ObservedSize != int64(len(big)+2) {
		t.Errorf("ObservedSize = %d, want %d", ep.ObservedSize, len(big)+2)
	}
	checkFields(t, []field{
		{"ObservedChunk", ep.ObservedChunk, ""},
		{"ObservedBlob", ep.ObservedBlob, git(t, repo, "hash-object", "big.txt")},
		{"HeadBlob", ep.HeadBlob, git(t, repo, "rev-parse", "HEAD:big.txt")},
		{"HeadChunk", ep.HeadChunk, ""},
	})
}

func TestPromptThenStop(t *testing.T) {
	s := newSession(t, physicalTempDir(t))
	before := "{\"line\":\"before\"}\n"
	writeFile(t, s.transcript, before)
	s.mustRun("prompt.json")

	open := s.takeOne(spool.KindTurnOpen)
	if open.Turn != (spool.TurnRef{ID: "prompt-1", TranscriptOffset: int64(len(before))}) {
		t.Errorf("Turn = %+v, want prompt-1 at offset %d", open.Turn, len(before))
	}
	checkFields(t, []field{
		{"ID", open.ID, "prompt-1:open"},
		{"Session.Title", open.Session.Title, "greeting work"},
		{"Session.Cwd", open.Session.Cwd, s.cwd},
		{"prompt", s.chunk(open.TurnOpen.PromptChunk), "rename the greeting"},
		{"origin", string(open.TurnOpen.Origin), "person"},
	})

	during := "{\"line\":1}\n{\"line\":2}\n"
	writeFile(t, s.transcript, before+during)
	s.mustRun("stop.json")
	closed := s.takeOne(spool.KindTurnClose)
	tc := closed.TurnClose
	if tc.Status != store.TurnClosed || tc.TranscriptEnd != int64(len(before+during)) ||
		closed.Turn.TranscriptOffset != int64(len(before)) {
		t.Errorf("TurnClose = %+v with turn %+v, want closed from %d to %d",
			tc, closed.Turn, len(before), len(before+during))
	}
	checkFields(t, []field{
		{"ID", closed.ID, "prompt-1:close"},
		{"final", s.chunk(tc.FinalChunk), "Renamed the greeting."},
		{"transcript", s.chunk(tc.TranscriptChunk), during},
	})
}

// TestSessionOrigin: a turn-open carries where the session's cwd sits in
// git, and every envelope the entrypoint and the valid tags the
// environment gives. A cwd outside any repository has no location.
func TestSessionOrigin(t *testing.T) {
	repo := newRepo(t)
	head := git(t, repo, "rev-parse", "HEAD")
	s := newSession(t, repo)
	s.env = map[string]string{EntrypointEnv: "sdk-cli", moondb.SessionTagsEnv: "eval, Bad,ci,eval"}
	s.mustRun("prompt.json")
	file := filepath.Join(repo, "a.txt")
	writeFile(t, file, "one\ndeux\n")
	s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_edit_1")
	s.mustRun("stop.json")

	envs := s.take()
	wantLoc := store.Location{
		Repo: "github.com/example/demo", Worktree: repo, Canonical: repo, Branch: "main", Head: head,
	}
	for _, env := range envs {
		ref := env.Session
		if ref.Entrypoint != "sdk-cli" || !slices.Equal(ref.Tags, []string{"ci", "eval"}) {
			t.Errorf(
				"%s: entrypoint %q tags %v, want sdk-cli and [ci eval]",
				env.ID,
				ref.Entrypoint,
				ref.Tags,
			)
		}
		want := store.Location{}
		if env.Kind == spool.KindTurnOpen {
			want = wantLoc
		}
		if ref.Location != want {
			t.Errorf("%s: session location %+v, want %+v", env.ID, ref.Location, want)
		}
	}

	outside := newSession(t, physicalTempDir(t))
	outside.mustRun("prompt.json")
	open := outside.takeOne(spool.KindTurnOpen).Session
	if open.Location != (store.Location{}) || open.Entrypoint != "" || open.Tags != nil {
		t.Errorf("session outside a repo with no environment = %+v, want no origin", open)
	}
}

// TestEntrypointBounded: the hook keeps an entrypoint only when it passes
// the session tag rule, so a list badge built from it stays short.
func TestEntrypointBounded(t *testing.T) {
	tests := map[string]string{
		"sdk-ts":                "sdk-ts",
		"claude-desktop":        "claude-desktop",
		"Claude Desktop":        "",
		strings.Repeat("x", 33): "",
		"sdk-cli;echo":          "",
	}
	for raw, want := range tests {
		s := newSession(t, physicalTempDir(t))
		s.env = map[string]string{EntrypointEnv: raw}
		s.mustRun("prompt.json")
		if got := s.takeOne(spool.KindTurnOpen).Session.Entrypoint; got != want {
			t.Errorf("entrypoint %q recorded as %q, want %q", raw, got, want)
		}
	}
}

func TestStopWithoutOpenOmitsTranscript(t *testing.T) {
	s := newSession(t, physicalTempDir(t))
	writeFile(t, s.transcript, "{\"line\":1}\n")
	s.mustRun("stop.json")

	tc := s.takeOne(spool.KindTurnClose).TurnClose
	if tc.TranscriptChunk != "" || tc.TranscriptEnd != 11 || tc.Status != store.TurnClosed {
		t.Errorf("TurnClose = %+v, want closed, no transcript, end 11", tc)
	}
}

func TestStopAfterCommitReportsHeadMoved(t *testing.T) {
	repo := newRepo(t)
	old := git(t, repo, "rev-parse", "HEAD")
	s := newSession(t, repo)
	writeFile(t, filepath.Join(repo, "a.txt"), "one\ndeux\n")
	s.mustRun("edit.json", "$FILE", filepath.Join(repo, "a.txt"), "$TOOL_USE_ID", "toolu_edit_1")
	s.take()
	writeFile(t, filepath.Join(repo, "c.txt"), "new\n")
	git(t, repo, "add", "a.txt", "c.txt")
	git(t, repo, "commit", "-q", "-m", "work")
	head := git(t, repo, "rev-parse", "HEAD")

	s.mustRun("stop.json")
	envs := s.take()
	wantKinds := []spool.Kind{spool.KindTurnClose, spool.KindHeadMoved, spool.KindSnapshot}
	if !slices.Equal(kinds(envs), wantKinds) {
		t.Fatalf("stop wrote %v, want turn-close, head-moved, snapshot", kinds(envs))
	}
	// The worktree is clean after the commit, so the snapshot holds the
	// one path the session edited, as committed.
	if files := envs[2].Snapshot.Files; len(files) != 1 || files[0].Path != "a.txt" ||
		files[0].Status != "" || s.chunk(files[0].Chunk) != "one\ndeux\n" {
		t.Errorf("snapshot files = %+v, want a.txt clean with its committed content", files)
	}
	hm := envs[1].HeadMoved
	want := []spool.ChangedFile{
		{Path: "a.txt", Blob: git(t, repo, "rev-parse", "HEAD:a.txt"), Status: "M"},
		{Path: "c.txt", Blob: git(t, repo, "rev-parse", "HEAD:c.txt"), Status: "A"},
	}
	if !slices.Equal(hm.Files, want) || hm.Truncated {
		t.Errorf("Files = %+v (truncated %v), want %+v", hm.Files, hm.Truncated, want)
	}
	checkFields(t, []field{
		{"ID", envs[1].ID, headMovedID("prompt-1", repo, head)},
		{"OldHead", hm.OldHead, old},
		{"NewHead", hm.NewHead, head},
		{"Location.Head", hm.Location.Head, head},
		{".heads", s.state(headsExt), repo + "\t" + head + "\n"},
	})

	s.mustRun("stop.json")
	if got := kinds(s.take()); !slices.Equal(got, []spool.Kind{spool.KindTurnClose}) {
		t.Errorf("a second stop with nothing changed wrote %v, want only turn-close", got)
	}
}

func TestBudgetStillWritesTheEdit(t *testing.T) {
	repo := newRepo(t)
	file := filepath.Join(repo, "a.txt")
	writeFile(t, file, "one\ndeux\n")

	t.Run("spent before git runs", func(t *testing.T) {
		s := newSession(t, repo)
		s.budget = time.Nanosecond
		s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_budget")
		ep := s.takeOne(spool.KindEdit).Edit
		if ep.Location != (store.Location{}) || ep.ObservedChunk != "" {
			t.Errorf("Edit = %+v, want no location and no disk read", ep)
		}
		checkFields(t, []field{
			{"Path", ep.Path, file},
			{"original", s.chunk(ep.OriginalChunk), "one\ntwo\n"},
		})
	})

	t.Run("slow git", func(t *testing.T) {
		bin := physicalTempDir(t)
		fake := filepath.Join(bin, "git")
		writeFile(t, fake, "#!/bin/sh\n[ \"$1\" = warm ] && exit 0\nexec /bin/sleep 5\n")
		if err := os.Chmod(fake, 0o700); err != nil {
			t.Fatal(err)
		}
		// A new executable's first launch can wait on the system's check of
		// it; take that wait here, not under the budget.
		if err := exec.Command(fake, "warm").Run(); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		s := newSession(t, repo)
		s.budget = 200 * time.Millisecond
		start := time.Now()
		s.mustRun("edit.json", "$FILE", file, "$TOOL_USE_ID", "toolu_slow")
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("Run took %v with a 200ms budget", took)
		}
		ep := s.takeOne(spool.KindEdit).Edit
		if ep.Location != (store.Location{}) || ep.PatchChunk == "" {
			t.Errorf("Edit = %+v, want no location but the payload's patch", ep)
		}
	})
}

func TestRunErrors(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
		want    []spool.Kind
	}{
		{"not JSON", "{not json", true, nil},
		{"not an object", "[1]", true, nil},
		{"no session id", `{"hook_event_name":"Stop","prompt_id":"p"}`, true, nil},
		{"stop without prompt id", `{"hook_event_name":"Stop","session_id":"s"}`, true, nil},
		{"tool use without id", `{"hook_event_name":"PostToolUse","session_id":"s",` +
			`"tool_name":"Edit","tool_input":{"file_path":"/x"}}`, true, nil},
		{"unknown event", `{"hook_event_name":"SessionStart","session_id":"s"}`, false, nil},
		{"subagent stop without agent id", `{"hook_event_name":"SubagentStop",` +
			`"session_id":"s"}`, true, nil},
		{"field of the wrong type", `{"hook_event_name":"UserPromptSubmit","session_id":"s",` +
			`"prompt_id":"p","prompt":42}`, false, []spool.Kind{spool.KindTurnOpen}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSession(t, physicalTempDir(t))
			if err := s.runRaw(tc.body); (err != nil) != tc.wantErr {
				t.Fatalf("Run() error = %v, want error %v", err, tc.wantErr)
			}
			if got := kinds(s.take()); !slices.Equal(got, tc.want) {
				t.Errorf("spool holds %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("tool without capture", func(t *testing.T) {
		s := newSession(t, physicalTempDir(t))
		s.mustRun("bash.json")
		if got := s.take(); len(got) != 0 {
			t.Errorf("Bash call wrote %v, want nothing yet", kinds(got))
		}
	})
	t.Run("unwritable spool", func(t *testing.T) {
		s := newSession(t, physicalTempDir(t))
		writeFile(t, filepath.Join(s.workdir, "spool"), "a file where the spool goes")
		if err := s.run("prompt.json"); err == nil {
			t.Error("Run() = nil with an unwritable spool, want an error")
		}
	})
	t.Run("no workdir", func(t *testing.T) {
		if err := Run(context.Background(), strings.NewReader("{}"), Options{}); err == nil {
			t.Error("Run() = nil without a workdir, want an error")
		}
	})
}

func TestParseDiffTree(t *testing.T) {
	blob := strings.Repeat("b", 40)
	zero := strings.Repeat("0", 40)
	rec := func(oldMode, newMode, oldID, newID, status string, paths ...string) string {
		return ":" + oldMode + " " + newMode + " " + oldID + " " + newID + " " + status + "\x00" +
			strings.Join(paths, "\x00") + "\x00"
	}
	out := rec("100644", "100644", blob, blob, "M", "mod.go") +
		rec("000000", "100644", zero, blob, "A", "dir/new file.go") +
		rec("100644", "000000", blob, zero, "D", "gone.go") +
		rec("100644", "100644", blob, blob, "R087", "old.go", "renamed.go") +
		rec("000000", "160000", zero, blob, "A", "vendor/sub")
	want := []spool.ChangedFile{
		{Path: "mod.go", Blob: blob, Status: "M"},
		{Path: "dir/new file.go", Blob: blob, Status: "A"},
		{Path: "gone.go", Status: "D"},
		{Path: "renamed.go", Blob: blob, Status: "R"},
	}
	cases := []struct {
		name          string
		out           string
		limit         int
		want          []spool.ChangedFile
		wantTruncated bool
	}{
		{"every status, submodule skipped", out, 10, want, false},
		{"capped", out, 2, want[:2], true},
		{"empty", "", 10, nil, false},
		{"malformed", "garbage\x00", 10, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := parseDiffTree(tc.out, tc.limit)
			if !slices.Equal(got, tc.want) || truncated != tc.wantTruncated {
				t.Errorf("parseDiffTree() = %+v, %v, want %+v, %v",
					got, truncated, tc.want, tc.wantTruncated)
			}
		})
	}
}

func TestReadTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	writeFile(t, path, "aaaa\nbbbb\ncccc\n")
	cases := []struct {
		name          string
		offset, limit int64
		want          string
	}{
		{"from the offset", 5, 100, "bbbb\ncccc\n"},
		{"over the cap keeps whole last lines", 0, 7, "cccc\n"},
		{"offset past the end", 99, 100, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, end := readTail(path, tc.offset, tc.limit)
			if string(got) != tc.want || end != 15 {
				t.Errorf(
					"readTail(%d, %d) = %q, %d, want %q, 15",
					tc.offset,
					tc.limit,
					got,
					end,
					tc.want,
				)
			}
		})
	}
}

func TestDetectEOL(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"no newline":   "",
		"a\nb\n":       "lf",
		"a\r\nb\r\n":   "crlf",
		"a\r\nb\nc\n":  "mixed",
		"single\r\n":   "crlf",
		"bare\rcr\n":   "lf",
		"\n\r\n\n\r\n": "mixed",
	}
	for in, want := range cases {
		if got := detectEOL([]byte(in)); got != want {
			t.Errorf("detectEOL(%q) = %q, want %q", in, got, want)
		}
	}
}

// BenchmarkRunEdit measures a warm Run on an Edit payload in a small repo:
// the latency the synchronous Edit hook adds to every edit.
func BenchmarkRunEdit(b *testing.B) {
	repo := newRepo(b)
	file := filepath.Join(repo, "a.txt")
	writeFile(b, file, "one\ndeux\n")
	s := newSession(b, repo)
	s.budget = moondb.RecordBudget
	raw := readFile(b, filepath.Join("testdata", "edit.json"))
	body := strings.NewReplacer("$CWD", repo, "$TRANSCRIPT", s.transcript, "$FILE", file).
		Replace(raw)
	n := 0
	for b.Loop() {
		n++
		id := "toolu_bench_" + strconv.Itoa(n)
		if err := s.runRaw(strings.Replace(body, "$TOOL_USE_ID", id, 1)); err != nil {
			b.Fatal(err)
		}
	}
}
