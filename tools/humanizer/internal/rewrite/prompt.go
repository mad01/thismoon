// Package rewrite is Layer B of the watermark text cleaner: a best-effort pass
// at statistical (token-sampling) watermarks, which Layer A's deterministic
// scrub cannot touch. It builds a rewrite prompt and, optionally, runs it
// against a local or remote chat model.
//
// The default backend is print-prompt: it returns the prompt and calls no
// model, so it is offline, deterministic, and safe under the MCP sandbox — the
// calling agent does the actual rewrite. The ollama and openai-compatible
// backends send text off-process and are CLI-only under the current sandbox.
//
// Security posture (mirrors the Python original): only http(s) endpoints are
// accepted, non-loopback hosts are denied unless explicitly opted in, redirects
// are refused so an API-key header can never be re-sent to an unvalidated host,
// and API keys come from the environment only — never a flag.
//
// Ported from watermarks-remover's rewrite_text.py (MIT,
// guillaumemeyer/watermarks-remover @28eca2d); see docs/MIGRATED-FROM.md.
package rewrite

import "fmt"

// Strength selects the rewrite instruction.
type Strength string

const (
	Paraphrase    Strength = "paraphrase"
	Backtranslate Strength = "backtranslate"
	Structural    Strength = "structural"
	Humanize      Strength = "humanize"
	Code          Strength = "code"
)

const (
	paraphrasePrompt = "Rewrite the following text so that it uses substantially different wording at " +
		"the token level. Change clause order, connectors, and transition words; vary " +
		"sentence boundaries and length; and replace both content words and function " +
		"words where meaning allows. Preserve all facts, numbers, names, and technical " +
		"identifiers. Do not add or remove claims. Output only the rewritten text.\n\n---\n%s"

	humanizePrompt = "Rewrite the following text so it reads as if a human wrote it from scratch. " +
		"Vary sentence rhythm and length, replace formulaic AI-style transitions and " +
		"filler with concrete natural phrasing, and use plain, varied wording. Preserve " +
		"all facts, numbers, names, and technical identifiers. Do not add or remove " +
		"claims. Output only the rewritten text.\n\n---\n%s"

	codePrompt = "Rewrite the natural-language parts of this code — comments, docstrings, and " +
		"string literals — using different wording. Rename local variables, function " +
		"parameters, and private helper names to semantically equivalent names. Preserve " +
		"program behavior, public API names, and all values that affect output. Output " +
		"only the rewritten code.\n\n---\n%s"
)

// BuildPrompt renders the instruction for the given strength around text.
func BuildPrompt(strength Strength, text, lang, originalLang string) (string, error) {
	switch strength {
	case Paraphrase:
		return fmt.Sprintf(paraphrasePrompt, text), nil
	case Humanize:
		return fmt.Sprintf(humanizePrompt, text), nil
	case Code:
		return fmt.Sprintf(codePrompt, text), nil
	case Backtranslate:
		return fmt.Sprintf(
			"Translate the text to %s, then translate that result back to %s. "+
				"Preserve all facts, numbers, and names. "+
				"Output only the final %s text.\n\n---\n%s",
			lang, originalLang, originalLang, text,
		), nil
	case Structural:
		return "First extract a bullet outline of all claims (no full sentences). " +
			"Then write a complete document from that outline in natural, varied human " +
			"prose without omitting any bullet. Output only the final document.\n\n---\n" +
			text, nil
	default:
		return "", fmt.Errorf("unknown strength: %q", strength)
	}
}
