// Package judge runs the holistic whole-passage AI-writing judgment: one LLM
// call over the full text (MAD-342). It complements the deterministic layers,
// which only see matched spans and whole-sample metrics; the judge reads the
// passage the way a human reader does and reports the gestalt verdict.
// Verdicts are advisory: a second opinion, never a replacement for the
// deterministic tools.
package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/humanizer/internal/backend"
)

// SystemPrompt is the static judgment rubric sent with every call. It
// enforces the JSON-only output contract that Run parses. Whole-passage
// framing is deliberate: asked about single words, models defend every
// common word as fine; asked about a passage, they judge well.
const SystemPrompt = `You are an AI-writing detector. Judge whether the passage reads as machine-written, taking it whole: word-choice density, rhythm, hedging, buzzword stacking, generic versus specific detail, structure, and tone uniformity. Judge the passage as a whole, never single words in isolation. Terse technical prose written by a human is common; being tidy or technical is not evidence of AI on its own.

Reply with a single JSON object and nothing else (no markdown fences, no commentary):
{
  "verdict": "likely_ai" | "likely_human" | "mixed",
  "confidence": <number between 0.0 and 1.0>,
  "signals": [
    {"pattern": "<short name of the tell>", "severity": "suggestion" | "warning" | "error", "excerpt": "<up to 15 words quoted from the passage>"}
  ],
  "summary": "<one sentence overall assessment>"
}

Use "mixed" when parts read human and parts read machine-written. List at most 6 signals, strongest first; an empty signals list is valid for clean human prose. The passage follows.`

// Signal is one concrete tell the judge observed in the passage.
type Signal struct {
	Pattern  string `json:"pattern"`
	Severity string `json:"severity"`
	Excerpt  string `json:"excerpt"`
}

// Verdict is the judge's whole-passage assessment. Backend and Model record
// which provider produced it so downstream reports stay comparable.
type Verdict struct {
	Verdict    string   `json:"verdict"`
	Confidence float64  `json:"confidence"`
	Signals    []Signal `json:"signals"`
	Summary    string   `json:"summary"`
	Backend    string   `json:"backend"`
	Model      string   `json:"model"`
}

// Run judges text with one completion against b, retrying once when the
// model breaks the JSON contract (the MAD-342 fallback ladder; after the
// second failure the caller falls back to deterministic-only results).
func Run(ctx context.Context, b backend.Backend, text string) (*Verdict, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("judge: text is empty")
	}
	raw, err := b.Complete(ctx, SystemPrompt, text)
	if err != nil {
		return nil, fmt.Errorf("judge: %w", err)
	}
	v, perr := parse(raw)
	if perr != nil {
		raw, err = b.Complete(ctx, SystemPrompt, text)
		if err != nil {
			return nil, fmt.Errorf("judge: retry: %w", err)
		}
		if v, perr = parse(raw); perr != nil {
			return nil, fmt.Errorf("judge: model broke the JSON contract twice: %w", perr)
		}
	}
	v.Backend = b.Name()
	v.Model = b.Model()
	return v, nil
}

// parse decodes and validates one model reply. Models occasionally wrap JSON
// in markdown fences despite the contract, so those are stripped first.
func parse(raw string) (*Verdict, error) {
	s := strings.TrimSpace(raw)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = after
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = after
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))

	var v Verdict
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("judge: decode verdict: %w", err)
	}
	switch v.Verdict {
	case "likely_ai", "likely_human", "mixed":
	default:
		return nil, fmt.Errorf("judge: invalid verdict %q", v.Verdict)
	}
	if v.Confidence < 0 || v.Confidence > 1 {
		return nil, fmt.Errorf("judge: confidence %v out of range", v.Confidence)
	}
	if v.Signals == nil {
		v.Signals = []Signal{}
	}
	return &v, nil
}
