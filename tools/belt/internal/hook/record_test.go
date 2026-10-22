package hook

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/ledger"
)

// recordOne runs the record path with the real builder on one payload and
// returns the single entry it posted.
func recordOne(t *testing.T, load Load, event string, payload map[string]string) notify.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var posted []notify.Event
	rc := recorder{
		load:  load,
		build: buildEntry,
		emit:  func(ev notify.Event) { posted = append(posted, ev) },
	}
	rc.run(event, strings.NewReader(string(raw)))
	if len(posted) != 1 {
		t.Fatalf("posted %d entries, want 1", len(posted))
	}
	return posted[0]
}

// TestRecordTagsDeclaredAgentTypes: ledger.agent_types from the config in
// effect reaches the subagent-start entry through the recorder. A declared
// type is tagged with its model and effort; an unknown type is not.
func TestRecordTagsDeclaredAgentTypes(t *testing.T) {
	declared := config.ResolveAgentTypes(map[string]any{
		"reviewer-deep*": map[string]any{"model": "opus", "effort": "high"},
	})
	load := func() (config.Config, error) {
		return config.Config{Ledger: config.Ledger{AgentTypes: declared}}, nil
	}
	tests := []struct {
		name, agentType, wantModel, wantEffort string
	}{
		{"declared type", "reviewer-deep", "opus", "high"},
		{"named worker", "reviewer-deep-csl-prune", "opus", "high"},
		{"unknown type", "Explore", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := recordOne(t, load, ledger.EventSubagentStart, map[string]string{
				"session_id": "s1", "agent_id": "a1", "agent_type": tt.agentType,
			})
			if ev.Tags["model"] != tt.wantModel || ev.Tags["effort"] != tt.wantEffort {
				t.Errorf("tags = %v, want model %q and effort %q",
					ev.Tags, tt.wantModel, tt.wantEffort)
			}
		})
	}
}

// TestRecordStopFailure feeds the StopFailure payload the Claude Code
// hooks docs show through the recorder and checks the entry that posts.
// The event argument decides what is recorded, as for every record event:
// a payload naming another hook, or none, still records a failed turn.
func TestRecordStopFailure(t *testing.T) {
	tests := []struct {
		name        string
		payload     map[string]string
		wantLevel   string
		wantTitle   string
		wantMessage string
		wantTags    map[string]string
	}{
		{
			name: "the documented payload",
			payload: map[string]string{
				"session_id":             "abc123",
				"transcript_path":        "/t/session.jsonl",
				"hook_event_name":        "StopFailure",
				"error":                  "rate_limit",
				"error_details":          "429 Too Many Requests",
				"last_assistant_message": "API Error: Rate limit reached",
			},
			wantLevel:   "warn",
			wantTitle:   "turn failed (rate_limit)",
			wantMessage: "API Error: Rate limit reached (429 Too Many Requests)",
			wantTags: map[string]string{
				"event":      "stop-failure",
				"session_id": "abc123",
				"error":      "rate_limit",
				"transcript": "/t/session.jsonl",
			},
		},
		{
			name:        "another hook's payload is still recorded",
			payload:     map[string]string{"hook_event_name": "Stop"},
			wantLevel:   "error",
			wantTitle:   "turn failed (unknown)",
			wantMessage: "No detail reported.",
			wantTags:    map[string]string{"event": "stop-failure", "error": "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := recordOne(t, defaults, ledger.EventStopFailure, tt.payload)
			if ev.Source != ledger.Source || ev.Level != tt.wantLevel {
				t.Errorf("source, level = %q, %q, want %q, %q",
					ev.Source, ev.Level, ledger.Source, tt.wantLevel)
			}
			if ev.Title != tt.wantTitle || ev.Message != tt.wantMessage {
				t.Errorf("title, message = %q, %q, want %q, %q",
					ev.Title, ev.Message, tt.wantTitle, tt.wantMessage)
			}
			if !maps.Equal(ev.Tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", ev.Tags, tt.wantTags)
			}
		})
	}
}

// TestRecordTagsTheLinkedWorktreeToplevel runs the real builder against a
// real repo with a linked worktree beside it. A payload cwd deep inside the
// worktree is tagged with the worktree's toplevel, never the cwd itself,
// and a cwd in the main checkout gets no worktree tag. Paths are compared
// physically because the temp dir may sit behind a symlink (/var on macOS)
// and git reports resolved paths.
func TestRecordTagsTheLinkedWorktreeToplevel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// Keep the user's and the system's gitconfig out of the fixture: a
	// commit.gpgsign or hooksPath there would break the setup commit.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	canonical := filepath.Join(home, "src", "thismoon")
	worktree := filepath.Join(home, ".worktrees", "thismoon", "feat-x")
	git(t, home, "init", "-q", "-b", "main", canonical)
	git(t, canonical, "-c", "user.name=t", "-c", "user.email=t@example.com",
		"commit", "-q", "--allow-empty", "-m", "init")
	git(t, canonical, "remote", "add", "origin", "git@github.com:mad01/thismoon.git")
	git(t, canonical, "worktree", "add", "-q", "-b", "feat/x", worktree)
	deepInWorktree := mkdirAll(t, filepath.Join(worktree, "tools", "belt"))
	deepInCanonical := mkdirAll(t, filepath.Join(canonical, "tools", "belt"))

	tests := []struct {
		name string
		cwd  string
		want map[string]string // "" means the tag must be absent
	}{
		{"cwd deep in a linked worktree", deepInWorktree, map[string]string{
			"worktree": worktree, "canonical": canonical, "branch": "feat/x",
			"repo": "github.com/mad01/thismoon",
		}},
		{"cwd deep in the main checkout", deepInCanonical, map[string]string{
			"worktree": "", "canonical": canonical, "branch": "main",
			"repo": "github.com/mad01/thismoon",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := recordOne(t, defaults, ledger.EventSubagentStart, map[string]string{
				"session_id": "s1", "agent_id": "a1", "agent_type": "Explore", "cwd": tt.cwd,
			})
			for key, want := range tt.want {
				got, set := ev.Tags[key]
				if got != want || set != (want != "") {
					t.Errorf("tag %s = %q (set %v), want %q", key, got, set, want)
				}
			}
		})
	}
}

// git runs one git command in dir and fails the test on error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// mkdirAll creates dir and its parents and returns it.
func mkdirAll(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
