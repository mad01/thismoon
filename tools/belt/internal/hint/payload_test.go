package hint

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJoinsMultipleHints(t *testing.T) {
	out := Render([]Advice{{Hint: "a", Text: "first"}, {Hint: "b", Text: "second"}})
	if !strings.Contains(out, "belt[a]: first") || !strings.Contains(out, "belt[b]: second") {
		t.Errorf("Render = %q, want both hints attributed", out)
	}
	if Render(nil) != "" {
		t.Error("Render(nil) must be empty so the caller emits no JSON")
	}
}

func TestReadTotalLines(t *testing.T) {
	tests := []struct {
		name, raw string
		want      int
	}{
		{
			"text read",
			`{"type":"text","file":{"filePath":"/r/a.go","content":"x","numLines":5,"startLine":10,"totalLines":401}}`,
			401,
		},
		{"image read", `{"type":"image","file":{"base64":"AAAA","type":"image/png"}}`, 0},
		{
			"notebook read",
			`{"type":"notebook","file":{"filePath":"/r/a.ipynb","cells":[{"cell_type":"code","source":"x"}]}}`,
			0,
		},
		{"no file object", `{"type":"text"}`, 0},
		{"no line count", `{"type":"text","file":{"filePath":"/r/a.go"}}`, 0},
		{"line count is a string", `{"type":"text","file":{"totalLines":"401"}}`, 0},
		{"line count is fractional", `{"type":"text","file":{"totalLines":4.5}}`, 0},
		{"negative line count", `{"type":"text","file":{"totalLines":-3}}`, 0},
		{"type is not a string", `{"type":1,"file":{"totalLines":401}}`, 0},
		{"string envelope", `"{\"type\":\"text\"}"`, 0},
		{"null", `null`, 0},
		{"empty", ``, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadTotalLines(json.RawMessage(tt.raw)); got != tt.want {
				t.Errorf("ReadTotalLines(%s) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestIsCSLRead(t *testing.T) {
	tests := []struct {
		tool string
		want bool
	}{
		{"mcp__csl__csl_read", true},
		{"mcp__code_search__csl_read", true},
		{"mcp__csl__csl_read_more", false},
		{"mcp__csl__csl_search", false},
		{"csl_read", false},
		{"Read", false},
	}
	for _, tt := range tests {
		if got := IsCSLRead(tt.tool); got != tt.want {
			t.Errorf("IsCSLRead(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}

// TestCSLReadFile pins the parse of csl_read's response in the shapes a
// hook can receive: content blocks whose first text block holds the text
// form (a header line) or the json form, a JSON string holding either, as
// Claude Code records a structured result, and the json form's object
// itself. The json form keeps repo and path apart; the text header joins
// them into one name.
func TestCSLReadFile(t *testing.T) {
	text := func(s string) string {
		raw, err := json.Marshal([]map[string]string{{"type": "text", "text": s}})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	str := func(s string) string {
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	const structured = `{"repo":"acme/widget","path":"internal/big.go",` +
		`"lines":[{"number":1,"text":"package widget"}],"total_lines":843,"truncated":true}`
	named := func(name string) CSLFile { return CSLFile{Name: name} }
	split := CSLFile{Repo: "acme/widget", Path: "internal/big.go"}
	tests := []struct {
		name, raw string
		wantFile  CSLFile
		wantLines int
	}{
		{
			"text form, whole read capped at 500 lines",
			text(
				"mad01/thismoon/services/present/internal/mermaid/mermaid.go lines 1-500 of 843\n" +
					"1:package mermaid\ntruncated: read more with start_line/end_line",
			),
			named("mad01/thismoon/services/present/internal/mermaid/mermaid.go"), 843,
		},
		{
			"text form, short file",
			text("acme/widget/big.go lines 1-12 of 12\n1:package widget"),
			named("acme/widget/big.go"), 12,
		},
		{
			"text form, a space in the path",
			text("acme/widget/my dir/big.go lines 1-500 of 900\n1:x"),
			named("acme/widget/my dir/big.go"), 900,
		},
		{"json form in a text block", text(structured), split, 843},
		{"json form as a JSON string", str(structured), split, 843},
		{"json form as an object", structured, split, 843},
		{
			"text form as a JSON string",
			str("acme/widget/big.go lines 1-500 of 843\n1:package widget"),
			named("acme/widget/big.go"), 843,
		},
		{"a JSON string of another format", str("repo: acme/widget"), CSLFile{}, 0},
		{
			"object, repo with a subgroup",
			`{"repo":"group/sub","path":"tool/cmd/main.go","total_lines":412}`,
			CSLFile{Repo: "group/sub", Path: "tool/cmd/main.go"},
			412,
		},
		{
			"json form without a line count",
			text(`{"repo":"acme/widget","path":"big.go","lines":[]}`),
			CSLFile{Repo: "acme/widget", Path: "big.go"},
			0,
		},
		{"json form without a repo", text(`{"path":"big.go","total_lines":843}`), CSLFile{}, 0},
		{"object without a path", `{"repo":"acme/widget","total_lines":843}`, CSLFile{}, 0},
		{
			"object with a string line count",
			`{"repo":"acme/widget","path":"big.go","total_lines":"843"}`,
			CSLFile{},
			0,
		},
		{
			"a text block after another block",
			`[{"type":"image","data":"AA"},{"type":"text","text":"acme/widget/big.go lines 1-500 of 843"}]`,
			named("acme/widget/big.go"), 843,
		},
		{"empty range", text("acme/widget/big.go no lines in range (843 lines)"), CSLFile{}, 0},
		{
			"another response format",
			text("repo: acme/widget\npath: big.go\ntotal_lines: 843"),
			CSLFile{},
			0,
		},
		{"error text", text("repo \"nope\" matched no repo"), CSLFile{}, 0},
		{
			"line count too large",
			text("acme/widget/big.go lines 1-2 of 99999999999999999999"),
			CSLFile{},
			0,
		},
		{"no text block", `[{"type":"image","data":"AA"}]`, CSLFile{}, 0},
		{"empty content", `[]`, CSLFile{}, 0},
		{
			"a lone text block object, not content blocks",
			`{"type":"text","text":"acme/widget/big.go lines 1-2 of 843"}`,
			CSLFile{},
			0,
		},
		{"null", `null`, CSLFile{}, 0},
		{"empty", ``, CSLFile{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, lines := CSLReadFile(json.RawMessage(tt.raw))
			if file != tt.wantFile || lines != tt.wantLines {
				t.Errorf(
					"CSLReadFile(%s) = %+v, %d; want %+v, %d",
					tt.raw, file, lines, tt.wantFile, tt.wantLines,
				)
			}
			if file.Known() != (tt.wantFile != CSLFile{}) {
				t.Errorf("Known() = %v for %+v", file.Known(), file)
			}
		})
	}
}
