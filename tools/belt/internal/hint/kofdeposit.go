package hint

import (
	"bytes"
	"os"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// depositMinToolUses is how much tool activity a session needs before the
// deposit nudge considers it substantial. Below this a session likely derived
// nothing worth pinning; the number errs high because a premature nudge
// teaches the model to ignore the hint (docs/adr/0008).
const depositMinToolUses = 30

// depositMarker is the synthetic seen-file id that makes the nudge fire at
// most once per session.
const depositMarker = "kof-deposit-nudge"

// toolUseKey and assertNameKeys are matched as raw JSON key/value substrings
// of the transcript. The key form is deliberate: the bare tool name appears
// in prose (instructions, this very hint's advice) while the `"name":`
// adjacency only occurs in a real tool_use block. Both the kof spelling and
// the pre-rename keep spelling count as a deposit, so sessions spanning the
// keep → keeper-of-facts cutover are not nudged twice.
var (
	toolUseKey     = []byte(`"type":"tool_use"`)
	assertNameKeys = [][]byte{
		[]byte(`"name":"mcp__kof__kof_assert"`),
		[]byte(`"name":"mcp__keep__keep_assert"`),
	}
)

// KofDeposit nudges a session that did substantial work to deposit what it
// derived. The consult half (kof-consult, kof-assertions) only pays off
// when earlier sessions wrote assertions, and nothing prompts that write —
// instruction-file prose alone does not trigger reliably.
type KofDeposit struct {
	cfg config.Config
}

func NewKofDeposit(cfg config.Config) *KofDeposit {
	return &KofDeposit{cfg: cfg}
}

func (h *KofDeposit) ID() string    { return "kof-deposit" }
func (h *KofDeposit) Event() string { return EventPrompt }

func (h *KofDeposit) Check(in Input) *Advice {
	// Without a session id the once-per-session guarantee is impossible, and
	// a nudge on every prompt is worse than none.
	path := seen.path(in.SessionID)
	if path == "" {
		return nil
	}
	if seen.load(path)[depositMarker] {
		return nil // already nudged this session; skip before the transcript read
	}
	raw, err := os.ReadFile(in.TranscriptPath)
	if err != nil {
		return nil
	}
	if bytes.Count(raw, toolUseKey) < depositMinToolUses {
		return nil
	}
	for _, key := range assertNameKeys {
		if bytes.Contains(raw, key) {
			return nil // the session already deposited on its own
		}
	}
	seen.record(path, []string{depositMarker})
	return &Advice{Hint: h.ID(), Text: depositAdvice}
}

const depositAdvice = "this session has done substantial work and deposited nothing in keeper-of-facts. " +
	"If it derived a non-obvious finding — a behavior, an invariant, a dead end — record it now " +
	"with kof_assert (one sentence, at least one evidence pin, subject like repo:org/name/path). " +
	"If nothing is worth keeping, carry on; this reminder fires once per session."
