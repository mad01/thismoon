// Package notify delivers a reminder as a native macOS notification. The
// Notifier interface keeps the side effect at the edge so the ticker can be
// tested against a fake.
package notify

import (
	"os/exec"
	"strings"
)

// Notifier delivers a fired reminder to the user.
type Notifier interface {
	Notify(title, body string) error
}

// Osascript posts a macOS notification via /usr/bin/osascript. No extra
// dependency — osascript ships with macOS.
type Osascript struct{}

// Notify shows a notification titled "Reminder: <title>" with body as the
// message. Empty body is fine.
func (Osascript) Notify(title, body string) error {
	script := "display notification " + appleScriptString(body) +
		" with title " + appleScriptString("Reminder: "+title)
	return exec.Command("/usr/bin/osascript", "-e", script).Run()
}

// appleScriptString renders s as an AppleScript double-quoted string literal,
// escaping backslashes and double quotes so titles/bodies with quotes or
// backslashes can't break out of the literal or inject script.
func appleScriptString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
