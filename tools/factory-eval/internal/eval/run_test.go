package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
	"github.com/mad01/thismoon/tools/factory-eval/internal/judge"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

var update = flag.Bool(
	"update",
	false,
	"rewrite schema/results.example.json from the end-to-end run",
)

const (
	fixturePath = "../../schema/results.example.json"
	schemaPath  = "../../schema/results.schema.json"
)

// gitEnv pins identity and dates, so the test repo's commit sha is the same
// on every machine and the fixture stays byte-stable.
var gitEnv = []string{
	"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	"GIT_AUTHOR_DATE=2026-10-16T12:00:00Z", "GIT_COMMITTER_DATE=2026-10-16T12:00:00Z",
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "commit.gpgsign=false",
		"-c", "init.defaultBranch=main",
	}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// outlineDocs returns a CLAUDE.md and README.md pair for c that follow the
// real outline, or deviate from it when broken is set.
func outlineDocs(c cases.Case, broken bool) map[string]string {
	claudeMD := fmt.Sprintf("# %s, a test component\n\n## Module layout\n\n## How it works\n\n"+
		"## Build / install / test\n\n## Gotchas\n\n## See also\n", c.Name())
	readme := fmt.Sprintf("# %s\n\n## Install\n\n## Usage\n\n## Develop\n\n## Docs\n", c.Name())
	if broken {
		claudeMD = fmt.Sprintf("# %s\n\n## Gotchas\n\n## Module layout\n", c.Name())
	}
	return map[string]string{
		c.Component + "/CLAUDE.md": claudeMD,
		c.Component + "/README.md": readme,
	}
}

// scriptedAgent stands in for both arms: it commits outline-conforming docs
// on each case branch, except the branches skip names, and reports the
// canned spend for its arm and model. The arm named by escape also makes a
// branch in the source repo at src, the way a run that reached the user's
// checkout would.
type scriptedAgent struct {
	t      *testing.T
	cases  []cases.Case
	spend  map[string]string
	wall   map[string]time.Duration
	skip   map[string]string
	escape string
	src    string
	called []string
}

func (a *scriptedAgent) Run(_ context.Context, inv claude.Invocation) (claude.Output, error) {
	arm := ArmFactory
	if denied := inv.Args[slices.Index(inv.Args, "--disallowedTools")+1]; strings.Contains(
		denied,
		"Agent",
	) {
		arm = ArmSingle
	}
	key := arm + "/" + inv.Args[slices.Index(inv.Args, "--model")+1]
	a.called = append(a.called, key)
	assertCleanStart(a.t, inv)
	if key == a.escape {
		git(a.t, a.src, "branch", "escaped")
	}
	pin := git(a.t, inv.Dir, "rev-parse", "HEAD")
	for _, c := range a.cases {
		if a.skip[key] == c.Branch {
			continue
		}
		git(a.t, inv.Dir, "switch", "--quiet", "-c", c.Branch)
		writeFiles(a.t, inv.Dir, outlineDocs(c, false))
		git(a.t, inv.Dir, "commit", "--quiet", "-am", "docs sweep")
		git(a.t, inv.Dir, "switch", "--quiet", "--detach", pin)
	}
	return claude.Output{Stdout: []byte(a.spend[key]), Wall: a.wall[key]}, nil
}

// assertCleanStart checks what an arm session starts with: a clone with no
// branch, no tag and no remote, and the harness settings file with its
// deny rules.
func assertCleanStart(t *testing.T, inv claude.Invocation) {
	t.Helper()
	if refs := git(t, inv.Dir, "for-each-ref", "refs/heads", "refs/tags"); refs != "" {
		t.Errorf("the clone holds refs when the arm starts:\n%s", refs)
	}
	if remotes := git(t, inv.Dir, "remote"); remotes != "" {
		t.Errorf("the clone has remotes when the arm starts: %s", remotes)
	}
	settings := inv.Args[slices.Index(inv.Args, "--settings")+1]
	raw, err := os.ReadFile(settings)
	if err != nil || !strings.Contains(string(raw), `"Bash(curl *)"`) {
		t.Errorf("settings file %s = %s, %v; want the deny rules", settings, raw, err)
	}
}

func spendReport(cost float64, input, output, cacheRead int64) string {
	return fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"session_id":"s",`+
		`"total_cost_usd":%.2f,"num_turns":40,"modelUsage":{"m":{"inputTokens":%d,"outputTokens":%d,`+
		`"cacheReadInputTokens":%d,"cacheCreationInputTokens":0,"costUSD":%.2f}},`+
		`"permission_denials":[],"structured_output":{"cases":[]}}`, cost, input, output, cacheRead, cost)
}

// cleanCmd is humanizer with no findings and a green make.
type cleanCmd struct{}

func (cleanCmd) Run(_ context.Context, _, name string, _ ...string) ([]byte, error) {
	if name == "humanizer" {
		return []byte(`{"findings":[]}`), nil
	}
	return nil, nil
}

// fixedScorer gives every judged case an 8.
type fixedScorer struct{}

func (fixedScorer) Score(_ context.Context, req judge.Request) (judge.Verdict, error) {
	if len(req.Glossary) == 0 || req.Glossary[0].Path != cases.RootGlossary {
		return judge.Verdict{}, fmt.Errorf("no root glossary in %+v", req.Glossary)
	}
	return judge.Verdict{Score: 8, Reasons: []string{"one sentence runs long"}, CostUSD: 0.01}, nil
}

// flakyScorer fails the first fails calls, at a cent each, then gives 7.
type flakyScorer struct {
	fails int
	calls int
}

func (f *flakyScorer) Score(context.Context, judge.Request) (judge.Verdict, error) {
	f.calls++
	if f.calls <= f.fails {
		return judge.Verdict{CostUSD: 0.01}, errors.New("judge: call ended error_during_execution")
	}
	return judge.Verdict{Score: 7, Reasons: []string{"fine"}, CostUSD: 0.01}, nil
}

func fixedSnapshot(context.Context) prereq.Installed {
	return prereq.Installed{
		Files: map[string]string{
			"~/.claude/skills/factory/SKILL.md":       "sha256:" + strings.Repeat("ab", 32),
			"~/.claude/agents/factory-worker-high.md": "missing",
		},
		BeltVersion: "d430918",
	}
}

// testRepo makes a source repo with the outline-breaking docs of f's
// cases and a root glossary, and pins f to its one commit.
func testRepo(t *testing.T, f *cases.File) string {
	t.Helper()
	src := t.TempDir()
	git(t, src, "init", "--quiet")
	writeFiles(t, src, map[string]string{cases.RootGlossary: "# test\n\n**Component**:\nA unit.\n"})
	for _, c := range f.Cases {
		writeFiles(t, src, outlineDocs(c, true))
	}
	git(t, src, "add", ".")
	git(t, src, "commit", "--quiet", "-m", "pin")
	f.Commit = git(t, src, "rev-parse", "HEAD")
	return src
}

// testConfig is a batch over src with stand-ins for claude, humanizer,
// make and the judge, one run per arm and tier.
func testConfig(
	t *testing.T,
	f *cases.File,
	src string,
	agent AgentRunner,
	scorer CaseScorer,
) Config {
	clock := time.Date(2026, 10, 16, 18, 0, 0, 0, time.UTC)
	return Config{
		Cases: f, CasesPath: "cases.yaml", Source: src, Home: "/Users/example",
		Arms: []string{ArmSingle, ArmFactory}, Tiers: f.Tiers, Runs: 1, TempParent: t.TempDir(),
		ClaudeVersion: "2.1.301 (Claude Code)",
		Prereqs: []prereq.Result{
			{Name: "claude version", OK: true, Detail: "2.1.301 (Claude Code)"},
			{
				Name: "worker definition factory-worker-medium", OK: true,
				Detail: "~/.claude/agents/factory-worker-medium.md",
			},
		},
		Agent: agent, Checker: judge.Deterministic{Outline: f.Outline, Humanizer: f.Humanizer, Cmd: cleanCmd{}},
		Scorer: scorer, Snapshot: fixedSnapshot, Progress: io.Discard,
		Now: func() time.Time { clock = clock.Add(time.Hour); return clock },
	}
}

// fixtureBatch runs the real loop over a test repo holding the first two
// docs-sweep cases, one run per arm and tier.
func fixtureBatch(t *testing.T) (Results, *scriptedAgent) {
	t.Helper()
	f := loadCases(t)
	f.Cases = f.Cases[:2]
	src := testRepo(t, f)
	agent := &scriptedAgent{
		t: t, cases: f.Cases, src: src,
		spend: map[string]string{
			"single/sonnet":  spendReport(6.10, 90_000, 60_000, 850_000),
			"factory/sonnet": spendReport(5.40, 120_000, 70_000, 1_100_000),
			"single/opus":    spendReport(13.20, 90_000, 80_000, 900_000),
			"factory/opus":   spendReport(11.80, 110_000, 90_000, 1_000_000),
		},
		wall: map[string]time.Duration{
			"single/sonnet": 42 * time.Minute, "factory/sonnet": 16 * time.Minute,
			"single/opus": 51 * time.Minute, "factory/opus": 19 * time.Minute,
		},
		skip: map[string]string{"factory/opus": f.Cases[1].Branch},
	}
	return Run(context.Background(), testConfig(t, f, src, agent, fixedScorer{})), agent
}

func TestRunScoresBothArmsAndAppliesThePassRule(t *testing.T) {
	res, agent := fixtureBatch(t)
	wantOrder := []string{"single/sonnet", "factory/sonnet", "single/opus", "factory/opus"}
	if !slices.Equal(agent.called, wantOrder) {
		t.Errorf("arm sessions ran %v, want interleaved %v", agent.called, wantOrder)
	}
	if len(res.Runs) != 4 {
		t.Fatalf("runs = %d, want 4", len(res.Runs))
	}
	for _, r := range res.Runs[:3] {
		if !r.Pass || r.KeptDir != "" || r.JudgeScore != 8 {
			t.Errorf("%s/%s = pass %v kept %q judge %v, want a clean pass at 8",
				r.Arm, r.Tier, r.Pass, r.KeptDir, r.JudgeScore)
		}
	}
	failed := res.Runs[3]
	if failed.Pass || failed.KeptDir == "" || failed.JudgeScore != 4 {
		t.Errorf(
			"factory/top = pass %v kept %q judge %v, want a failure scored 4 with its dir kept",
			failed.Pass,
			failed.KeptDir,
			failed.JudgeScore,
		)
	}
	if _, err := os.Stat(failed.KeptDir); err != nil {
		t.Errorf("kept dir %s is gone: %v", failed.KeptDir, err)
	}
	// One run per arm against the cases file's three: the PASS is provisional.
	wantPrefix := "provisional, 1 of the cases file's 3 runs per arm: PASS at tier mid"
	if !res.Summary.Pass || !strings.HasPrefix(res.Summary.Decision, wantPrefix) {
		t.Errorf("summary = %+v, want a provisional PASS at the primary tier", res.Summary)
	}
	if res.Summary.Decisions[1].Pass {
		t.Errorf("top decision = %+v, want a failure on the judge score", res.Summary.Decisions[1])
	}
}

// TestResultsFixtureMatchesTheSchemaAndTheCode keeps the committed fixture
// honest: the end-to-end run must reproduce it byte for byte (run with
// -update to rewrite it), and it must validate against the schema file.
func TestResultsFixtureMatchesTheSchemaAndTheCode(t *testing.T) {
	res, _ := fixtureBatch(t)
	normalize(&res)
	got, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture (run with -update to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale; rerun the test with -update and review the diff", fixturePath)
	}
	if err := validate(t, want); err != nil {
		t.Errorf("fixture does not validate against the schema: %v", err)
	}
	if err := validate(t, []byte(`{"schema":"factory-eval/results/v1","runs":"none"}`)); err == nil {
		t.Error("a broken document validates, so the schema check proves nothing")
	}
}

// normalize replaces what changes from machine to machine, or with every
// edit to cases.yaml, with fixed values that read like a real batch.
func normalize(res *Results) {
	const root = "/tmp/factory-eval-123456789"
	res.Commit = "45518e8b8461d491dc2a1aafaf1a8aa56702858a"
	res.CasesDigest = strings.Repeat("5e", 32)
	res.SignOff = cases.SignOff{}
	res.SettingsDeny = Isolation{
		Home: "/Users/example", Source: "/Users/example/code/src/github.com/mad01/thismoon",
	}.DenyRules()
	for i := range res.Runs {
		res.Runs[i].SettingsFile = root + "/" + SettingsFileName
		if res.Runs[i].KeptDir != "" {
			res.Runs[i].KeptDir = root
		}
	}
}

func validate(t *testing.T, doc []byte) error {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	var instance any
	if err := json.Unmarshal(doc, &instance); err != nil {
		t.Fatal(err)
	}
	return resolved.Validate(instance)
}

// TestARunThatReachesTheSourceIsTainted makes the factory arm branch in the
// source repo: the run is tainted, counts as errored, and the tier cannot
// be decided.
func TestARunThatReachesTheSourceIsTainted(t *testing.T) {
	f := loadCases(t)
	f.Cases = f.Cases[:1]
	f.Tiers = f.Tiers[:1]
	src := testRepo(t, f)
	agent := &scriptedAgent{
		t: t, cases: f.Cases, src: src, escape: "factory/sonnet",
		spend: map[string]string{
			"single/sonnet":  spendReport(6, 90_000, 60_000, 850_000),
			"factory/sonnet": spendReport(5, 90_000, 60_000, 850_000),
		},
	}
	res := Run(context.Background(), testConfig(t, f, src, agent, fixedScorer{}))
	single, factory := res.Runs[0], res.Runs[1]
	if single.Tainted || !single.Pass {
		t.Errorf("single = tainted %v pass %v, want a clean pass", single.Tainted, single.Pass)
	}
	if !factory.Tainted || factory.Pass || !factory.Errored() || factory.KeptDir == "" {
		t.Errorf("factory = %+v, want a tainted, errored, kept run", factory)
	}
	if !slices.Contains(factory.Taint, "ref added: refs/heads/escaped") {
		t.Errorf("Taint = %q, want the escaped branch named", factory.Taint)
	}
	if res.Summary.Pass || !strings.Contains(res.Summary.Decision, "incomplete") {
		t.Errorf("decision = %q, want an incomplete failure", res.Summary.Decision)
	}
}

func TestTheJudgeGetsOneRetry(t *testing.T) {
	for _, tt := range []struct {
		name     string
		fails    int
		score    bool
		attempts int
		dollars  float64
	}{
		{"recovers on the retry", 1, true, 2, 0.02},
		{"fails twice", 2, false, 2, 0.02},
		{"first call answers", 0, true, 1, 0.01},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := loadCases(t)
			f.Cases, f.Tiers = f.Cases[:1], f.Tiers[:1]
			src := testRepo(t, f)
			agent := &scriptedAgent{t: t, cases: f.Cases, src: src, spend: map[string]string{
				"single/sonnet": spendReport(6, 90_000, 60_000, 850_000),
			}}
			cfg := testConfig(t, f, src, agent, &flakyScorer{fails: tt.fails})
			cfg.Arms = []string{ArmSingle}
			run := Run(context.Background(), cfg).Runs[0]
			cr := run.Cases[0]
			if (cr.JudgeScore != nil) != tt.score || cr.JudgeAttempts != tt.attempts {
				t.Errorf("case = %+v, want score=%v after %d attempts", cr, tt.score, tt.attempts)
			}
			// A judge that failed twice on an existing branch errors the run,
			// so its 0 never lowers the arm's mean.
			if run.Errored() == tt.score || run.Pass == !tt.score {
				t.Errorf("run = errored %v pass %v (error %q), want errored=%v",
					run.Errored(), run.Pass, run.Error, !tt.score)
			}
			if !tt.score && !strings.Contains(run.Error, "the judge could not score belt") {
				t.Errorf("run error = %q, want it to name the case the judge failed on", run.Error)
			}
			if run.JudgeDollars != tt.dollars {
				t.Errorf(
					"JudgeDollars = %v, want %v: every attempt is paid for",
					run.JudgeDollars,
					tt.dollars,
				)
			}
			if !tt.score && !strings.HasPrefix(cr.JudgeError, "after 2 attempts:") {
				t.Errorf("JudgeError = %q, want it to count the attempts", cr.JudgeError)
			}
		})
	}
}

// apiErrorReport is the result an API error that ends the turn prints:
// subtype success, is_error true, api_error_status, and no errors array.
const apiErrorReport = `{"type":"result","subtype":"success","is_error":true,` +
	`"api_error_status":529,"result":"API Error: 529 overloaded","session_id":"s",` +
	`"total_cost_usd":1.20,"num_turns":3,"modelUsage":{"m":{"inputTokens":9000,` +
	`"outputTokens":800,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"costUSD":1.20}},` +
	`"permission_denials":[]}`

// TestASuccessReportWithIsErrorCountsAsErrored mirrors the probe that found
// a PASS on a run an API error ended: such a run must count as errored, so
// its lowered judge score never enters the mean and the tier is incomplete.
func TestASuccessReportWithIsErrorCountsAsErrored(t *testing.T) {
	f := loadCases(t)
	f.Cases, f.Tiers = f.Cases[:1], f.Tiers[:1]
	src := testRepo(t, f)
	agent := &scriptedAgent{t: t, cases: f.Cases, src: src, spend: map[string]string{
		"single/sonnet":  apiErrorReport,
		"factory/sonnet": spendReport(5, 90_000, 60_000, 850_000),
	}}
	res := Run(context.Background(), testConfig(t, f, src, agent, fixedScorer{}))
	broken := res.Runs[0]
	if broken.Stop != claude.SubtypeSuccess || !broken.Errored() || broken.Pass || broken.KeptDir == "" {
		t.Errorf("run = stop %q errored %v pass %v kept %q, want an errored, kept run",
			broken.Stop, broken.Errored(), broken.Pass, broken.KeptDir)
	}
	for _, want := range []string{"is_error", "api_error_status 529", "API Error: 529 overloaded"} {
		if !strings.Contains(broken.Error, want) {
			t.Errorf("Error = %q, want it to contain %q", broken.Error, want)
		}
	}
	if !strings.Contains(res.Summary.Decision, "single has 0 of 1 runs without error") {
		t.Errorf("decision = %q, want an incomplete tier", res.Summary.Decision)
	}

	broken.Arm, broken.Tier, broken.JudgeScore = ArmSingle, "mid", 2
	a := stats([]RunResult{ok(ArmSingle, "mid", 7, 60_000, 1000), broken}, ArmSingle, "mid")
	if a.RunsErrored != 1 || a.MeanJudgeScore != 7 {
		t.Errorf("stats = %+v, want the errored run left out of the mean of 7", a)
	}
}

func TestProvisionalMarksABatchShortOfTheCasesFileRuns(t *testing.T) {
	if got := provisional("PASS at tier mid", 3, 3); got != "PASS at tier mid" {
		t.Errorf("provisional(3 of 3) = %q, want the decision unchanged", got)
	}
	want := "provisional, 1 of the cases file's 3 runs per arm: PASS at tier mid"
	if got := provisional("PASS at tier mid", 1, 3); got != want {
		t.Errorf("provisional(1 of 3) = %q, want %q", got, want)
	}
}
