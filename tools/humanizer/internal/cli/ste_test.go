package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	steCleanBrief  = "../../testdata/ste/brief-clean.md"
	steSloppyBrief = "../../testdata/ste/brief-sloppy.md"
	// steScopedRepo is a two-context fixture tree: a root glossary plus
	// alpha/ and beta/ glossaries that apply only below their directories.
	steScopedRepo = "../../testdata/ste/scoped"
)

// runSTECmd executes `humanizer ste` with args and returns its output, the
// exit code the process would use, and the error Execute returned. Flags bind
// to package vars cobra does not reset, so the helper restores the defaults
// first, and the style-pack cache goes to a temp dir.
func runSTECmd(t *testing.T, args ...string) (string, int, error) {
	t.Helper()
	t.Setenv("HUMANIZER_CACHE_DIR", t.TempDir())
	steProfile, steRepo, stePath, steJSON, steMinSeverity, steGlossaryOut = "", "", "", false, "", ""
	pendingExitCode = 0
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"ste"}, args...))
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		pendingExitCode = 0
	})
	err := rootCmd.Execute()
	code := pendingExitCode
	if err != nil {
		code = ExitCode(err)
	}
	return buf.String(), code, err
}

func requireVale(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vale"); err != nil {
		t.Skip("vale not installed")
	}
}

// TestSTECmdJSON pins the integration contract: exit 1 with findings and 0
// without, and a payload carrying findings, summary, engine, and profile.
func TestSTECmdJSON(t *testing.T) {
	requireVale(t)
	tests := []struct {
		name     string
		file     string
		wantCode int
		wantRule string
	}{
		{
			name:     "sloppy brief has findings",
			file:     steSloppyBrief,
			wantCode: 1,
			wantRule: "STE.ProcedureSentenceLength",
		},
		{name: "clean brief is clean", file: steCleanBrief, wantCode: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code, err := runSTECmd(t, "--profile", "procedure", "--json", tt.file)
			if err != nil || code != tt.wantCode {
				t.Fatalf("exit = %d, err = %v; want %d, nil\n%s", code, err, tt.wantCode, out)
			}
			var payload struct {
				Findings []struct {
					RuleID   string `json:"rule_id"`
					Severity string `json:"severity"`
					Line     int    `json:"line"`
				} `json:"findings"`
				Summary struct {
					Total int `json:"total"`
				} `json:"summary"`
				Engine  string `json:"engine"`
				Profile string `json:"profile"`
			}
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatalf("decode payload: %v\n%s", err, out)
			}
			if payload.Engine != "vale" || payload.Profile != "procedure" {
				t.Errorf(
					"engine, profile = %q, %q; want vale, procedure",
					payload.Engine,
					payload.Profile,
				)
			}
			if payload.Findings == nil || payload.Summary.Total != len(payload.Findings) {
				t.Errorf(
					"findings = %v, summary total = %d",
					payload.Findings,
					payload.Summary.Total,
				)
			}
			if tt.wantRule != "" && !strings.Contains(out, `"rule_id": "`+tt.wantRule+`"`) {
				t.Errorf("payload has no %s finding:\n%s", tt.wantRule, out)
			}
		})
	}
}

// TestSTECmdErrorsExitTwo keeps exit 1 for findings only: usage and input
// errors exit 2.
func TestSTECmdErrorsExitTwo(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing profile", args: []string{steCleanBrief}},
		{name: "unknown profile", args: []string{"--profile", "steps", steCleanBrief}},
		{name: "unknown flag", args: []string{"--profile", "procedure", "--bogus"}},
		{
			name: "two files",
			args: []string{"--profile", "procedure", steCleanBrief, steSloppyBrief},
		},
		{name: "missing file", args: []string{"--profile", "procedure", "no-such-file.md"}},
		{name: "glossary with an argument", args: []string{"glossary", "extra"}},
		{name: "glossary with an unknown flag", args: []string{"glossary", "--bogus"}},
		{
			name: "missing repo",
			args: []string{"--profile", "procedure", "--repo", "/no/such/repo", steCleanBrief},
		},
		{
			name: "path without repo",
			args: []string{"--profile", "procedure", "--path", "README.md", steCleanBrief},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code, err := runSTECmd(t, tt.args...)
			if err == nil || code != steExitError {
				t.Errorf(
					"exit = %d, err = %v; want %d and an error\n%s",
					code,
					err,
					steExitError,
					out,
				)
			}
		})
	}
}

func TestSTEGlossaryCmd(t *testing.T) {
	repo := t.TempDir()
	ctx := "**Tool**:\nA CLI.\n_Avoid_: binary, utility\n"
	if err := os.WriteFile(filepath.Join(repo, "CONTEXT.md"), []byte(ctx), 0o600); err != nil {
		t.Fatalf("write CONTEXT.md: %v", err)
	}
	out, code, err := runSTECmd(t, "glossary", "--repo", repo)
	if err != nil || code != 0 || !strings.Contains(out, "'binary': 'Tool'") {
		t.Fatalf("glossary to stdout: exit %d, err %v\n%s", code, err, out)
	}
	file := filepath.Join(t.TempDir(), "Glossary.yml")
	if _, _, err := runSTECmd(t, "glossary", "--repo", repo, "-o", file); err != nil {
		t.Fatalf("glossary -o: %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != out {
		t.Errorf("-o file = %q, %v; want the stdout rule", got, err)
	}
	if _, code, err := runSTECmd(t, "glossary"); err == nil || code != steExitError {
		t.Errorf("glossary without --repo: exit %d, err %v; want %d", code, err, steExitError)
	}
}

// glossaryMatches runs `humanizer ste --json` with args and returns the
// matched text of its STE.Glossary findings.
func glossaryMatches(t *testing.T, args ...string) []string {
	t.Helper()
	out, _, err := runSTECmd(t, append([]string{"--profile", "description", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("ste %v: %v\n%s", args, err, out)
	}
	var payload struct {
		Findings []struct {
			RuleID string `json:"rule_id"`
			Match  string `json:"match"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, out)
	}
	var matches []string
	for _, f := range payload.Findings {
		if f.RuleID == "STE.Glossary" {
			matches = append(matches, f.Match)
		}
	}
	return matches
}

// TestSTECmdScopesGlossaryByFile checks that a file argument picks the
// glossaries on its own path, and that --path overrides it.
func TestSTECmdScopesGlossaryByFile(t *testing.T) {
	requireVale(t)
	readme := filepath.Join(steScopedRepo, "README.md")
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root file", args: []string{readme}, want: []string{"manifest", "worker"}},
		{
			name: "beta file",
			args: []string{filepath.Join(steScopedRepo, "beta", "notes.md")},
			want: []string{"manifest", "runner"},
		},
		{
			name: "path overrides the file argument",
			args: []string{"--path", filepath.Join(steScopedRepo, "alpha"), readme},
			want: []string{"manifest", "version", "worker", "slideshow"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := glossaryMatches(t, append([]string{"--repo", steScopedRepo}, tt.args...)...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("glossary hits = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSTEGlossaryCmdPathAndWarnings checks that `ste glossary --path` prints
// the scoped rule and that a synonym the rule cannot match is reported on
// stderr instead of rendered.
func TestSTEGlossaryCmdPathAndWarnings(t *testing.T) {
	repo := t.TempDir()
	root := "**Tool**:\nA CLI.\n_Avoid_: binary, --bin\n"
	if err := os.WriteFile(filepath.Join(repo, "CONTEXT.md"), []byte(root), 0o600); err != nil {
		t.Fatalf("write CONTEXT.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "deck"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	deck := "**Step**:\nOne press.\n_Avoid_: stage\n"
	if err := os.WriteFile(filepath.Join(repo, "deck", "CONTEXT.md"), []byte(deck), 0o600); err != nil {
		t.Fatalf("write deck/CONTEXT.md: %v", err)
	}
	readme := filepath.Join(repo, "README.md")
	out, code, err := runSTECmd(t, "glossary", "--repo", repo, "--path", readme)
	if err != nil || code != 0 {
		t.Fatalf("glossary --path: exit %d, err %v\n%s", code, err, out)
	}
	if !strings.Contains(out, "'binary': 'Tool'") || strings.Contains(out, "'stage'") {
		t.Errorf("rule at README.md should ban binary only:\n%s", out)
	}
	if !strings.Contains(out, `humanizer: CONTEXT.md:3: skipped "--bin"`) ||
		strings.Contains(out, `'\-\-bin'`) {
		t.Errorf("want a stderr warning for --bin and no swap key:\n%s", out)
	}
	out, _, err = runSTECmd(t, "glossary", "--repo", repo, "--path", filepath.Join(repo, "deck"))
	if err != nil || !strings.Contains(out, "'stage': 'Step'") {
		t.Errorf("rule at deck/ should ban stage: err %v\n%s", err, out)
	}
}
