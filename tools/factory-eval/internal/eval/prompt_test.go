package eval

import (
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
)

func loadCases(t *testing.T) *cases.File {
	t.Helper()
	f, err := cases.Load("../../cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

var testInput = PromptInput{
	Repo: "/tmp/run/thismoon", Worktrees: "/tmp/run/worktrees", Commit: "abc123",
	Model: "sonnet", Effort: "medium", Worker: "factory-worker-medium",
}

func TestBothArmsGetTheSameCaseText(t *testing.T) {
	f := loadCases(t)
	single, err := ArmPrompt(f, f.Arms.Single, testInput)
	if err != nil {
		t.Fatalf("ArmPrompt(single) error = %v", err)
	}
	factory, err := ArmPrompt(f, f.Arms.Factory, testInput)
	if err != nil {
		t.Fatalf("ArmPrompt(factory) error = %v", err)
	}
	_, singleCases, _ := strings.Cut(single, "## The shared outline")
	_, factoryCases, _ := strings.Cut(factory, "## The shared outline")
	if singleCases == "" || singleCases != factoryCases {
		t.Error("the outline and case text differ between the arms, want identical wording")
	}
	for _, c := range f.Cases {
		if !strings.Contains(single, strings.TrimSpace(c.Task)) {
			t.Errorf("prompt lacks the %s task verbatim", c.Component)
		}
	}
	for _, want := range []string{"/tmp/run/worktrees", "factory-worker-medium", "effort medium"} {
		if !strings.Contains(factory, want) {
			t.Errorf("factory preamble lacks %q", want)
		}
	}
	if strings.Contains(single, "{{") || strings.Contains(factory, "{{") {
		t.Error("a preamble placeholder survived rendering")
	}
}

// TestPreamblesMatchWhatTheRunAllows keeps the preambles from sending an
// arm into a call the profile denies: preflight check 3 reads the repo's
// identity settings, which the deny list blocks, so the factory arm runs
// check 4 alone and the harness sets the identity itself.
func TestPreamblesMatchWhatTheRunAllows(t *testing.T) {
	f := loadCases(t)
	for name, arm := range map[string]cases.Arm{"single": f.Arms.Single, "factory": f.Arms.Factory} {
		if !strings.Contains(arm.Preamble, "csl and kof tools are not attached") {
			t.Errorf("%s preamble does not say csl and kof are missing", name)
		}
	}
	factory := f.Arms.Factory.Preamble
	if !strings.Contains(factory, "Run preflight check 4 only.") ||
		strings.Contains(factory, "checks 3") {
		t.Error("factory preamble must run preflight check 4 only")
	}
	if !strings.Contains(factory, "The harness sets the git identity") {
		t.Error("factory preamble does not say the harness sets the git identity")
	}
	if !slices.Contains(denied, "Bash(git -C * config*)") {
		t.Error("the deny list no longer blocks the -C identity read, so recheck the preamble")
	}
}

func TestSharedBlockListsTheOutlineInOrder(t *testing.T) {
	f := loadCases(t)
	block := sharedBlock(f)
	last := -1
	for _, s := range f.Outline.ClaudeMD.Sections {
		i := strings.Index(block, "`## "+s.Heading+"`")
		if i <= last {
			t.Fatalf("%q is missing or out of order in the shared block", s.Heading)
		}
		last = i
	}
	for _, rule := range f.Humanizer.IgnoreRules {
		if !strings.Contains(block, rule) {
			t.Errorf("shared block does not name the ignored rule %s", rule)
		}
	}
}

func TestArmPromptRejectsUnknownPlaceholders(t *testing.T) {
	f := loadCases(t)
	if _, err := ArmPrompt(f, cases.Arm{Preamble: "work in {{.Nowhere}}"}, testInput); err == nil {
		t.Error("ArmPrompt() with an unknown placeholder = nil error")
	}
}

func TestProfileKeepsRunsInsideTheClone(t *testing.T) {
	co := &checkout.Checkout{Root: "/tmp/run"}
	tier := cases.Tier{Model: "sonnet", Effort: "medium", BudgetUSD: 10}
	for _, arm := range []string{ArmSingle, ArmFactory} {
		args := armArgs(arm, tier, co, "/tmp/run/settings.json")
		joined := strings.Join(args, " ")
		for _, want := range []string{
			"Bash(git push*)", "Bash(git -C * push*)", "Bash(git fetch*)", "Bash(git * --output*)",
			"Bash(make * install*)",
			"Bash(go install *)", "--strict-mcp-config", "--add-dir /tmp/run", "--max-budget-usd 10.00",
			"--settings /tmp/run/settings.json",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s args lack %q", arm, want)
			}
		}
		allowed := args[slices.Index(args, "--allowedTools")+1]
		denied := args[slices.Index(args, "--disallowedTools")+1]
		spawns := strings.Contains(allowed, "Agent")
		if spawns != (arm == ArmFactory) || strings.Contains(denied, "Agent") == spawns {
			t.Errorf(
				"%s: Agent allowed=%v, want only the factory arm to spawn workers",
				arm,
				spawns,
			)
		}
	}
}
