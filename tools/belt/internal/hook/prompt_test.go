package hook

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// kofRepo stands up what kof-consult needs from the machine: a kof serve
// stub holding one assertion about mad01/thismoon on KOF_PORT, a git repo
// whose origin is that repo, and a HOME of its own for the seen file. It
// returns the repo directory.
func kofRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"assertions":[{
			"id":"a1",
			"subject":"repo:mad01/thismoon/tools/belt",
			"statement":"the seen file dedupes kof assertions per session",
			"status":"fresh",
			"pins":[{"repo":"github.com/mad01/thismoon","file":"tools/belt/internal/hint/seen.go",
				"start_line":1,"end_line":9}]
		}]}`))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOF_PORT", u.Port())
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "git@github.com:mad01/thismoon.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// promptPayload is a UserPromptSubmit payload as Claude Code sends it,
// captured from a live hook: the prompt rides as a top-level string.
func promptPayload(t *testing.T, cwd, prompt string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]string{
		"session_id":      "s1",
		"transcript_path": "",
		"cwd":             cwd,
		"permission_mode": "default",
		"hook_event_name": "UserPromptSubmit",
		"prompt_id":       "p1",
		"prompt":          prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestRunHintPromptRunsKofConsult pins the prompt path end to end: the
// payload's prompt reaches kof-consult, the advice goes out as plain text,
// and the events entry carries the assertion and pin tags beside the ones
// the hook owns.
func TestRunHintPromptRunsKofConsult(t *testing.T) {
	dir := kofRepo(t)
	raw := promptPayload(t, dir, "why does the seen file dedupe per session?")
	var out bytes.Buffer
	var a archived
	runner{load: defaults, emit: a.emit}.hints("prompt", strings.NewReader(raw), &out)

	got := out.String()
	if !strings.HasPrefix(got, "belt[kof-consult]:") {
		t.Fatalf("prompt advice = %q, want plain text from kof-consult", got)
	}
	if !strings.Contains(got, "the seen file dedupes kof assertions per session") {
		t.Errorf("advice %q does not carry the assertion", got)
	}
	if len(a.events) != 1 {
		t.Fatalf("archived %d events, want 1: %+v", len(a.events), a.events)
	}
	want := map[string]string{
		"hint":       "kof-consult",
		"event":      "prompt",
		"session_id": "s1",
		"assertions": "a1",
		"pins":       "github.com/mad01/thismoon/tools/belt/internal/hint/seen.go",
	}
	if tags := a.events[0].Tags; !maps.Equal(tags, want) {
		t.Errorf("event tags = %v, want %v", tags, want)
	}
	if title := a.events[0].Title; title != "hinted (kof-consult)" {
		t.Errorf("event title = %q, want no tool name on a prompt event", title)
	}
}

// TestRunHintSessionStartSkipsKofConsult: with the same repo and kof stub,
// a session start says nothing about kof. The consult waits for a prompt.
func TestRunHintSessionStartSkipsKofConsult(t *testing.T) {
	dir := kofRepo(t)
	raw := `{"session_id":"s1","cwd":"` + dir + `","hook_event_name":"SessionStart"}`
	var out bytes.Buffer
	var a archived
	runner{load: defaults, emit: a.emit}.hints("session-start", strings.NewReader(raw), &out)
	if strings.Contains(out.String(), "kof-consult") || len(a.events) != 0 {
		t.Errorf("session-start wrote %q and archived %+v, want no kof-consult",
			out.String(), a.events)
	}
}

// TestRunHintPromptToleratesAMistypedPrompt: no guard reads the prompt, so
// a value of another type is dropped like a mistyped session id. The run
// stays silent and leaves no decode warning.
func TestRunHintPromptToleratesAMistypedPrompt(t *testing.T) {
	dir := kofRepo(t)
	raw := `{"session_id":"s1","cwd":"` + dir + `","prompt":{"text":"the seen file dedupes"}}`
	var out bytes.Buffer
	var a archived
	runner{load: defaults, emit: a.emit}.hints("prompt", strings.NewReader(raw), &out)
	if out.Len() != 0 || len(a.events) != 0 {
		t.Errorf("wrote %q and archived %+v, want silence", out.String(), a.events)
	}
}

func TestAdviceTagsKeepTheHookOwnedTags(t *testing.T) {
	advice := hint.Advice{Hint: "kof-consult", Tags: map[string]string{
		"hint":       "spoofed",
		"event":      "spoofed",
		"session_id": "spoofed",
		"agent_type": "spoofed",
		"assertions": "a1,a2",
	}}
	got := adviceTags(advice, "prompt", payload{SessionID: "s1"})
	want := map[string]string{
		"hint":       "kof-consult",
		"event":      "prompt",
		"session_id": "s1",
		"assertions": "a1,a2",
	}
	if !maps.Equal(got, want) {
		t.Errorf("adviceTags = %v, want %v (agent_type dropped: the payload had none)", got, want)
	}
}

func TestToHintInputCarriesThePrompt(t *testing.T) {
	p := payload{Prompt: "rank against this"}
	if got := toHintInput(hint.EventPrompt, p).Prompt; got != "rank against this" {
		t.Errorf("prompt input = %q, want the payload's prompt", got)
	}
	if got := toHintInput(hint.EventSessionStart, p).Prompt; got != "" {
		t.Errorf("session-start input carries prompt %q, want none", got)
	}
}
