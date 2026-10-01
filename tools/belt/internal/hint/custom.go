package hint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// pipeGrace bounds how long a run waits for the external's stdout pipe to
// close after the process itself has exited or been killed.
const pipeGrace = 100 * time.Millisecond

// warnGrace bounds how long a silent outcome's warn event may delay the
// hint's return. The POST keeps going in the background past it, so a
// healthy events service gets the event and a hung one costs the session
// this much and no more. Hints run serially on a session-start hook, which
// is why the wait cannot be the emitter's own one-second budget.
const warnGrace = 50 * time.Millisecond

// Custom is a config-registered hint that shells out to an external command
// instead of a compiled-in rule. The external receives the hook payload
// fields as JSON on stdin and answers on stdout: its output, trailing
// whitespace trimmed, is the advice, rendered with the usual belt[<name>]:
// prefix. A non-zero exit, a run past the budget, a start failure, or empty
// stdout all end in silence plus a warn event, so a broken external costs a
// session nothing but the budget. The commands run with the user's full
// environment on purpose: they are user-configured, not agent-configured,
// and belt never writes its own config.
//
// It exists so machine-private context (a daily journal, say) can ride the
// session-start hook under belt's toggles, budget, and doctor report rather
// than as a second SessionStart entry outside belt, and without a compiled
// hint landing in a public-bound repo.
type Custom struct {
	id   string
	cfg  config.CustomHint
	full config.Config // for exclude_repos, which may also sit on the hints: toggle
	// resolveRepo resolves a directory to its canonical host/owner/repo
	// identity for exclude_repos matching; overridable for tests.
	resolveRepo func(dir string) string
	// emit sends fail-silent warnings to the events service. It runs off
	// the hint's goroutine (see warn), so it may block for its own budget
	// without holding the session. Injectable for tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewCustom builds one named external hint from its config entry.
func NewCustom(id string, cfg config.Config) *Custom {
	return &Custom{
		id:          id,
		cfg:         cfg.CustomHints[id],
		full:        cfg,
		resolveRepo: guard.CanonicalRepoAt,
		emit:        notify.EmitEventSync,
	}
}

func (c *Custom) ID() string    { return c.id }
func (c *Custom) Event() string { return c.cfg.Event }

// Config exposes the hint's config entry for introspection (belt doctor
// reports the command, budget, and exclusions).
func (c *Custom) Config() config.CustomHint { return c.cfg }

// Check runs the external command against the input. Exclusion is checked
// before anything is exec'd, so an opted-out repo costs no process launch.
func (c *Custom) Check(in Input) *Advice {
	if len(c.cfg.Command) == 0 {
		return nil // rejected at load; kept so a hand-built config cannot panic
	}
	if c.excluded(in) {
		return nil
	}
	payload, err := json.Marshal(c.payload(in))
	if err != nil {
		return nil
	}

	timeout := c.cfg.Timeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := c.command(ctx, payload)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err = cmd.Run()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		c.warn(fmt.Sprintf("external command timed out after %s — staying silent", timeout))
		return nil
	case errors.Is(err, exec.ErrWaitDelay):
		// The external exited 0 but a child it left behind still held
		// stdout past the grace period. What it printed before exiting is
		// its answer; the straggler is its own business.
	case err != nil:
		c.warn(fmt.Sprintf("external command failed (%v) — staying silent", err))
		return nil
	}

	// Trailing whitespace only: the prefix is prepended on its own line
	// start, so leading indentation on line one is the external's layout
	// to keep, while trailing newlines would only pad the block.
	text := strings.TrimRightFunc(stdout.String(), unicode.IsSpace)
	if text == "" {
		c.warn("external command exited 0 with empty stdout — staying silent")
		return nil
	}
	return &Advice{Hint: c.id, Text: text}
}

// command builds the external's exec.Cmd with the payload on stdin and the
// process lifetime bounded by ctx. The external runs in its own process
// group and the whole group is killed on timeout: killing only the direct
// child would leave a hung grandchild (a curl inside the script, say) to
// outlive the hook on every session start. Killing the group also closes
// the stdout pipe those grandchildren would otherwise hold open, so the
// WaitDelay grace is for stragglers outside the group only.
func (c *Custom) command(ctx context.Context, payload []byte) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.cfg.Command[0], c.cfg.Command[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = pipeGrace
	return cmd
}

// excluded reports whether the session's repo is opted out of this hint.
// Resolution runs only when there is a list to match, since it execs git.
func (c *Custom) excluded(in Input) bool {
	if len(c.cfg.ExcludeRepos)+len(c.full.Hints[c.id].ExcludeRepos) == 0 {
		return false
	}
	return c.full.HintRepoExcluded(c.id, c.resolveRepo(nearestDir(in.Cwd)))
}

// payload is the JSON object the external reads on stdin: the session
// fields belt extracted from the hook payload, plus the event name so one
// script can serve several events once more are supported.
func (c *Custom) payload(in Input) map[string]string {
	return map[string]string{
		"event":           in.Event,
		"cwd":             in.Cwd,
		"session_id":      in.SessionID,
		"transcript_path": in.TranscriptPath,
	}
}

// warn records a silent outcome so a broken external is visible in the
// events log instead of indistinguishable from a hint with nothing to say.
// The emit runs on its own goroutine and the hint waits for it only up to
// warnGrace: long enough for a local events service that answers, never
// long enough for one that hangs to eat the session-start budget. A POST
// still in flight when the hook process exits is lost, which is the same
// outcome a hung service produces by itself.
func (c *Custom) warn(msg string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.emit("belt", "warn", "custom hint "+c.id+" error", msg,
			map[string]string{"hint": c.id, "event": c.cfg.Event})
	}()
	select {
	case <-done:
	case <-time.After(warnGrace):
	}
}

// Timeout is the budget one run gets, for the doctor line.
func (c *Custom) Timeout() time.Duration { return c.cfg.Timeout() }
