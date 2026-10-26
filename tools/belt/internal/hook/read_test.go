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
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	p["tool_input"].(map[string]any)["file_path"] = file
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
