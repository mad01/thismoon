// Package llm selects and drives the hosted model behind a component's
// one-shot completions: humanizer's judge pass and csl's bulk reader. The
// deterministic paths of those tools never touch it; only their opt-in
// model calls do.
//
// Selection is env-driven so one binary works on machines with different
// providers, in a fixed order: a LiteLLM proxy when LITELLM_BASE_URL is
// set, then the claude CLI on PATH when the caller allows it (a headless
// `claude -p` on whatever the CLI is signed into: a subscription at home,
// Claude Enterprise at work), then OpenRouter when OPENROUTER_API_KEY is
// set. OpenRouter is last on purpose: it is the one that bills per token
// outside any plan. No local model is a backend: the callers run beside
// other hosted agents, and a local model would measure a machine, not the
// tool.
package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Backend is one provider a caller can complete against. Implementations
// are one-shot: a system prompt plus user text in, the raw model output
// back. No tools, no multi-turn.
type Backend interface {
	Complete(ctx context.Context, system, user string) (string, error)
	// Name reports the backend id (litellm, claude or openrouter) for
	// result metadata.
	Name() string
	// Model reports the model id the backend puts on the wire.
	Model() string
}

// Backend names: what Options.Backend forces and what Name reports.
const (
	LiteLLM    = "litellm"
	Claude     = "claude"
	OpenRouter = "openrouter"
)

// DefaultModel is the canonical id every backend starts from: Haiku 5.5,
// the cheapest current Claude. The csl_ask eval found it gets the facts
// right where the larger models only add tokens, and humanizer's judge was
// tuned on it.
const DefaultModel = "claude-haiku-5-5"

// ErrNoBackend means auto-selection found nothing configured. A caller
// that treats the model call as optional matches on it and skips.
var ErrNoBackend = errors.New("llm: no backend configured")

// CallInfo is what a backend reported about one exchange: the model that
// served it, the token usage, the status, and the latency of the exchange
// alone. Status is the HTTP status for the chat backends; the claude CLI
// reports 200 for a reply and 1 for a process that failed.
type CallInfo struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	// ReasoningTokens is the thinking share of CompletionTokens when the
	// provider breaks it out (OpenRouter does); zero otherwise.
	ReasoningTokens int           `json:"reasoning_tokens"`
	Status          int           `json:"status"`
	Latency         time.Duration `json:"latency"`
	// CostUSD is the price the backend itself reported (the claude CLI's
	// total_cost_usd); zero when the backend reports none. It follows the
	// CLI's prompt cache, so it is not comparable across runs: price from
	// the tokens for that.
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// Options steer Select. The zero value auto-selects DefaultModel on the
// first configured backend, leaving the claude CLI out.
type Options struct {
	// Backend forces one by name; empty auto-selects.
	Backend string
	// Model is the canonical Anthropic id (claude-haiku-5-5), which each
	// backend translates for its provider. A provider-specific id passes
	// through unchanged. Empty is DefaultModel.
	Model string
	// Effort is the reasoning effort (low, medium, high, xhigh or max);
	// empty keeps the provider default.
	Effort string
	// AllowCLI lets auto-selection pick the claude CLI. A caller inside a
	// sandbox that cannot spawn it, or whose results were tuned on a direct
	// API call, leaves it off. Forcing Backend to Claude ignores it.
	AllowCLI bool
	// Observe, when set, receives one CallInfo per exchange that produced a
	// response, whatever its status.
	Observe func(CallInfo)
	// Temperature, when set, goes on the wire of the chat backends. The
	// Claude 5.5 generation rejects any value but the default, so only an
	// eval that reproduces an older request shape sets it.
	Temperature *float64
}

// Select resolves a Backend from opts and the environment.
func Select(opts Options) (Backend, error) {
	switch opts.Backend {
	case "":
		return auto(opts)
	case LiteLLM:
		return liteLLMFromEnv(opts)
	case Claude:
		return claudeFromPath(opts)
	case OpenRouter:
		return openRouterFromEnv(opts)
	case "vertex", "anthropic":
		return nil, fmt.Errorf("llm: backend %q is not implemented yet", opts.Backend)
	default:
		return nil, fmt.Errorf(
			"llm: unknown backend %q (want %s, %s or %s)",
			opts.Backend, LiteLLM, Claude, OpenRouter,
		)
	}
}

// auto takes the first backend present in the fixed order the package doc
// gives.
func auto(opts Options) (Backend, error) {
	if os.Getenv("LITELLM_BASE_URL") != "" {
		return liteLLMFromEnv(opts)
	}
	if opts.AllowCLI {
		if _, err := exec.LookPath(claudeBin); err == nil {
			return claudeFromPath(opts)
		}
	}
	if os.Getenv("OPENROUTER_API_KEY") != "" {
		return openRouterFromEnv(opts)
	}
	hint := "set LITELLM_BASE_URL or OPENROUTER_API_KEY"
	if opts.AllowCLI {
		hint = "set LITELLM_BASE_URL, put claude on PATH, or set OPENROUTER_API_KEY"
	}
	return nil, fmt.Errorf("%w (%s)", ErrNoBackend, hint)
}

func liteLLMFromEnv(opts Options) (Backend, error) {
	c, err := NewLiteLLM(ChatConfig{
		APIKey:      os.Getenv("LITELLM_API_KEY"),
		BaseURL:     os.Getenv("LITELLM_BASE_URL"),
		Model:       opts.Model,
		Effort:      opts.Effort,
		Temperature: opts.Temperature,
		Observe:     opts.Observe,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func openRouterFromEnv(opts Options) (Backend, error) {
	c, err := NewOpenRouter(ChatConfig{
		APIKey:      os.Getenv("OPENROUTER_API_KEY"),
		Model:       opts.Model,
		Effort:      opts.Effort,
		Temperature: opts.Temperature,
		Observe:     opts.Observe,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func claudeFromPath(opts Options) (Backend, error) {
	bin, err := exec.LookPath(claudeBin)
	if err != nil {
		return nil, fmt.Errorf("llm: claude CLI: %w", err)
	}
	return NewClaude(ClaudeConfig{
		Bin:     bin,
		Model:   opts.Model,
		Effort:  opts.Effort,
		Observe: opts.Observe,
	}), nil
}

// canonicalRE matches a canonical Anthropic id: family, major, and an
// optional minor version.
var canonicalRE = regexp.MustCompile(`^claude-([a-z]+)-(\d+)(?:-(\d+))?$`)

// OpenRouterID returns the id OpenRouter serves a canonical Anthropic id
// under: claude-haiku-5-5 is anthropic/claude-haiku-5.5. Any other id
// passes through, so a caller can name an OpenRouter route directly.
func OpenRouterID(model string) string {
	m := canonicalRE.FindStringSubmatch(model)
	if m == nil {
		return model
	}
	id := "anthropic/claude-" + m[1] + "-" + m[2]
	if m[3] != "" {
		id += "." + m[3]
	}
	return id
}

// CanonicalID is the inverse of OpenRouterID for an anthropic/ id and the
// identity for anything else, so a table keyed by the canonical id finds a
// row whichever backend served it.
func CanonicalID(model string) string {
	id, ok := strings.CutPrefix(model, "anthropic/")
	if !ok {
		return model
	}
	return strings.ReplaceAll(id, ".", "-")
}

// truncate trims s to something loggable.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
