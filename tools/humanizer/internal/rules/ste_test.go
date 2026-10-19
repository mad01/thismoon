package rules

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/humanizer/internal/glossary"
)

const (
	cleanBrief  = "../../testdata/ste/brief-clean.md"
	sloppyBrief = "../../testdata/ste/brief-sloppy.md"
	// fixtureRepo holds a fixture CONTEXT.md, so glossary tests do not depend
	// on the live glossaries of this repo.
	fixtureRepo = "../../testdata/ste/repo"
	// scopedRepo is a two-context fixture tree: a root glossary plus alpha/
	// and beta/ glossaries that apply only below their own directories.
	scopedRepo = "../../testdata/ste/scoped"
	// repoRoot is the thismoon checkout, four levels above this package.
	repoRoot = "../../../.."
)

func TestParseProfile(t *testing.T) {
	tests := []struct {
		in      string
		want    Profile
		wantErr bool
	}{
		{in: "procedure", want: ProfileProcedure},
		{in: " Description ", want: ProfileDescription},
		{in: "", wantErr: true},
		{in: "steps", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseProfile(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseProfile(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("ParseProfile(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRuleFilter(t *testing.T) {
	tests := []struct {
		name      string
		profile   Profile
		requested []string
		wantIn    []string
		wantOut   []string
		wantErr   string
	}{
		{
			name:    "procedure runs procedure rules and the glossary",
			profile: ProfileProcedure,
			wantIn:  []string{"STE.ProcedureSentenceLength", "STE.OneInstruction", "STE.Glossary"},
			wantOut: []string{"STE.DescriptionSentenceLength", "Humanizer.AIVocabulary"},
		},
		{
			name:    "description drops the procedure-only rules",
			profile: ProfileDescription,
			wantIn:  []string{"STE.DescriptionSentenceLength", "STE.IngForm"},
			wantOut: []string{"STE.ProcedureSentenceLength", "STE.OneInstruction"},
		},
		{
			name:      "a requested rule narrows the profile",
			profile:   ProfileProcedure,
			requested: []string{"STE.IngForm"},
			wantIn:    []string{"STE.IngForm"},
			wantOut:   []string{"STE.OneInstruction"},
		},
		{
			name:      "no profile keeps the caller's list",
			requested: []string{"Humanizer.AIVocabulary"},
			wantIn:    []string{"Humanizer.AIVocabulary"},
		},
		{
			name:      "an STE rule without a profile is an error",
			requested: []string{"STE.IngForm"},
			wantErr:   "runs only with a profile",
		},
		{
			name:      "a Humanizer rule in a profile is an error",
			profile:   ProfileProcedure,
			requested: []string{"Humanizer.AIVocabulary"},
			wantErr:   "not in the procedure profile",
		},
		{
			name:      "a description-only rule in procedure is an error",
			profile:   ProfileProcedure,
			requested: []string{"STE.DescriptionSentenceLength"},
			wantErr:   "not in the procedure profile",
		},
		{name: "an unknown profile is an error", profile: "steps", wantErr: "unknown profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ruleFilter(tt.profile, tt.requested)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ruleFilter error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ruleFilter: %v", err)
			}
			for _, id := range tt.wantIn {
				if !slices.Contains(got, id) {
					t.Errorf("filter %v is missing %s", got, id)
				}
			}
			for _, id := range tt.wantOut {
				if slices.Contains(got, id) {
					t.Errorf("filter %v should not hold %s", got, id)
				}
			}
		})
	}
}

func TestRunKey(t *testing.T) {
	if got := runKey(nil); got != defaultRunKey {
		t.Errorf("runKey(nil) = %q, want %q", got, defaultRunKey)
	}
	a, b := runKey([]byte("swap: {a: b}")), runKey([]byte("swap: {a: c}"))
	if a == b {
		t.Errorf("different glossaries share run key %q", a)
	}
	if a != runKey([]byte("swap: {a: b}")) {
		t.Error("the same glossary got two run keys")
	}
	if len(a) != runKeyHexLen {
		t.Errorf("run key %q has length %d, want %d", a, len(a), runKeyHexLen)
	}
}

// TestEnsureSTERunDir pins the run directory layout vale reads: the config,
// the STE style copy, and Glossary.yml only in a glossary's own directory.
func TestEnsureSTERunDir(t *testing.T) {
	cache := t.TempDir()
	gloss := glossary.Render([]glossary.Substitution{{Avoid: "delete", Use: "Retract"}}, nil)

	plain, err := ensureSTERunDir(cache, nil)
	if err != nil {
		t.Fatalf("ensureSTERunDir(nil): %v", err)
	}
	withGloss, err := ensureSTERunDir(cache, gloss)
	if err != nil {
		t.Fatalf("ensureSTERunDir(gloss): %v", err)
	}
	if again, err := ensureSTERunDir(cache, gloss); err != nil || again != withGloss {
		t.Fatalf("second ensureSTERunDir = %q, %v; want %q, nil", again, err, withGloss)
	}
	for _, cfg := range []string{plain, withGloss} {
		ini, err := os.ReadFile(cfg)
		if err != nil || !strings.Contains(string(ini), "BasedOnStyles = STE") {
			t.Errorf("%s: config = %q, %v", cfg, ini, err)
		}
		rule := filepath.Join(filepath.Dir(cfg), "styles", "STE", "IngForm.yml")
		if _, err := os.Stat(rule); err != nil {
			t.Errorf("STE style not copied: %v", err)
		}
	}
	plainGloss := filepath.Join(filepath.Dir(plain), "styles", "STE", glossaryFileName)
	if _, err := os.Stat(plainGloss); !os.IsNotExist(err) {
		t.Errorf("default run dir holds a glossary: stat err = %v", err)
	}
	got, err := os.ReadFile(
		filepath.Join(filepath.Dir(withGloss), "styles", "STE", glossaryFileName),
	)
	if err != nil || string(got) != string(gloss) {
		t.Errorf("glossary in run dir = %q, %v; want the rendered rule", got, err)
	}
}

func requireVale(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vale"); err != nil {
		t.Skip("vale not installed")
	}
}

func detectFixture(t *testing.T, path string, opts DetectOptions) []Finding {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return detectText(t, string(text), opts)
}

func detectText(t *testing.T, text string, opts DetectOptions) []Finding {
	t.Helper()
	opts.CacheDir = t.TempDir()
	findings, err := Detect(context.Background(), text, opts)
	if err != nil {
		t.Fatalf("Detect %q: %v", text, err)
	}
	return findings
}

// repoGlossary loads the glossaries under root and renders the rule for
// target, failing the test when no rule applies.
func repoGlossary(t *testing.T, root, target string) []byte {
	t.Helper()
	repo, err := glossary.Load(root)
	if err != nil {
		t.Fatalf("glossary.Load(%s): %v", root, err)
	}
	gloss := repo.Rule(target)
	if gloss == nil {
		t.Fatalf("glossary rule for %s at %q is empty", root, target)
	}
	return gloss
}

func fixtureGlossary(t *testing.T) []byte {
	t.Helper()
	return repoGlossary(t, fixtureRepo, "")
}

func ruleIDs(findings []Finding) []string {
	ids := make([]string, 0, len(findings))
	for _, f := range findings {
		ids = append(ids, f.RuleID)
	}
	return ids
}

// TestBriefFixtures pins the two task briefs: the clean brief passes the
// procedure profile, with and without the fixture glossary, and the sloppy
// one-sentence brief trips the procedure sentence cap.
func TestBriefFixtures(t *testing.T) {
	requireVale(t)
	fixtureGloss := fixtureGlossary(t)
	for name, gloss := range map[string][]byte{"no glossary": nil, "fixture glossary": fixtureGloss} {
		t.Run("clean brief, "+name, func(t *testing.T) {
			got := detectFixture(
				t,
				cleanBrief,
				DetectOptions{Profile: ProfileProcedure, Glossary: gloss},
			)
			if len(got) != 0 {
				t.Errorf("clean brief findings = %+v, want none", got)
			}
		})
	}
	t.Run("sloppy brief", func(t *testing.T) {
		got := ruleIDs(detectFixture(t, sloppyBrief, DetectOptions{Profile: ProfileProcedure}))
		if !slices.Contains(got, "STE.ProcedureSentenceLength") {
			t.Errorf("sloppy brief rules = %v, want STE.ProcedureSentenceLength", got)
		}
		for _, id := range got {
			if !isSTERule(id) || id == "STE.DescriptionSentenceLength" {
				t.Errorf("procedure run reported %s", id)
			}
		}
	})
}

// TestRealRepoGlossaryLoads runs the glossary generated from this repo's own
// CONTEXT.md files through vale. It asserts only that vale accepts the rule:
// the finding count depends on what the glossaries ban.
func TestRealRepoGlossaryLoads(t *testing.T) {
	requireVale(t)
	gloss := repoGlossary(t, repoRoot, "")
	detectFixture(t, cleanBrief, DetectOptions{Profile: ProfileProcedure, Glossary: gloss})
}

// TestIngFormExceptionsIgnoreCase keeps a capitalized exception quiet at the
// start of a sentence and in a heading, while a real -ing verb still fires.
func TestIngFormExceptionsIgnoreCase(t *testing.T) {
	requireVale(t)
	opts := DetectOptions{Profile: ProfileDescription, Rules: []string{"STE.IngForm"}}
	quiet := "Nothing else failed. Something broke. String values pass. Warning: stop.\n\n" +
		"## Heading\n\nThe setting is nothing during building.\n"
	if got := detectText(t, quiet, opts); len(got) != 0 {
		t.Errorf("capitalized exceptions fired: %+v", got)
	}
	if got := detectText(t, "Updating the file is the next thing.\n", opts); len(got) != 1 {
		t.Errorf("findings = %+v, want one for Updating", got)
	}
}

// TestOneInstructionSkipsConditionals flags a joined "and then" pair but not
// a ", then" that closes a condition.
func TestOneInstructionSkipsConditionals(t *testing.T) {
	requireVale(t)
	opts := DetectOptions{Profile: ProfileProcedure, Rules: []string{"STE.OneInstruction"}}
	if got := detectText(t, "If the build fails, then stop.\n", opts); len(got) != 0 {
		t.Errorf("conditional flagged: %+v", got)
	}
	if got := detectText(t, "Run the tests and then push the branch.\n", opts); len(got) != 1 {
		t.Errorf("findings = %+v, want one for the and-then pair", got)
	}
}

// TestGlossaryKeepsMultiWordTerms checks a banned word inside a correct
// multi-word term stays quiet, in any case, while the bare word still fires.
func TestGlossaryKeepsMultiWordTerms(t *testing.T) {
	requireVale(t)
	const text = "Create an ephemeral page. Share the Ephemeral Page. Open the page.\n"
	got := detectText(t, text, DetectOptions{
		Profile:  ProfileDescription,
		Glossary: fixtureGlossary(t),
		Rules:    []string{"STE.Glossary"},
	})
	bare := strings.LastIndex(text, "page") + 1
	if len(got) != 1 || got[0].Match != "page" || got[0].Column != bare {
		t.Errorf("findings = %+v, want one for the bare page at column %d", got, bare)
	}
}

// TestDefaultDetectSkipsSTE keeps the Humanizer path unchanged: a run with no
// profile never reports an STE finding.
func TestDefaultDetectSkipsSTE(t *testing.T) {
	requireVale(t)
	for _, id := range ruleIDs(detectFixture(t, sloppyBrief, DetectOptions{})) {
		if isSTERule(id) {
			t.Errorf("default run reported %s", id)
		}
	}
}

// TestGlossaryRuleFires runs a generated glossary through vale: the banned
// synonym is flagged at suggestion level with the term in the message.
func TestGlossaryRuleFires(t *testing.T) {
	requireVale(t)
	gloss := glossary.Render([]glossary.Substitution{
		{Avoid: "delete", Use: "Retract"},
		{Avoid: "API key", Use: "Author key"},
	}, nil)
	findings, err := Detect(
		context.Background(),
		"Delete the assertion. Rotate the api key.\n",
		DetectOptions{CacheDir: t.TempDir(), Profile: ProfileDescription, Glossary: gloss},
	)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want two glossary hits", findings)
	}
	for _, f := range findings {
		if f.RuleID != "STE.Glossary" || f.Severity != "suggestion" || f.Category != "ste" {
			t.Errorf("finding = %+v, want an STE.Glossary suggestion in category ste", f)
		}
	}
	if !strings.Contains(findings[0].Message, "Retract") {
		t.Errorf("message = %q, want the glossary term", findings[0].Message)
	}
}

func TestDetectRejectsGlossaryWithoutProfile(t *testing.T) {
	_, err := Detect(context.Background(), "Delete it.\n", DetectOptions{
		CacheDir: t.TempDir(),
		Glossary: []byte("swap: {}\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "only with a profile") {
		t.Fatalf("Detect error = %v, want a glossary-needs-profile error", err)
	}
}

// TestGlossaryScopedByContext runs the fixture tree's scoped rules through
// vale: the same sentence is checked as a file in each context. A synonym
// alpha bans is not flagged in beta's files, and "worker", which the root
// glossary bans but beta defines as a term, is not flagged in beta's files.
func TestGlossaryScopedByContext(t *testing.T) {
	requireVale(t)
	text, err := os.ReadFile(filepath.Join(scopedRepo, "README.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	tests := []struct {
		target string
		want   []string
	}{
		{target: "README.md", want: []string{"manifest", "worker"}},
		{target: "alpha/notes.md", want: []string{"manifest", "version", "worker", "slideshow"}},
		{target: "beta/notes.md", want: []string{"manifest", "runner"}},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := detectText(t, string(text), DetectOptions{
				Profile:  ProfileDescription,
				Glossary: repoGlossary(t, scopedRepo, tt.target),
				Rules:    []string{glossaryRuleID},
			})
			var matches []string
			for _, f := range got {
				matches = append(matches, f.Match)
			}
			if !slices.Equal(matches, tt.want) {
				t.Errorf("glossary hits = %q, want %q", matches, tt.want)
			}
		})
	}
}

// TestBeingReportedOnce keeps one owner for "being": AuxiliaryChain reports
// it and IngForm does not.
func TestBeingReportedOnce(t *testing.T) {
	requireVale(t)
	got := detectText(t, "The file is being updated.\n", DetectOptions{Profile: ProfileDescription})
	if len(got) != 1 || got[0].RuleID != "STE.AuxiliaryChain" || got[0].Match != "being" {
		t.Errorf("findings = %+v, want one STE.AuxiliaryChain hit on being", got)
	}
}

// TestGlossaryRuleNeedsGlossary turns a run that asks for STE.Glossary with
// no glossary into an error, instead of a run that silently finds nothing.
func TestGlossaryRuleNeedsGlossary(t *testing.T) {
	_, err := Detect(context.Background(), "Delete it.\n", DetectOptions{
		CacheDir: t.TempDir(),
		Profile:  ProfileDescription,
		Rules:    []string{glossaryRuleID},
	})
	if err == nil || !strings.Contains(err.Error(), "no glossary to check") {
		t.Fatalf("Detect error = %v, want a no-glossary error", err)
	}
}

// TestEnsureSTERunDirPrunes checks the run dir housekeeping: a run prunes the
// directories unused for longer than runDirMaxAge and keeps recent ones and
// its own, even when its own directory is older on disk.
func TestEnsureSTERunDirPrunes(t *testing.T) {
	cache := t.TempDir()
	runRoot := filepath.Join(cache, steRunRoot)
	gloss := glossary.Render([]glossary.Substitution{{Avoid: "delete", Use: "Retract"}}, nil)
	old := time.Now().Add(-runDirMaxAge - time.Hour)
	recent := time.Now().Add(-time.Hour)
	ages := map[string]time.Time{"stale": old, "recent": recent, runKey(gloss): old}
	for name, at := range ages {
		dir := filepath.Join(runRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Chtimes(dir, at, at); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	if _, err := ensureSTERunDir(cache, gloss); err != nil {
		t.Fatalf("ensureSTERunDir: %v", err)
	}
	for name, wantKept := range map[string]bool{"stale": false, "recent": true, runKey(gloss): true} {
		_, err := os.Stat(filepath.Join(runRoot, name))
		if kept := err == nil; kept != wantKept {
			t.Errorf("run dir %s kept = %v (stat err %v), want %v", name, kept, err, wantKept)
		}
	}
}
