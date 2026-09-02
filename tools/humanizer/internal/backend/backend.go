// Package backend selects and drives the LLM provider behind humanizer's
// holistic judgment pass (MAD-342). The deterministic detectors never touch
// this package; only the opt-in judge path calls a model. Selection is
// env-driven so the same binary works on machines with different providers:
// an explicit HUMANIZER_BACKEND override wins, then the first provider whose
// credentials are present, then ErrNoBackend so callers can skip gracefully.
package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Backend is one LLM provider the holistic pass can complete against.
// Implementations are one-shot: a single system prompt plus user text in,
// the raw model output back. No tools, no multi-turn.
type Backend interface {
	Complete(ctx context.Context, system, user string) (string, error)
	// Name reports the provider id (e.g. "openrouter") for result metadata.
	Name() string
	// Model reports the resolved model id the provider will call.
	Model() string
}

// ErrNoBackend means no provider credentials were found in the environment.
// Callers that treat the holistic pass as optional match on this and skip.
var ErrNoBackend = errors.New(
	"backend: no LLM backend configured (set LITELLM_BASE_URL or OPENROUTER_API_KEY)",
)

// Select resolves a Backend. name forces a provider ("litellm" or
// "openrouter"); empty falls back to the HUMANIZER_BACKEND env var and then
// credential auto-detection, where a LiteLLM base URL wins over an
// OpenRouter key because pointing at a proxy is the more deliberate signal.
// model overrides the provider's default model id; empty falls back to
// HUMANIZER_MODEL and then the provider default.
// Vertex and Anthropic are planned in MAD-342 but not implemented yet.
func Select(name, model string) (Backend, error) {
	if name == "" {
		name = os.Getenv("HUMANIZER_BACKEND")
	}
	switch name {
	case "":
		// Credential auto-detection, in the MAD-342 order. Vertex and
		// Anthropic join here once implemented.
		if os.Getenv("LITELLM_BASE_URL") != "" {
			return newLiteLLMFromEnv(model)
		}
		if os.Getenv("OPENROUTER_API_KEY") != "" {
			return newOpenRouterFromEnv(model)
		}
		return nil, ErrNoBackend
	case "litellm":
		return newLiteLLMFromEnv(model)
	case "openrouter":
		return newOpenRouterFromEnv(model)
	case "vertex", "anthropic":
		return nil, fmt.Errorf(
			"backend: %q is not implemented yet (MAD-342 tracks vertex and anthropic)",
			name,
		)
	default:
		return nil, fmt.Errorf("backend: unknown backend %q (want litellm or openrouter)", name)
	}
}

// envModel applies the HUMANIZER_MODEL fallback shared by every provider.
func envModel(model string) string {
	if model != "" {
		return model
	}
	return os.Getenv("HUMANIZER_MODEL")
}
