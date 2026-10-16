package eval

import (
	"bytes"
	"context"
	"encoding/json"
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
// canned spend for its arm and model.
type scriptedAgent struct {
	t      *testing.T
	cases  []cases.Case
	spend  map[string]string
	wall   map[string]time.Duration
	skip   map[string]string
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

func (fixedScorer) Score(context.Context, string, []judge.FilePair) (judge.Verdict, error) {
	return judge.Verdict{Score: 8, Reasons: []string{"one sentence runs long"}, CostUSD: 0.01}, nil
}

// fixtureBatch runs the real loop over a test repo holding the first two
// docs-sweep cases, one run per arm and tier, with stand-ins for claude,
// humanizer, make and the judge.
func fixtureBatch(t *testing.T) (Results, *scriptedAgent) {
	t.Helper()
	f := loadCases(t)
	f.Cases = f.Cases[:2]
	src := t.TempDir()
	git(t, src, "init", "--quiet")
	for _, c := range f.Cases {
		writeFiles(t, src, outlineDocs(c, true))
	}
	git(t, src, "add", ".")
	git(t, src, "commit", "--quiet", "-m", "pin")
	f.Commit = git(t, src, "rev-parse", "HEAD")
	agent := &scriptedAgent{
		t: t, cases: f.Cases,
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
	clock := time.Date(2026, 10, 16, 18, 0, 0, 0, time.UTC)
	res := Run(context.Background(), Config{
		Cases: f, CasesPath: "cases.yaml", Source: src, Arms: []string{ArmSingle, ArmFactory},
		Tiers: f.Tiers, Runs: 1, TempParent: t.TempDir(), ClaudeVersion: "2.1.301 (Claude Code)",
		Prereqs: []prereq.Result{
			{Name: "claude version", OK: true, Detail: "2.1.301 (Claude Code)"},
			{
				Name: "worker definition factory-worker-medium", OK: true,
				Detail: "~/.claude/agents/factory-worker-medium.md",
			},
		},
		Agent: agent, Checker: judge.Deterministic{Outline: f.Outline, Humanizer: f.Humanizer, Cmd: cleanCmd{}},
		Scorer: fixedScorer{}, Progress: io.Discard,
		Now: func() time.Time { clock = clock.Add(time.Hour); return clock },
	})
	return res, agent
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
	if !res.Summary.Pass || !strings.HasPrefix(res.Summary.Decision, "PASS at tier mid") {
		t.Errorf("summary = %+v, want a PASS at the primary tier", res.Summary)
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
	res.Commit = "45518e8b8461d491dc2a1aafaf1a8aa56702858a"
	for i := range res.Runs {
		if res.Runs[i].KeptDir != "" {
			res.Runs[i].KeptDir = "/tmp/factory-eval-123456789"
		}
	}
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
