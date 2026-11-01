package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/firstlaunchtest"
)

// fakeClaudeScript stands in for the CLI. It records its arguments, its
// stdin and whether the session variable reached it, then prints the reply
// file or fails with FAKE_EXIT. The --warm argument is for the first-launch
// warm-up only.
const fakeClaudeScript = `#!/bin/sh
[ "$1" = --warm ] && exit 0
printf '%s\n' "$@" > "$FAKE_DIR/args"
cat > "$FAKE_DIR/stdin"
printf '%s' "${CLAUDECODE-unset}" > "$FAKE_DIR/claudecode"
if [ -n "$FAKE_EXIT" ]; then
  echo boom >&2
  exit "$FAKE_EXIT"
fi
cat "$FAKE_DIR/reply"
`

const fakeReply = `{"result":"- ok","is_error":false,"total_cost_usd":0.01,"duration_api_ms":42,` +
	`"usage":{"input_tokens":10,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":5},` +
	`"modelUsage":{"claude-haiku-5-5":{}}}`

// fakeClaude writes the script and the reply it should print, points the
// script's env at its directory, and returns a client on it.
func fakeClaude(t *testing.T, reply string) (*ClaudeClient, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, claudeBin)
	if err := os.WriteFile(bin, []byte(fakeClaudeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reply"), []byte(reply), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DIR", dir)
	t.Setenv("FAKE_EXIT", "")
	t.Setenv("CLAUDECODE", "1")
	firstlaunchtest.Warm(t, bin, []string{"--warm"}, nil)
	return NewClaude(ClaudeConfig{Bin: bin, Effort: "low"}), dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestClaudeComplete(t *testing.T) {
	var seen []CallInfo
	c, dir := fakeClaude(t, fakeReply)
	c.observe = func(ci CallInfo) { seen = append(seen, ci) }

	out, err := c.Complete(context.Background(), "be terse", "the corpus")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if out != "- ok" {
		t.Errorf("Complete() = %q, want the result field", out)
	}
	args := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(dir, "args"))), "\n")
	for _, want := range [][]string{
		{"-p"},
		{"--model", DefaultModel},
		{"--tools", ""},
		{"--setting-sources", ""},
		{"--no-session-persistence"},
		{"--output-format", "json"},
		{"--system-prompt", "be terse"},
		{"--effort", "low"},
	} {
		if !hasArgs(args, want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if got := readFile(t, filepath.Join(dir, "stdin")); got != "the corpus" {
		t.Errorf("stdin = %q, want the user text", got)
	}
	if got := readFile(t, filepath.Join(dir, "claudecode")); got != "unset" {
		t.Errorf("CLAUDECODE reached the CLI as %q, want it stripped", got)
	}
	if len(seen) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(seen))
	}
	ci := seen[0]
	if ci.Model != DefaultModel || ci.PromptTokens != 15 || ci.CompletionTokens != 3 ||
		ci.Status != 200 || ci.CostUSD != 0.01 || ci.Latency != 42*time.Millisecond {
		t.Errorf(
			"CallInfo = %+v, want the served model, 15 in (cache included), 3 out, 42ms, $0.01",
			ci,
		)
	}
}

// hasArgs reports whether want appears in args as a contiguous run.
func hasArgs(args, want []string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestClaudeCompleteErrors(t *testing.T) {
	cases := []struct {
		name       string
		reply      string
		exit       string
		want       string
		wantStatus int
	}{
		{
			name:       "process failure carries the exit code and stderr",
			reply:      fakeReply,
			exit:       "3",
			want:       "exited 3: boom",
			wantStatus: 1,
		},
		{
			name:       "is_error surfaces the result text",
			reply:      `{"result":"rate limited","is_error":true,"usage":{"input_tokens":1}}`,
			want:       "errored: rate limited",
			wantStatus: 200,
		},
		{
			name:       "empty result is no content",
			reply:      `{"result":"  ","is_error":false}`,
			want:       "no content",
			wantStatus: 200,
		},
		{
			name:       "unparseable output",
			reply:      "not json",
			want:       "decode claude -p output",
			wantStatus: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen []CallInfo
			c, _ := fakeClaude(t, tc.reply)
			c.observe = func(ci CallInfo) { seen = append(seen, ci) }
			t.Setenv("FAKE_EXIT", tc.exit)
			_, err := c.Complete(context.Background(), "s", "u")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Complete() error = %v, want containing %q", err, tc.want)
			}
			if len(seen) != 1 || seen[0].Status != tc.wantStatus {
				t.Errorf("observer saw %+v, want one call with status %d", seen, tc.wantStatus)
			}
		})
	}
}

func TestClaudeEnvStripsSessionVariables(t *testing.T) {
	in := []string{
		"PATH=/bin",
		"CLAUDECODE=1",
		"CLAUDE_CODE_EFFORT_LEVEL=high",
		"HOME=/x",
		"CLAUDE_FAKE=keep",
	}
	got := claudeEnv(in)
	want := []string{"PATH=/bin", "HOME=/x", "CLAUDE_FAKE=keep"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("claudeEnv() = %v, want %v", got, want)
	}
}
