package trace

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb"
)

func TestPromptLine(t *testing.T) {
	long := strings.Repeat("ab ", 60)
	cases := []struct{ name, prompt, want string }{
		{"first line", "fix the bug\nand more", "fix the bug"},
		{"blank lines first", "\n  \n  fix it  \n", "fix it"},
		{
			"tag line skipped",
			"<teammate-message teammate_id=\"lead\" summary=\"x\">\nbuild it\n",
			"build it",
		},
		{"inline tags dropped", "<command-name>/review</command-name> now", "/review now"},
		{"spaces collapsed", "a \t  b", "a b"},
		{"only tags", "<a>\n</a>\n", ""},
		{
			"long",
			long,
			string([]rune(strings.TrimSpace(long))[:moondb.SessionPromptLineRunes]) + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PromptLine(c.prompt); got != c.want {
				t.Errorf("PromptLine(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}

// TestFormatTotalsCost: totals with no cost read as no cost when nothing
// was billed, and as unknown when a model has no price.
func TestFormatTotalsCost(t *testing.T) {
	cases := []struct {
		name string
		t    Totals
		want string
	}{
		{"nothing billed", Totals{}, "-"},
		{"unpriced", Totals{Unpriced: []string{"m"}}, "unknown"},
		{"priced", Totals{Cost: new(1.5)}, "$1.50"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatTotalsCost(c.t); got != c.want {
				t.Errorf("FormatTotalsCost = %q, want %q", got, c.want)
			}
		})
	}
}

// TestPromptLabel names the prompts the harness writes by what started the
// turn, from their real shapes, and a person's prompt by its first line.
func TestPromptLabel(t *testing.T) {
	cases := []struct{ name, prompt, want string }{
		{"person", "fix the flaky test\nin ingest", "fix the flaky test"},
		{
			"task", "<task-notification>\n<task-id>bjgyw2ry2</task-id>\n" +
				"<tool-use-id>toolu_01A</tool-use-id>\n<status>completed</status>\n" +
				"<summary>Background command \"Wait for CI\" completed (exit code 0)</summary>\n" +
				"</task-notification>",
			"background task finished: Background command \"Wait for CI\" completed (exit code 0)",
		},
		{
			"task without summary", "<task-notification>\n<task-id>b1</task-id>\n</task-notification>",
			"background task finished",
		},
		{
			"teammate via another session", "Another Claude session sent a message:\n" +
				"<teammate-message teammate_id=\"capture-builder\" color=\"green\" " +
				"summary=\"Capture PR ready\">\nBuilt it.\n</teammate-message>",
			"teammate message from capture-builder: Capture PR ready",
		},
		{"teammate without summary", "<teammate-message teammate_id=\"team-lead\">\nGo.\n" +
			"</teammate-message>", "teammate message from team-lead"},
		{
			"subagent message", "<agent-message from=\"worker [3fa9c1]\">\nready\n</agent-message>",
			"message from subagent worker [3fa9c1]",
		},
		{
			"subagent via another session", "Another Claude session sent a message:\n" +
				"<agent-message from=\"worker\">\nDone.\n</agent-message>",
			"message from subagent worker",
		},
		{
			"subagent without sender", "<agent-message>\nready\n</agent-message>",
			"message from subagent",
		},
		{
			"command",
			"<command-name>/compact</command-name>\n<command-args></command-args>",
			"/compact",
		},
		{"continuation", "This session is being continued from a previous conversation that ran " +
			"out of context.", "continued after compaction"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PromptLabel(c.prompt); got != c.want {
				t.Errorf("PromptLabel = %q, want %q", got, c.want)
			}
		})
	}
}
