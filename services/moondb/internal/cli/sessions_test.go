package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/server"
)

// TestPrintSessionsByRepo: a repo listing prints the whole cost beside the
// part attributed to the repository, a dash where a session has none; a
// plain listing prints neither.
func TestPrintSessionsByRepo(t *testing.T) {
	repos := []string{"example.com/o/r"}
	rows := []server.SessionSummary{
		{ID: "sess-priced", Title: "split", Repos: repos, Cost: new(2.5), RepoCost: new(1.25)},
		{ID: "sess-none", Title: "unpriced", Repos: repos},
	}
	var plain, byRepo bytes.Buffer
	printSessions(&plain, rows, false)
	printSessions(&byRepo, rows, true)
	if strings.Contains(plain.String(), "ATTRIBUTED") || strings.Contains(plain.String(), "$") {
		t.Errorf("plain listing has cost columns:\n%s", plain.String())
	}
	// Every cell of these rows is one word, so a column is a field index.
	lines := strings.Split(byRepo.String(), "\n")
	head := strings.Fields(lines[0])
	cost, attributed := slices.Index(head, "COST"), slices.Index(head, "ATTRIBUTED")
	if cost < 0 || attributed != cost+1 {
		t.Fatalf("header = %q, want COST then ATTRIBUTED", lines[0])
	}
	for i, want := range [][2]string{{"$2.50", "$1.25"}, {"-", "-"}} {
		f := strings.Fields(lines[i+1])
		if len(f) != len(head) || f[cost] != want[0] || f[attributed] != want[1] {
			t.Errorf("row %d = %q, want cost %s and attributed %s", i, lines[i+1], want[0], want[1])
		}
	}
}
