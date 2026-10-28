package hook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunHintSessionStartListsInflightFiles feeds `belt hint session-start`
// the raw SessionStart payload Claude Code sends, from a checkout holding an
// untracked file a fake moondb names, and expects the moondb-inflight block
// in the SessionStart envelope. HOME and the events service point away from
// the machine so the other session-start hints stay silent.
func TestRunHintSessionStartListsInflightFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("EVENTS_BASE_URL", "http://127.0.0.1:1")
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(dir) // git names the checkout by its real path
	if err != nil {
		t.Fatal(err)
	}

	var asked url.Values
	moondb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		at := time.Now().Add(-5 * time.Minute)
		_ = json.NewEncoder(w).Encode(map[string]any{"worktree": root, "files": []any{
			map[string]any{
				"path": "a.go", "last_edit_at": at, "linked_commit": nil,
				"sessions": []any{map[string]any{
					"id": "0f1e2d3c-other", "title": "fix the parser", "last_edit_at": at, "edits": 2,
				}},
			},
		}})
	}))
	t.Cleanup(moondb.Close)
	u, err := url.Parse(moondb.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOONDB_PORT", u.Port())

	cwd, err := json.Marshal(dir)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"session_id":"s1","transcript_path":"/nowhere.jsonl","cwd":` + string(cwd) +
		`,"hook_event_name":"SessionStart","source":"startup"}`
	var out bytes.Buffer
	RunHint("session-start", defaults, strings.NewReader(payload), &out)

	var a adviceDecision
	if err := json.Unmarshal(out.Bytes(), &a); err != nil {
		t.Fatalf("decode advice from %q: %v", out.String(), err)
	}
	if a.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want SessionStart", a.HookSpecificOutput.HookEventName)
	}
	want := "belt[moondb-inflight]: 1 file in this checkout has uncommitted edits by another session:\n" +
		"  a.go: \"fix the parser\" (0f1e2d3c), 5m ago\n"
	if got := a.HookSpecificOutput.AdditionalContext; !strings.Contains(got, want) {
		t.Errorf("additionalContext = %q, want it to contain %q", got, want)
	}
	if asked.Get("worktree") != root || asked.Get("exclude_session") != "s1" {
		t.Errorf("moondb was asked %v, want worktree %s and exclude_session s1", asked, root)
	}
}
