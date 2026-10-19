// Package claude builds the headless claude invocations the eval makes and
// parses the JSON report each one prints. The report shape is the Agent SDK
// result message that `claude -p --output-format json` emits: cost, usage,
// per-model usage, duration, and the structured output a --json-schema run
// returns.
package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Result subtypes a report can carry. Every subtype but SubtypeSuccess
// means the session stopped before finishing its turn.
const (
	SubtypeSuccess   = "success"
	SubtypeMaxBudget = "error_max_budget_usd"
)

// Invocation is one headless claude call: argv after the binary, the
// working dir, and the prompt, which goes on stdin so no variadic flag can
// swallow it.
type Invocation struct {
	Dir   string
	Args  []string
	Stdin string
}

// Report is the result message one call printed. An API error that ends
// the turn reports subtype success with IsError set, APIErrorStatus beside
// it, and no Errors; Finished and ErrorText both treat that as a failure.
type Report struct {
	Type              string                `json:"type"`
	Subtype           string                `json:"subtype"`
	IsError           bool                  `json:"is_error"`
	Result            string                `json:"result"`
	SessionID         string                `json:"session_id"`
	TotalCostUSD      float64               `json:"total_cost_usd"`
	DurationMS        int64                 `json:"duration_ms"`
	NumTurns          int                   `json:"num_turns"`
	Usage             Usage                 `json:"usage"`
	ModelUsage        map[string]ModelUsage `json:"modelUsage"`
	PermissionDenials []json.RawMessage     `json:"permission_denials"`
	StructuredOutput  json.RawMessage       `json:"structured_output"`
	Errors            []string              `json:"errors"`
	APIErrorStatus    json.RawMessage       `json:"api_error_status"`
}

// Usage is the main thread's token usage.
type Usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// ModelUsage is one model's usage across the whole session, subagents
// included.
type ModelUsage struct {
	InputTokens              int64   `json:"inputTokens"`
	OutputTokens             int64   `json:"outputTokens"`
	CacheReadInputTokens     int64   `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64   `json:"cacheCreationInputTokens"`
	CostUSD                  float64 `json:"costUSD"`
}

// Tokens is a token count split the way the results file records it.
type Tokens struct {
	Input         int64 `json:"input"`
	Output        int64 `json:"output"`
	CacheRead     int64 `json:"cache_read"`
	CacheCreation int64 `json:"cache_creation"`
}

// Total is every token the session processed, cache reads included. The
// pass rule compares totals.
func (t Tokens) Total() int64 { return t.Input + t.Output + t.CacheRead + t.CacheCreation }

// In is every input-side token: fresh input, cache reads and cache writes.
func (t Tokens) In() int64 { return t.Input + t.CacheRead + t.CacheCreation }

// Tokens sums per-model usage, which covers subagents, and falls back to
// the main thread's usage when the report carries no per-model block.
func (r Report) Tokens() Tokens {
	if len(r.ModelUsage) == 0 {
		return Tokens{
			Input:         r.Usage.InputTokens,
			Output:        r.Usage.OutputTokens,
			CacheRead:     r.Usage.CacheReadInputTokens,
			CacheCreation: r.Usage.CacheCreationInputTokens,
		}
	}
	var t Tokens
	for _, m := range r.ModelUsage {
		t.Input += m.InputTokens
		t.Output += m.OutputTokens
		t.CacheRead += m.CacheReadInputTokens
		t.CacheCreation += m.CacheCreationInputTokens
	}
	return t
}

// Finished reports whether the session ended its turn normally.
func (r Report) Finished() bool { return r.Subtype == SubtypeSuccess && !r.IsError }

// maxResultText caps how much of the result text ErrorText quotes.
const maxResultText = 300

// ErrorText says why the session failed: its errors when it lists any.
// Without them, a report with IsError set names its api_error_status and
// quotes the start of its result text. It is empty for a clean report.
func (r Report) ErrorText() string {
	if len(r.Errors) > 0 {
		return strings.Join(r.Errors, "; ")
	}
	if !r.IsError {
		return ""
	}
	msg := "the session ended with is_error set"
	if s := strings.TrimSpace(string(r.APIErrorStatus)); s != "" && s != "null" {
		msg += ", api_error_status " + s
	}
	if res := []rune(strings.TrimSpace(r.Result)); len(res) > 0 {
		if len(res) > maxResultText {
			res = append(res[:maxResultText], []rune("...")...)
		}
		msg += ": " + string(res)
	}
	return msg
}

// ParseReport finds the result message in a call's stdout. The JSON output
// format prints one object; a line-by-line scan from the end also accepts a
// stream that ends in the result message.
func ParseReport(stdout []byte) (Report, error) {
	var r Report
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &r); err == nil && r.Type == "result" {
		return r, nil
	}
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(stdout))
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var cand Report
		if json.Unmarshal([]byte(lines[i]), &cand) == nil && cand.Type == "result" {
			return cand, nil
		}
	}
	return Report{}, errors.New("claude: no result message in output")
}

// ArmFlags configures one arm session.
type ArmFlags struct {
	Model        string
	Effort       string
	BudgetUSD    float64
	AddDir       string
	ReportSchema string
	MCPConfig    string
	Allowed      []string
	Disallowed   []string
	// Settings is the harness's own settings file, loaded beside the
	// user's. It repeats Allowed and Disallowed because settings-file
	// rules are documented to reach subagents; no paid run has confirmed
	// that workers obey them yet. Its deny rules beat any allow rule the
	// user's settings hold.
	Settings string
}

// ArmArgs is the argv for one unattended arm session: JSON output, the
// dollar cap, the dontAsk mode with prompts turned into denials, only the
// MCP servers the config names, the harness settings file, and the run's
// temp dir as the one extra directory the file tools may reach.
func ArmArgs(f ArmFlags) []string {
	return []string{
		"-p",
		"--output-format", "json",
		"--model", f.Model,
		"--effort", f.Effort,
		"--max-budget-usd", USD(f.BudgetUSD),
		"--permission-mode", "dontAsk",
		"--permission-prompts", "none",
		"--json-schema", f.ReportSchema,
		"--strict-mcp-config",
		"--mcp-config", f.MCPConfig,
		"--allowedTools", strings.Join(f.Allowed, ","),
		"--disallowedTools", strings.Join(f.Disallowed, ","),
		"--settings", f.Settings,
		"--add-dir", f.AddDir,
	}
}

// JudgeArgs is the argv for one judge call: no tools, no MCP servers, no
// settings files, no saved session, structured output against schema.
func JudgeArgs(model string, budgetUSD float64, schema string) []string {
	return []string{
		"-p",
		"--output-format", "json",
		"--model", model,
		"--max-budget-usd", USD(budgetUSD),
		"--json-schema", schema,
		"--no-session-persistence",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--tools", "",
	}
}

// USD formats a dollar amount the way --max-budget-usd takes it.
func USD(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// ParseVersion reads the semver triple from `claude --version` output such
// as "2.1.301 (Claude Code)".
func ParseVersion(out string) ([3]int, error) {
	var v [3]int
	field, _, _ := strings.Cut(strings.TrimSpace(out), " ")
	parts := strings.Split(field, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("claude: version %q is not major.minor.patch", field)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, fmt.Errorf("claude: version %q: %w", field, err)
		}
		v[i] = n
	}
	return v, nil
}
