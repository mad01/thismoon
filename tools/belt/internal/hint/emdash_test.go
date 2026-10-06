package hint

import (
	"os"
	"strings"
	"testing"
)

// dash is U+2014, escaped so the test source carries no literal em dash.
const dash = "\u2014"

func TestEmDashExternalText(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  int // em dashes reported; 0 means silence
	}{
		{
			name:  "pull request body",
			tool:  "mcp__gh_com__create_pull_request",
			input: map[string]any{"owner": "o", "repo": "r", "title": "t", "body": "a " + dash + " b"},
			want:  1,
		},
		{
			name: "every string counts, nested ones too",
			tool: "mcp__gh_com__create_pull_request",
			input: map[string]any{
				"title": "x" + dash + "y",
				"body":  dash + dash,
				"meta":  map[string]any{"labels": []any{"a" + dash}},
			},
			want: 4,
		},
		{
			name:  "slack message",
			tool:  "mcp__slack__slack_send_message_draft",
			input: map[string]any{"text": "done " + dash + " shipped"},
			want:  1,
		},
		{
			name:  "clean text is silent",
			tool:  "mcp__gh_com__add_issue_comment",
			input: map[string]any{"body": "plain - hyphen and – en dash"},
		},
		{
			name:  "read verb is skipped",
			tool:  "mcp__gh_com__pull_request_read",
			input: map[string]any{"body": dash},
		},
		{
			name:  "non-MCP tool is skipped",
			tool:  "Bash",
			input: map[string]any{"command": dash},
		},
	}
	h := NewEmDash(testConfig(), EventExternalText)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.Check(Input{Event: EventExternalText, ToolName: tt.tool, ToolInput: tt.input})
			checkEmDashAdvice(t, got, tt.want)
		})
	}
}

func TestEmDashBash(t *testing.T) {
	files := map[string]string{
		"/repo/body.md":  "intro " + dash + " details " + dash,
		"/abs/clean.md":  "nothing to see",
		"/other/note.md": dash,
	}
	tests := []struct {
		name    string
		command string
		want    int
	}{
		{"pr create body flag", `gh pr create --title t --body "a ` + dash + ` b"`, 1},
		{"pr edit heredoc spans segments", "gh pr edit 1 --body \"$(cat <<'EOF'\nline " + dash + " one\nEOF\n)\"", 1},
		{"pr comment", `gh pr comment 3 -b "x` + dash + `"`, 1},
		{"issue create", `gh issue create -t "t` + dash + `" -b b`, 1},
		{"issue edit", `gh issue edit 4 --body "` + dash + dash + `"`, 2},
		{"issue comment", `gh issue comment 4 --body "` + dash + `"`, 1},
		{"body file relative to cwd", "gh pr create --title t --body-file body.md", 2},
		{"body file -F", "gh issue comment 4 -F body.md", 2},
		{"body file after cd", "cd /other && gh pr edit 1 --body-file note.md", 1},
		{"clean body file", "gh pr create --body-file /abs/clean.md", 0},
		{"missing body file", "gh pr create --body-file nope.md", 0},
		{"api with fields", `gh api repos/o/r/issues/1/comments -f body="` + dash + `"`, 1},
		{"api with field file", "gh api repos/o/r/issues/1/comments -F body=@body.md", 2},
		{"api without fields is a read", `gh api repos/o/r/pulls --jq '.[] | "` + dash + `"'`, 0},
		{"pr view is a read", `gh pr view 1 --json body --jq '"` + dash + `"'`, 0},
		{"pr merge is not text", `gh pr merge 1 --squash # ` + dash, 0},
		{"no gh at all", `echo "` + dash + `"`, 0},
		{"clean pr body", `gh pr create --title t --body "colon: fine"`, 0},
	}
	h := NewEmDash(testConfig(), EventBash)
	h.readFile = func(path string) ([]byte, error) {
		if s, ok := files[path]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			checkEmDashAdvice(t, got, tt.want)
		})
	}
}

// TestEmDashNoSessionDedupe pins that every offending publish is flagged:
// the same session publishing twice hears about it twice.
func TestEmDashNoSessionDedupe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	h := NewEmDash(testConfig(), EventExternalText)
	in := Input{
		Event:     EventExternalText,
		ToolName:  "mcp__gh_com__update_pull_request",
		ToolInput: map[string]any{"body": dash},
		SessionID: "dedupe",
	}
	for i := range 2 {
		if h.Check(in) == nil {
			t.Errorf("publish %d: em-dash must fire on every offending publish", i+1)
		}
	}
}

func checkEmDashAdvice(t *testing.T, got *Advice, want int) {
	t.Helper()
	if want == 0 {
		if got != nil {
			t.Fatalf("Check = %q, want silence", got.Text)
		}
		return
	}
	if got == nil {
		t.Fatalf("Check = nil, want advice about %d em dash(es)", want)
	}
	if got.Hint != EmDashID {
		t.Errorf("hint id = %q, want %q", got.Hint, EmDashID)
	}
	noun := " em dashes;"
	if want == 1 {
		noun = " em dash;"
	}
	if !strings.Contains(got.Text, "contains "+string(rune('0'+want))+noun) {
		t.Errorf("advice %q does not report %d em dash(es)", got.Text, want)
	}
	if strings.Contains(got.Text, dash) {
		t.Errorf("advice %q itself carries an em dash", got.Text)
	}
}
