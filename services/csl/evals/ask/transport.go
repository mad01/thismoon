package main

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

// transport is one way to reach a worker model. Every transport takes the
// canonical Anthropic model id and translates it itself.
type transport interface {
	name() string
	complete(ctx context.Context, model, effort, system, user string) (string, CallInfo, error)
}

const (
	viaAuto       = "auto"
	viaLiteLLM    = "litellm"
	viaClaude     = "claude"
	viaOpenRouter = "openrouter"
)

// pickTransport resolves -via. Auto takes the first backend present in a
// fixed order: a LiteLLM proxy (LITELLM_BASE_URL, the most deliberate
// signal), then the claude CLI on PATH (headless claude -p on whatever it
// is signed into: a subscription at home, Claude Enterprise at work), then
// OpenRouter (OPENROUTER_API_KEY). OpenRouter is last on purpose: it is
// the one that bills per token outside any plan.
func pickTransport(via string) (transport, error) {
	switch via {
	case viaAuto:
		if os.Getenv("LITELLM_BASE_URL") != "" {
			return pickTransport(viaLiteLLM)
		}
		if _, err := exec.LookPath("claude"); err == nil {
			return pickTransport(viaClaude)
		}
		if os.Getenv("OPENROUTER_API_KEY") != "" {
			return pickTransport(viaOpenRouter)
		}
		return nil, errors.New(
			"no backend: set LITELLM_BASE_URL, put claude on PATH, or set OPENROUTER_API_KEY",
		)
	case viaLiteLLM:
		base := os.Getenv("LITELLM_BASE_URL")
		if base == "" {
			return nil, errors.New("LITELLM_BASE_URL is not set")
		}
		return newLiteLLM(base, os.Getenv("LITELLM_API_KEY")), nil
	case viaClaude:
		bin, err := exec.LookPath("claude")
		if err != nil {
			return nil, fmt.Errorf("claude CLI: %w", err)
		}
		return &claudeTransport{bin: bin}, nil
	case viaOpenRouter:
		key := os.Getenv("OPENROUTER_API_KEY")
		if key == "" {
			return nil, errors.New("OPENROUTER_API_KEY is not set (source ~/.secrets.sh)")
		}
		return newOpenRouter(key), nil
	}
	return nil, fmt.Errorf("unknown -via %q (auto, litellm, claude, openrouter)", via)
}

// sessionEnv names the variables a headless run must not inherit. They tie
// a process to the calling Claude Code session or set its effort behind the
// arm's back; the list is the harness's (evals/harness/claude.go).
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

// claudeTransport runs the worker as one headless `claude -p` turn with no
// tools, no hooks, no settings and no session persistence, the corpus on
// stdin. It draws on whatever the CLI is signed into, so the dollar cost it
// reports is the list price of the call, not a charge.
type claudeTransport struct {
	bin string
}

func (t *claudeTransport) name() string { return viaClaude }

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

func (t *claudeTransport) complete(
	ctx context.Context, model, effort, system, user string,
) (string, CallInfo, error) {
	args := []string{
		"-p",
		"--model", model,
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
		"--settings", `{"disableAllHooks":true}`,
		"--no-session-persistence",
		"--output-format", "json",
		"--system-prompt", system,
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	cmd := exec.CommandContext(ctx, t.bin, args...)
	cmd.Stdin = strings.NewReader(user)
	cmd.Dir = os.TempDir()
	cmd.Env = claudeEnv(os.Environ())
	start := time.Now()
	out, err := cmd.Output()
	wall := time.Since(start)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", CallInfo{Status: 1, Latency: wall}, fmt.Errorf(
				"claude -p exited %d: %s",
				exitErr.ExitCode(),
				truncate(string(exitErr.Stderr), 300),
			)
		}
		return "", CallInfo{}, fmt.Errorf("exec claude -p: %w", err)
	}
	var res claudeOutput
	if err := json.Unmarshal(out, &res); err != nil {
		return "", CallInfo{Status: 1, Latency: wall}, fmt.Errorf(
			"decode claude -p output: %w: %s", err, truncate(string(out), 200),
		)
	}
	info := CallInfo{
		Model:            servedModel(res.ModelUsage, model),
		PromptTokens:     res.Usage.InputTokens + res.Usage.CacheCreation + res.Usage.CacheRead,
		CompletionTokens: res.Usage.OutputTokens,
		Status:           200,
		Latency:          wall,
		CostUSD:          res.TotalCostUSD,
	}
	if res.DurationAPIMS > 0 {
		info.Latency = time.Duration(res.DurationAPIMS) * time.Millisecond
	}
	if res.IsError {
		return "", info, fmt.Errorf("claude -p errored: %s", truncate(res.Result, 300))
	}
	if strings.TrimSpace(res.Result) == "" {
		return "", info, errors.New("no content in response")
	}
	return res.Result, info, nil
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
