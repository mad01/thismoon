package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture loads a PostToolUse payload captured from Claude Code for the
// Read tool (testdata, file content trimmed), points its file_path at file,
// applies edit to the decoded payload, and returns it re-encoded.
func readFixture(t *testing.T, name, file string, edit func(p map[string]any)) string {
	t.Helper()
	return fixture(t, name, func(p map[string]any) {
		p["tool_input"].(map[string]any)["file_path"] = file
		if edit != nil {
			edit(p)
		}
	})
}

// cslReadFixture loads the csl_read PostToolUse payload: the envelope of the
// captured Read payload around csl_read's input and its text response as
// Claude Code hands an MCP tool's response over, a content array. It reads
// acme/widget/big.go whole, 843 lines capped at 500.
func cslReadFixture(t *testing.T, edit func(p map[string]any)) string {
	t.Helper()
	return fixture(t, "posttooluse-csl-read-whole.json", edit)
}

// fixture loads a testdata payload, applies edit to it when set, and
// returns it re-encoded.
func fixture(t *testing.T, name string, edit func(p map[string]any)) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(p)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// indexedRepoFile builds a hermetic home holding a csl shard for acme/widget
// and a working tree for it, and returns a file inside that tree.
func indexedRepoFile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	index := filepath.Join(home, ".config", "csl", "search-index")
	root := filepath.Join(home, "code", "acme", "widget")
	for _, dir := range []string{index, filepath.Join(root, ".git")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "big.go")
	for _, f := range []string{filepath.Join(index, "acme%2Fwidget_v16.00000.zoekt"), file} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return file
}

// TestRunHintReadOnCapturedPayloads runs `belt hint read` end to end on
// the captured payloads: a ranged read is silent, a whole read of a large
// file in an indexed repo advises once (a null offset and limit count as
// absent), and the same read again is silent. A subagent, which carries the parent's
// session id plus its own agent_id, gets its own nudge for the same file.
func TestRunHintReadOnCapturedPayloads(t *testing.T) {
	file := indexedRepoFile(t)
	inSubagent := func(p map[string]any) { p["agent_id"] = "a-explore" }
	nullRange := func(p map[string]any) {
		p["tool_input"].(map[string]any)["offset"] = nil
		p["tool_input"].(map[string]any)["limit"] = nil
	}
	steps := []struct {
		name, fixture string
		edit          func(p map[string]any)
		wantAdvice    bool
	}{
		{"ranged read", "posttooluse-read-ranged.json", nil, false},
		{"null offset and limit read as no range", "posttooluse-read-whole.json", nullRange, true},
		{"whole read of the same file", "posttooluse-read-whole.json", nil, false},
		{"whole read in a subagent", "posttooluse-read-whole.json", inSubagent, true},
		{"whole read in that subagent again", "posttooluse-read-whole.json", inSubagent, false},
	}
	for _, s := range steps {
		var out bytes.Buffer
		var a archived
		raw := readFixture(t, s.fixture, file, s.edit)
		runner{load: defaults, emit: a.emit}.hints("read", strings.NewReader(raw), &out)
		if !s.wantAdvice {
			if out.Len() != 0 || len(a.events) != 0 {
				t.Errorf(
					"%s: wrote %q and archived %+v, want silence",
					s.name,
					out.String(),
					a.events,
				)
			}
			continue
		}
		var d adviceDecision
		if err := json.Unmarshal(out.Bytes(), &d); err != nil {
			t.Fatalf("%s: decode advice from %q: %v", s.name, out.String(), err)
		}
		got := d.HookSpecificOutput
		if got.HookEventName != "PostToolUse" ||
			!strings.HasPrefix(got.AdditionalContext, "belt[read-size]: "+file+" is 401 lines") {
			t.Errorf(
				"%s: advice = %+v, want PostToolUse belt[read-size] naming the file and 401 lines",
				s.name,
				got,
			)
		}
		wantAsk := `csl_ask(question: "<what you need from it>", queries: ["f:^big\\.go$"], repo: "^acme/widget$")`
		if !strings.Contains(got.AdditionalContext, wantAsk) {
			t.Errorf("%s: advice %q does not name %s", s.name, got.AdditionalContext, wantAsk)
		}
		if len(a.events) != 1 || a.events[0].Tags["hint"] != "read-size" {
			t.Errorf("%s: archived %+v, want one event tagged hint read-size", s.name, a.events)
		}
	}
}

// TestRunHintReadStaysQuietOnShapesItDoesNotKnow: a mistyped offset or an
// unknown response shape is silence with no warn event. Only a mistyped
// file_path, the one key decoded strictly, warns (see decode_test.go).
func TestRunHintReadStaysQuietOnShapesItDoesNotKnow(t *testing.T) {
	tests := []struct {
		name string
		edit func(p map[string]any)
	}{
		{
			"string offset",
			func(p map[string]any) { p["tool_input"].(map[string]any)["offset"] = "10" },
		},
		{"image response", func(p map[string]any) {
			p["tool_response"] = map[string]any{
				"type": "image",
				"file": map[string]any{"base64": "AA"},
			}
		}},
		{"notebook response", func(p map[string]any) {
			p["tool_response"] = map[string]any{"type": "notebook", "file": map[string]any{
				"filePath": "/r/a.ipynb", "cells": []any{map[string]any{"cell_type": "code"}},
			}}
		}},
		{"string line count", func(p map[string]any) {
			p["tool_response"].(map[string]any)["file"].(map[string]any)["totalLines"] = "401"
		}},
		{"no tool_response", func(p map[string]any) { delete(p, "tool_response") }},
		{"no session id", func(p map[string]any) { delete(p, "session_id") }},
		{"numeric session id", func(p map[string]any) { p["session_id"] = 7 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := indexedRepoFile(t)
			var out bytes.Buffer
			var a archived
			raw := readFixture(t, "posttooluse-read-whole.json", file, tt.edit)
			runner{load: defaults, emit: a.emit}.hints("read", strings.NewReader(raw), &out)
			if out.Len() != 0 || len(a.events) != 0 {
				t.Errorf("wrote %q and archived %+v, want silence", out.String(), a.events)
			}
		})
	}
}

// TestRunHintReadOnCSLReadPayloads runs `belt hint read` end to end on
// csl_read payloads. A ranged read is silent, and a start_line of 0 reads as
// no range, as csl reads it. The whole read advises once, naming the file
// as csl does, and a later Read of the same file is silent: both tools key
// the same marker. A subagent's json-form read gets its own nudge, whether
// the json arrives in a text block, as a string, or as an object.
func TestRunHintReadOnCSLReadPayloads(t *testing.T) {
	file := indexedRepoFile(t)
	input := func(key string, v any) func(p map[string]any) {
		return func(p map[string]any) { p["tool_input"].(map[string]any)[key] = v }
	}
	ranged := func(p map[string]any) {
		input("start_line", 1)(p)
		input("end_line", 80)(p)
	}
	jsonInSubagent := func(p map[string]any) {
		p["agent_id"] = "a-explore"
		input("response_format", "json")(p)
		p["tool_response"] = []any{map[string]any{"type": "text", "text": `{"repo":"acme/widget",` +
			`"path":"big.go","lines":[{"number":1,"text":"package widget"}],"total_lines":843,"truncated":true}`}}
	}
	const structured = `{"repo":"acme/widget","path":"big.go",` +
		`"lines":[{"number":1,"text":"package widget"}],"total_lines":843,"truncated":true}`
	jsonStringInSubagent := func(p map[string]any) {
		p["agent_id"] = "a-review"
		input("response_format", "json")(p)
		p["tool_response"] = structured
	}
	structuredInSubagent := func(p map[string]any) {
		p["agent_id"] = "a-plan"
		input("response_format", "json")(p)
		var obj map[string]any
		if err := json.Unmarshal([]byte(structured), &obj); err != nil {
			t.Fatal(err)
		}
		p["tool_response"] = obj
	}
	steps := []struct {
		name, raw  string
		wantAdvice bool
	}{
		{"ranged csl_read", cslReadFixture(t, ranged), false},
		{"start_line 0 reads as no range", cslReadFixture(t, input("start_line", 0)), true},
		{"whole csl_read again", cslReadFixture(t, nil), false},
		{"Read of the same file", readFixture(t, "posttooluse-read-whole.json", file, nil), false},
		{"json form in a subagent", cslReadFixture(t, jsonInSubagent), true},
		{"json string in another subagent", cslReadFixture(t, jsonStringInSubagent), true},
		{"json object in a third subagent", cslReadFixture(t, structuredInSubagent), true},
	}
	for _, s := range steps {
		var out bytes.Buffer
		var a archived
		runner{load: defaults, emit: a.emit}.hints("read", strings.NewReader(s.raw), &out)
		if !s.wantAdvice {
			if out.Len() != 0 || len(a.events) != 0 {
				t.Errorf(
					"%s: wrote %q and archived %+v, want silence",
					s.name,
					out.String(),
					a.events,
				)
			}
			continue
		}
		var d adviceDecision
		if err := json.Unmarshal(out.Bytes(), &d); err != nil {
			t.Fatalf("%s: decode advice from %q: %v", s.name, out.String(), err)
		}
		got := d.HookSpecificOutput
		wantHead := "belt[read-size]: acme/widget/big.go is 843 lines and was read without a range through csl_read."
		if got.HookEventName != "PostToolUse" ||
			!strings.HasPrefix(got.AdditionalContext, wantHead) {
			t.Errorf("%s: advice = %+v, want PostToolUse starting %q", s.name, got, wantHead)
		}
		wantAsk := `csl_ask(question: "<what you need from it>", queries: ["f:^big\\.go$"], repo: "^acme/widget$")`
		if !strings.Contains(got.AdditionalContext, wantAsk) {
			t.Errorf("%s: advice %q does not name %s", s.name, got.AdditionalContext, wantAsk)
		}
		if len(a.events) != 1 || a.events[0].Tags["hint"] != "read-size" {
			t.Errorf("%s: archived %+v, want one event tagged hint read-size", s.name, a.events)
		}
	}
}

// TestRunHintCSLReadStaysQuietOnShapesItDoesNotKnow: a mistyped bound reads
// as a range, and a response that is neither a content array nor csl's
// structured object, or whose text carries no header, reads as an unknown
// line count. All are silence with
// no warn event.
func TestRunHintCSLReadStaysQuietOnShapesItDoesNotKnow(t *testing.T) {
	tests := []struct {
		name string
		edit func(p map[string]any)
	}{
		{
			"string end_line",
			func(p map[string]any) { p["tool_input"].(map[string]any)["end_line"] = "80" },
		},
		{"response is an object without repo and path", func(p map[string]any) {
			p["tool_response"] = map[string]any{
				"type": "text",
				"text": "acme/widget/big.go lines 1-500 of 843",
			}
		}},
		{"another response format", func(p map[string]any) {
			p["tool_response"] = []any{
				map[string]any{"type": "text", "text": "repo: acme/widget\ntotal_lines: 843"},
			}
		}},
		{"no tool_response", func(p map[string]any) { delete(p, "tool_response") }},
		{"no session id", func(p map[string]any) { delete(p, "session_id") }},
		{"repo csl does not index", func(p map[string]any) {
			p["tool_response"] = []any{
				map[string]any{"type": "text", "text": "acme/other/big.go lines 1-500 of 843"},
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexedRepoFile(t)
			var out bytes.Buffer
			var a archived
			raw := cslReadFixture(t, tt.edit)
			runner{load: defaults, emit: a.emit}.hints("read", strings.NewReader(raw), &out)
			if out.Len() != 0 || len(a.events) != 0 {
				t.Errorf("wrote %q and archived %+v, want silence", out.String(), a.events)
			}
		})
	}
}
