package hint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// depositTranscript writes a fake transcript with n tool_use blocks and the
// given extra content, and points $HOME at a temp dir so the seen file never
// touches the real cache.
func depositTranscript(t *testing.T, n int, extra string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var b strings.Builder
	for range n {
		b.WriteString(
			`{"message":{"content":[{"type":"tool_use","name":"mcp__csl__csl_search"}]}}` + "\n",
		)
	}
	b.WriteString(extra)
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeepDepositNudgesOnceAboveThreshold(t *testing.T) {
	path := depositTranscript(t, depositMinToolUses, "")
	h := NewKeepDeposit(testConfig())
	in := Input{Event: EventPrompt, SessionID: "deposit-test", TranscriptPath: path}

	got := h.Check(in)
	if got == nil {
		t.Fatal("Check above threshold = nil, want the deposit nudge")
	}
	if !strings.Contains(got.Text, "keep_assert") {
		t.Errorf("advice %q does not mention keep_assert", got.Text)
	}
	if again := h.Check(in); again != nil {
		t.Errorf("second Check = %+v, want nil: the nudge must fire once per session", again)
	}
}

func TestKeepDepositSilentBelowThreshold(t *testing.T) {
	path := depositTranscript(t, depositMinToolUses-1, "")
	h := NewKeepDeposit(testConfig())
	if got := h.Check(Input{Event: EventPrompt, SessionID: "small", TranscriptPath: path}); got != nil {
		t.Errorf("Check below threshold = %+v, want nil", got)
	}
}

func TestKeepDepositSilentWhenSessionDeposited(t *testing.T) {
	deposited := `{"message":{"content":[{"type":"tool_use","name":"mcp__keep__keep_assert"}]}}` + "\n"
	path := depositTranscript(t, depositMinToolUses, deposited)
	h := NewKeepDeposit(testConfig())
	if got := h.Check(Input{Event: EventPrompt, SessionID: "kept", TranscriptPath: path}); got != nil {
		t.Errorf("Check after a real keep_assert = %+v, want nil", got)
	}
}

// The bug this pins: the bare tool name appears in prose (instruction files
// echoed into the transcript) without the session ever depositing, so the
// check must match the tool_use JSON key form, not the name as a substring.
func TestKeepDepositIgnoresProseMentions(t *testing.T) {
	prose := `{"message":{"content":[{"type":"text","text":"deposit via keep_assert (mcp__keep__keep_assert)"}]}}` + "\n"
	path := depositTranscript(t, depositMinToolUses, prose)
	h := NewKeepDeposit(testConfig())
	if got := h.Check(Input{Event: EventPrompt, SessionID: "prose", TranscriptPath: path}); got == nil {
		t.Error("Check = nil: a prose mention of keep_assert must not count as a deposit")
	}
}

func TestKeepDepositSilentWithoutSessionID(t *testing.T) {
	path := depositTranscript(t, depositMinToolUses, "")
	h := NewKeepDeposit(testConfig())
	if got := h.Check(Input{Event: EventPrompt, TranscriptPath: path}); got != nil {
		t.Errorf("Check without a session id = %+v, want nil: once-per-session needs an id", got)
	}
}

func TestKeepDepositSilentWithoutTranscript(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	h := NewKeepDeposit(testConfig())
	in := Input{Event: EventPrompt, SessionID: "gone", TranscriptPath: "/nonexistent/t.jsonl"}
	if got := h.Check(in); got != nil {
		t.Errorf("Check with a missing transcript = %+v, want nil", got)
	}
}
