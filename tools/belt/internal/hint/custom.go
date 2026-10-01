package hint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// pipeGrace bounds how long a run waits for the external's stdout pipe to
// close after the process itself has exited or been killed.
const pipeGrace = 100 * time.Millisecond

// Custom is a config-registered hint that shells out to an external command
// instead of a compiled-in rule. The external receives the hook payload
// fields as JSON on stdin and answers on stdout: its trimmed output is the
// advice, rendered with the usual belt[<name>]: prefix. A non-zero exit, a
// run past the budget, a start failure, or empty stdout all end in silence
// plus a warn event, so a broken external costs a session nothing but the
// budget. The commands run with the user's full environment on purpose:
// they are user-configured, not agent-configured, and belt never writes its
// own config.
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
	// emit sends fail-silent warnings to the events service. Injectable for
	// tests.
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
	cmd := exec.CommandContext(ctx, c.cfg.Command[0], c.cfg.Command[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// Killing the external on deadline does not close stdout if a child it
	// spawned still holds the pipe; without a grace period Run would wait
	// for that child and the budget would mean nothing.
	cmd.WaitDelay = pipeGrace

	err = cmd.Run()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		c.warn(fmt.Sprintf("external command timed out after %s — staying silent", timeout))
		return nil
	case err != nil:
		c.warn(fmt.Sprintf("external command failed (%v) — staying silent", err))
		return nil
	}

	// Whitespace at the ends only: the external owns its line structure, and
	// a multi-line journal is the expected shape. Trailing newlines would
	// pad the block, and leading blank lines or indentation on line one
	// would read as part of the prefix, so both ends go.
	text := strings.TrimSpace(stdout.String())
	if text == "" {
		c.warn("external command exited 0 with empty stdout — staying silent")
		return nil
	}
	return &Advice{Hint: c.id, Text: text}
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
func (c *Custom) warn(msg string) {
	c.emit("belt", "warn", "custom hint "+c.id+" error", msg,
		map[string]string{"hint": c.id, "event": c.cfg.Event})
}

// Timeout is the budget one run gets, for the doctor line.
func (c *Custom) Timeout() time.Duration { return c.cfg.Timeout() }
