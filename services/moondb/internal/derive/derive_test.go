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

// grown is before with line 6 replaced by two lines, as grownHunks say.
var (
	grown      = strings.Replace(before, "\treturn 1\n", "\tx := 2\n\treturn x\n", 1)
	grownHunks = []patch.Hunk{{
		OldStart: 5, OldLines: 3, NewStart: 5, NewLines: 4,
		Lines: []string{" func A() int {", "-\treturn 1", "+\tx := 2", "+\treturn x", " }"},
	}}
)

// expandedHunks are editHunks as the harness writes them: every tab as two
// spaces, while the file and the tool input keep the tab.
var expandedHunks = func() []patch.Hunk {
	h := editHunks[0]
	lines := make([]string, len(h.Lines))
	for i, l := range h.Lines {
		lines[i] = strings.ReplaceAll(l, "\t", "  ")
	}
	h.Lines = lines
	return []patch.Hunk{h}
}()

// tabbedHunks append a function to tabbedBefore. They write its
// tab-indented line with two spaces, as the harness does, so a strict apply
// gives the spaces and the disk read, tabbedAfter, has the tab.
const (
	tabbedBefore = "package x\n\nfunc a() {}\n"
	tabbedAfter  = tabbedBefore + "func b() {\n\treturn\n}\n"
)

var tabbedHunks = []patch.Hunk{{
	OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 6,
	Lines: []string{" package x", " ", " func a() {}", "+func b() {", "+  return", "+}"},
}}

// formattedHunks take before to formatted in one hunk, tab-expanded as the
// harness writes them: both the return line and the Println line change.
var formattedHunks = []patch.Hunk{{
	OldStart: 3, OldLines: 9, NewStart: 3, NewLines: 9,
	Lines: []string{
		` import "fmt"`, " ", " func A() int {", "-  return 1", "+  return 2", " }", " ",
		" func B() {", `-  fmt.Println( "b" )`, `+  fmt.Println("b")`, " }",
	},
}}

// editInput is the Edit tool input behind editHunks.
var editInput = inputOf(map[string]any{
	"file_path": "/wt/a.go", "old_string": "\treturn 1", "new_string": "\treturn 2",
})

func inputOf(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// padding pushes a file past the size at which the harness stops
// shipping originalFile.
var padding = strings.Repeat("// padding line to grow the file past ten kilobytes\n", 250)

// evidence is what one test hook gathered. A nil field was not shipped;
// a nil hunks means no patch chunk, an empty one an empty patch.
type evidence struct {
	tool, kind, input                 string
	hunks                             []patch.Hunk
	original, written, observed, head *string
	observedBlob                      string
	chain                             *string
	excluded, userModified            bool
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
		Excluded: ev.excluded, UserModified: ev.userModified,
	}
	if ev.input != "" {
		p.Input = json.RawMessage(ev.input)
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
func chainOf(
	c memChunks,
	prev *string,
) func(context.Context, store.ChainQuery) (store.Edit, error) {
	return func(_ context.Context, q store.ChainQuery) (store.Edit, error) {
		if prev == nil {
			return store.Edit{}, store.ErrNotFound
		}
		if q.Worktree != "/wt" || q.Path != "a.go" {
			return store.Edit{}, fmt.Errorf("chain asked for %s %s", q.Worktree, q.Path)
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
			name: "tab-expanded hunks, original shipped",
			ev: evidence{
				tool: "Edit", kind: "update", input: editInput, hunks: expandedHunks,
				original: ptr(before), observed: ptr(after), chain: ptr(stale),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "tab-expanded hunks, previous edit's after",
			ev: evidence{
				tool: "Edit", kind: "update", input: editInput, hunks: expandedHunks,
				observed: ptr(bigAfter), chain: ptr(bigBefore),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeChain,
				before: ptr(bigBefore), after: ptr(bigAfter), observed: matched, eol: "lf",
			},
		},
		{
			name: "occurrence rule rejects a stale chain",
			ev: evidence{
				tool: "Edit", kind: "update", input: editInput, hunks: expandedHunks,
				observed: ptr(after), chain: ptr(stale), head: ptr(before),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeHead,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "occurrence rule rejects a chain with the old text twice",
			ev: evidence{
				tool: "Edit", kind: "update", input: editInput, hunks: expandedHunks,
				observed: ptr(after), chain: ptr(before + "\treturn 1\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "disk read with the new text twice cannot be undone",
			ev: evidence{
				tool: "Edit", kind: "update", input: editInput, hunks: expandedHunks,
				observed: ptr(after + "\treturn 2\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeUnknown,
				after: ptr(after + "\treturn 2\n"), eol: "lf",
			},
		},
		{
			name: "replace_all swaps every occurrence",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{
					"old_string": "\tx", "new_string": "\ty", "replace_all": true,
				}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
					Lines: []string{"-a  x", "-b  x", "+a  y", "+b  y"},
				}},
				original: ptr("a\tx\nb\tx\n"), observed: ptr("a\ty\nb\ty\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr("a\tx\nb\tx\n"), after: ptr("a\ty\nb\ty\n"),
				observed: matched, eol: "lf",
			},
		},
		{
			name: "without replace_all two occurrences reject the original",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{"old_string": "\tx", "new_string": "\ty"}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []string{"-a  x", "+a  y"},
				}},
				original: ptr("a\tx\nb\tx\n"), observed: ptr("a\ty\nb\tx\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeUnknown,
				after: ptr("a\ty\nb\tx\n"), eol: "lf",
			},
		},
		{
			name: "multi-edit applies its edits in order",
			ev: evidence{
				tool: "MultiEdit", kind: "update",
				input: inputOf(map[string]any{"edits": []map[string]any{
					{"old_string": "\treturn 1", "new_string": "\treturn 9"},
					{"old_string": "\treturn 9", "new_string": "\treturn 2"},
					{"old_string": `( "b" )`, "new_string": `("b")`},
				}}),
				hunks: formattedHunks, chain: ptr(before), observed: ptr(formatted),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeChain,
				before: ptr(before), after: ptr(formatted), observed: matched, eol: "lf",
			},
		},
		{
			name: "multi-edit undone from the disk read",
			ev: evidence{
				tool: "MultiEdit", kind: "update",
				input: inputOf(map[string]any{"edits": []map[string]any{
					{"old_string": "\treturn 1", "new_string": "\treturn 2"},
					{"old_string": `( "b" )`, "new_string": `("b")`},
				}}),
				hunks: formattedHunks, observed: ptr(formatted),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr(before), after: ptr(formatted), observed: matched, eol: "lf",
			},
		},
		{
			name: "write over a tab-indented file, tab-expanded hunks",
			ev: evidence{
				tool: "Write", kind: "update", input: inputOf(map[string]any{"content": after}),
				hunks: expandedHunks, written: ptr(after), observed: ptr(after),
				chain: ptr(before),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeChain,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "tab-expanded hunks without an input confirm the before only",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: expandedHunks,
				original: ptr(before), observed: ptr(after),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(before), after: ptr(after), eol: "lf",
			},
		},
		{
			name: "crlf file, multi-line old_string matched on the LF view",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: editHunks,
				input: inputOf(map[string]any{
					"old_string": "func A() int {\n\treturn 1", "new_string": "func A() int {\n\treturn 2",
				}),
				original: ptr(crlf(before)), observed: ptr(crlf(after)),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(crlf(before)), after: ptr(crlf(after)), observed: matched, eol: "crlf",
			},
		},
		{
			name: "crlf file, new_string's added line gets CRLF",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: grownHunks,
				input: inputOf(map[string]any{
					"old_string": "\treturn 1", "new_string": "\tx := 2\n\treturn x",
				}),
				original: ptr(crlf(before)), observed: ptr(crlf(grown)),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(crlf(before)), after: ptr(crlf(grown)), observed: matched, eol: "crlf",
			},
		},
		{
			name: "crlf disk read undone on its LF view",
			ev: evidence{
				tool: "Edit", kind: "update", hunks: expandedHunks,
				input: inputOf(map[string]any{
					"old_string": "func A() int {\n\treturn 1", "new_string": "func A() int {\n\treturn 2",
				}),
				observed: ptr(crlf(after)),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr(crlf(before)), after: ptr(crlf(after)), observed: matched, eol: "crlf",
			},
		},
		{
			name: "text missing from the candidate, the hunks stand in",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{
					"old_string": `say("hi")`, "new_string": `say("bye")`,
				}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []string{"-say(“hi”)", "+say(“bye”)"},
				}},
				original: ptr("say(“hi”)\n"), observed: ptr("say(“bye”)\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr("say(“hi”)\n"), after: ptr("say(“bye”)\n"),
				observed: matched, eol: "lf",
			},
		},
		{
			name: "hunks reject a chain that holds old_string elsewhere",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{"old_string": "a := 1", "new_string": "a := 2"}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []string{"-a := 1", "+a := 2"},
				}},
				chain: ptr("zzz\nqqq\na := 1\n"),
			},
			want: want{kind: store.EditUpdate, source: store.BeforeUnknown},
		},
		{
			name: "replace_all is undone from the disk read by the hunks",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{
					"old_string": "x", "new_string": "y", "replace_all": true,
				}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []string{"-x := y", "+y := y"},
				}},
				observed: ptr("y := y\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr("x := y\n"), after: ptr("y := y\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "a deletion is undone from the disk read by the hunks",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{"old_string": "b\n", "new_string": ""}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 2,
					Lines: []string{" a", "-b", " c"},
				}},
				observed: ptr("a\nc\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr("a\nb\nc\n"), after: ptr("a\nc\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "user-modified edit takes the hunks, not the input",
			ev: evidence{
				tool: "Edit", kind: "update", userModified: true,
				input: inputOf(map[string]any{"old_string": "a := 1", "new_string": "a := 2"}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []string{"-a := 1", "+c := 3"},
				}},
				original: ptr("a := 1\n"), observed: ptr("c := 3\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr("a := 1\n"), after: ptr("c := 3\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "edit with an empty old_string creates the file",
			ev: evidence{
				tool: "Edit", kind: "update",
				input:    inputOf(map[string]any{"old_string": "", "new_string": "hello\n"}),
				observed: ptr("hello\n"),
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeAbsent,
				after: ptr("hello\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "imported edit create, hunks without a disk read",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{"old_string": "", "new_string": "hello\n"}),
				hunks: []patch.Hunk{{
					OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 1, Lines: []string{"+hello"},
				}},
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeAbsent, after: ptr("hello\n"), eol: "lf",
			},
		},
		{
			name: "user-modified edit, the disk read keeps the tab the hunks wrote as spaces",
			ev: evidence{
				tool: "Edit", kind: "update", userModified: true,
				input: inputOf(map[string]any{
					"old_string": "func a() {}", "new_string": "func a() {}\nfunc b() {\n\treturn\n}",
				}),
				hunks:    tabbedHunks,
				original: ptr(tabbedBefore), observed: ptr(tabbedAfter),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr(tabbedBefore), after: ptr(tabbedAfter), observed: matched, eol: "lf",
			},
		},
		{
			name: "user-modified edit without a disk read keeps the hunks' spaces",
			ev: evidence{
				tool: "Edit", kind: "update", userModified: true,
				hunks: tabbedHunks, original: ptr(tabbedBefore),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal, before: ptr(tabbedBefore),
				after: ptr(strings.ReplaceAll(tabbedAfter, "\t", "  ")), eol: "lf",
			},
		},
		{
			name: "hunks that contradict the swapped content stand in for it",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{"old_string": "a", "new_string": "b  "}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
					Lines: []string{"-a", "+b", " c"},
				}},
				original: ptr("a\nc\n"), observed: ptr("b\nc\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeOriginal,
				before: ptr("a\nc\n"), after: ptr("b\nc\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "swap undone at the wrong occurrence, the hunks reverse the disk read",
			ev: evidence{
				tool: "Edit", kind: "update",
				input: inputOf(map[string]any{"old_string": "x", "new_string": "aa"}),
				hunks: []patch.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []string{"-ax", "+aaa"},
				}},
				observed: ptr("aaa\n"),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeObserved,
				before: ptr("ax\n"), after: ptr("aaa\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "user-modified edit with an empty old_string creates the file",
			ev: evidence{
				tool: "Edit", kind: "update", userModified: true,
				input: inputOf(map[string]any{"old_string": "", "new_string": "hello\n"}),
				hunks: []patch.Hunk{{
					OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 1, Lines: []string{"+hi"},
				}},
				observed: ptr("hi\n"),
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeAbsent,
				after: ptr("hi\n"), observed: matched, eol: "lf",
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
		{
			// Bash's hunks keep the tab, so they apply strictly.
			name: "Bash diff of a sed on a tab-indented file",
			ev: evidence{
				tool: "Bash", kind: "update", hunks: editHunks,
				observed: ptr(after), head: ptr(before),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeHead,
				before: ptr(before), after: ptr(after), observed: matched, eol: "lf",
			},
		},
		{
			name: "Bash diff of a file the command created",
			ev: evidence{
				tool: "Bash", kind: "create", observed: ptr("new\n"),
				hunks: []patch.Hunk{{NewStart: 1, NewLines: 1, Lines: []string{"+new"}}},
			},
			want: want{
				kind: store.EditCreate, source: store.BeforeAbsent,
				after: ptr("new\n"), observed: matched, eol: "lf",
			},
		},
		{
			name: "Bash diff of a file the command deleted",
			ev: evidence{
				tool: "Bash", kind: "delete", head: ptr("bye\n"),
				hunks: []patch.Hunk{{OldStart: 1, OldLines: 1, Lines: []string{"-bye"}}},
			},
			want: want{kind: store.EditDelete, source: store.BeforeHead, before: ptr("bye\n")},
		},
		{
			// No hunks: nothing checks the before, so the latest content
			// known of the path is taken, and the disk read is the after.
			name: "Bash changed path without a diff, chain before",
			ev: evidence{
				tool: "Bash", kind: "update", observed: ptr(after), chain: ptr(before),
				head: ptr(stale),
			},
			want: want{
				kind: store.EditUpdate, source: store.BeforeChain,
				before: ptr(before), after: ptr(after), eol: "lf",
			},
		},
		{
			name: "Bash changed path without a diff, HEAD before",
			ev:   evidence{tool: "Bash", kind: "update", observed: ptr(after), head: ptr(before)},
			want: want{
				kind: store.EditUpdate, source: store.BeforeHead,
				before: ptr(before), after: ptr(after), eol: "lf",
			},
		},
		{
			name: "Bash changed path without a diff, nothing known before",
			ev:   evidence{tool: "Bash", kind: "update", observed: ptr(after)},
			want: want{
				kind:   store.EditUnknown,
				source: store.BeforeUnknown,
				after:  ptr(after),
				eol:    "lf",
			},
		},
		{
			name: "Bash changed path without a diff, gone from disk",
			ev:   evidence{tool: "Bash", kind: "delete", chain: ptr(before)},
			want: want{kind: store.EditDelete, source: store.BeforeChain, before: ptr(before)},
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
		LatestAfter: func(context.Context, store.ChainQuery) (store.Edit, error) {
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

func TestEditChainQuery(t *testing.T) {
	// The chain is the latest edit of the path made no later than this one,
	// and never this one: a replayed envelope finds its own Edit recorded.
	c := memChunks{}
	raw, _ := json.Marshal(editHunks)
	p := &spool.EditPayload{
		Tool: "Edit", ToolUseID: "toolu_1", Path: "a.go", Kind: store.EditUpdate,
		Location: store.Location{Worktree: "/wt"},
	}
	p.PatchChunk, _ = c.Put(context.Background(), raw)
	var asked []store.ChainQuery
	d := &Deriver{
		Chunks: c,
		LatestAfter: func(_ context.Context, q store.ChainQuery) (store.Edit, error) {
			asked = append(asked, q)
			return store.Edit{}, store.ErrNotFound
		},
	}
	if _, _, err := d.Edit(context.Background(), envelope(p)); err != nil {
		t.Fatal(err)
	}
	want := store.ChainQuery{Worktree: "/wt", Path: "a.go", At: at, ExcludeID: "toolu_1:0"}
	if len(asked) != 1 || asked[0] != want {
		t.Errorf("chain lookups = %+v, want one %+v", asked, want)
	}
}
