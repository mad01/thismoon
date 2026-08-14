package rewrite

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/humanizer/internal/scrub"
)

// Backend selects how the rewrite is produced.
type Backend string

const (
	// PrintPrompt returns the prompt without calling any model. Offline,
	// deterministic, sandbox-safe.
	PrintPrompt Backend = "print-prompt"
	// Ollama POSTs to an Ollama /api/chat endpoint.
	Ollama Backend = "ollama"
	// OpenAICompatible POSTs to an OpenAI-style /v1/chat/completions endpoint.
	OpenAICompatible Backend = "openai-compatible"
)

// Options configures a Rewrite call.
type Options struct {
	Backend      Backend
	Model        string
	BaseURL      string
	APIKey       string // from the environment only; never a flag
	Strength     Strength
	Lang         string
	OriginalLang string
	Timeout      time.Duration
	LayerAAfter  bool // scrub the model output with Layer A
	Temperature  float64
	Candidates   int
	AllowRemote  bool
}

// Info is the metadata a Rewrite call reports alongside its output.
type Info struct {
	Backend         string       `json:"backend"`
	Strength        string       `json:"strength"`
	Model           string       `json:"model,omitempty"`
	BaseURL         string       `json:"base_url,omitempty"`
	Temperature     float64      `json:"temperature"`
	PromptChars     int          `json:"prompt_chars"`
	InputChars      int          `json:"input_chars"`
	OutputChars     int          `json:"output_chars,omitempty"`
	Mode            string       `json:"mode"`
	Candidates      int          `json:"candidates,omitempty"`
	CandidateScores []float64    `json:"candidate_scores,omitempty"`
	LayerAAfter     *scrub.Stats `json:"layer_a_after,omitempty"`
	Warning         string       `json:"warning,omitempty"`
	Note            string       `json:"note,omitempty"`
}

// Rewrite builds the prompt and, for a model backend, runs it and returns the
// chosen rewrite. For print-prompt it returns the prompt itself.
func Rewrite(text string, opts Options) (string, Info, error) {
	prompt, err := BuildPrompt(opts.Strength, text, opts.Lang, opts.OriginalLang)
	if err != nil {
		return "", Info{}, err
	}
	info := Info{
		Backend:     string(opts.Backend),
		Strength:    string(opts.Strength),
		Model:       opts.Model,
		BaseURL:     opts.BaseURL,
		Temperature: opts.Temperature,
		PromptChars: len(prompt),
		InputChars:  len(text),
	}

	if opts.Backend == PrintPrompt {
		info.Mode = "print-prompt"
		return prompt, info, nil
	}

	if opts.Model == "" {
		return "", Info{}, fmt.Errorf("rewrite: --model required for %s backend", opts.Backend)
	}
	if opts.BaseURL == "" {
		return "", Info{}, fmt.Errorf("rewrite: --base-url required for %s backend", opts.Backend)
	}
	warning, err := CheckRemote(opts.BaseURL, opts.AllowRemote)
	if err != nil {
		return "", Info{}, err
	}
	info.Warning = warning

	n := max(opts.Candidates, 1)
	outs := make([]string, 0, n)
	for range n {
		var out string
		switch opts.Backend {
		case Ollama:
			out, err = callOllama(opts.BaseURL, opts.Model, prompt, opts.Timeout, opts.Temperature)
		case OpenAICompatible:
			out, err = callOpenAICompatible(opts.BaseURL, opts.Model, prompt, opts.APIKey, opts.Timeout, opts.Temperature)
		default:
			return "", Info{}, fmt.Errorf("rewrite: unknown backend %q", opts.Backend)
		}
		if err != nil {
			return "", Info{}, err
		}
		outs = append(outs, out)
	}

	out := outs[0]
	if len(outs) > 1 {
		info.Candidates = n
		best, scores := SelectCandidate(text, outs)
		out, info.CandidateScores = best, scores
	}

	if opts.LayerAAfter {
		cleaned, stats := scrub.Clean(out, scrub.DefaultOptions())
		out = cleaned
		info.LayerAAfter = &stats
	}

	info.OutputChars = len(out)
	info.Mode = "rewritten"
	info.Note = "Layer B is best-effort against statistical token-sampling watermarks; " +
		"cannot certify removal against a vendor detector."
	return out, info, nil
}

// FlagEnv reports whether an environment variable is set to a truthy value.
func FlagEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Env returns the environment variable, or def when unset/empty.
func Env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
