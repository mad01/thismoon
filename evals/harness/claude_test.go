package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestInvocationArgs(t *testing.T) {
	inv := invocation{
		SessionID: "sid", MaxTurns: 8, Model: "haiku", Effort: "low", PermissionMode: "acceptEdits",
		DisallowedTools: []string{"Edit", "Bash(git push:*)"}, MaxBudgetUSD: 1.5,
		Prompt: "-starts with a dash",
	}
	got := strings.Join(inv.args(), " ")
	want := "-p --output-format stream-json --verbose --session-id sid " +
		"--max-turns 8 --model haiku --effort low --permission-mode acceptEdits --max-budget-usd 1.5 " +
		"--disallowedTools Edit Bash(git push:*) -- -starts with a dash"
	if got != want {
		t.Errorf("args =\n%s\nwant\n%s", got, want)
	}

	bare := invocation{SessionID: "sid", MaxTurns: 3, Prompt: "hi"}.args()
	for _, flag := range []string{"--model", "--effort", "--permission-mode", "--max-budget-usd", "--disallowedTools"} {
		if slices.Contains(bare, flag) {
			t.Errorf("unset option still passes %s: %v", flag, bare)
		}
	}
	if bare[len(bare)-2] != "--" || bare[len(bare)-1] != "hi" {
		t.Errorf("prompt is not last after --: %v", bare)
	}
}

func TestClaudeEnv(t *testing.T) {
	got := claudeEnv([]string{
		"HOME=/home/x", "CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=abc",
		"CLAUDE_CODE_MESSAGING_SOCKET=/tmp/s", "ANTHROPIC_API_KEY=k", "PATH=/bin",
		"CLAUDE_CODE_EFFORT_LEVEL=max", "CLAUDE_EFFORT=high", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1",
	})
	want := []string{"HOME=/home/x", "ANTHROPIC_API_KEY=k", "PATH=/bin"}
	if !slices.Equal(got, want) {
		t.Errorf("env = %v, want %v", got, want)
	}
}

func TestParseJudgeOutput(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		wantPass bool
		wantErr  bool
	}{
		{
			"plain", `{"result":"{\"pass\": true, \"reason\": \"ok\"}","total_cost_usd":0.001}`,
			true, false,
		},
		{
			"fenced", "{\"result\":\"```json\\n{\\\"pass\\\": false, \\\"reason\\\": \\\"no\\\"}\\n```\"}",
			false, false,
		},
		{"no object", `{"result":"I think it passes"}`, false, true},
		{"errored", `{"result":"overloaded","is_error":true}`, false, true},
		{"not json", `garbage`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseJudgeOutput([]byte(tt.out))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if v.Pass != tt.wantPass {
				t.Errorf("pass = %v, want %v", v.Pass, tt.wantPass)
			}
		})
	}
	v, _ := parseJudgeOutput([]byte(tests[0].out))
	if v.CostUSD != 0.001 || v.Reason != "ok" {
		t.Errorf("verdict = %+v", v)
	}
}

func TestFetchScorecard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sessions/known/scorecard" {
			_, _ = w.Write([]byte(`{"turns":3}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	ctx := context.Background()

	body, status := fetchScorecard(ctx, srv.Client(), srv.URL, "known")
	if status != "ok" || string(body) != `{"turns":3}` {
		t.Errorf("known session: %q %s", status, body)
	}
	if body, status := fetchScorecard(ctx, srv.Client(), srv.URL, "missing"); status != scorecardUnavailable ||
		body != nil {
		t.Errorf("missing session: %q %s", status, body)
	}
	if _, status := fetchScorecard(ctx, srv.Client(), "", "known"); status != scorecardUnavailable {
		t.Errorf("no base URL: %q", status)
	}
}

func TestFlagTemplate(t *testing.T) {
	got := strings.Join(flagTemplate("", "high", 2), " ")
	for _, want := range []string{
		"--session-id <uuid>", "--max-turns <case max_turns>", "--effort high",
		"--max-budget-usd 2", "-- <case prompt>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("template %q missing %q", got, want)
		}
	}
}
