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
//
// The confidence half of the rubric is calibration, not decoration (MAD-348):
// a free-floating "0.0 to 1.0" ask made the model answer 0.92 for every
// input, AI slop and terse human changelogs alike. Three things spread it.
// An integer 0-100 scale, which models divide more finely than a fraction.
// An arithmetic the model shows its work in ("confidence_basis"), so the
// number follows from the signals it just listed instead of from a feeling
// about the passage. And three worked examples at the high, moderate, and
// low ends, so no band is left to imagination. parse normalizes the integer
// back to the 0-1 float the CLI and MCP contract publishes, and drops the
// basis: it is the model's scratch work, not part of the contract.
const SystemPrompt = `You are an AI-writing detector. Judge whether the passage reads as machine-written, taking it whole: word-choice density, rhythm, hedging, buzzword stacking, generic versus specific detail, structure, and tone uniformity. Judge the passage as a whole, never single words in isolation. Terse technical prose written by a human is common; being tidy or technical is not evidence of AI on its own.

Reply with a single JSON object and nothing else (no markdown fences, no commentary):
{
  "verdict": "likely_ai" | "likely_human" | "mixed",
  "signals": [
    {"pattern": "<short name of the tell>", "severity": "suggestion" | "warning" | "error", "excerpt": "<up to 15 words quoted from the passage>"}
  ],
  "confidence_basis": "<the confidence arithmetic, written out>",
  "confidence": <integer from 0 to 100>,
  "summary": "<one sentence overall assessment>"
}

Use "mixed" when parts read human and parts read machine-written. List at most 6 signals, strongest first; an empty signals list is valid for clean human prose.

Confidence is a whole number from 0 to 100, and you compute it rather than guess it. Write the arithmetic into "confidence_basis" first, then copy the total into "confidence". Start at 50 and apply every step that fits this passage. Confidence is how sure you are of the verdict you gave, so the same evidence counts differently depending on which verdict that is.

For a likely_ai verdict, the signals are your evidence: add 12 for each one you listed at severity "error", 7 for each "warning", 3 for each "suggestion".

For a likely_human verdict, the signals argue against you: subtract 6 for each one you listed at "error" or "warning". Add 6 for each thing you can quote that a model rarely writes, counting at most four: an aside, an opinion, a joke, an exact number or identifier, a sentence that breaks the passage's own rhythm.

For a mixed verdict, apply both steps.

Then, whatever the verdict:
+10 when your evidence runs through the whole passage instead of clustering in one section.
-15 when the passage is under 150 words, or -8 when it is under 400.
-10 when the verdict rests on the absence of tells rather than on evidence you can quote.
-8 when a genre convention (release notes, a changelog, an API reference, a reference table) explains your strongest signal as well as a model would.
-10 when a whole section of the passage argues for the opposite verdict.

Clamp the total to the range 5 through 95: a text sample is never proof, so neither end of the scale is available to you. Two passages that differ should not land on the same number, so if the total matches the number you would have guessed before doing the arithmetic, recheck the steps rather than the arithmetic. As a sanity check, under 30 means you could not really tell and over 85 means nearly every sentence carries evidence.

Three worked examples:

Passage: "Our comprehensive platform seamlessly integrates with your existing workflow, empowering teams to unlock new possibilities. Whether you are a seasoned professional or just getting started, the intuitive interface adapts to your unique needs."
Verdict likely_ai. Basis: 50, +12 promotional stack (error), +7 template whether-or sentence (warning), +7 no concrete detail anywhere (warning), +3 second-person marketing address (suggestion), +10 both sentences carry it, -15 under 150 words = 74.

Passage: "Fixed the retry loop: it slept 30s between attempts instead of the configured backoff. Also drops the unused --verbose flag, which nobody used and which shadowed the global one."
Verdict likely_human. Basis: 50, no signals to subtract, +6 exact number "30s", +6 real flag name "--verbose", +6 offhand aside "which nobody used", -15 under 150 words, -8 changelog convention = 45.

Passage: "The scanner walks the module graph and reports advisories per package. This approach ensures that transitive dependencies are covered, providing a complete picture of the supply chain."
Verdict mixed. Basis: 50, +7 "ensures that" filler (warning), +3 participial tail "providing" (suggestion), +6 specific "walks the module graph", -15 under 150 words, -10 the first sentence argues the opposite, -8 API-reference convention = 33.

The passage follows.`

// Signal is one concrete tell the judge observed in the passage.
type Signal struct {
	Pattern  string `json:"pattern"`
	Severity string `json:"severity"`
	Excerpt  string `json:"excerpt"`
}

// Verdict is the judge's whole-passage assessment. Confidence is a 0-1
// fraction, the published shape for the CLI and MCP callers, whatever scale
// the model replied on. Backend and Model record which provider produced it
// so downstream reports stay comparable.
type Verdict struct {
	Verdict    string   `json:"verdict"`
	Confidence float64  `json:"confidence"`
	Signals    []Signal `json:"signals"`
	Summary    string   `json:"summary"`
	Backend    string   `json:"backend"`
	Model      string   `json:"model"`
}

// modelReply is the wire shape of one model completion. It is separate from
// Verdict because the two scales differ: the prompt asks for an integer
// 0-100, the published contract is a 0-1 float. Decoding straight into
// Verdict would make a Verdict re-decode itself at a different scale.
type modelReply struct {
	Verdict    string   `json:"verdict"`
	Confidence float64  `json:"confidence"`
	Signals    []Signal `json:"signals"`
	Summary    string   `json:"summary"`
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

	var reply modelReply
	if err := json.Unmarshal([]byte(s), &reply); err != nil {
		return nil, fmt.Errorf("judge: decode verdict: %w", err)
	}
	switch reply.Verdict {
	case "likely_ai", "likely_human", "mixed":
	default:
		return nil, fmt.Errorf("judge: invalid verdict %q", reply.Verdict)
	}
	confidence, err := normalizeConfidence(reply.Confidence)
	if err != nil {
		return nil, err
	}
	signals := reply.Signals
	if signals == nil {
		signals = []Signal{}
	}
	return &Verdict{
		Verdict:    reply.Verdict,
		Confidence: confidence,
		Signals:    signals,
		Summary:    reply.Summary,
	}, nil
}

// normalizeConfidence maps whatever scale the model replied on onto the 0-1
// fraction Verdict publishes. The prompt asks for an integer 0-100, but a
// model that ignores it and answers 0.8 still means eighty percent, so
// anything at or below 1 is taken as an already-normalized fraction and
// anything above it as a percentage. The two scales only collide at a
// literal 1, which reads as certainty rather than as one percent.
func normalizeConfidence(c float64) (float64, error) {
	if c < 0 || c > 100 {
		return 0, fmt.Errorf("judge: confidence %v out of range", c)
	}
	if c > 1 {
		return c / 100, nil
	}
	return c, nil
}
