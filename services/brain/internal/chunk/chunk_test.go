package chunk

import (
	"strings"
	"testing"
)

func TestChunk_SmallNote_OneChunk(t *testing.T) {
	note := []byte(`---
title: Tiny
id: 01JABCDEFGHJKMNPQRSTVWXYZ0
tags: [a/b]
type: note
status: validated
---
This is a short body.
Two lines.
`)

	chunks, err := Split("notes/tiny.md", "notes", note, Options{})
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	c := chunks[0]
	if c.Title != "Tiny" {
		t.Errorf("Title = %q", c.Title)
	}
	if c.Tier != "notes" {
		t.Errorf("Tier = %q", c.Tier)
	}
	if c.Path != "notes/tiny.md" {
		t.Errorf("Path = %q", c.Path)
	}
	if c.NoteID != "01JABCDEFGHJKMNPQRSTVWXYZ0" {
		t.Errorf("NoteID = %q, want the frontmatter id", c.NoteID)
	}
	if !strings.Contains(c.Body, "short body") {
		t.Errorf("Body missing content: %q", c.Body)
	}
	if c.HeadingPath != "" {
		t.Errorf("HeadingPath = %q (want empty for prelude)", c.HeadingPath)
	}
	if c.Hash == "" {
		t.Error("Hash empty")
	}
	if c.ID == "" {
		t.Error("ID empty")
	}
}

func TestChunk_HeadingsCarvSections(t *testing.T) {
	note := []byte(`---
title: Sections
---
prelude paragraph
# Top
top body
## Sub A
a body
## Sub B
b body
# Other
other body
`)
	chunks, err := Split("notes/sec.md", "notes", note, Options{})
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}

	wantPaths := []string{
		"",
		"Top",
		"Top › Sub A",
		"Top › Sub B",
		"Other",
	}
	if len(chunks) != len(wantPaths) {
		for i, c := range chunks {
			t.Logf("[%d] %q\n%s\n---", i, c.HeadingPath, c.Body)
		}
		t.Fatalf("got %d chunks, want %d", len(chunks), len(wantPaths))
	}
	for i, c := range chunks {
		if c.HeadingPath != wantPaths[i] {
			t.Errorf("[%d] HeadingPath = %q, want %q", i, c.HeadingPath, wantPaths[i])
		}
	}
}

func TestChunk_FencedCodePreservedAcrossHeadingDetection(t *testing.T) {
	// A # inside a fence must not be treated as a heading.
	note := []byte(`---
title: Code
---
# Real Heading
prose
` + "```go\n# this is not a heading\nfunc f() {}\n```\n" + `more prose
`)
	chunks, err := Split("notes/code.md", "notes", note, Options{})
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	c := chunks[0]
	if c.HeadingPath != "Real Heading" {
		t.Errorf("HeadingPath = %q", c.HeadingPath)
	}
	if !strings.Contains(c.Body, "# this is not a heading") {
		t.Errorf("code fence content lost: %s", c.Body)
	}
}

func TestChunk_LargeSection_SplitsWithOverlap(t *testing.T) {
	// 100 paragraphs of 10 words each = 1000 words. With Target=200 + overlap=30
	// we should get multiple chunks, each ≤ ~230 words, and adjacent chunks
	// must share trailing words.
	var b strings.Builder
	b.WriteString("---\ntitle: Big\n---\n# H\n")
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			b.WriteString("word")
			b.WriteByte('\n')
			_ = j
		}
		b.WriteByte('\n') // blank line → paragraph break
		_ = i
	}
	chunks, err := Split(
		"notes/big.md",
		"notes",
		[]byte(b.String()),
		Options{TargetWords: 200, OverlapWords: 30},
	)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 4 {
		t.Fatalf("got %d chunks, want at least 4 with target=200/words=1000", len(chunks))
	}
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1].Body
		cur := chunks[i].Body
		if !strings.Contains(cur, lastTokens(prev, 5)) {
			t.Errorf("chunk %d does not start with overlap from %d", i, i-1)
		}
	}
}

func TestChunk_DeterministicIDs(t *testing.T) {
	note := []byte(`---
title: Det
---
body one
body two
`)
	a, _ := Split("notes/det.md", "notes", note, Options{})
	b, _ := Split("notes/det.md", "notes", note, Options{})
	if len(a) != len(b) {
		t.Fatalf("len mismatch")
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("[%d] ID drift: %q vs %q", i, a[i].ID, b[i].ID)
		}
		if a[i].Hash != b[i].Hash {
			t.Errorf("[%d] Hash drift", i)
		}
	}
}

func TestChunk_EvidenceFromTag(t *testing.T) {
	note := []byte(`---
title: Graded
tags:
  - evidence/D
---
body
`)
	chunks, err := Split("notes/g.md", "notes", note, Options{})
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d", len(chunks))
	}
	if chunks[0].Evidence != "D" {
		t.Errorf("Evidence = %q, want D", chunks[0].Evidence)
	}
}

func TestChunk_EmptyBody(t *testing.T) {
	note := []byte(`---
title: Empty
---
`)
	chunks, err := Split("notes/e.md", "notes", note, Options{})
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("want 0 chunks, got %d", len(chunks))
	}
}

func TestChunk_FromTokens(t *testing.T) {
	o := FromTokens(800, 100)
	if o.TargetWords != 600 {
		t.Errorf("TargetWords = %d, want 600", o.TargetWords)
	}
	if o.OverlapWords != 75 {
		t.Errorf("OverlapWords = %d, want 75", o.OverlapWords)
	}
}

func lastTokens(s string, n int) string {
	fs := strings.Fields(s)
	if len(fs) <= n {
		return strings.Join(fs, " ")
	}
	return strings.Join(fs[len(fs)-n:], " ")
}
