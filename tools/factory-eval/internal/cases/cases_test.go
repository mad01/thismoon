package cases

import (
	"os"
	"regexp"
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

// signOffValue matches the three sign-off lines, whatever the committed
// file carries. The fixture tests want the file as it reads before a
// sign-off, so validFile blanks them the way the digest does.
var signOffValue = regexp.MustCompile(`(?m)^(  (?:by|date|digest)):.*$`)

func validFile(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return signOffValue.ReplaceAllString(string(raw), `$1: ""`)
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
		{
			"unknown severity", "min_severity: warning", "min_severity: warn",
			`min_severity "warn", want one of suggestion, warning, error`,
		},
		{
			"glossary outside the repo", "glossary: [CONTEXT.md, tools/suspenders/CONTEXT.md]",
			"glossary: [../CONTEXT.md]", `glossary "../CONTEXT.md" is not a clean repo path`,
		},
		{
			"flow-style sign-off", "sign_off:\n  by: \"\"\n  date: \"\"\n  digest: \"\"\n",
			"sign_off: {by: \"\", date: \"\", digest: \"\"}\n", "sign_off must be a block mapping",
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

// signed fills in the sign-off lines of raw, the way the user does after
// reading the dry run.
func signed(raw, by, date, digest string) string {
	return strings.Replace(raw, "sign_off:\n  by: \"\"\n  date: \"\"\n  digest: \"\"\n",
		"sign_off:\n  by: "+by+"\n  date: \""+date+"\"   # approved\n  digest: "+digest+"\n", 1)
}

func TestFillingInTheSignOffKeepsTheDigest(t *testing.T) {
	raw := validFile(t)
	blank, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(blank.Digest()) != 64 {
		t.Fatalf("Digest() = %q, want 64 hex characters", blank.Digest())
	}
	filled, err := Parse([]byte(signed(raw, "Someone Else", "2026-10-17", blank.Digest())))
	if err != nil {
		t.Fatal(err)
	}
	if filled.Digest() != blank.Digest() {
		t.Errorf("signing changed the digest: %s, was %s", filled.Digest(), blank.Digest())
	}
	if filled.SignOff.By != "Someone Else" {
		t.Errorf("SignOff.By = %q, the rewrite reached the parsed value", filled.SignOff.By)
	}
}

func TestCheckSignOff(t *testing.T) {
	raw := validFile(t)
	unsigned, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	digest := unsigned.Digest()
	editedTask := strings.Replace(raw, "Move and reword text only.", "Move text only.", 1)
	tests := []struct {
		name, raw, want string
	}{
		{"digest matches", signed(raw, "someone", "2026-10-17", digest), ""},
		{"blank by", signed(raw, `"  "`, "2026-10-17", digest), "sign_off.by is blank"},
		{"bad date", signed(raw, "someone", "17/10/2026", digest), "is not YYYY-MM-DD"},
		{"no digest", signed(raw, "someone", "2026-10-17", `""`), "sign_off.digest is empty"},
		{
			"task edited after sign-off", signed(editedTask, "someone", "2026-10-17", digest),
			"the file changed after the sign-off",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			err = f.CheckSignOff()
			if tt.want == "" && err != nil {
				t.Errorf("CheckSignOff() = %v, want nil", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Errorf("CheckSignOff() = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestGlossaryFilesDefaultsToTheRootGlossary(t *testing.T) {
	f, err := Load("../../cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Cases[0].GlossaryFiles(); len(got) != 1 || got[0] != RootGlossary {
		t.Errorf("belt GlossaryFiles() = %v, want [%s]", got, RootGlossary)
	}
	if got := f.Cases[3].GlossaryFiles(); len(got) != 2 || got[1] != "tools/suspenders/CONTEXT.md" {
		t.Errorf("suspenders GlossaryFiles() = %v, want the root and its own glossary", got)
	}
}
