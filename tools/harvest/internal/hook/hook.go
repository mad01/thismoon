// Package hook is the SessionEnd hook. It reads the payload Claude Code
// sends, decides whether the session should be harvested, and starts
// `harvest run` detached. SessionEnd hooks share a budget of about a second
// and a half, so this path reads one small payload, checks one file and
// forks; the digest and the model call happen in the detached run.
//
// A hook must never fail a session's end, so every guard returns nil and the
// caller exits 0 whatever Handle returns.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/mad01/thismoon/tools/harvest/internal/settings"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// Payload is the part of the SessionEnd payload harvest reads.
type Payload struct {
	TranscriptPath string `json:"transcript_path"`
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	AgentID        string `json:"agent_id"`
}

// Config is what Handle needs.
type Config struct {
	Getenv func(string) string
	Now    func() time.Time
	// Start runs `harvest <args...>` and returns without waiting for it.
	// Detached is the real one.
	Start func(args []string) error
}

// Handle runs the hook over the payload on stdin. A skip it can explain goes
// to the log; the returned error is a log write or a start that failed, for
// the caller to print.
func Handle(cfg Config, stdin io.Reader) error {
	// The harvest's own child is a session too, and its end must not
	// harvest it.
	if cfg.Getenv(settings.EnvChild) != "" || cfg.Getenv(settings.EnvOff) != "" {
		return nil
	}
	dir, err := settings.StateDir(cfg.Getenv)
	if err != nil {
		return err
	}
	st := state.New(dir, cfg.Now)
	if err := st.Ensure(); err != nil {
		return err
	}

	raw, err := io.ReadAll(stdin)
	if err != nil {
		return st.Log(fmt.Sprintf("skip: read the payload: %v", err))
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return st.Log(fmt.Sprintf("skip: the payload is not JSON: %v", err))
	}
	// A subagent's end is not a session's end.
	if p.AgentID != "" {
		return nil
	}
	if p.SessionID == "" || !isFile(p.TranscriptPath) {
		return st.Log(fmt.Sprintf("skip: no transcript for session '%s'", p.SessionID))
	}
	if err := cfg.Start([]string{"run", p.TranscriptPath, p.SessionID, p.Cwd}); err != nil {
		if logErr := st.Log(fmt.Sprintf("fail %s: %v", p.SessionID, err)); logErr != nil {
			return logErr
		}
		return err
	}
	return nil
}

// Detached returns a Start func that runs exe with args and does not wait.
// The run gets a session of its own (setsid), so it outlives the hook and
// the Claude Code process that called it. Its stdin, stdout and stderr are
// the null device: it holds none of the hook's pipes open, so the hook is
// done the moment it returns.
func Detached(exe string) func(args []string) error {
	return func(args []string) error {
		cmd := exec.Command(exe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("hook: start %s: %w", exe, err)
		}
		if err := cmd.Process.Release(); err != nil {
			return fmt.Errorf("hook: release the run: %w", err)
		}
		return nil
	}
}

// isFile reports whether path names a regular file, as `test -f` does.
func isFile(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
