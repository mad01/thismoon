package agentfile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

const workerFile = "testdata/agent-aworker-1234abcd.jsonl"

// TestReadAgent reads a teammate's transcript: who it was from the meta
// file, the turns and place from its lines, and the counts of what it kept.
func TestReadAgent(t *testing.T) {
	c, err := Read(context.Background(), workerFile, "worker")
	if err != nil {
		t.Fatal(err)
	}
	want := Capture{
		ID: "aworker-1234abcd", Type: "general-purpose", Name: "worker",
		Description: "Build the thing", Model: "claude-opus-5-5",
		TurnIDs: []string{"p1", "p2"}, Cwd: "/work/repo", GitBranch: "main",
		StartedAt: time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC),
		LastAt:    time.Date(2026, 10, 25, 10, 1, 6, 0, time.UTC),
		Lines:     store.AgentLines{Read: 13, Kept: 11},
	}
	got := c
	got.Transcript = nil
	if !equalCapture(got, want) {
		t.Errorf("Read = %+v\nwant %+v", got, want)
	}
	if n := bytes.Count(c.Transcript, []byte("\n")); n != 11 {
		t.Errorf("transcript has %d lines, want 11", n)
	}
}

func equalCapture(a, b Capture) bool {
	return a.ID == b.ID && a.Type == b.Type && a.Name == b.Name &&
		a.Description == b.Description && a.Model == b.Model &&
		slices.Equal(a.TurnIDs, b.TurnIDs) && a.Cwd == b.Cwd && a.GitBranch == b.GitBranch &&
		a.StartedAt.Equal(b.StartedAt) && a.LastAt.Equal(b.LastAt) && a.Lines == b.Lines
}

// TestTranscriptCutsToHeads: the kept lines lose attachments, tool result
// objects and request bodies, and every text is cut to its head, while
// usage and the ids a trace pairs stay as written.
func TestTranscriptCutsToHeads(t *testing.T) {
	c, err := Read(context.Background(), workerFile, "")
	if err != nil {
		t.Fatal(err)
	}
	text := string(c.Transcript)
	for _, gone := range []string{
		`"attachment"`, "toolUseResult", "wireToolInputs", "serverClassifierRequest",
		`"params"`, `"signature"`, "hookInfos", `"caller"`, "base64",
	} {
		if strings.Contains(text, gone) {
			t.Errorf("transcript still holds %s", gone)
		}
	}
	for _, kept := range []string{
		`"tool_use_id":"toolu_1"`, `"is_error":true`, `"name":"Write"`, `"promptId":"p2"`,
		`"subtype":"stop_hook_summary"`, `"isSidechain":true`, `"stop_reason":"end_turn"`,
		`"ephemeral_5m_input_tokens":100`, `"thinking_tokens":10`, `"inference_geo":"global"`,
		`"input":{"file_path":"/work/repo/a.go"}`, "no such file\\n[image]",
	} {
		if !strings.Contains(text, kept) {
			t.Errorf("transcript lost %s", kept)
		}
	}
	for line := range strings.Lines(text) {
		if !json.Valid([]byte(line)) {
			t.Errorf("line is not JSON: %.80s", line)
		}
	}
}

// TestHead cuts at a rune boundary and counts the bytes it cut; a short
// string stays whole.
func TestHead(t *testing.T) {
	n := moondb.AgentHeadBytes
	if got := head("short"); got != "short" {
		t.Errorf("head(short) = %q", got)
	}
	long := strings.Repeat("a", n-1) + "é" + "tail"
	got := head(long)
	wantCut := len(long) - (n - 1)
	if got != strings.Repeat("a", n-1)+"...[+"+strconv.Itoa(wantCut)+" bytes]" {
		t.Errorf("head cut mid-rune or miscounted: %q", got[n-4:])
	}
}

// TestCapHead drops whole lines from the head until the rest fits, and
// keeps the last line even when it alone is over the limit.
func TestCapHead(t *testing.T) {
	lines := [][]byte{[]byte("aaaa"), []byte("bbbb"), []byte("cccc")}
	cases := []struct {
		limit   int
		want    string
		wantCut int
	}{
		{100, "aaaa\nbbbb\ncccc\n", 0},
		{10, "bbbb\ncccc\n", 1},
		{9, "cccc\n", 2},
		{2, "cccc\n", 2},
	}
	for _, c := range cases {
		got, cut := capHead(lines, c.limit)
		if string(got) != c.want || cut != c.wantCut {
			t.Errorf("capHead(%d) = %q, %d; want %q, %d", c.limit, got, cut, c.want, c.wantCut)
		}
	}
}

// stopAfter is a context that ends after n checks, so a read stops partway
// through a file.
type stopAfter struct {
	context.Context
	n int
}

func (s *stopAfter) Err() error {
	if s.n <= 0 {
		return context.DeadlineExceeded
	}
	s.n--
	return nil
}

// TestReadPartial: a budget that ends inside the file keeps what was read
// and marks the capture partial.
func TestReadPartial(t *testing.T) {
	c, err := Read(&stopAfter{Context: context.Background(), n: 6}, workerFile, "")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Lines.Partial || c.Lines.Read != 6 || c.Lines.Kept != 5 {
		t.Errorf("Lines = %+v, want 6 read, 5 kept, partial", c.Lines)
	}
	if slices.Contains(c.TurnIDs, "p2") {
		t.Errorf("TurnIDs = %v, read past the budget", c.TurnIDs)
	}
}

// TestReadNoLines: a file of attachments only, or no file, is an error.
func TestReadNoLines(t *testing.T) {
	if _, err := Read(context.Background(), "testdata/agent-aempty-1.jsonl", ""); !errors.Is(
		err, ErrNoLines) {
		t.Errorf("Read(attachments only) error = %v, want ErrNoLines", err)
	}
	if _, err := Read(context.Background(), "testdata/agent-nope.jsonl", ""); err == nil {
		t.Error("Read(missing file) returned no error")
	}
}

// TestKind maps the meta file shapes the harness writes, and a hook's
// agent_type, to a type and a name.
func TestKind(t *testing.T) {
	cases := []struct {
		name               string
		meta               Meta
		hook               string
		wantType, wantName string
	}{
		{"named teammate", Meta{
			AgentType: "ui-builder", Name: "ui-builder",
			SpawnedAgentType: "general-purpose",
		}, "ui-builder", "general-purpose", "ui-builder"},
		{
			"named, kind unknown",
			Meta{AgentType: "probe", Name: "probe"},
			"probe",
			"subagent",
			"probe",
		},
		{
			"named, hook says kind",
			Meta{AgentType: "probe", Name: "probe"},
			"Explore",
			"Explore",
			"probe",
		},
		{"unnamed built-in", Meta{AgentType: "Explore"}, "Explore", "Explore", ""},
		{
			"plugin agent",
			Meta{AgentType: "feature-dev:code-reviewer"},
			"",
			"feature-dev:code-reviewer",
			"",
		},
		{"no meta, hook type", Meta{}, "Plan", "Plan", ""},
		{"nothing", Meta{}, "", "internal", ""},
		{"name equals kind", Meta{Name: "Explore", SpawnedAgentType: "Explore"}, "", "Explore", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			typ, name := Kind(c.meta, c.hook)
			if typ != c.wantType || name != c.wantName {
				t.Errorf("Kind = %q, %q; want %q, %q", typ, name, c.wantType, c.wantName)
			}
		})
	}
}

func TestIDFromPath(t *testing.T) {
	if got := IDFromPath("/x/subagents/agent-aui-builder-ec75.jsonl"); got != "aui-builder-ec75" {
		t.Errorf("IDFromPath = %q", got)
	}
	if got := MetaPath("/x/agent-a1.jsonl"); got != "/x/agent-a1.meta.json" {
		t.Errorf("MetaPath = %q", got)
	}
}

// TestTranscriptTracesAsTheFile: the agent transcript gives the same trace
// as the file it was cut from, so nothing a trace reads is lost.
func TestTranscriptTracesAsTheFile(t *testing.T) {
	c, err := Read(context.Background(), workerFile, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(workerFile)
	if err != nil {
		t.Fatal(err)
	}
	a := store.Agent{ID: c.ID, StartedAt: c.StartedAt, EndedAt: c.LastAt, TurnIDs: c.TurnIDs}
	slim := trace.BuildAgent(a, c.Transcript, nil, trace.Prices{})
	full := trace.BuildAgent(a, raw, nil, trace.Prices{})
	if slim.Totals.ModelCalls != 3 || slim.Totals.ToolCalls != 2 || slim.Totals.ToolErrors != 1 {
		t.Errorf("slim totals = %+v, want 3 model calls, 2 tool calls, 1 failed", slim.Totals)
	}
	if !reflect.DeepEqual(slim.Totals, full.Totals) ||
		!reflect.DeepEqual(slim.Segments, full.Segments) ||
		!reflect.DeepEqual(slim.Spans, full.Spans) ||
		!reflect.DeepEqual(slim.Gaps, full.Gaps) {
		t.Errorf(
			"slim trace differs from the file's:\nslim %+v\nfile %+v",
			slim.Totals,
			full.Totals,
		)
	}
}
