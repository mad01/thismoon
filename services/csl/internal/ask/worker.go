package ask

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// backends are the names ask.backend accepts; empty auto-selects.
var backends = []string{llm.LiteLLM, llm.Claude, llm.OpenRouter}

// efforts are the reasoning efforts ask.effort accepts.
var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// Settings pick the worker: the ask keys of config.yaml with the defaults
// applied.
type Settings struct {
	Backend string
	Model   string
	Effort  string
}

// SettingsFrom reads the worker settings from a loaded config. A nil config
// gives the defaults.
func SettingsFrom(cfg *config.Config) Settings {
	return Settings{Backend: cfg.AskBackend(), Model: cfg.AskModel(), Effort: cfg.AskEffort()}
}

// Validate refuses a backend or effort kit/llm would not run, naming the
// config key, so a typo in config.yaml reads as one and not as a provider
// failure.
func (s Settings) Validate() error {
	if s.Backend != "" && !slices.Contains(backends, s.Backend) {
		return fmt.Errorf(
			"ask.backend %q is not a backend (want %s, or leave it empty to auto-select)",
			s.Backend, strings.Join(backends, ", "),
		)
	}
	if !slices.Contains(efforts, s.Effort) {
		return fmt.Errorf(
			"ask.effort %q is not an effort (want %s)",
			s.Effort, strings.Join(efforts, ", "),
		)
	}
	return nil
}

// NewWorker resolves the worker backend from the settings; observe, when
// set, receives every exchange the backend makes. The claude CLI is allowed:
// it answers on whatever account it is signed into. With nothing configured
// the error is kit/llm's ErrNoBackend, which names what to set.
func NewWorker(s Settings, observe func(llm.CallInfo)) (llm.Backend, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	b, err := llm.Select(llm.Options{
		Backend:  s.Backend,
		Model:    s.Model,
		Effort:   s.Effort,
		AllowCLI: true,
		Observe:  observe,
	})
	if err != nil {
		return nil, fmt.Errorf("ask: no worker model: %w", err)
	}
	return b, nil
}

// prices is USD per million tokens, input then output: Anthropic's list
// prices read on 2026-11-01, keyed by the canonical model id. Cost comes
// from the usage a provider reported, never from text length, and carries
// no cache discount, so two calls compare on the same footing whichever
// backend served them. The claude CLI reports its own cached figure, which
// falls with each call after the first; the eval keeps that in its traces.
var prices = map[string][2]float64{
	"claude-haiku-5-5":  {0.10, 0.50},
	"claude-sonnet-5-5": {2.00, 10.00},
	"claude-opus-5-5":   {4.00, 20.00},
	"claude-fable-5-1":  {10.00, 50.00},
}

// CostUSD is the list price of one call's tokens, zero for a model the
// table does not know. An OpenRouter id prices as its canonical id.
func CostUSD(model string, promptTokens, completionTokens int) float64 {
	p, ok := prices[llm.CanonicalID(model)]
	if !ok {
		return 0
	}
	return (float64(promptTokens)*p[0] + float64(completionTokens)*p[1]) / 1e6
}
