package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// TestPrintInflight: one row per file, its sessions latest first with
// their edit counts, and a dash for a file no commit ever took.
func TestPrintInflight(t *testing.T) {
	at := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	v := server.InflightView{Worktree: "/w", Files: []store.InFlightFile{
		{
			Path: "a.go", LastEditAt: at, LinkedCommit: new("0123456789abcdef"),
			Sessions: []store.InFlightSession{
				{ID: "sess-one-long-id", Title: "fix the parser", Edits: 2},
				{ID: "sess-two", Title: "write docs", Edits: 1},
			},
		},
		{
			Path:       "b.go",
			LastEditAt: at,
			Sessions:   []store.InFlightSession{{ID: "s3", Title: "t", Edits: 1}},
		},
	}}
	var b bytes.Buffer
	printInflight(&b, v)
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "PATH") {
		t.Fatalf("output:\n%s", b.String())
	}
	if !strings.Contains(lines[1], "01234567") ||
		!strings.HasSuffix(lines[1], "sess-one fix the parser (2), sess-two write docs (1)") {
		t.Errorf("a.go row = %q", lines[1])
	}
	if f := strings.Fields(lines[2]); f[0] != "b.go" || f[3] != "-" {
		t.Errorf("b.go row = %q, want a dash for the commit", lines[2])
	}

	b.Reset()
	printInflight(&b, server.InflightView{Worktree: "/w"})
	if got := b.String(); got != "no files in /w have edits no commit has taken yet\n" {
		t.Errorf("empty output = %q", got)
	}
}
