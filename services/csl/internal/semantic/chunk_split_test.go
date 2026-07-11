package semantic

import (
	"strings"
	"testing"
)

func TestSplitOversizedPreservesContentAndLines(t *testing.T) {
	// Enough 100-char lines to reach ~3x the budget, so the chunk must split
	// into contiguous, gap-free sub-chunks covering all lines.
	rows := make([]string, maxChunkBodyChars*3/100)
	for i := range rows {
		rows[i] = strings.Repeat("x", 100)
	}
	lastLine := 10 + len(rows) - 1
	c := Chunk{
		Repo: "r", Path: "p.go", Lang: "go", Kind: "function_declaration",
		StartLine: 10, EndLine: lastLine, Text: strings.Join(rows, "\n"),
	}

	got := splitOversized([]Chunk{c})
	if len(got) < 2 {
		t.Fatalf("expected split into >=2 sub-chunks, got %d", len(got))
	}
	for _, g := range got {
		if len(g.Text) > maxChunkBodyChars {
			t.Errorf("sub-chunk body %d exceeds budget %d", len(g.Text), maxChunkBodyChars)
		}
		if g.Kind != "function_declaration" {
			t.Errorf("kind not preserved: %q", g.Kind)
		}
		if !strings.Contains(g.EmbedText, "// File: p.go") {
			t.Errorf("breadcrumb missing in EmbedText")
		}
	}
	if got[0].StartLine != 10 {
		t.Errorf("first StartLine = %d, want 10", got[0].StartLine)
	}
	if last := got[len(got)-1]; last.EndLine != lastLine {
		t.Errorf("last EndLine = %d, want %d", last.EndLine, lastLine)
	}
	for i := 1; i < len(got); i++ {
		if got[i].StartLine != got[i-1].EndLine+1 {
			t.Errorf("gap/overlap at sub-chunk %d: StartLine=%d prev EndLine=%d",
				i, got[i].StartLine, got[i-1].EndLine)
		}
	}
}

func TestSplitOversizedTruncatesSingleLongLine(t *testing.T) {
	c := Chunk{
		Repo: "r", Path: "p", Kind: "window",
		StartLine: 1, EndLine: 1, Text: strings.Repeat("a", maxChunkBodyChars*2),
	}
	got := splitOversized([]Chunk{c})
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if n := len([]rune(got[0].Text)); n > maxChunkBodyChars {
		t.Errorf("overlong line not truncated: %d runes", n)
	}
}

func TestSplitOversizedPassThrough(t *testing.T) {
	c := Chunk{Repo: "r", Path: "p", Kind: "window", StartLine: 1, EndLine: 1, Text: "small"}
	got := splitOversized([]Chunk{c})
	if len(got) != 1 || got[0].Text != "small" {
		t.Fatalf("small chunk should pass through unchanged, got %+v", got)
	}
}
