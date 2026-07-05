package notify

import (
	"os/exec"
	"strings"
	"testing"
)

// Banner posts a native macOS notification via osascript — the same mechanism
// deps, reminder, and sandbox-watch use. Best-effort: errors are ignored, and
// tests never notify.
func Banner(title, body string) {
	if testing.Testing() {
		return
	}
	script := "display notification " + appleScriptString(body) +
		" with title " + appleScriptString(title) +
		` sound name "Funk"`
	_ = exec.Command("/usr/bin/osascript", "-e", script).Run()
}

// appleScriptString renders s as an AppleScript double-quoted string literal,
// escaping backslashes and double quotes so service names can't break out of
// the literal.
func appleScriptString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
