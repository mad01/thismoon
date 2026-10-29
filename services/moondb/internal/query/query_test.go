package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/ingest/faketest"
	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const repo = "github.com/o/r"

var t0 = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func at(minute int) time.Time { return t0.Add(time.Duration(minute) * time.Minute) }

type fixture struct {
	t   *testing.T
	st  *faketest.Store
	svc *Service
}

// newFixture is a Service over a fake store holding session s1 and its
// turn p1.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, st: faketest.New()}
	f.svc = &Service{Store: f.st}
	f.seed(
		store.Session{ID: "s1", Harness: "claude-code", StartedAt: at(0)},
		store.Turn{ID: "p1", SessionID: "s1", Status: store.TurnClosed, StartedAt: at(0)},
	)
	return f
}

func (f *fixture) seed(recs ...any) {
	f.t.Helper()
	ctx := context.Background()
	for _, r := range recs {
		var err error
		switch v := r.(type) {
		case store.Session:
			err = f.st.PutSession(ctx, v)
		case store.Turn:
			err = f.st.PutTurn(ctx, v)
		case store.Edit:
			err = f.st.AppendEdit(ctx, v)
		case store.CommitLink:
			err = f.st.AppendCommitLink(ctx, v)
		case store.Agent:
			err = f.st.PutAgent(ctx, v)
		default:
			err = fmt.Errorf("cannot seed %T", r)
		}
		if err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *fixture) chunk(data string) string {
	f.t.Helper()
	key, err := f.st.Chunks().Put(context.Background(), []byte(data))
	if err != nil {
		f.t.Fatal(err)
	}
	return key
}

// adding is an edit whose hunks add lines to the top of the file.
func (f *fixture) adding(e store.Edit, lines ...string) store.Edit {
	f.t.Helper()
	h := patch.Hunk{NewStart: 1, NewLines: len(lines)}
	for _, l := range lines {
		h.Lines = append(h.Lines, "+"+l)
	}
	raw, err := json.Marshal([]patch.Hunk{h})
	if err != nil {
		f.t.Fatal(err)
	}
	e.PatchChunk = f.chunk(string(raw))
	return e
}

func edit(id string, minute int, worktree, path string) store.Edit {
	return store.Edit{
		ID: id, ToolUseID: id, SessionID: "s1", TurnID: "p1", Tool: "Edit", At: at(minute),
		Location: store.Location{Repo: repo, Worktree: worktree}, Path: path,
		Kind: store.EditUpdate, BeforeSource: store.BeforeChain,
	}
}

func ids(edits []store.Edit) string {
	var out []string
	for _, e := range edits {
		out = append(out, e.ID)
	}
	return strings.Join(out, " ")
}

func TestHistory(t *testing.T) {
	f := newFixture(t)
	f.seed(
		edit("e1", 1, "/wt/a", "a.go"),
		edit("e2", 2, "/wt/a", "a.go"),
		edit("e3", 3, "/wt/b", "a.go"),
		edit("e4", 4, "/wt/a", "b.go"),
	)
	for i := range defaultHistoryLimit + 1 {
		f.seed(edit(fmt.Sprintf("c%d", i), 10+i, "/wt/a", "c.go"))
	}
	tests := []struct {
		name string
		req  HistoryRequest
		want string
		n    int
	}{
		{"one worktree", HistoryRequest{Worktree: "/wt/a", Path: "a.go"}, "e2 e1", 2},
		{"every worktree of the repo", HistoryRequest{Repo: repo, Path: "a.go"}, "e3 e2 e1", 3},
		{"limit", HistoryRequest{Repo: repo, Path: "a.go", Limit: 1}, "e3", 1},
		{"default limit", HistoryRequest{Path: "c.go"}, "", defaultHistoryLimit},
		{"nothing recorded", HistoryRequest{Path: "none.go"}, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.svc.History(context.Background(), tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.n || tc.want != "" && ids(got) != tc.want || got == nil {
				t.Errorf("History = %d edits [%s], want %d [%s]", len(got), ids(got), tc.n, tc.want)
			}
		})
	}
	if _, err := f.svc.History(context.Background(), HistoryRequest{}); !errors.Is(
		err, ErrInvalidRequest) {
		t.Errorf("History without a path = %v, want ErrInvalidRequest", err)
	}
}

const committed = "package a\n\nfunc One() int { return 1 }\n"

// gitRepo creates a repository with a.go holding committed, committed by
// a fixed author, and returns its root and HEAD.
func gitRepo(t *testing.T) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "a.go"), committed)
	git("add", "a.go")
	git("commit", "-q", "-m", "init")
	return dir, git("rev-parse", "HEAD")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWhyByBlame(t *testing.T) {
	dir, head := gitRepo(t)
	f := newFixture(t)
	blob := gitlink.BlobID([]byte(committed))
	made := edit("e1", 1, dir, "a.go")
	made.AfterBlob = blob
	copied := edit("decoy", 2, dir, "other.go")
	copied.AfterBlob = blob
	link := store.CommitLink{
		ID: head + ":a.go", Repo: repo, Commit: head, Path: "a.go", Blob: blob,
		EditID: "e1", TurnID: "p1", SessionID: "s1", At: at(3),
	}
	f.seed(made, copied, link)

	res, err := f.svc.Why(context.Background(), WhyRequest{Worktree: dir, Path: "a.go", Line: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != MethodBlame || res.Commit != head ||
		res.Text != "func One() int { return 1 }" {
		t.Errorf("Why = method %s commit %s text %q", res.Method, res.Commit, res.Text)
	}
	if res.Edit == nil || res.Edit.ID != "e1" {
		t.Errorf("Edit = %+v, want e1 (same path beats the newer copy)", res.Edit)
	}
	if res.Turn == nil || res.Turn.ID != "p1" || res.Session == nil || res.Session.ID != "s1" {
		t.Errorf("Turn = %+v, Session = %+v; want p1, s1", res.Turn, res.Session)
	}
	if res.CommitLink == nil || res.CommitLink.ID != link.ID {
		t.Errorf("CommitLink = %+v, want %s", res.CommitLink, link.ID)
	}
}

func TestWhyFallsThroughToContent(t *testing.T) {
	const added = "func Two() int { return compute(2) }"
	dir, _ := gitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), committed+added+"\n")
	tests := []struct {
		name string
		line int
		text string
	}{
		{"uncommitted line", 4, added},
		{"committed line no edit holds", 3, "func One() int { return 1 }"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.seed(f.adding(edit("e2", 2, dir, "a.go"), tc.text))
			res, err := f.svc.Why(context.Background(),
				WhyRequest{Worktree: dir, Path: "a.go", Line: tc.line})
			if err != nil {
				t.Fatal(err)
			}
			if res.Method != MethodContent || res.Text != tc.text || res.Commit != "" {
				t.Errorf("Why = method %s text %q commit %q, want content %q", res.Method,
					res.Text, res.Commit, tc.text)
			}
			if res.Edit == nil || res.Edit.ID != "e2" {
				t.Errorf("Edit = %+v, want e2", res.Edit)
			}
		})
	}
}

func TestWhyByContent(t *testing.T) {
	f := newFixture(t)
	created := edit("e0", 0, "/wt/a", "a.go")
	created.Kind, created.Tool = store.EditCreate, "Write"
	created.AfterChunk = f.chunk("package a\nx := one()\n")
	f.seed(
		created,
		f.adding(edit("e1", 1, "/wt/a", "a.go"), "x := one()", "y := two()"),
		f.adding(edit("e2", 2, "/wt/b", "a.go"), "z := three()"),
		f.adding(edit("e3", 3, "/wt/b", "a.go"), "\tx  :=  one()", "x := one()"),
		f.adding(edit("e4", 4, "/wt/a", "b.go"), "x := one()"),
		store.CommitLink{
			ID: "c9:a.go", Repo: repo, Commit: "c9", Path: "a.go", EditID: "e3",
			TurnID: "p1", SessionID: "s1", At: at(5),
		},
	)
	res, err := f.svc.Why(context.Background(),
		WhyRequest{Repo: repo, Path: "a.go", Text: "  x := one()"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{
		{Score: 1, Reason: "added the line (2 times)"},
		{Score: rankDecay, Reason: "added the line"},
		{Score: createWeight * rankDecay * rankDecay, Reason: "created the file with the line"},
	}
	wantIDs := []string{"e3", "e1", "e0"}
	if res.Method != MethodContent || len(res.Candidates) != len(want) {
		t.Fatalf("Why = method %s with %d candidates, want content with %d", res.Method,
			len(res.Candidates), len(want))
	}
	for i, c := range res.Candidates {
		if c.Edit.ID != wantIDs[i] || math.Abs(c.Score-want[i].Score) > 1e-9 ||
			c.Reason != want[i].Reason {
			t.Errorf("candidate %d = %s %.3f %q, want %s %.3f %q", i, c.Edit.ID, c.Score,
				c.Reason, wantIDs[i], want[i].Score, want[i].Reason)
		}
	}
	if res.Edit == nil || res.Edit.ID != "e3" || res.Turn == nil || res.Session == nil {
		t.Errorf("Edit = %+v, Turn = %+v, Session = %+v; want e3 with its turn and session",
			res.Edit, res.Turn, res.Session)
	}
	if res.Commit != "c9" || res.CommitLink == nil || res.CommitLink.ID != "c9:a.go" {
		t.Errorf("Commit = %q, CommitLink = %+v; want e3's link to c9", res.Commit, res.CommitLink)
	}
}

// TestWhyNamesTheSubagent: an edit a subagent made answers with the agent
// and its brief beside the turn. A record from before briefs has none, an
// agent known only from its edits has no record, and the main
// conversation's edit names no agent.
func TestWhyNamesTheSubagent(t *testing.T) {
	f := newFixture(t)
	brief := f.chunk("Order the lanes by start time.")
	f.seed(store.Agent{
		ID: "a-new", SessionID: "s1", Type: "general-purpose", Name: "builder",
		Description: "trace order", BriefChunk: brief,
	}, store.Agent{ID: "a-old", SessionID: "s1", Type: "Explore"})
	cases := []struct {
		agentID, agentType string
		want               *WhyAgent
	}{
		{"a-new", "builder", &WhyAgent{
			ID: "a-new", Type: "general-purpose", Name: "builder",
			Description: "trace order", BriefChunk: brief, Recorded: true,
		}},
		{"a-old", "Explore", &WhyAgent{ID: "a-old", Type: "Explore", Recorded: true}},
		{"a-gone", "worker", &WhyAgent{ID: "a-gone", Type: "worker"}},
		{"", "", nil},
	}
	for i, tc := range cases {
		path := fmt.Sprintf("f%d.go", i)
		e := f.adding(edit(fmt.Sprintf("e%d", i), i, "/wt/a", path), "lanes := byStart(turns)")
		e.AgentID, e.AgentType = tc.agentID, tc.agentType
		f.seed(e)
		res, err := f.svc.Why(context.Background(),
			WhyRequest{Repo: repo, Path: path, Text: "lanes := byStart(turns)"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Turn == nil || !equalAgent(res.Agent, tc.want) {
			t.Errorf("agent %q: Agent = %+v with turn %v, want %+v", tc.agentID, res.Agent,
				res.Turn != nil, tc.want)
		}
	}
}

func equalAgent(a, b *WhyAgent) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestWhyNone(t *testing.T) {
	f := newFixture(t)
	f.seed(f.adding(edit("e1", 1, "/wt/a", "a.go"), "}", "return nil, err", "x := one()"))
	for _, text := range []string{"}", "return nil, err", "import \"fmt\"", "unseen := line()"} {
		res, err := f.svc.Why(context.Background(),
			WhyRequest{Repo: repo, Path: "a.go", Line: 7, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		if res.Method != MethodNone || res.Text != text || res.Edit != nil ||
			len(res.Candidates) != 0 {
			t.Errorf("Why(%q) = method %s edit %+v, want none with the text kept", text,
				res.Method, res.Edit)
		}
	}
}

func TestWhyInvalidRequest(t *testing.T) {
	f := newFixture(t)
	for _, req := range []WhyRequest{
		{Line: 1},
		{Path: "a.go"},
		{Path: "a.go", Line: -1, Text: "x"},
		{Worktree: "wt/a", Path: "a.go", Line: 1},
	} {
		if _, err := f.svc.Why(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("Why(%+v) = %v, want ErrInvalidRequest", req, err)
		}
	}
}

func TestParseBlame(t *testing.T) {
	const sha = "1a612e4313cddd60ef596c66f2fccaacea2e8410"
	tests := []struct {
		name string
		out  string
		want blamed
	}{
		{
			name: "committed",
			out: sha + " 3 3 1\nauthor t\nsummary init\nprevious " + sha + " old.go\n" +
				"filename a.go\n\tfunc One() int { return 1 }",
			want: blamed{commit: sha, path: "a.go", text: "func One() int { return 1 }"},
		},
		{
			name: "not committed yet",
			out:  strings.Repeat("0", 40) + " 4 4 1\nauthor Not Committed Yet\nfilename a.go\n\tx",
			want: blamed{path: "a.go", text: "x"},
		},
		{name: "git failed", out: "", want: blamed{}},
		{name: "not porcelain", out: "fatal: no such path", want: blamed{}},
	}
	for _, tc := range tests {
		if got := parseBlame(tc.out); got != tc.want {
			t.Errorf("%s: parseBlame = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
