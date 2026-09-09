package hint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// humanizerTranscript writes a transcript holding the given content and points
// $HOME at a temp dir so the seen file never touches the real cache.
func humanizerTranscript(t *testing.T, content string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHumanizerNudgesOncePerSession(t *testing.T) {
	path := humanizerTranscript(
		t,
		`{"message":{"content":[{"type":"tool_use","name":"mcp__csl__csl_search"}]}}`+"\n",
	)
	h := NewHumanizer(testConfig())
	in := Input{
		Event:          EventExternalText,
		ToolName:       "mcp__github__add_issue_comment",
		SessionID:      "humanizer-test",
		TranscriptPath: path,
	}

	got := h.Check(in)
	if got == nil {
		t.Fatal("Check on an external write = nil, want the humanizer nudge")
	}
	if !strings.Contains(got.Text, "humanizer_detect") {
		t.Errorf("advice %q does not name humanizer_detect", got.Text)
	}
	if again := h.Check(in); again != nil {
		t.Errorf("second Check = %+v, want nil: the nudge must fire once per session", again)
	}
}

func TestHumanizerSilentWhenSessionAlreadyLinted(t *testing.T) {
	// A humanizer tool_use block anywhere in the transcript means the session
	// already lints its own prose and does not need telling.
	path := humanizerTranscript(
		t,
		`{"message":{"content":[{"type":"tool_use","name":"mcp__humanizer__humanizer_detect"}]}}`+"\n",
	)
	h := NewHumanizer(testConfig())
	in := Input{
		Event:          EventExternalText,
		ToolName:       "mcp__slack__slack_send_message_draft",
		SessionID:      "linted",
		TranscriptPath: path,
	}
	if got := h.Check(in); got != nil {
		t.Errorf("Check after a humanizer call = %+v, want nil", got)
	}
}

func TestHumanizerCountsToolUseNotProse(t *testing.T) {
	// The advice text itself names humanizer_detect and lands in the
	// transcript verbatim; only the tool_use JSON key form may suppress it.
	path := humanizerTranscript(
		t,
		`{"message":{"content":[{"type":"text","text":"run humanizer_detect on it"}]}}`+"\n",
	)
	h := NewHumanizer(testConfig())
	in := Input{
		Event:          EventExternalText,
		ToolName:       "mcp__tracker__save_comment",
		SessionID:      "prose",
		TranscriptPath: path,
	}
	if got := h.Check(in); got == nil {
		t.Error("Check with only a prose mention = nil, want the nudge")
	}
}

func TestHumanizerSilentWithoutSessionID(t *testing.T) {
	path := humanizerTranscript(t, "")
	h := NewHumanizer(testConfig())
	in := Input{
		Event:          EventExternalText,
		ToolName:       "mcp__github__add_issue_comment",
		TranscriptPath: path,
	}
	if got := h.Check(in); got != nil {
		t.Errorf(
			"Check without a session id = %+v, want nil: once-per-session is unenforceable",
			got,
		)
	}
}

func TestHumanizerNudgesWhenTranscriptMissing(t *testing.T) {
	// An unreadable transcript must not silence the hint: the nudge is cheap
	// and a missing file says nothing about whether the session linted.
	t.Setenv("HOME", t.TempDir())
	h := NewHumanizer(testConfig())
	in := Input{
		Event:          EventExternalText,
		ToolName:       "mcp__github__add_issue_comment",
		SessionID:      "gone",
		TranscriptPath: "/nonexistent/t.jsonl",
	}
	if got := h.Check(in); got == nil {
		t.Error("Check with a missing transcript = nil, want the nudge")
	}
}

func TestHumanizerAdviceQuotesPublishedText(t *testing.T) {
	path := humanizerTranscript(t, "")
	h := NewHumanizer(testConfig())
	in := Input{
		Event: EventExternalText, ToolName: "mcp__github__add_issue_comment",
		SessionID: "quoted", TranscriptPath: path,
		ToolInput: map[string]any{"body": "This PR description delves into the changes."},
	}
	got := h.Check(in)
	if got == nil {
		t.Fatal("Check on an external write = nil, want the humanizer nudge")
	}
	if !strings.Contains(got.Text, `"This PR description delves into the changes."`) {
		t.Errorf("advice %q does not quote the published text", got.Text)
	}
	if !strings.Contains(got.Text, "mcp__github__add_issue_comment") {
		t.Errorf("advice %q does not name the publishing tool", got.Text)
	}
}

func TestPublishedText(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{name: "nil input", input: nil, want: ""},
		{name: "body key wins over a longer identifier", input: map[string]any{
			"url":  "https://example.com/a/very/long/identifier/path/that/is/not/prose",
			"body": "short note",
		}, want: "short note"},
		{name: "longest body key wins", input: map[string]any{
			"comment": "longer of the two bodies",
			"text":    "short",
		}, want: "longer of the two bodies"},
		{name: "fallback walks nested values", input: map[string]any{
			"fields": map[string]any{"inner": []any{"the nested prose that was sent"}},
		}, want: "the nested prose that was sent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publishedText(tt.input); got != tt.want {
				t.Errorf("publishedText(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("short", 200); got != "short" {
		t.Errorf("truncateRunes(short) = %q, want unchanged", got)
	}
	long := strings.Repeat("ä", 150) // 300 bytes, so the cut lands mid-rune
	got := truncateRunes(long, 200)
	if !utf8.ValidString(got) {
		t.Errorf("truncateRunes cut mid-rune: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated text %q missing ellipsis", got)
	}
}

func TestPublishesText(t *testing.T) {
	tests := []struct {
		tool string
		want bool
	}{
		{"mcp__github__add_issue_comment", true},
		{"mcp__github__create_pull_request", true},
		{"mcp__github__add_comment_to_pending_review", true},
		{"mcp__slack__slack_send_message_draft", true},
		{"mcp__tracker__save_issue", true},
		{"mcp__tracker__save_comment", true},
		// Reads reach the hint whenever the matcher names a whole server.
		{"mcp__github__get_file_contents", false},
		{"mcp__github__list_issues", false},
		{"mcp__github__search_code", false},
		{"mcp__tracker__list_comments", false},
		// A trailing verb reads as clearly as a leading one.
		{"mcp__github__issue_read", false},
		{"mcp__github__issue_write", true},
		// Non-MCP tools are matcher mistakes, not external writes.
		{"Bash", false},
		{"Write", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := publishesText(tt.tool); got != tt.want {
			t.Errorf("publishesText(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}
