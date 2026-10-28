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
