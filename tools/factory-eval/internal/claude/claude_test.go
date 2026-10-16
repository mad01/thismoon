package claude

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const successReport = `{"type":"result","subtype":"success","is_error":false,
"result":"done","session_id":"s1","total_cost_usd":1.25,"duration_ms":90000,
"num_turns":12,"usage":{"input_tokens":10,"output_tokens":20,
"cache_creation_input_tokens":30,"cache_read_input_tokens":40},
"modelUsage":{"claude-sonnet-5-5":{"inputTokens":100,"outputTokens":200,
"cacheReadInputTokens":300,"cacheCreationInputTokens":400,"costUSD":1.0},
"claude-haiku-5-5":{"inputTokens":1,"outputTokens":2,"cacheReadInputTokens":3,
"cacheCreationInputTokens":4,"costUSD":0.25}},"permission_denials":[{"tool_name":"Bash"}],
"structured_output":{"cases":[]}}`

func TestParseReportSumsModelUsageAcrossModels(t *testing.T) {
	r, err := ParseReport([]byte(successReport))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	want := Tokens{Input: 101, Output: 202, CacheRead: 303, CacheCreation: 404}
	if got := r.Tokens(); got != want {
		t.Errorf("Tokens() = %+v, want %+v", got, want)
	}
	if got := r.Tokens().Total(); got != 1010 {
		t.Errorf("Total() = %d, want 1010", got)
	}
	if got := r.Tokens().In(); got != 808 {
		t.Errorf("In() = %d, want 808", got)
	}
	if !r.Finished() || r.TotalCostUSD != 1.25 || len(r.PermissionDenials) != 1 {
		t.Errorf("report = %+v, want a finished $1.25 run with one denial", r)
	}
	if string(r.StructuredOutput) != `{"cases":[]}` {
		t.Errorf("StructuredOutput = %s", r.StructuredOutput)
	}
}

func TestParseReportFallsBackToMainThreadUsage(t *testing.T) {
	raw := `{"type":"result","subtype":"error_max_budget_usd","is_error":true,
"usage":{"input_tokens":5,"output_tokens":6},"errors":["budget"]}`
	r, err := ParseReport([]byte(raw))
	if err != nil {
		t.Fatalf("ParseReport() error = %v", err)
	}
	if got := r.Tokens(); got != (Tokens{Input: 5, Output: 6}) {
		t.Errorf("Tokens() = %+v, want the main-thread usage", got)
	}
	if r.Finished() {
		t.Error("Finished() = true for a budget stop")
	}
}

func TestParseReportFindsResultAtStreamEnd(t *testing.T) {
	raw := "{\"type\":\"system\",\"subtype\":\"init\"}\n" +
		"{\"type\":\"result\",\"subtype\":\"success\",\"total_cost_usd\":0.5}\n"
	r, err := ParseReport([]byte(raw))
	if err != nil || r.TotalCostUSD != 0.5 {
		t.Errorf("ParseReport() = %+v, %v; want the trailing result", r, err)
	}
	if _, err := ParseReport([]byte("Error: not logged in\n")); err == nil {
		t.Error("ParseReport(plain text) = nil error")
	}
}

func TestArmArgsCapsBudgetAndTurnsPromptsIntoDenials(t *testing.T) {
	args := ArmArgs(ArmFlags{
		Model: "sonnet", Effort: "medium", BudgetUSD: 10, AddDir: "/tmp/run",
		ReportSchema: "{}", MCPConfig: "{}",
		Allowed: []string{"Read", "Bash(git add *)"}, Disallowed: []string{"Agent"},
		Settings: "/tmp/run/settings.json",
	})
	for _, pair := range [][2]string{
		{"--max-budget-usd", "10.00"},
		{"--model", "sonnet"},
		{"--effort", "medium"},
		{"--permission-mode", "dontAsk"},
		{"--permission-prompts", "none"},
		{"--output-format", "json"},
		{"--allowedTools", "Read,Bash(git add *)"},
		{"--disallowedTools", "Agent"},
		{"--settings", "/tmp/run/settings.json"},
		{"--add-dir", "/tmp/run"},
	} {
		i := slices.Index(args, pair[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != pair[1] {
			t.Errorf("args lack %s %s: %v", pair[0], pair[1], args)
		}
	}
	if args[0] != "-p" {
		t.Errorf("args[0] = %q, want -p", args[0])
	}
}

func TestVariadicFlagsAreFollowedByAFlagOrTheEnd(t *testing.T) {
	variadic := []string{
		"--allowedTools",
		"--disallowedTools",
		"--add-dir",
		"--mcp-config",
		"--tools",
	}
	for name, args := range map[string][]string{
		"arm":   ArmArgs(ArmFlags{Model: "m", Effort: "low", BudgetUSD: 1}),
		"judge": JudgeArgs("haiku", 0.5, "{}"),
	} {
		for i, a := range args {
			if !slices.Contains(variadic, a) {
				continue
			}
			if i+2 < len(args) && !strings.HasPrefix(args[i+2], "--") {
				t.Errorf("%s: %s value is followed by %q, which it would swallow",
					name, a, args[i+2])
			}
		}
	}
}

func TestParseVersion(t *testing.T) {
	v, err := ParseVersion("2.1.301 (Claude Code)\n")
	if err != nil || v != [3]int{2, 1, 301} {
		t.Errorf("ParseVersion() = %v, %v", v, err)
	}
	if _, err := ParseVersion("dev"); err == nil {
		t.Error("ParseVersion(dev) = nil error")
	}
}

func TestExecPassesStdinAndKeepsTheExitCode(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\ncat\necho \"args:$*\" >&2\nexit 3\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := Exec{Bin: bin}.Run(context.Background(), Invocation{
		Dir: t.TempDir(), Args: []string{"-p", "--model", "haiku"}, Stdin: "the prompt",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out.Stdout) != "the prompt" || out.ExitCode != 3 {
		t.Errorf("Run() = stdout %q exit %d, want the prompt echoed and exit 3",
			out.Stdout, out.ExitCode)
	}
	if !strings.Contains(string(out.Stderr), "args:-p --model haiku") {
		t.Errorf("stderr = %q, want the argv", out.Stderr)
	}
}
