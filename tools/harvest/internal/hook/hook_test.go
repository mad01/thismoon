package hook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var fixed = time.Date(2026, 11, 14, 9, 30, 0, 0, time.Local)

func TestHandle(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := func(session, extra string) string {
		return fmt.Sprintf(`{"session_id":%q,"transcript_path":%q,"cwd":"/repo"%s}`,
			session, transcript, extra)
	}

	tests := []struct {
		name      string
		env       map[string]string
		payload   string
		wantStart []string // nil: the hook must not start a run
		wantLog   string   // "": the hook must not log
	}{
		{
			name:      "an ended session starts a run",
			payload:   payload("hook-1", ""),
			wantStart: []string{"run", transcript, "hook-1", "/repo"},
		},
		{
			name:    "the harvest's own child",
			env:     map[string]string{"HARVEST_CHILD": "1"},
			payload: payload("hook-child", ""),
		},
		{
			name:    "switched off",
			env:     map[string]string{"HARVEST_OFF": "1"},
			payload: payload("hook-off", ""),
		},
		{
			name:    "a subagent's end",
			payload: payload("hook-agent", `,"agent_id":"a1"`),
		},
		{
			name:    "the transcript is gone",
			payload: `{"session_id":"hook-gone","transcript_path":"/nonexistent.jsonl"}`,
			wantLog: "skip: no transcript for session 'hook-gone'",
		},
		{
			name:    "the transcript is a directory",
			payload: fmt.Sprintf(`{"session_id":"hook-dir","transcript_path":%q}`, dir),
			wantLog: "skip: no transcript for session 'hook-dir'",
		},
		{
			name:    "no session id",
			payload: fmt.Sprintf(`{"transcript_path":%q}`, transcript),
			wantLog: "skip: no transcript for session ''",
		},
		{
			name:    "a payload that is not JSON",
			payload: "not json",
			wantLog: "skip: the payload is not JSON",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := filepath.Join(t.TempDir(), "state")
			env := map[string]string{"HARVEST_STATE_DIR": stateDir}
			for k, v := range tc.env {
				env[k] = v
			}
			var started []string
			cfg := Config{
				Getenv: func(k string) string { return env[k] },
				Now:    func() time.Time { return fixed },
				Start: func(args []string) error {
					started = args
					return nil
				},
			}
			if err := Handle(cfg, strings.NewReader(tc.payload)); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if !slices.Equal(started, tc.wantStart) {
				t.Errorf("started %q, want %q", started, tc.wantStart)
			}
			log, _ := os.ReadFile(filepath.Join(stateDir, "harvest.log"))
			switch {
			case tc.wantLog == "" && len(log) > 0:
				t.Errorf("the hook logged %q, want nothing", log)
			case tc.wantLog != "" && !strings.Contains(string(log), tc.wantLog):
				t.Errorf("log = %q, want it to contain %q", log, tc.wantLog)
			}
		})
	}
}

func TestHandleLogsAFailedStart(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	transcript := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	startErr := errors.New("fork refused")
	cfg := Config{
		Getenv: func(k string) string {
			return map[string]string{"HARVEST_STATE_DIR": stateDir}[k]
		},
		Now:   func() time.Time { return fixed },
		Start: func([]string) error { return startErr },
	}
	payload := fmt.Sprintf(`{"session_id":"s1","transcript_path":%q}`, transcript)
	if err := Handle(cfg, strings.NewReader(payload)); !errors.Is(err, startErr) {
		t.Fatalf("Handle() error = %v, want %v", err, startErr)
	}
	log, _ := os.ReadFile(filepath.Join(stateDir, "harvest.log"))
	if !strings.Contains(string(log), "fail s1: fork refused") {
		t.Errorf("log = %q, want the fail line", log)
	}
}

// TestDetached checks the contract the hook budget depends on: Start returns
// while the run is still going, and the run sits in a session of its own.
func TestDetached(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	doneFile := filepath.Join(dir, "done")
	script := fmt.Sprintf("echo $$ > %s.tmp && mv %s.tmp %s; sleep 1; : > %s",
		pidFile, pidFile, pidFile, doneFile)

	if err := Detached("/bin/sh")([]string{"-c", script}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := os.Stat(doneFile); err == nil {
		t.Fatal("Start() waited for the run to finish")
	}

	pid := waitForPID(t, pidFile)
	sid, err := syscall.Getsid(pid)
	if err != nil {
		t.Fatalf("Getsid(run) error = %v", err)
	}
	own, err := syscall.Getsid(0)
	if err != nil {
		t.Fatal(err)
	}
	if sid == own {
		t.Errorf("the run shares the caller's session %d", own)
	}
	if sid != pid {
		t.Errorf("the run's session is %d, want it to lead its own (%d)", sid, pid)
	}
}

func TestDetachedMissingBinary(t *testing.T) {
	err := Detached(filepath.Join(t.TempDir(), "no-such-harvest"))([]string{"run"})
	if err == nil {
		t.Fatal("Start() of a missing binary returned nil")
	}
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err != nil {
				t.Fatalf("pid file holds %q", b)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run never wrote its pid")
	return 0
}
