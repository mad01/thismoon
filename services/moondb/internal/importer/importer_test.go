package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb/internal/ingest"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

const fixtureOrigin = "https://example.com/fixture/repo.git"

// world is the testdata tree copied into a temp dir with its placeholders
// filled: {CWD} is the sessions' working directory, a git repo on main, as
// the transcripts spell it; {REAL} is the same directory with symlinks
// resolved. sess-old.jsonl is aged past the default window.
type world struct {
	cwd, real, projects, history, spool string
	since                               time.Time
}

func newWorld(t *testing.T) *world {
	t.Helper()
	root := t.TempDir()
	w := &world{
		cwd:      filepath.Join(root, "repo"),
		projects: filepath.Join(root, "projects"),
		history:  filepath.Join(root, "file-history"),
		spool:    filepath.Join(root, "spool"),
		since:    time.Now().Add(-30 * 24 * time.Hour),
	}
	if err := os.MkdirAll(w.cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(w.cwd)
	if err != nil {
		t.Fatal(err)
	}
	w.real = real
	git(t, w.cwd, "init", "-q", "-b", "main")
	git(t, w.cwd, "remote", "add", "origin", fixtureOrigin)
	if err := os.WriteFile(filepath.Join(w.cwd, "README"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, w.cwd, "add", "README")
	git(t, w.cwd, "commit", "-q", "-m", "init")
	fill := strings.NewReplacer("{CWD}", w.cwd, "{REAL}", w.real)
	copyTree(t, filepath.Join("testdata", "projects"), w.projects, fill)
	copyTree(t, filepath.Join("testdata", "file-history"), w.history, fill)
	old := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(w.transcript("sess-old"), old, old); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *world) transcript(id string) string {
	return filepath.Join(w.projects, "-fixture-project", id+".jsonl")
}

func (w *world) options() Options {
	return Options{
		Projects:    w.projects,
		FileHistory: w.history,
		Since:       w.since,
		Sink:        &spool.Writer{Dir: w.spool},
	}
}

// run imports with opts and returns the counts and the envelopes in the
// order serve would ingest them.
func (w *world) run(t *testing.T, opts Options) (Counts, []spool.Envelope) {
	t.Helper()
	counts, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return counts, w.envelopes(t)
}

func (w *world) envelopes(t *testing.T) []spool.Envelope {
	t.Helper()
	r := &spool.Reader{Dir: w.spool}
	paths, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	envs := make([]spool.Envelope, 0, len(paths))
	for _, p := range paths {
		env, err := r.Read(p)
		if err != nil {
			t.Fatal(err)
		}
		envs = append(envs, env)
	}
	return envs
}

// chunk returns a spooled chunk's text, "" for an empty key.
func (w *world) chunk(t *testing.T, key string) string {
	t.Helper()
	if key == "" {
		return ""
	}
	data, err := (&spool.Reader{Dir: w.spool}).Chunk(key)
	if err != nil {
		t.Fatalf("chunk %s: %v", key, err)
	}
	return string(data)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{
		"-c", "user.name=moondb", "-c", "user.email=moondb@example.com", "-c", "commit.gpgsign=false",
	}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// copyTree copies src to dst, filling placeholders in every file.
func copyTree(t *testing.T, src, dst string, fill *strings.Replacer) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, []byte(fill.Replace(string(data))), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// wantCounts is what the fixture tree yields with the default window.
// sess-1 has two turns and eight edits (one excluded, one from a subagent),
// a Bash call, a failed call and an unanswered one. sess-3 is a transcript
// without prompt ids: two turns, one edit, and a call that names no file.
// sess-2 has no edit, and sess-old is outside the window.
func wantCounts() Counts {
	return Counts{
		Transcripts:         4,
		SubagentTranscripts: 1,
		SessionsImported:    2,
		Skipped:             Skipped{OutsideSince: 1, NoEdits: 1},
		Turns:               4,
		Edits: map[string]int{
			"Edit": 5, "Write": 2, "MultiEdit": 1, "NotebookEdit": 1,
		},
		EditsWithoutPatch:    1,
		Placement:            Placement{Live: 9},
		OriginalsFromHistory: 1,
		FailedCalls:          1,
		UnansweredCalls:      1,
		BashCalls:            1,
		Agents:               1,
		Envelopes:            18,
		UndecodableLines:     1,
		UnknownTypes:         map[string]int{"mystery-entry": 1},
	}
}

func TestImportCounts(t *testing.T) {
	w := newWorld(t)
	got, _ := w.run(t, w.options())
	if want := wantCounts(); !reflect.DeepEqual(got, want) {
		t.Errorf("counts:\n got %+v\nwant %+v", got, want)
	}
}

func TestImportOrder(t *testing.T) {
	w := newWorld(t)
	_, envs := w.run(t, w.options())
	var got []string
	for _, e := range envs {
		got = append(got, e.ID)
	}
	want := []string{
		// sess-3 started first; without prompt ids a turn is keyed by its
		// prompt's uuid.
		"w-1:open", "toolu_r1:0", "w-1:close", "w-5:open", "w-5:close",
		"p1:open", "toolu_e1:0", "toolu_w1:0", "toolu_env:0", "toolu_m1:0", "p1:close",
		"p2:open", "toolu_big1:0", "toolu_big2:0", "toolu_sub:0", "toolu_nb:0", "p2:close",
		// a session's agents follow its turns
		"aworker-1:close:4",
	}
	if !slices.Equal(got, want) {
		t.Errorf("envelope order:\n got %q\nwant %q", got, want)
	}
}

func TestImportMarksEveryEnvelopeBackfill(t *testing.T) {
	w := newWorld(t)
	_, envs := w.run(t, w.options())
	if len(envs) == 0 {
		t.Fatal("no envelopes written")
	}
	for _, env := range envs {
		if !env.Backfill || env.Version != spool.Version {
			t.Errorf("%s: backfill %v version %d, want true and %d",
				env.ID, env.Backfill, env.Version, spool.Version)
		}
	}
}

func byID(envs []spool.Envelope) map[string]spool.Envelope {
	m := map[string]spool.Envelope{}
	for _, e := range envs {
		m[e.ID] = e
	}
	return m
}

func TestImportTurns(t *testing.T) {
	w := newWorld(t)
	_, envs := w.run(t, w.options())
	got := byID(envs)
	raw, err := os.ReadFile(w.transcript("sess-1"))
	if err != nil {
		t.Fatal(err)
	}
	p1 := int64(bytes.Index(raw, []byte(`{"type":"user","uuid":"u-p1"`)))
	p2 := int64(bytes.Index(raw, []byte(`{"type":"user","uuid":"u-p2"`)))
	tests := []struct {
		id, kind, prompt, final string
		offset                  int64
	}{
		{"p1:open", "turn-open", "add a greeting", "", p1},
		{"p1:close", "turn-close", "", "Turn one is done.", p1},
		// the meta entry under p2 is not the prompt
		{"p2:open", "turn-open", "make the numbers bigger", "", p2},
		{"p2:close", "turn-close", "", "Turn two is done.", p2},
		{"w-1:open", "turn-open", "rename the variable", "", 0},
		{"w-5:open", "turn-open", "thanks", "", 0},
	}
	for _, tc := range tests {
		env, ok := got[tc.id]
		if !ok {
			t.Errorf("%s: not written", tc.id)
			continue
		}
		if string(env.Kind) != tc.kind || env.Turn.ID != strings.Split(tc.id, ":")[0] {
			t.Errorf("%s: kind %s turn %q", tc.id, env.Kind, env.Turn.ID)
		}
		if env.Turn.TranscriptOffset != tc.offset && env.Session.ID == "sess-1" {
			t.Errorf(
				"%s: transcript offset %d, want %d",
				tc.id,
				env.Turn.TranscriptOffset,
				tc.offset,
			)
		}
		if env.TurnOpen != nil && w.chunk(t, env.TurnOpen.PromptChunk) != tc.prompt {
			t.Errorf(
				"%s: prompt %q, want %q",
				tc.id,
				w.chunk(t, env.TurnOpen.PromptChunk),
				tc.prompt,
			)
		}
		if env.TurnClose != nil && w.chunk(t, env.TurnClose.FinalChunk) != tc.final {
			t.Errorf("%s: final %q, want %q", tc.id, w.chunk(t, env.TurnClose.FinalChunk), tc.final)
		}
	}
	// p1's transcript chunk is its lines: from its prompt up to p2's.
	tc := got["p1:close"].TurnClose
	if want := string(raw[p1:p2]); w.chunk(t, tc.TranscriptChunk) != want {
		t.Errorf("p1 transcript chunk:\n%s\nwant\n%s", w.chunk(t, tc.TranscriptChunk), want)
	}
	if tc.TranscriptEnd != p2 || tc.Status != store.TurnClosed {
		t.Errorf("p1 close: end %d status %q, want %d closed", tc.TranscriptEnd, tc.Status, p2)
	}
	s := got["p1:open"].Session
	want := spool.SessionRef{
		ID: "sess-1", Cwd: w.cwd, Title: "fixture session", TitleSource: store.TitleNamed,
		Model: "model-a", TranscriptPath: w.transcript("sess-1"),
	}
	if s != want || got["p1:open"].Harness != "claude-code" {
		t.Errorf("session ref %+v harness %q, want %+v", s, got["p1:open"].Harness, want)
	}
	at := time.Date(2026, 10, 1, 10, 0, 14, 0, time.UTC)
	if !got["p1:close"].At.Equal(at) {
		t.Errorf("p1 closed at %v, want the turn's last entry %v", got["p1:close"].At, at)
	}
}

func TestImportEdits(t *testing.T) {
	w := newWorld(t)
	_, envs := w.run(t, w.options())
	got := byID(envs)
	history, err := os.ReadFile(filepath.Join(w.history, "sess-1", "abc123@v1"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id, tool, path, turn string
		kind                 store.EditKind
		agent                spool.AgentRef
		original, content    string
		excluded, noPatch    bool
		userModified         bool
	}{
		{
			id: "toolu_e1:0", tool: "Edit", path: "greet.go", turn: "p1", kind: store.EditUpdate,
			original: "package greet\n\nvar Name = 1\n",
		},
		{
			id: "toolu_w1:0", tool: "Write", path: "new.txt", turn: "p1", kind: store.EditCreate,
			content: "hello\n",
		},
		{
			id: "toolu_env:0", tool: "Write", path: ".env", turn: "p1", kind: store.EditUpdate,
			excluded: true, noPatch: true,
		},
		{
			id: "toolu_m1:0", tool: "MultiEdit", path: "multi.go", turn: "p1",
			kind: store.EditUpdate, original: "one\ntwo\n", userModified: true,
		},
		// over the original cap: the file history holds the file as found
		{
			id: "toolu_big1:0", tool: "Edit", path: "big.go", turn: "p2", kind: store.EditUpdate,
			original: string(history),
		},
		// the same file again: the history copy predates big1, so none
		{id: "toolu_big2:0", tool: "Edit", path: "big.go", turn: "p2", kind: store.EditUpdate},
		{
			id: "toolu_sub:0", tool: "Edit", path: "sub.go", turn: "p2", kind: store.EditUpdate,
			agent: spool.AgentRef{ID: "aworker-1", Type: "worker"}, original: "old\n",
		},
		{
			id: "toolu_nb:0", tool: "NotebookEdit", path: "nb.ipynb", turn: "p2",
			kind: store.EditUnknown, noPatch: true,
		},
	}
	for _, tc := range tests {
		env, ok := got[tc.id]
		if !ok || env.Edit == nil {
			t.Errorf("%s: no edit envelope", tc.id)
			continue
		}
		ep := env.Edit
		if ep.Tool != tc.tool || ep.Path != tc.path || ep.Kind != tc.kind || ep.Index != 0 {
			t.Errorf(
				"%s: tool %s path %s kind %s index %d",
				tc.id,
				ep.Tool,
				ep.Path,
				ep.Kind,
				ep.Index,
			)
		}
		if env.Turn.ID != tc.turn || env.Agent != tc.agent {
			t.Errorf(
				"%s: turn %q agent %+v, want %q %+v",
				tc.id,
				env.Turn.ID,
				env.Agent,
				tc.turn,
				tc.agent,
			)
		}
		if ep.AbsPath != filepath.Join(w.real, tc.path) {
			t.Errorf("%s: abs path %s", tc.id, ep.AbsPath)
		}
		if g := w.chunk(t, ep.OriginalChunk); g != tc.original {
			t.Errorf("%s: original %q, want %q", tc.id, g, tc.original)
		}
		if g := w.chunk(t, ep.ContentChunk); g != tc.content {
			t.Errorf("%s: content %q, want %q", tc.id, g, tc.content)
		}
		if ep.Excluded != tc.excluded || (ep.PatchChunk == "") != tc.noPatch {
			t.Errorf("%s: excluded %v patch %q", tc.id, ep.Excluded, ep.PatchChunk)
		}
		if ep.Excluded && (len(ep.Input) > 0 || ep.OriginalChunk != "" || ep.ContentChunk != "") {
			t.Errorf("%s: an excluded edit carries content", tc.id)
		}
		if !ep.Excluded && len(ep.Input) == 0 {
			t.Errorf("%s: no tool input", tc.id)
		}
		if ep.UserModified != tc.userModified {
			t.Errorf("%s: user modified %v", tc.id, ep.UserModified)
		}
		if ep.ObservedChunk != "" || ep.ObservedBlob != "" || ep.HeadChunk != "" {
			t.Errorf("%s: an import has no disk read and no HEAD content", tc.id)
		}
		// HEAD now says nothing about HEAD then; the branch is the one the
		// transcript recorded.
		wantLoc := store.Location{
			Repo: gitloc.CanonicalRepo(fixtureOrigin), Worktree: w.real, Branch: "feature-x",
		}
		if ep.Location.Repo != wantLoc.Repo || ep.Location.Worktree != wantLoc.Worktree ||
			ep.Location.Branch != wantLoc.Branch || ep.Location.Head != "" {
			t.Errorf("%s: location %+v, want %+v", tc.id, ep.Location, wantLoc)
		}
	}
	if p := got["toolu_e1:0"].Edit; w.chunk(t, p.PatchChunk) == "" || p.EOL != "lf" {
		t.Errorf("e1: patch %q eol %q", w.chunk(t, p.PatchChunk), p.EOL)
	}
	// sess-3 records no branch, so the worktree's current one stands.
	if b := got["toolu_r1:0"].Edit.Location.Branch; b != "main" {
		t.Errorf("r1 branch %q, want main", b)
	}
	at := time.Date(2026, 10, 1, 10, 0, 3, 0, time.UTC)
	if !got["toolu_e1:0"].At.Equal(at) {
		t.Errorf("e1 at %v, want the answer's time %v", got["toolu_e1:0"].At, at)
	}
}

func TestImportSkips(t *testing.T) {
	errDown := errors.New("serve down")
	tests := []struct {
		name     string
		known    map[string]bool
		knownErr error
		force    bool
		allTime  bool
		imported int
		inStore  int
		old      int
		wantErr  error
	}{
		{name: "new sessions", imported: 2, old: 1},
		{
			name:     "captured live",
			known:    map[string]bool{"sess-1": true},
			imported: 1,
			inStore:  1,
			old:      1,
		},
		{name: "forced", known: map[string]bool{"sess-1": true}, force: true, imported: 2, old: 1},
		{name: "no window", allTime: true, imported: 3},
		{name: "store unreachable", knownErr: errDown, wantErr: errDown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			opts := w.options()
			opts.Force = tc.force
			if tc.allTime {
				opts.Since = time.Time{}
			}
			opts.Known = func(_ context.Context, id string) (bool, error) {
				return tc.known[id], tc.knownErr
			}
			got, err := Run(context.Background(), opts)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Run error %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if got.SessionsImported != tc.imported || got.Skipped.InStore != tc.inStore ||
				got.Skipped.OutsideSince != tc.old || got.Skipped.NoEdits != 1 {
				t.Errorf("imported %d, skipped %+v", got.SessionsImported, got.Skipped)
			}
		})
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	w := newWorld(t)
	opts := w.options()
	opts.Sink = nil
	got, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if want := wantCounts(); !reflect.DeepEqual(got, want) {
		t.Errorf("dry-run counts:\n got %+v\nwant %+v", got, want)
	}
	if _, err := os.Stat(w.spool); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry run created the spool: %v", err)
	}
}

// TestImportIngests drains an import into a local store the way serve
// does, then imports and drains again: the second pass must change
// nothing, which is what lets an import overlap a live capture.
func TestImportIngests(t *testing.T) {
	w := newWorld(t)
	st, err := local.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	drain := func() {
		t.Helper()
		in := &ingest.Ingester{Store: st, Spool: &spool.Reader{Dir: w.spool}}
		paths, err := in.Spool.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			env, err := in.Spool.Read(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Ingest(ctx, env); err != nil {
				t.Fatalf("ingest %s: %v", env.ID, err)
			}
			if err := in.Spool.Remove(p); err != nil {
				t.Fatal(err)
			}
		}
	}
	w.run(t, w.options())
	drain()

	sd, err := st.Session(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if sd.Title != "fixture session" || len(sd.Turns) != 2 {
		t.Errorf("session: title %q, %d turns", sd.Title, len(sd.Turns))
	}
	td, err := st.Turn(ctx, "p2")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]store.BeforeSource{}
	for _, e := range td.Edits {
		sources[e.ID] = e.BeforeSource
	}
	want := map[string]store.BeforeSource{
		"toolu_big1:0": store.BeforeOriginal, // from the file history
		"toolu_big2:0": store.BeforeChain,    // big1's after-content
		"toolu_sub:0":  store.BeforeOriginal,
		"toolu_nb:0":   store.BeforeUnknown,
	}
	if !reflect.DeepEqual(sources, want) {
		t.Errorf("p2 before sources %v, want %v", sources, want)
	}

	edits := func() []store.Edit {
		t.Helper()
		all, err := st.Edits(ctx, store.EditQuery{SessionID: "sess-1"})
		if err != nil {
			t.Fatal(err)
		}
		return all
	}
	before := edits()
	opts := w.options()
	opts.Force = true
	w.run(t, opts)
	drain()
	if after := edits(); !reflect.DeepEqual(after, before) {
		t.Errorf("a second import changed the edits:\n%v\n%v", before, after)
	}
}

// TestFixtureIsJSONLines keeps the fixtures honest: every line but the
// one malformed on purpose decodes.
func TestFixtureIsJSONLines(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "projects", "*", "*.jsonl"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	bad := 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for line := range bytes.Lines(data) {
			if !json.Valid(bytes.TrimSpace(line)) {
				bad++
			}
		}
	}
	if bad != 1 {
		t.Errorf("%d malformed fixture lines, want the 1 planted", bad)
	}
}

// TestImportAgents writes each subagent transcript of an imported session
// as the SubagentStop hook would: the agent transcript, who the meta file
// says the agent was, the turns it ran under and where it worked.
func TestImportAgents(t *testing.T) {
	w := newWorld(t)
	_, envs := w.run(t, w.options())
	env, ok := byID(envs)["aworker-1:close:4"]
	if !ok {
		t.Fatal("aworker-1 not written")
	}
	ac := env.AgentClose
	if env.Kind != spool.KindAgentClose || env.Agent.ID != "aworker-1" ||
		env.Session.ID != "sess-1" {
		t.Errorf("envelope = %+v", env)
	}
	if ac.Type != "worker" || ac.Name != "" || ac.Description != "edit sub.go" ||
		ac.Model != "model-b" || !slices.Equal(ac.TurnIDs, []string{"p2"}) ||
		ac.Cwd != w.cwd || ac.Location.Worktree != w.real || ac.Location.Repo == "" {
		t.Errorf("AgentClose = %+v", ac)
	}
	start := time.Date(2026, 10, 1, 10, 5, 6, 0, time.UTC)
	if !ac.StartedAt.Equal(start) || !ac.EndedAt.Equal(start.Add(2500*time.Millisecond)) ||
		ac.Lines != (store.AgentLines{Read: 4, Kept: 4}) {
		t.Errorf("times %v to %v, lines %+v", ac.StartedAt, ac.EndedAt, ac.Lines)
	}
	if lines := w.chunk(t, ac.TranscriptChunk); !strings.Contains(
		lines,
		`"tool_use_id":"toolu_sub"`,
	) {
		t.Errorf("agent transcript = %q, want the tool result's ids", lines)
	}
}
