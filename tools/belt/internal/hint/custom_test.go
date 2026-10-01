package hint

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// writeHintScript drops an executable /bin/sh script into dir. Scripts are
// generated per test rather than committed as testdata so the exec bit never
// depends on checkout behavior.
func writeHintScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// generousBudget keeps every test that is not about the timeout off the
// timeout path: a freshly written script's first launch can take seconds
// under a full-repo `go test ./...`, and the production 400ms budget would
// turn that into a spurious silence.
const generousBudget = 10_000

// newCustomHint builds the hint under test with a warning recorder, a
// repo resolver that never resolves (so exclude_repos cannot match unless a
// test says so), and the generous budget.
func newCustomHint(entry config.CustomHint, warned *[]string) *Custom {
	if entry.Event == "" {
		entry.Event = EventSessionStart
	}
	if entry.TimeoutMS == 0 {
		entry.TimeoutMS = generousBudget
	}
	cfg := config.Config{CustomHints: map[string]config.CustomHint{"test-hint": entry}}
	c := NewCustom("test-hint", cfg)
	c.resolveRepo = func(string) string { return "" }
	c.emit = func(_, _, _, message string, _ map[string]string) {
		*warned = append(*warned, message)
	}
	return c
}

func TestCustomHintOutcomes(t *testing.T) {
	dir := t.TempDir()
	speaks := writeHintScript(t, dir, "speaks.sh",
		"printf 'open actions:\\n  - review the release PR  \\n\\n'")
	fails := writeHintScript(t, dir, "fails.sh", "echo partial\nexit 1")
	silent := writeHintScript(t, dir, "silent.sh", "exit 0")
	whitespace := writeHintScript(t, dir, "whitespace.sh", "printf '  \\n\\n'")

	tests := []struct {
		name     string
		command  []string
		wantText string // "" means silence
		wantWarn string // substring of the warn message; "" means no warn
	}{
		{
			"exit 0 with output advises, trailing whitespace trimmed, lines kept",
			[]string{speaks},
			"open actions:\n  - review the release PR",
			"",
		},
		{"non-zero exit is silent with a warn", []string{fails}, "", "failed"},
		{"empty stdout is silent with a warn", []string{silent}, "", "empty stdout"},
		{"whitespace-only stdout counts as empty", []string{whitespace}, "", "empty stdout"},
		{
			"missing binary is silent with a warn",
			[]string{filepath.Join(dir, "nope")},
			"",
			"failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			h := newCustomHint(config.CustomHint{Command: tt.command}, &warned)

			got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"})

			if tt.wantText == "" && got != nil {
				t.Errorf("Check = %+v, want silence", got)
			}
			if tt.wantText != "" && (got == nil || got.Text != tt.wantText) {
				t.Errorf("Check = %+v, want text %q", got, tt.wantText)
			}
			if got != nil && got.Hint != "test-hint" {
				t.Errorf("advice attributed to %q, want test-hint", got.Hint)
			}
			if tt.wantWarn == "" && len(warned) > 0 {
				t.Errorf("unexpected warnings %v", warned)
			}
			if tt.wantWarn != "" &&
				(len(warned) != 1 || !strings.Contains(warned[0], tt.wantWarn)) {
				t.Errorf("warnings = %v, want one containing %q", warned, tt.wantWarn)
			}
		})
	}
}

// TestCustomHintTimeout pins the budget: a tiny timeout_ms against an
// external that sleeps far past it must end in silence plus a warn, and
// promptly, even though the script's sleep child still holds stdout after
// the script itself is killed.
func TestCustomHintTimeout(t *testing.T) {
	slow := writeHintScript(t, t.TempDir(), "slow.sh", "sleep 5\necho late")
	var warned []string
	h := newCustomHint(config.CustomHint{Command: []string{slow}, TimeoutMS: 50}, &warned)

	if got := h.Check(Input{Event: EventSessionStart, Cwd: "/", SessionID: "s"}); got != nil {
		t.Errorf("timeout should be silent, got %+v", got)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "timed out after 50ms") {
		t.Errorf("timeout warning missing: %v", warned)
	}
}

func TestCustomHintPayload(t *testing.T) {
	dir := t.TempDir()
	captured := filepath.Join(dir, "payload.json")
	capture := writeHintScript(t, dir, "capture.sh", "cat > "+captured+"\necho ok")
	var warned []string
	h := newCustomHint(config.CustomHint{Command: []string{capture}}, &warned)

	h.Check(Input{
		Event:          EventSessionStart,
		Cwd:            "/work",
		SessionID:      "s1",
		TranscriptPath: "/t.jsonl",
	})

	raw, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"event":           "session-start",
		"cwd":             "/work",
		"session_id":      "s1",
		"transcript_path": "/t.jsonl",
	}
	if !maps.Equal(got, want) {
		t.Errorf("payload = %v, want %v", got, want)
	}
}

// TestCustomHintExcludeRepos pins both places the opt-out list may sit and
// that an excluded repo costs no process launch: the script leaves a marker
// when it runs, and the marker must be absent.
func TestCustomHintExcludeRepos(t *testing.T) {
	tests := []struct {
		name      string
		entry     []string // custom_hints.<name>.exclude_repos
		toggle    []string // hints.<name>.exclude_repos
		wantSpoke bool
	}{
		{"no lists", nil, nil, true},
		{"entry list excludes", []string{"github.com/mad01/thismoon"}, nil, false},
		{"toggle list excludes", nil, []string{"github.com/mad01/*"}, false},
		{
			"lists naming other repos",
			[]string{"github.com/you/x"},
			[]string{"github.com/you/y"},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "ran")
			script := writeHintScript(t, dir, "speaks.sh", "touch "+marker+"\necho hi")
			cfg := config.Config{
				CustomHints: map[string]config.CustomHint{"test-hint": {
					Event:        EventSessionStart,
					Command:      []string{script},
					TimeoutMS:    generousBudget,
					ExcludeRepos: tt.entry,
				}},
				Hints: map[string]config.Toggle{"test-hint": {ExcludeRepos: tt.toggle}},
			}
			h := NewCustom("test-hint", cfg)
			h.resolveRepo = func(string) string { return "github.com/mad01/thismoon" }
			h.emit = func(string, string, string, string, map[string]string) {}

			got := h.Check(Input{Event: EventSessionStart, Cwd: dir, SessionID: "s"})

			if (got != nil) != tt.wantSpoke {
				t.Errorf("Check = %+v, want spoke %v", got, tt.wantSpoke)
			}
			_, err := os.Stat(marker)
			if ran := err == nil; ran != tt.wantSpoke {
				t.Errorf("external ran = %v, want %v", ran, tt.wantSpoke)
			}
		})
	}
}

func TestCustomHintsRegisterInAll(t *testing.T) {
	off := false
	cfg := config.Config{CustomHints: map[string]config.CustomHint{
		"zeta-journal":  {Event: EventSessionStart, Command: []string{"true"}},
		"alpha-journal": {Event: EventSessionStart, Command: []string{"true"}, Enabled: &off},
	}}
	var ids []string
	for _, h := range All(cfg) {
		ids = append(ids, h.ID())
	}
	// Custom hints come after the built-ins, alphabetical by name.
	n := len(ids)
	if n < 2 || ids[n-2] != "alpha-journal" || ids[n-1] != "zeta-journal" {
		t.Errorf("custom hints not appended sorted: %v", ids)
	}
	if cfg.HintEnabled("alpha-journal") {
		t.Error("disabled custom hint reported enabled")
	}
	if !cfg.HintEnabled("zeta-journal") {
		t.Error("default custom hint reported disabled")
	}
	var active []string
	for _, h := range ForEvent(EventSessionStart, cfg) {
		active = append(active, h.ID())
	}
	if slices.Contains(active, "alpha-journal") {
		t.Error("disabled custom hint still runs")
	}
	if !slices.Contains(active, "zeta-journal") {
		t.Error("enabled custom hint does not run on session-start")
	}
}

// TestHintFieldsListsEveryBuiltinHint pins config.HintFields to the hints
// All registers: the table is what config validation reads to refuse a
// custom hint that shadows a built-in name, so a hint added here without a
// row there would be shadowable.
func TestHintFieldsListsEveryBuiltinHint(t *testing.T) {
	var ids []string
	for _, h := range All(config.Config{}) {
		ids = append(ids, h.ID())
	}
	slices.Sort(ids)
	if want := slices.Sorted(maps.Keys(config.HintFields)); !slices.Equal(ids, want) {
		t.Errorf("hint.All ids = %v, config.HintFields keys = %v", ids, want)
	}
}
