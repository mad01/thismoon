package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// claudeBin is the CLI auto-selection looks for on PATH.
const claudeBin = "claude"

// ClaudeConfig configures the claude CLI backend.
type ClaudeConfig struct {
	// Bin is the resolved path of the CLI; empty runs "claude" from PATH.
	Bin string
	// Model is the canonical Anthropic id; empty is DefaultModel.
	Model string
	// Effort is passed as --effort when non-empty.
	Effort string
	// Observe, when set, receives one CallInfo per run that produced
	// output, a reply or a reported error alike.
	Observe func(CallInfo)
}

// ClaudeClient runs a completion as one headless `claude -p` turn with no
// tools, no hooks, no settings and no session persistence, the user text
// on stdin. It draws on whatever the CLI is signed into, so the dollar
// cost it reports is the list price of the call, not a charge. The CLI
// wraps the prompt with a few thousand tokens of its own, so an answer
// can differ from the same model reached through a direct API call.
type ClaudeClient struct {
	bin     string
	model   string
	effort  string
	observe func(CallInfo)
}

// NewClaude returns a backend on the claude CLI. It does not probe the
// binary: Select resolves it from PATH and a caller with its own path
// passes it in.
func NewClaude(cfg ClaudeConfig) *ClaudeClient {
	if cfg.Bin == "" {
		cfg.Bin = claudeBin
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	return &ClaudeClient{bin: cfg.Bin, model: cfg.Model, effort: cfg.Effort, observe: cfg.Observe}
}

// Name implements Backend.
func (c *ClaudeClient) Name() string { return Claude }

// Model implements Backend.
func (c *ClaudeClient) Model() string { return c.model }

// sessionEnv names the variables a headless run must not inherit. They tie
// a process to the calling Claude Code session or set its effort behind the
// caller's back; the list is the harness's (evals/harness/claude.go).
var sessionEnv = map[string]bool{
	"CLAUDECODE":                           true,
	"CLAUDE_CODE_ENTRYPOINT":               true,
	"CLAUDE_CODE_SESSION_ID":               true,
	"CLAUDE_CODE_CHILD_SESSION":            true,
	"CLAUDE_CODE_SESSION_ATTENDED":         true,
	"CLAUDE_CODE_MESSAGING_SOCKET":         true,
	"CLAUDE_CODE_MESSAGING_TOKEN":          true,
	"CLAUDE_PROJECT_DIR":                   true,
	"CLAUDE_PID":                           true,
	"CLAUDE_CODE_EFFORT_LEVEL":             true,
	"CLAUDE_EFFORT":                        true,
	"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": true,
}

// claudeEnv returns environ without the session variables.
func claudeEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !sessionEnv[name] {
			out = append(out, kv)
		}
	}
	return out
}

// claudeOutput is the --output-format json shape of one headless turn.
type claudeOutput struct {
	Result        string  `json:"result"`
	IsError       bool    `json:"is_error"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	DurationAPIMS int64   `json:"duration_api_ms"`
	Usage         struct {
		InputTokens   int `json:"input_tokens"`
		OutputTokens  int `json:"output_tokens"`
		CacheCreation int `json:"cache_creation_input_tokens"`
		CacheRead     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

// Complete implements Backend with one headless turn. The process runs in
// the temp directory so no project instructions load, and without the
// session variables so it cannot attach to a calling session.
func (c *ClaudeClient) Complete(ctx context.Context, system, user string) (string, error) {
	args := []string{
		"-p",
		"--model", c.model,
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
		"--settings", `{"disableAllHooks":true}`,
		"--no-session-persistence",
		"--output-format", "json",
		"--system-prompt", system,
	}
	if c.effort != "" {
		args = append(args, "--effort", c.effort)
	}
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Stdin = strings.NewReader(user)
	cmd.Dir = os.TempDir()
	cmd.Env = claudeEnv(os.Environ())
	start := time.Now()
	out, err := cmd.Output()
	wall := time.Since(start)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			c.report(CallInfo{Status: 1, Latency: wall})
			return "", fmt.Errorf(
				"llm: claude -p exited %d: %s",
				exitErr.ExitCode(), truncate(string(exitErr.Stderr), 300),
			)
		}
		return "", fmt.Errorf("llm: exec claude -p: %w", err)
	}
	var res claudeOutput
	if err := json.Unmarshal(out, &res); err != nil {
		c.report(CallInfo{Status: 1, Latency: wall})
		return "", fmt.Errorf(
			"llm: decode claude -p output: %w: %s",
			err,
			truncate(string(out), 200),
		)
	}
	info := CallInfo{
		Model:            servedModel(res.ModelUsage, c.model),
		PromptTokens:     res.Usage.InputTokens + res.Usage.CacheCreation + res.Usage.CacheRead,
		CompletionTokens: res.Usage.OutputTokens,
		Status:           200,
		Latency:          wall,
		CostUSD:          res.TotalCostUSD,
	}
	if res.DurationAPIMS > 0 {
		info.Latency = time.Duration(res.DurationAPIMS) * time.Millisecond
	}
	c.report(info)
	if res.IsError {
		return "", fmt.Errorf("llm: claude -p errored: %s", truncate(res.Result, 300))
	}
	if strings.TrimSpace(res.Result) == "" {
		return "", errors.New("llm: claude returned no content")
	}
	return res.Result, nil
}

func (c *ClaudeClient) report(info CallInfo) {
	if c.observe != nil {
		c.observe(info)
	}
}

// servedModel reads the model the CLI billed from its modelUsage map; a
// missing map means the asked model, which the CLI does not substitute.
func servedModel(usage map[string]json.RawMessage, asked string) string {
	keys := make([]string, 0, len(usage))
	for k := range usage {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return asked
	}
	slices.Sort(keys)
	return keys[0]
}
