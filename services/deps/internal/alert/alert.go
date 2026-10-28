// Package alert delivers a flagged dependency as a native macOS notification —
// the same osascript mechanism the reminder tool and the sandbox-watch script
// use. The Notifier interface keeps the side effect at the edge so the scanner
// can be tested against a fake.
//
// It is deliberately not called "notify": archiving an event to the events
// service is kit/notify's job, and the scanner does both in the same file.
package alert

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// osascriptTimeout bounds one banner. The scanner notifies under a lock, so a
// hung osascript would otherwise stall every manual check behind it.
const osascriptTimeout = 10 * time.Second

// Notifier delivers one alert to the user.
type Notifier interface {
	Notify(title, body string) error
}

// Osascript posts a macOS notification via /usr/bin/osascript. No extra
// dependency — osascript ships with macOS.
type Osascript struct{}

// Notify shows a notification with the given title and body. Empty body is
// fine. An osascript still running after osascriptTimeout is killed and
// reported as an error, so the caller retries the banner next cycle.
func (Osascript) Notify(title, body string) error {
	script := "display notification " + appleScriptString(body) +
		" with title " + appleScriptString(title) +
		` sound name "Funk"`
	return runWithTimeout(osascriptTimeout, "/usr/bin/osascript", "-e", script)
}

// runWithTimeout runs name with args and kills it once timeout passes. A kill
// comes back as an error wrapping context.DeadlineExceeded.
func runWithTimeout(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := exec.CommandContext(ctx, name, args...).Run()
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("%s still running after %s: %w", filepath.Base(name), timeout, ctx.Err())
	}
	return err
}

// appleScriptString renders s as an AppleScript double-quoted string literal,
// escaping backslashes and double quotes so package names/summaries with quotes
// or backslashes can't break out of the literal or inject script.
func appleScriptString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
