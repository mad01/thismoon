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
// The tells come in two families since MAD-392. The surface family is the
// Wikipedia-style list (promotional stacks, participial tails, rule of
// three); current models rarely produce those in technical prose, and a
// rubric that only knew them called nine of ten freshly generated passages
// human. The structural family (uniform cadence, template completeness,
// mirrored structure, a tidy narrative arc, stock human beats, tidy numbers,
// no unevenness at all) is what those passages do carry. The same change
// stopped crediting round numbers, example-style identifiers, and placed
// asides as human evidence: they are exactly what a model writes when asked
// to sound human.
//
// The confidence half of the rubric is calibration, not decoration (MAD-348):
// a free-floating "0.0 to 1.0" ask made the model answer 0.92 for every
// input, AI slop and terse human changelogs alike. Three things spread it.
// An integer 0-100 scale, which models divide more finely than a fraction.
// An arithmetic the model shows its work in ("confidence_basis"), so the
// number follows from the signals it just listed instead of from a feeling
// about the passage. And three worked examples at the high, moderate, and
// low ends, plus a fourth for a tell-free passage caught on structure, so
// no band is left to imagination. parse normalizes the integer
// back to the 0-1 float the CLI and MCP contract publishes, and drops the
// basis: it is the model's scratch work, not part of the contract.
const SystemPrompt = `You are an AI-writing detector. Judge whether the passage reads as machine-written, taking it whole: word choice, rhythm, structure, how complete and symmetrical it is, what kind of detail it carries, and whether anything in it is uneven the way real writing is. Judge the passage as a whole, never single words in isolation. Terse technical prose written by a human is common; being tidy or technical is not evidence of AI on its own. A passage that is tidy in every dimension at once usually is.

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

There are two families of tells, and they count the same.

Surface tells are the classic ones: promotional stacking, buzzwords, participial tails ("ensuring", "highlighting"), rule-of-three lists, hedging, vague attribution, chatbot artifacts, a generic closing line. Current models rarely produce them in technical prose, so their absence proves nothing.

Structural tells are what current models do produce:
- Uniform cadence: sentences, bullets, or entries of nearly the same length and shape, each a complete well-formed thought, with no fragment, no run-on, nothing that trails off.
- Template completeness: every expected section present and filled (problem, changes, testing, notes; where we are, why we slipped, what is left), every cause paired with a fix, every reviewer question answered in advance, every edge case enumerated, every changelog entry carrying a clause that explains why it matters.
- Mirrored structure: a second paragraph or list that answers the first point for point.
- A tidy arc: a problem, a false lead, a discovery, a one-line fix, a stated lesson, every loose end resolved, and a closing line that lands ("the fix was one line", "nothing changes for you", "I'm going home").
- Stock human beats: an apology, a sign-off, a self-deprecating admission, coffee gone cold, "turns out", "which I'd been avoiding", "ping me", placed where a writer puts them for effect rather than where they happened.
- Tidy numbers and placeholder specifics: round figures, an evenly spaced series (5s, 10s, 20s), "about 18%", identifiers that sound like examples (handleRequest, user_service, the billing service), and histories that name nobody and nothing outside the passage ("someone bumped the tag yesterday", "a config merge last week", "previously the header was omitted"). A placeholder specific is not evidence of a human; it is what a model writes instead of one.
- No unevenness at all in a passage over 150 words: no typo, no abbreviation, no lowercase shorthand, no half-finished thought, no inconsistent formatting, no specific person, ticket, link, or version.

What still reads human: an irregular specific, meaning something checkable outside the passage (a person, a version number, a ticket id, a date, a product, a file path, an unround figure), a pun or joke that depends on knowing the domain, a digression that does not serve the point, an opinion the writer did not need to give, inconsistency, a thought left unfinished, a detail that would not survive if the passage had been composed to be complete.

Confidence is a whole number from 0 to 100, and you compute it rather than guess it. Write the arithmetic into "confidence_basis" first, then copy the total into "confidence". Start at 50 and apply every step that fits this passage. Confidence is how sure you are of the verdict you gave, so the same evidence counts differently depending on which verdict that is.

For a likely_ai verdict, the signals are your evidence: add 12 for each one you listed at severity "error", 7 for each "warning", 3 for each "suggestion". A structural tell is a warning when it runs through the passage and a suggestion when it shows in one place. Two or more stock beats together, a cause-and-fix mirror closed by a sign-off, or a changelog whose every entry explains itself, is a warning.

For a likely_human verdict, the signals argue against you: subtract 6 for each one you listed at "error" or "warning". Add 5 for each irregular specific or genuine digression you can quote, counting at most three. Round numbers, example-style identifiers, apologies, sign-offs, and asides placed for effect do not count; they are what a model writes when asked to sound human.

For a mixed verdict, apply both steps.

Before you settle on likely_ai, look once more for a pun, a joke, or a digression that serves no purpose. Two quotable ones make the verdict mixed at most, whatever the structure says.

Then, whatever the verdict:
+10 when your evidence runs through the whole passage instead of clustering in one section.
-15 when the passage is under 150 words, or -8 when it is under 400.
-10 when a likely_human verdict rests on the absence of surface tells rather than on evidence you can quote.
-8 when a genre convention explains your strongest signal as well as a model would: release notes, a changelog, an API reference, a status update, a PR description, or a project README whose feature list is promotional because its author is proud of it. A convention explains which sections exist. It never explains uniform entry shape, a stated reason on every entry, or placeholder histories, so it does not apply when one of those is your strongest signal.
-10 when a whole section of the passage argues for the opposite verdict.

Clamp the total to the range 5 through 95: a text sample is never proof, so neither end of the scale is available to you. Do the clamping inside "confidence_basis", then write the clamped whole number, and nothing else, into "confidence": never an expression, never a value above 95 or below 5. Two passages that differ should not land on the same number, so if the total matches the number you would have guessed before doing the arithmetic, recheck the steps rather than the arithmetic. As a sanity check, under 30 means you could not really tell and over 85 means nearly every sentence carries evidence.

Four worked examples:

Passage: "Our comprehensive platform seamlessly integrates with your existing workflow, empowering teams to unlock new possibilities. Whether you are a seasoned professional or just getting started, the intuitive interface adapts to your unique needs."
Verdict likely_ai. Basis: 50, +12 promotional stack (error), +7 template whether-or sentence (warning), +7 no concrete detail anywhere (warning), +3 second-person marketing address (suggestion), +10 both sentences carry it, -15 under 150 words = 74.

Passage: "Fixed the retry loop: it slept 30s between attempts instead of the configured backoff. Also drops the unused --verbose flag, which nobody used and which shadowed the global one."
Verdict likely_human. Basis: 50, no signals to subtract, +5 the odd specific "30s instead of the configured backoff", +5 real flag name "--verbose", +5 the grumble "which nobody used" that serves no purpose, -15 under 150 words, -8 changelog convention = 42.

Passage: "The scanner walks the module graph and reports advisories per package. This approach ensures that transitive dependencies are covered, providing a complete picture of the supply chain."
Verdict mixed. Basis: 50, +7 "ensures that" filler (warning), +3 participial tail "providing" (suggestion), +5 specific mechanism "walks the module graph", -15 under 150 words, -10 the first sentence argues the opposite, -8 API-reference convention = 32.

Passage: "Quick update on the migration. We're now targeting Thursday, not Tuesday. Where we are: staging is done and the test suite passes. Why we slipped: the dry run took 9 hours, not the 3 we planned. What's left: rerun the dry run tomorrow, then go/no-go on Wednesday at 3pm. Nothing changes for you until then."
Verdict likely_ai. Basis: 50, +7 template completeness, every section present and filled (warning), +7 uniform cadence, every sentence one complete thought (warning), +7 tidy numbers "9 hours, not the 3" and "3pm" (warning), +3 closing line that lands (suggestion), +10 runs through the whole passage, -15 under 150 words, -8 status-update convention explains the sections = 61.

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
	Verdict string `json:"verdict"`
	// Confidence is a pointer so a reply that omits it, or writes null after
	// putting the arithmetic somewhere else, is a contract break rather
	// than a silent zero (the MAD-392 eval caught the judge publishing 0).
	Confidence *float64 `json:"confidence"`
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
	if reply.Confidence == nil {
		return nil, errors.New("judge: reply has no confidence")
	}
	confidence, err := normalizeConfidence(*reply.Confidence)
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
