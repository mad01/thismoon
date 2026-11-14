package upgradenotes

import (
	"os"
	"strings"
	"testing"
)

// repoDoc is the page this repository ships.
const repoDoc = "../../docs/UPGRADING.md"

// TestRepoDocFollowsTheForm is the gate: a pull request that adds a malformed
// entry to docs/UPGRADING.md fails here.
func TestRepoDocFollowsTheForm(t *testing.T) {
	b, err := os.ReadFile(repoDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", repoDoc, err)
	}
	entries := Parse(string(b))
	if len(entries) == 0 {
		t.Fatalf("%s: no entry found", repoDoc)
	}
	for _, p := range Check(entries) {
		t.Errorf("%s: %s", repoDoc, p)
	}
}

// sample is one well-formed page: an entry in the full form above a legacy
// entry with no Run and no Since. The cases below break it one way each.
const sample = `# Upgrading

Introduction. Every entry has these lines:

- ` + "`Change:`" + ` the PR and what changed.

## 2026-11-20 belt, moondb: the hook entry moves

Change: thismoon #510. The hook entry has a new name.

Run: With ralph: ` + "`ralph up`" + `.

Without ralph: rename the entry by hand.

Verify: ` + "`belt doctor`" + ` is green.

Since: belt 2.12.0, moondb unreleased.

## 2026-10-25 moondb: first install, service before hooks

Change: thismoon #279. The service starts before the hooks.

Verify: ` + "`moondb doctor`" + ` is green.
`

func TestParseReadsEntries(t *testing.T) {
	entries := Parse(sample)

	if got := len(entries); got != 2 {
		t.Fatalf("entries = %d, want 2", got)
	}
	first := entries[0]
	if first.Date != "2026-11-20" || first.Components != "belt, moondb" ||
		first.Title != "the hook entry moves" {
		t.Errorf("heading parsed as date=%q components=%q title=%q",
			first.Date, first.Components, first.Title)
	}
	if !strings.Contains(first.Run, "Without ralph: rename") {
		t.Errorf("Run lost its continuation lines: %q", first.Run)
	}
	if first.Since != "belt 2.12.0, moondb unreleased." {
		t.Errorf("Since = %q", first.Since)
	}
	if problems := Check(entries); len(problems) != 0 {
		t.Errorf("well-formed sample has problems: %v", problems)
	}
}

func TestCheckReportsProblems(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantMsg string
	}{
		{
			name:    "new entry without since",
			old:     "Since: belt 2.12.0, moondb unreleased.\n",
			new:     "",
			wantMsg: "once each, in that order",
		},
		{
			name:    "new entry with labels out of order",
			old:     "Verify: `belt doctor` is green.\n\nSince: belt 2.12.0, moondb unreleased.",
			new:     "Since: belt 2.12.0.\n\nVerify: `belt doctor` is green.",
			wantMsg: "once each, in that order",
		},
		{
			name:    "since names no version",
			old:     "Since: belt 2.12.0, moondb unreleased.",
			new:     "Since: the next belt release.",
			wantMsg: "Since: items must be",
		},
		{
			name:    "no pull request number",
			old:     "Change: thismoon #510. The hook",
			new:     "Change: The hook",
			wantMsg: "must name the pull request",
		},
		{
			name:    "legacy entry without verify",
			old:     "Verify: `moondb doctor` is green.\n",
			new:     "",
			wantMsg: "no text for Verify:",
		},
		{
			name:    "heading without a date",
			old:     "## 2026-11-20 belt, moondb: the hook entry moves",
			new:     "## belt: the hook entry moves",
			wantMsg: "must read",
		},
		{
			name:    "older entry above a newer one",
			old:     "## 2026-10-25 moondb",
			new:     "## 2026-11-25 moondb",
			wantMsg: "newest goes first",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(sample, tt.old) {
				t.Fatalf("sample does not contain %q", tt.old)
			}
			problems := Check(Parse(strings.Replace(sample, tt.old, tt.new, 1)))
			for _, p := range problems {
				if strings.Contains(p.Message, tt.wantMsg) {
					return
				}
			}
			t.Errorf("want a problem containing %q, got %v", tt.wantMsg, problems)
		})
	}
}
