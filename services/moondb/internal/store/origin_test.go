package store

import "testing"

// TestPromptOrigin classifies the prompt heads the harness writes, as they
// appear in real transcripts, and a person's prompts around them.
func TestPromptOrigin(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   TurnOrigin
	}{
		{"typed", "fix the flaky test in ingest", OriginPerson},
		{"slash command", "<command-name>/compact</command-name>", OriginPerson},
		{"task notification", "<task-notification>\n<task-id>bjgyw2ry2</task-id>\n" +
			"<tool-use-id>toolu_01AdvBsYMyMf66TR8wAp3GCr</tool-use-id>\n<status>completed</status>\n" +
			"<summary>Background command \"Wait for CI\" completed (exit code 0)</summary>\n" +
			"</task-notification>", OriginTask},
		{"teammate via another session", "Another Claude session sent a message:\n" +
			"<teammate-message teammate_id=\"capture-builder\" color=\"green\" summary=\"done\">\n" +
			"Built it.\n</teammate-message>", OriginTeammate},
		{"teammate message", "<teammate-message teammate_id=\"team-lead\" summary=\"Build it\">\n" +
			"Build the PR.\n</teammate-message>", OriginTeammate},
		{
			"agent message", "<agent-message from=\"worker [3fa9c1]\">\nready\n</agent-message>",
			OriginAgent,
		},
		{"cross-session message", "<cross-session-message from=\"worker\">\nhi\n" +
			"</cross-session-message>", OriginAgent},
		{"continuation", "This session is being continued from a previous conversation that " +
			"ran out of context. The summary below covers the earlier portion.", OriginContinuation},
		{"leading space", "  \n<task-notification>\n</task-notification>", OriginTask},
		{"empty", "", OriginNone},
		{"blank", " \n\t", OriginNone},
		{"quoted head later", "why does <task-notification> show up as a prompt?", OriginPerson},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PromptOrigin(c.prompt); got != c.want {
				t.Errorf("PromptOrigin(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}
