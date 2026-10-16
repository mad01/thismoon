package cases

import (
	"os"
	"strings"
	"testing"
)

// TestCommittedCasesFileIsValid keeps the file the user signs off loadable:
// a typo in a key or a missing provenance label fails here, not in a paid run.
func TestCommittedCasesFileIsValid(t *testing.T) {
	f, err := Load("../../cases.yaml")
	if err != nil {
		t.Fatalf("Load(cases.yaml) error = %v", err)
	}
	if len(f.Cases) != 4 {
		t.Errorf("cases = %d, want the four docs-sweep cases", len(f.Cases))
	}
	if got := f.PrimaryTier().Name; got != "mid" {
		t.Errorf("PrimaryTier() = %q, want mid", got)
	}
	for _, c := range f.Cases {
		for _, p := range []string{c.Component + "/README.md", c.Component + "/CLAUDE.md"} {
			if !strings.Contains(c.Task, p) {
				t.Errorf("%s task does not name %s", c.Component, p)
			}
		}
		if !strings.Contains(c.Task, c.Branch) || !strings.Contains(c.Done, c.Branch) {
			t.Errorf("%s task or done criterion does not name the branch %s", c.Component, c.Branch)
		}
	}
}

func validFile(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestParseRejectsBrokenFiles(t *testing.T) {
	tests := []struct {
		name, old, new, want string
	}{
		{"unknown key", "runs: 3", "runz: 3", "field runz not found"},
		{
			"short commit", "commit: 45518e8b8461d491dc2a1aafaf1a8aa56702858a", "commit: 45518e8",
			"not a full 40-character sha",
		},
		{
			"bad provenance", "provenance: measured\n\njudge:", "provenance: guessed\n\njudge:",
			`humanizer provenance "guessed"`,
		},
		{
			"path outside component", "allowed_paths: [tools/belt/README.md,",
			"allowed_paths: [tools/t-man/README.md,", "allows \"tools/t-man/README.md\" outside it",
		},
		{"bad effort", "effort: medium", "effort: extreme", `effort "extreme" unknown`},
		{
			"repeated branch", "branch: eval/docs-sweep-speak", "branch: eval/docs-sweep-belt",
			`branch "eval/docs-sweep-belt" repeated`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := validFile(t)
			if !strings.Contains(raw, tt.old) {
				t.Fatalf("fixture lacks %q", tt.old)
			}
			_, err := Parse([]byte(strings.Replace(raw, tt.old, tt.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestSignOffNeedsNameAndDate(t *testing.T) {
	for _, s := range []SignOff{{}, {By: "someone"}, {Date: "2026-10-17"}} {
		if s.Signed() {
			t.Errorf("%+v.Signed() = true", s)
		}
	}
	if !(SignOff{By: "someone", Date: "2026-10-17"}).Signed() {
		t.Error("a named, dated sign-off is not Signed()")
	}
}
