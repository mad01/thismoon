package scorecard

import (
	"encoding/json"
	"testing"
)

func TestReadStats(t *testing.T) {
	calls := ParseCalls(chunk(
		// A whole file of 320 lines: large, and the largest.
		use(1, "big", "Read", `{"file_path":"/w/repo/tool-results/out.txt"}`),
		result(
			2,
			"big",
			str("1\t..."),
			false,
			fileOwn("/w/repo/tool-results/out.txt", 320, 1, 320),
		),
		// A range of a long file: not large however long the file.
		use(3, "part", "Read", `{"file_path":"/w/repo/record_test.go","offset":490,"limit":50}`),
		result(4, "part", str("490\t..."), false, fileOwn("/w/repo/record_test.go", 50, 490, 735)),
		// No toolUseResult: the numbered lines of the result count.
		use(5, "small", "Read", `{"file_path":"/w/repo/a.go"}`),
		result(
			6,
			"small",
			str("1\tpackage a\n2\t\n3\tfunc A() {}\n<system-reminder>x</system-reminder>"),
			false,
			"",
		),
		use(7, "gone", "Read", `{"file_path":"/w/repo/gone.go"}`),
		result(8, "gone", str("File does not exist."), true, ""),
	), "")
	calls = append(calls, ParseCalls(chunk(
		agentUse(9, "cut", "Read", `{"file_path":"/w/repo/docs/guide.md"}`),
		agentResult(10, "cut", "1\t# Guide\n2\t\n3\tSome processes run...[+2876 bytes]"),
	), "agent-1")...)

	got := readStats(calls)
	if got.Calls != 5 || got.Errors != 1 || got.Unranged != 3 || got.Large != 1 ||
		got.Lines != 373 || got.Unsized != 1 || got.LargeLines != 300 {
		t.Errorf("stats = %+v", got)
	}
	want := ReadCall{Path: "/w/repo/tool-results/out.txt", Lines: 320, TotalLines: 320}
	if got.Largest == nil || *got.Largest != want {
		t.Errorf("largest = %+v, want %+v", got.Largest, want)
	}
}

func TestReadLargeAtThreshold(t *testing.T) {
	for _, tt := range []struct {
		lines int
		large int
	}{{299, 0}, {300, 1}} {
		calls := ParseCalls(chunk(
			use(1, "r", "Read", `{"file_path":"/w/a.go"}`),
			result(2, "r", str("..."), false, fileOwn("/w/a.go", tt.lines, 1, tt.lines)),
		), "")
		if got := readStats(calls).Large; got != tt.large {
			t.Errorf("%d lines: large = %d, want %d", tt.lines, got, tt.large)
		}
	}
}

// TestWholeFile: the tool's own result decides whether a read took the
// whole file when the transcript holds it, whatever the input asked for;
// otherwise the input's range keys do.
func TestWholeFile(t *testing.T) {
	tests := []struct {
		name  string
		input string
		own   string // the toolUseResult, "" for none
		whole bool
	}{
		{name: "no range", input: `{"file_path":"/w/a.go"}`, whole: true},
		{name: "offset, no result", input: `{"file_path":"/w/a.go","offset":10}`},
		{
			name: "offset 1 of the whole file", input: `{"file_path":"/w/a.go","offset":1}`,
			own: fileOwn("/w/a.go", 400, 1, 400), whole: true,
		},
		{
			name: "limit past the end", input: `{"file_path":"/w/a.go","limit":2000}`,
			own: fileOwn("/w/a.go", 500, 1, 500), whole: true,
		},
		{
			name: "limit 0", input: `{"file_path":"/w/a.go","limit":0}`,
			own: fileOwn("/w/a.go", 310, 1, 310), whole: true,
		},
		{
			name: "the head of a long file", input: `{"file_path":"/w/a.go","limit":100}`,
			own: fileOwn("/w/a.go", 100, 1, 900),
		},
		{
			name: "from line 10", input: `{"file_path":"/w/a.go","offset":10}`,
			own: fileOwn("/w/a.go", 391, 10, 400),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := ParseCalls(chunk(
				use(1, "r", "Read", tt.input),
				result(2, "r", str("1\tx"), false, tt.own),
			), "")
			_, ranged := parseReadInput(calls[0].Input)
			if got := wholeFile(calls[0], ranged); got != tt.whole {
				t.Errorf("whole = %t, want %t", got, tt.whole)
			}
		})
	}
}

func TestParseReadInput(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPath   string
		wantRanged bool
	}{
		{name: "whole file", input: `{"file_path":"/w/a.go"}`, wantPath: "/w/a.go"},
		{
			name:       "offset",
			input:      `{"file_path":"/w/a.go","offset":10}`,
			wantPath:   "/w/a.go",
			wantRanged: true,
		},
		{
			name:       "limit",
			input:      `{"file_path":"/w/a.go","limit":40}`,
			wantPath:   "/w/a.go",
			wantRanged: true,
		},
		{
			name:       "pages",
			input:      `{"file_path":"/w/a.pdf","pages":"1-3"}`,
			wantPath:   "/w/a.pdf",
			wantRanged: true,
		},
		{name: "null limit", input: `{"file_path":"/w/a.go","limit":null}`, wantPath: "/w/a.go"},
		{
			name:     "head of a cut input",
			input:    str(`{"file_path":"/w/very/long/a.go","offset":10,"li...[+40 bytes]`),
			wantPath: "/w/very/long/a.go", wantRanged: true,
		},
		{
			name:     "head without a range",
			input:    str(`{"file_path":"/w/very/long/a.go","other":"...[+40 bytes]`),
			wantPath: "/w/very/long/a.go",
		},
		{name: "not an object", input: `42`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, ranged := parseReadInput(json.RawMessage(tt.input))
			if path != tt.wantPath || ranged != tt.wantRanged {
				t.Errorf("= %q, %t; want %q, %t", path, ranged, tt.wantPath, tt.wantRanged)
			}
		})
	}
}

func TestTouched(t *testing.T) {
	calls := ParseCalls(chunk(
		use(1, "r", "Read", `{"file_path":"/w/repo/a.go"}`),
		result(2, "r", str("1\tx"), false, ""),
		use(3, "e", "Edit", `{"file_path":"/w/repo/b.go","old_string":"a","new_string":"b"}`),
		result(4, "e", str("The file has been updated."), false, ""),
		use(5, "n", "NotebookEdit", `{"notebook_path":"/w/repo/n.ipynb"}`),
		result(6, "n", str("ok"), false, ""),
		use(7, "c", "mcp__csl__csl_read", `{"repo":"acme/repo$","file":"services/c.go"}`),
		result(8, "c", texts("acme/repo/services/c.go lines 1-3 of 3"), false, ""),
		use(9, "x", "Edit", `{"file_path":"/w/repo/failed.go"}`),
		result(10, "x", str("String to replace not found"), true, ""),
		use(11, "b", "Bash", `{"command":"cat /w/repo/d.go"}`),
		result(12, "b", str("x"), false, ""),
	), "")
	// A subagent's inputs over the head size are cut to a string head,
	// which still names the file.
	calls = append(calls, ParseCalls(chunk(
		agentUse(13, "ae", "Edit",
			str(`{"file_path":"/w/repo/e.go","old_string":"func old() {\n\treturn...[+300 bytes]`)),
		agentResult(14, "ae", "The file /w/repo/e.go has been updated."),
		agentUse(15, "aw", "Write",
			str(`{"file_path":"/w/repo/f.go","content":"package f...[+900 bytes]`)),
		agentResult(16, "aw", "File created successfully at: /w/repo/f.go"),
		agentUse(17, "ax", "Write", str(`{"content":"package g\n...[+900 bytes]`)),
		agentResult(18, "ax", "File created"),
	), "agent-1")...)
	var got []string
	for _, tp := range touched(calls) {
		got = append(got, tp.path+"|"+tp.repo)
	}
	want := []string{
		"/w/repo/a.go|",
		"/w/repo/b.go|",
		"/w/repo/n.ipynb|",
		"services/c.go|acme/repo$",
		"/w/repo/e.go|",
		"/w/repo/f.go|",
	}
	if len(got) != len(want) {
		t.Fatalf("touched = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("touched[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
