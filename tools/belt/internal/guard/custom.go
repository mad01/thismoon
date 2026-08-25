package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/notify"
)

// customTimeout bounds one external guard run. A slow external allows with a
// warn event — a user-configured check must never stall every tool call.
const customTimeout = 5 * time.Second

// Custom is a config-registered guard that shells out to an external command
// instead of a compiled-in rule. The external receives the tool-call fields
// as JSON on stdin and answers with its exit code: 0 allows, 1 denies with
// stdout as the reason, anything else (or a timeout, or a start failure)
// allows with a warn event so a broken external fails open, never closed.
// The commands run with the user's full environment on purpose: they are
// user-configured, not agent-configured.
type Custom struct {
	id  string
	cfg config.CustomGuard
	// timeout bounds one external run. Shortened in tests.
	timeout time.Duration
	// emit sends fail-open warnings and soft-mode denials to the events
	// service. Injectable for tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewCustom builds one named external guard from its config entry.
func NewCustom(id string, cfg config.CustomGuard) *Custom {
	return &Custom{id: id, cfg: cfg, timeout: customTimeout, emit: notify.EmitEvent}
}

func (c *Custom) ID() string    { return c.id }
func (c *Custom) Event() string { return c.cfg.Event }

// Config exposes the guard's config entry for introspection (belt doctor
// reports the command, mode, and match gate).
func (c *Custom) Config() config.CustomGuard { return c.cfg }

// Check runs the external command against the input. The match gate keeps
// the external from being exec'd on every tool call: when set, the command
// (bash) or file path (write) must contain it as a substring.
func (c *Custom) Check(in Input) *Denial {
	if len(c.cfg.Command) == 0 {
		return nil
	}
	if c.cfg.Match != "" && !strings.Contains(c.matchTarget(in), c.cfg.Match) {
		return nil
	}
	payload, err := json.Marshal(c.payload(in))
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.cfg.Command[0], c.cfg.Command[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err = cmd.Run()
	if err == nil {
		return nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		c.warn(fmt.Sprintf("external command timed out after %s — allowing", c.timeout))
		return nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		c.warn(fmt.Sprintf("external command failed (%v) — allowing", err))
		return nil
	}

	reason, _, _ := strings.Cut(strings.TrimSpace(stdout.String()), "\n")
	if reason == "" {
		reason = "denied by external guard (no reason on stdout)"
	}
	if c.cfg.Soft() {
		c.emit("belt", "warn", "custom guard "+c.id+" (soft)",
			reason+" — allowed (soft mode)",
			map[string]string{"guard": c.id, "event": c.cfg.Event})
		return nil
	}
	return Reasonf(c.id, "%s", reason)
}

// matchTarget is what the match substring gates on per event.
func (c *Custom) matchTarget(in Input) string {
	if in.Event == EventWrite {
		return in.FilePath
	}
	return in.Command
}

// payload is the JSON object the external reads on stdin: the same fields
// belt extracted from the hook payload for this event.
func (c *Custom) payload(in Input) map[string]string {
	if in.Event == EventWrite {
		return map[string]string{"file_path": in.FilePath, "content": in.Content, "cwd": in.Cwd}
	}
	return map[string]string{"command": in.Command, "cwd": in.Cwd}
}

// warn records a fail-open allow so a broken external is visible in the
// events log instead of silently checking nothing.
func (c *Custom) warn(msg string) {
	c.emit("belt", "warn", "custom guard "+c.id+" error", msg,
		map[string]string{"guard": c.id, "event": c.cfg.Event})
}
