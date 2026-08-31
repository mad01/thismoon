// Package narrative carries the StoryScope narrative-feature rubric: the 30
// core discourse-level features that separate human-written from AI-generated
// fiction (Russell et al., COLM 2026). Unlike the vale span rules, these
// features need a reader's judgment rather than a regex — the package serves
// the rubric and a ready-to-use judge prompt; scoring a passage against it is
// left to the calling agent (typically the humanizer skill's headless
// `claude -p` pass).
package narrative

import (
	"fmt"
	"strings"
)

// Source is the paper the rubric is derived from.
const Source = "https://arxiv.org/abs/2604.03136"

// Signal states which authorship a matching answer points to.
type Signal string

// The two authorship directions a feature can indicate.
const (
	SignalAI    Signal = "ai"
	SignalHuman Signal = "human"
)

// Feature is one core narrative feature from the StoryScope rubric. Question
// text is taken from the paper's core-feature tables; Direction describes the
// answer that counts as a match for Signal, and Stat carries the observed
// human-vs-AI gap where the paper reports one.
type Feature struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Question  string `json:"question"`
	Dimension string `json:"dimension"`
	Signal    Signal `json:"signal"`
	Direction string `json:"direction"`
	Stat      string `json:"stat,omitempty"`
	Theme     string `json:"theme"`
}

// Theme names, in presentation order. They follow the paper's §4 findings:
// AI over-explains meaning, keeps plots tidy and linear, renders emotion
// through the body, avoids the real world, and draws from a narrower range.
const (
	ThemeOverExplanation = "over-explanation"
	ThemePlotTidiness    = "plot-tidiness"
	ThemeEmbodiment      = "embodiment-and-senses"
	ThemeOutsideWorld    = "outside-world"
	ThemeRange           = "range"
)

// Themes returns the theme names in presentation order.
func Themes() []string {
	return []string{
		ThemeOverExplanation,
		ThemePlotTidiness,
		ThemeEmbodiment,
		ThemeOutsideWorld,
		ThemeRange,
	}
}

// Features returns the 30 core features in rubric order. The returned slice
// is a copy; callers may reorder or filter it freely.
func Features() []Feature {
	out := make([]Feature, len(features))
	copy(out, features)
	return out
}

// Prompt renders the rubric as a self-contained judge prompt: instructions
// plus every feature grouped by theme. Append the passage (or pipe it on
// stdin) and hand the whole thing to an LLM for a narrative-level verdict.
func Prompt() string {
	var b strings.Builder
	b.WriteString(`You are judging whether a narrative passage reads as AI-authored or ` +
		`human-authored from its NARRATIVE CHOICES, not its surface style ` +
		`(StoryScope rubric, ` + Source + `). For each feature below, answer the ` +
		`question about the passage; when the answer matches the listed direction, ` +
		`count it toward the listed authorship. A few features carry a ` +
		`counter-direction ("if instead ..."); when the answer matches that clause, ` +
		`count it toward the other authorship. Judge the balance of the counts, ` +
		`never a single feature. In short: AI-leaning stories over-explain their ` +
		`themes, keep one tidy linear plot, render emotion through the body, and ` +
		`avoid naming the real world; human-leaning stories tolerate loose ends, ` +
		`jump across time, name real works and places, and address the reader.` + "\n")
	for _, theme := range Themes() {
		fmt.Fprintf(&b, "\n## %s\n", theme)
		for _, f := range features {
			if f.Theme != theme {
				continue
			}
			fmt.Fprintf(
				&b,
				"- %s [%s-leaning] — %s Match: %s.",
				f.Name,
				f.Signal,
				f.Question,
				f.Direction,
			)
			if f.Stat != "" {
				fmt.Fprintf(&b, " (%s)", f.Stat)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n" +
		`The text to judge follows this rubric (appended below or on stdin). ` +
		`Full stories or 1,000+ word sections give reliable verdicts; on shorter ` +
		`excerpts treat features that simply do not appear as unknown, not as ` +
		`evidence either way — absence on a fragment is not an AI signal. ` +
		`End your answer with exactly one line: VERDICT|confidence|features, ` +
		`where VERDICT is AI or HUMAN, confidence is 0-100, and features names ` +
		`the two or three strongest matches.` + "\n")
	return b.String()
}

// Rubric bundles the payload served by both `humanizer narrative --json`
// and the humanizer_narrative_rubric MCP tool, so the two surfaces share
// one shape and cannot drift apart.
type Rubric struct {
	Features []Feature `json:"features"`
	Total    int       `json:"total"`
	Themes   []string  `json:"themes"`
	Prompt   string    `json:"prompt"`
	Source   string    `json:"source"`
}

// BuildRubric assembles the served rubric payload.
func BuildRubric() Rubric {
	fs := Features()
	return Rubric{
		Features: fs,
		Total:    len(fs),
		Themes:   Themes(),
		Prompt:   Prompt(),
		Source:   Source,
	}
}

var features = []Feature{
	{
		ID:        "thematic-explicitness",
		Name:      "Thematic explicitness and moralizing",
		Question:  "How explicitly does the story articulate its themes or morals?",
		Dimension: "situatedness",
		Signal:    SignalAI,
		Direction: "high — the narrator states the lesson outright",
		Stat:      "theme stated outright in 77% of AI stories vs 52% of human",
		Theme:     ThemeOverExplanation,
	},
	{
		ID:        "thematic-unity",
		Name:      "Thematic unity",
		Question:  "To what extent do subplots and flourishes serve a central thematic concern?",
		Dimension: "plot",
		Signal:    SignalAI,
		Direction: "high — everything serves one theme",
		Theme:     ThemeOverExplanation,
	},
	{
		ID:        "narratorial-commentary",
		Name:      "Narratorial thematic commentary",
		Question:  "Does the narrator explicitly comment on themes beyond characters' perspectives?",
		Dimension: "situatedness",
		Signal:    SignalAI,
		Direction: "yes",
		Theme:     ThemeOverExplanation,
	},
	{
		ID:        "moral-weighting",
		Name:      "Moral and philosophical weighting",
		Question:  "How heavily does the story foreground moral or philosophical questions?",
		Dimension: "situatedness",
		Signal:    SignalAI,
		Direction: "high",
		Theme:     ThemeOverExplanation,
	},
	{
		ID:        "dialogue-function",
		Name:      "Dialogue as philosophical debate",
		Question:  "What main functions does dialogue serve?",
		Dimension: "perspective",
		Signal:    SignalAI,
		Direction: "philosophical debate",
		Stat:      "59% of AI stories vs 34% of human",
		Theme:     ThemeOverExplanation,
	},
	{
		ID:        "reference-explicitness",
		Name:      "Reference explicitness",
		Question:  "Are intertextual gestures primarily explicit or diffuse?",
		Dimension: "situatedness",
		Signal:    SignalAI,
		Direction: "implicit echoes and vague allusion; if instead a balanced explicit/implicit mix, count toward human",
		Stat:      "vague allusions 72% AI vs 50% human; balanced mix 37% human vs 16% AI",
		Theme:     ThemeOverExplanation,
	},
	{
		ID:        "causal-chain",
		Name:      "Causal chain continuity",
		Question:  "How continuous is the single causal chain from inciting incident to ending?",
		Dimension: "event",
		Signal:    SignalAI,
		Direction: "high — one unbroken chain, no loose ends",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "subplot-integration",
		Name:      "Subplot integration",
		Question:  "How directly do subplots echo the central theme?",
		Dimension: "plot",
		Signal:    SignalAI,
		Direction: "no subplots at all; if instead subplots run thematically parallel, count toward human",
		Stat:      "no subplots 79% AI vs 57% human; parallel subplots 42% human vs 21% AI",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "resolution-agency",
		Name:      "Agency in resolution",
		Question:  "Is resolution driven by the protagonist's choices or external events?",
		Dimension: "plot",
		Signal:    SignalAI,
		Direction: "protagonist choice",
		Stat:      "69% of AI stories vs 46% of human",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "resolution-mode",
		Name:      "Mode of resolution",
		Question:  "Is the main event chain resolved through internal acceptance or external action?",
		Dimension: "event",
		Signal:    SignalAI,
		Direction: "resolved internally, through understanding or acceptance",
		Stat:      "47% of AI stories vs 27% of human",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "chronological-discontinuity",
		Name:      "Chronological discontinuity",
		Question:  "How often does the narrative jump across time?",
		Dimension: "temporal",
		Signal:    SignalHuman,
		Direction: "frequent time jumps",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "anachrony",
		Name:      "Anachrony intensity",
		Question:  "How heavily does the narrative rely on flashbacks or flash-forwards?",
		Dimension: "temporal",
		Signal:    SignalHuman,
		Direction: "heavy",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "nonlinear-disclosure",
		Name:      "Nonlinear framing for delayed disclosure",
		Question:  "To what extent does the story use time jumps to stage revelations?",
		Dimension: "revelation",
		Signal:    SignalHuman,
		Direction: "high",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "recontextualization",
		Name:      "Recontextualization after surprise",
		Question:  "How extensively does a revelation force reinterpretation of earlier scenes?",
		Dimension: "revelation",
		Signal:    SignalHuman,
		Direction: "high — the reveal demands re-reading",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "pre-threat-investment",
		Name:      "Pre-threat character investment",
		Question:  "How much does the story build investment before major jeopardy?",
		Dimension: "revelation",
		Signal:    SignalAI,
		Direction: "high — a long calm ramp before danger",
		Theme:     ThemePlotTidiness,
	},
	{
		ID:        "emotional-expression",
		Name:      "Dominant emotional expression",
		Question:  "How are characters' emotions most commonly conveyed?",
		Dimension: "agent",
		Signal:    SignalAI,
		Direction: "embodied metaphors; if instead emotions get explicit labels, count toward human",
		Stat:      "embodied 81% AI vs 38% human; explicit labels 29% human vs 8% AI",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "sensory-modalities",
		Name:      "Olfactory emphasis",
		Question:  "Which sensory modalities does the story most frequently engage?",
		Dimension: "setting",
		Signal:    SignalAI,
		Direction: "smell is prominent",
		Stat:      "smell imagery in 82% of AI stories vs 57% of human",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "sensory-density",
		Name:      "Sensory description density",
		Question:  "How dense is sensory description across the narrative?",
		Dimension: "setting",
		Signal:    SignalAI,
		Direction: "high",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "setting-mirror",
		Name:      "Setting as inner-state mirror",
		Question:  "To what degree does the physical environment mirror characters' inner states?",
		Dimension: "setting",
		Signal:    SignalAI,
		Direction: "high — weather and rooms track feelings",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "interior-access",
		Name:      "Depth of interior access",
		Question:  "How deep into characters' inner life does narration go?",
		Dimension: "perspective",
		Signal:    SignalAI,
		Direction: "deep",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "character-introduction",
		Name:      "Character introduction device",
		Question:  "What narrative device primarily introduces the central character?",
		Dimension: "agent",
		Signal:    SignalAI,
		Direction: "external description",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "ecological-emphasis",
		Name:      "Environmental and ecological emphasis",
		Question:  "How prominent is the natural environment or ecology in the narrative?",
		Dimension: "setting",
		Signal:    SignalAI,
		Direction: "high",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "opening-grounding",
		Name:      "Opening spatial grounding",
		Question:  "How clearly does the opening ground the reader in a specific physical setting?",
		Dimension: "setting",
		Signal:    SignalAI,
		Direction: "clear local and global grounding from the first lines",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "spatial-granularity",
		Name:      "Spatial granularity",
		Question:  "How fine-grained is the story's depiction of physical space?",
		Dimension: "setting",
		Signal:    SignalAI,
		Direction: "high",
		Theme:     ThemeEmbodiment,
	},
	{
		ID:        "intertextual-strategy",
		Name:      "Explicit named references",
		Question:  "What kinds of intertextual engagement does the story employ?",
		Dimension: "situatedness",
		Signal:    SignalHuman,
		Direction: "explicit named works, brands, and places",
		Stat:      "named references in 47% of human stories vs 24% of AI",
		Theme:     ThemeOutsideWorld,
	},
	{
		ID:        "reader-address",
		Name:      "Direct reader address",
		Question:  "How often does the text directly address the reader?",
		Dimension: "perspective",
		Signal:    SignalHuman,
		Direction: "occasional asides or structural address",
		Stat:      "28% of human stories vs 7% of AI",
		Theme:     ThemeOutsideWorld,
	},
	{
		ID:        "fourth-wall",
		Name:      "Fourth-wall permeability",
		Question:  "To what extent does the story break the boundary between story-world and reader?",
		Dimension: "situatedness",
		Signal:    SignalHuman,
		Direction: "breaks present",
		Stat:      "fourth-wall breaks in 67% of human stories vs 39% of AI",
		Theme:     ThemeOutsideWorld,
	},
	{
		ID:        "dialogue-proportion",
		Name:      "Dialogue-to-narration proportion",
		Question:  "What proportion of the text is direct dialogue versus narration?",
		Dimension: "perspective",
		Signal:    SignalHuman,
		Direction: "dialogue-heavy",
		Theme:     ThemeRange,
	},
	{
		ID:        "location-variety",
		Name:      "Location variety",
		Question:  "How many distinct physical locales does the story inhabit?",
		Dimension: "setting",
		Signal:    SignalHuman,
		Direction: "many",
		Theme:     ThemeRange,
	},
	{
		ID:        "protagonist-polarity",
		Name:      "Moral polarity toward protagonist",
		Question:  "Does the narrative frame the protagonist's choices as morally clear or ambiguous?",
		Dimension: "plot",
		Signal:    SignalHuman,
		Direction: "ambivalent or mixed",
		Stat:      "morally ambivalent protagonists in 59% of human stories vs 38% of AI",
		Theme:     ThemeRange,
	},
}
