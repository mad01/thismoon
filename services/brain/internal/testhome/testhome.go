// Package testhome isolates a test from the machine's brain config and
// state. Tests import it; the binary never does.
package testhome

import (
	"testing"

	"github.com/mad01/thismoon/services/brain"
)

// Isolate points HOME at a fresh temp dir and clears every variable that
// could route brain to a real config or state directory, then returns the
// new home. It uses t.Setenv, so the caller cannot run in parallel.
func Isolate(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv(brain.ConfigEnv, "")
	return home
}
