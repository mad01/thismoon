package derive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
	"github.com/mad01/thismoon/services/moondb/internal/patch"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// memChunks is an in-memory store.Chunks keyed like the real one.
type memChunks map[string][]byte

func key(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (m memChunks) Put(_ context.Context, b []byte) (string, error) {
	k := key(b)
	m[k] = bytes.Clone(b)
	return k, nil
}

func (m memChunks) Get(_ context.Context, k string) ([]byte, error) {
	b, ok := m[k]
	if !ok {
		return nil, store.ErrNotFound
	}
	return b, nil
}

func (m memChunks) Has(_ context.Context, k string) (bool, error) {
	_, ok := m[k]
	return ok, nil
}

// The edited file: line 6 changes from "return 1" to "return 2", and the
// hunk carries three lines of context on each side, as the harness sends.
const (
	before = "package a\n\nimport \"fmt\"\n\nfunc A() int {\n\treturn 1\n}\n\n" +
		"func B() {\n\tfmt.Println( \"b\" )\n}\n"
	after = "package a\n\nimport \"fmt\"\n\nfunc A() int {\n\treturn 2\n}\n\n" +
		"func B() {\n\tfmt.Println( \"b\" )\n}\n"
	// formatted is after with a formatter's fix outside the hunk.
	formatted = "package a\n\nimport \"fmt\"\n\nfunc A() int {\n\treturn 2\n}\n\n" +
		"func B() {\n\tfmt.Println(\"b\")\n}\n"
	// stale is an older version the hunk's removed line does not fit.
	stale = "package a\n\nimport \"fmt\"\n\nfunc A() int {\n\treturn 0\n}\n\n" +
		"func B() {\n\tfmt.Println( \"b\" )\n}\n"
	// rewritten is a disk read where someone changed the added line again.
	rewritten = "package a\n\nimport \"fmt\"\n\nfunc A() int {\n\treturn 3\n}\n\n" +
		"func B() {\n\tfmt.Println( \"b\" )\n}\n"
)

var editHunks = []patch.Hunk{{
	OldStart: 3, OldLines: 7, NewStart: 3, NewLines: 7,
	Lines: []string{
		` import "fmt"`, " ", " func A() int {", "-\treturn 1", "+\treturn 2", " }", " ", " func B() {",
	},
}}

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

// padding pushes a file past the size at which the harness stops
// shipping originalFile.
var padding = strings.Repeat("// padding line to grow the file past ten kilobytes\n", 250)

// evidence is what one test hook gathered. A nil field was not shipped;
// a nil hunks means no patch chunk, an empty one an empty patch.
type evidence struct {
	tool, kind                        string
	hunks                             []patch.Hunk
	original, written, observed, head *string
	observedBlob                      string
	chain                             *string
	excluded                          bool
}

func ptr(s string) *string { return &s }

// run derives the Edit for ev with a fresh chunk store and returns the
// store too, so tests can read derived chunks back.
func run(t *testing.T, ev evidence) (store.Edit, *store.UnattributedDelta, memChunks) {
	t.Helper()
	c := memChunks{}
	put := func(s *string) string {
		if s == nil {
			return ""
		}
		k, _ := c.Put(context.Background(), []byte(*s))
		return k
	}
	p := &spool.EditPayload{
		Tool: ev.tool, ToolUseID: "toolu_1", Index: 0, AbsPath: "/wt/a.go", Path: "a.go",
		Location:      store.Location{Repo: "github.com/o/r", Worktree: "/wt", Head: "abc"},
		Kind:          store.EditKind(ev.kind),
		OriginalChunk: put(ev.original), ContentChunk: put(ev.written),
		ObservedChunk: put(ev.observed), ObservedBlob: ev.observedBlob, HeadChunk: put(ev.head),
		Excluded: ev.excluded,
	}
	if ev.observed != nil {
		p.ObservedBlob = gitlink.BlobID([]byte(*ev.observed))
	}
	if ev.hunks != nil {
		raw, err := json.Marshal(ev.hunks)
		if err != nil {
			t.Fatal(err)
		}
		p.PatchChunk = put(ptr(string(raw)))
	}
	d := &Deriver{Chunks: c, LatestAfter: chainOf(c, ev.chain)}
	e, delta, err := d.Edit(context.Background(), envelope(p))
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	return e, delta, c
}

// chainOf fakes the store's chain lookup: the previous edit's after-content
// is prev, or there is no previous edit when prev is nil.
func chainOf(c memChunks, prev *string) func(context.Context, string, string) (store.Edit, error) {
	return func(_ context.Context, worktree, path string) (store.Edit, error) {
		if prev == nil {
			return store.Edit{}, store.ErrNotFound
		}
		if worktree != "/wt" || path != "a.go" {
			return store.Edit{}, fmt.Errorf("chain asked for %s %s", worktree, path)
		}
		k, _ := c.Put(context.Background(), []byte(*prev))
		return store.Edit{ID: "toolu_0:0", AfterChunk: k}, nil
	}
}

var at = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

func envelope(p *spool.EditPayload) spool.Envelope {
	return spool.Envelope{
		Version: spool.Version, Kind: spool.KindEdit, ID: "env-1", At: at, Harness: "test-harness",
		Session: spool.SessionRef{ID: "sess-1"},
		Turn:    spool.TurnRef{ID: "turn-1"},
		Agent:   spool.AgentRef{ID: "agent-1", Type: "builder"},
		Edit:    p,
	}
}

// verdict is what an Edit's ObservedMatches says.
type verdict string

const (
	unchecked  verdict = "unchecked"
	matched    verdict = "matched"
	mismatched verdict = "mismatched"
)

func verdictOf(m *bool) verdict {
	switch {
	case m == nil:
		return unchecked
	case *m:
		return matched
	default:
		return mismatched
	}
}

// want is the part of an Edit a scenario pins down. A nil content means
// no chunk; a zero observed means unchecked.
type want struct {
	kind          store.EditKind
	source        store.BeforeSource
	before, after *string
	observed      verdict
	eol           string
	delta         bool
}

func TestEdit(t *testing.T) {
	bigBefore, bigAfter := before+padding, after+padding
	tests := []struct {
		name string
		ev   evidence
		want want
	}{
		{
			name: "harness shipped the original",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				original: ptr(before), observed: ptr(after), chain: ptr(stale),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "no original past 10 KB, previous edit's after fits",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				observed: ptr(bigAfter), chain: ptr(bigBefore), head: ptr(stale + padding),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeChain,
				before: ptr(bigBefore), after: ptr(bigAfter), observed: matched, eol: "lf",
			},
		},
		{
			name: "stale chain falls through to head",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				observed: ptr(after), chain: ptr(stale), head: ptr(before),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeHead,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "only the disk read, reversed",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				observed: ptr(after), head: ptr(stale),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "no source fits, after from the disk read",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				original: ptr(stale), observed: ptr(rewritten),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeUnknown,
				after: ptr(rewritten), eol: "lf",
			},
		},
		{
			name: "formatter rewrote the file after the edit",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				original: ptr(before), observed: ptr(formatted),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(
					before,
				), after: ptr(after), observed: mismatched, eol: "lf", delta: true,
			},
		},
		{
			name: "crlf file, hunks without carriage returns",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				original: ptr(crlf(before)), observed: ptr(crlf(after)),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(crlf(before)), after: ptr(crlf(after)), observed: matched, eol: "crlf",
			},
		},
		{
			name: "write creates a file",
			ev: evidence{
				tool: "Write", kind: "create",
				hunks: []patch.Hunk{{
					OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2,
					Lines: []string{"+package b", "+"},
				}},
				written: ptr("package b\n\n"), observed: ptr("package b\n\n"),
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeAbsent,
				after: ptr("package b\n\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "write creates a file, empty patch",
			ev: evidence{
				tool: "Write", kind: "create", hunks: []patch.Hunk{},
				written: ptr("x\n"), observed: ptr("x\n"),
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeAbsent,
				after: ptr("x\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "write update checks the chain against the written content",
			ev: evidence{
				tool: "Write", kind: "update", hunks: editHunks,
				written: ptr(after), observed: ptr(after), chain: ptr(before),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeChain,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "write rejects a chain the hunks fit but the written content does not",
			ev: evidence{
				tool: "Write", kind: "update", hunks: editHunks,
				written: ptr(after), observed: ptr(after),
				chain: ptr(strings.Replace(before, `( "b" )`, `("b")`, 1)),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "write without a kind and no before is a create; read past the cap",
			ev: evidence{
				tool: "Write", hunks: editHunks, written: ptr(after),
				observedBlob: gitlink.BlobID([]byte(after)),
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeUnknown,
				after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "notebook edit has the disk read only",
			ev: evidence{
				tool: "NotebookEdit", kind: "update", observed: ptr("{\"cells\": []}\n"),
			},
			want: want{kind: store.EditUnknown, source: store.BeforeUnknown},
		},
		{
			name: "delete the payload reports and the missing file backs",
			ev:   evidence{tool: "Bash", kind: "delete", original: ptr(before)},
			want: want{kind: store.EditDelete, source: store.BeforeOriginal, before: ptr(before)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, delta, c := run(t, tt.ev)
			checkEdit(t, e, c, tt.want)
			if (delta != nil) != tt.want.delta {
				t.Fatalf("delta = %+v, want one: %v", delta, tt.want.delta)
			}
			if tt.ev.observed != nil && e.ObservedChunk != key([]byte(*tt.ev.observed)) {
				t.Errorf("ObservedChunk = %q, want the disk read's key", e.ObservedChunk)
			}
		})
	}
}

func checkEdit(t *testing.T, e store.Edit, c memChunks, w want) {
	t.Helper()
	if e.ID != "toolu_1:0" || e.ToolUseID != "toolu_1" || e.SessionID != "sess-1" ||
		e.TurnID != "turn-1" || e.AgentID != "agent-1" || e.AgentType != "builder" ||
		!e.At.Equal(at) || e.Path != "a.go" || e.Worktree != "/wt" || e.Repo != "github.com/o/r" {
		t.Errorf("identity = %+v", e)
	}
	if e.Kind != w.kind || e.BeforeSource != w.source || e.EOL != w.eol {
		t.Errorf("kind, source, eol = %s, %s, %q; want %s, %s, %q",
			e.Kind, e.BeforeSource, e.EOL, w.kind, w.source, w.eol)
	}
	checkChunk(t, c, "BeforeChunk", e.BeforeChunk, w.before)
	checkChunk(t, c, "AfterChunk", e.AfterChunk, w.after)
	wantBlob := ""
	if w.after != nil {
		wantBlob = gitlink.BlobID([]byte(*w.after))
	}
	if e.AfterBlob != wantBlob {
		t.Errorf("AfterBlob = %q, want %q", e.AfterBlob, wantBlob)
	}
	wantObserved := w.observed
	if wantObserved == "" {
		wantObserved = unchecked
	}
	if got := verdictOf(e.ObservedMatches); got != wantObserved {
		t.Errorf("ObservedMatches = %s, want %s", got, wantObserved)
	}
}

func checkChunk(t *testing.T, c memChunks, field, k string, want *string) {
	t.Helper()
	if want == nil {
		if k != "" {
			t.Errorf("%s = %q (%q), want none", field, k, c[k])
		}
		return
	}
	got, ok := c[k]
	if !ok {
		t.Fatalf("%s = %q, not in the chunk store", field, k)
	}
	if string(got) != *want {
		t.Errorf("%s holds %q, want %q", field, got, *want)
	}
}

func TestEditObservedDelta(t *testing.T) {
	e, delta, c := run(t, evidence{
		tool: "Edit", kind: "update", hunks: editHunks,
		original: ptr(before), observed: ptr(formatted),
	})
	if delta == nil {
		t.Fatal("no delta for a disk read that differs from the derived content")
	}
	if delta.ID != "toolu_1:0:observed" || delta.Cause != store.DeltaObservedMismatch {
		t.Errorf("delta id, cause = %s, %s", delta.ID, delta.Cause)
	}
	if delta.SessionID != e.SessionID || delta.TurnID != e.TurnID || !delta.At.Equal(e.At) ||
		delta.Location != e.Location || delta.Path != e.Path {
		t.Errorf("delta identity = %+v, want the edit's", delta)
	}
	if delta.FromChunk != e.AfterChunk || delta.ToChunk != e.ObservedChunk {
		t.Errorf("delta from, to = %s, %s; want the edit's after and observed chunks",
			delta.FromChunk, delta.ToChunk)
	}
	want := "--- a/a.go\n+++ b/a.go\n@@ -7,5 +7,5 @@\n }\n \n func B() {\n" +
		"-\tfmt.Println( \"b\" )\n+\tfmt.Println(\"b\")\n }\n"
	if got := string(c[delta.PatchChunk]); got != want {
		t.Errorf("delta patch =\n%s\nwant\n%s", got, want)
	}
}

func TestEditExcluded(t *testing.T) {
	e, delta, c := run(t, evidence{tool: "Edit", kind: "update", excluded: true})
	if delta != nil {
		t.Errorf("delta = %+v, want none", delta)
	}
	want := store.Edit{
		ID: "toolu_1:0", ToolUseID: "toolu_1", SessionID: "sess-1", TurnID: "turn-1",
		AgentID: "agent-1", AgentType: "builder", Tool: "Edit", At: at,
		Location: store.Location{Repo: "github.com/o/r", Worktree: "/wt", Head: "abc"},
		Path:     "a.go", Kind: store.EditUpdate, BeforeSource: store.BeforeUnknown, Excluded: true,
	}
	if e != want {
		t.Errorf("Edit =\n%+v\nwant\n%+v", e, want)
	}
	if len(c) != 0 {
		t.Errorf("stored %d chunks for an excluded edit, want none", len(c))
	}
}

func TestEditErrors(t *testing.T) {
	boom := errors.New("boom")
	p := &spool.EditPayload{
		Tool: "Edit", ToolUseID: "toolu_1", Path: "a.go", Kind: store.EditUpdate,
		Location: store.Location{Worktree: "/wt"},
	}
	c := memChunks{}
	raw, _ := json.Marshal(editHunks)
	p.PatchChunk, _ = c.Put(context.Background(), raw)
	p.ObservedChunk, _ = c.Put(context.Background(), []byte(after))

	d := &Deriver{
		Chunks: c,
		LatestAfter: func(context.Context, string, string) (store.Edit, error) {
			return store.Edit{}, boom
		},
	}
	if _, _, err := d.Edit(context.Background(), envelope(p)); !errors.Is(err, boom) {
		t.Errorf("chain lookup failure: err = %v, want it to wrap %v", err, boom)
	}

	missing := *p
	missing.OriginalChunk = key([]byte("never stored"))
	d = &Deriver{Chunks: c}
	if _, _, err := d.Edit(context.Background(), envelope(&missing)); !errors.Is(
		err,
		store.ErrNotFound,
	) {
		t.Errorf("missing chunk: err = %v, want it to wrap %v", err, store.ErrNotFound)
	}

	if _, _, err := d.Edit(context.Background(), spool.Envelope{ID: "x", Kind: spool.KindTurnOpen}); err == nil {
		t.Error("an envelope without an edit payload derived without error")
	}
}

func TestEditChainSkipsItself(t *testing.T) {
	// A replayed envelope finds its own Edit at the head of the chain. A line
	// inserted above identical lines fits its own after-content too.
	insertHunks := []patch.Hunk{{
		OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4, Lines: []string{"+a", " a", " a", " a"},
	}}
	c := memChunks{}
	raw, _ := json.Marshal(insertHunks)
	self, _ := c.Put(context.Background(), []byte("a\na\na\na\n"))
	p := &spool.EditPayload{
		Tool:      "Edit",
		ToolUseID: "toolu_1",
		Path:      "a.go",
		Kind:      store.EditUpdate,
	}
	p.PatchChunk, _ = c.Put(context.Background(), raw)
	d := &Deriver{
		Chunks: c,
		LatestAfter: func(context.Context, string, string) (store.Edit, error) {
			return store.Edit{ID: "toolu_1:0", AfterChunk: self}, nil
		},
	}
	e, _, err := d.Edit(context.Background(), envelope(p))
	if err != nil {
		t.Fatal(err)
	}
	if e.BeforeSource != store.BeforeUnknown {
		t.Errorf("BeforeSource = %s, want %s", e.BeforeSource, store.BeforeUnknown)
	}
}
