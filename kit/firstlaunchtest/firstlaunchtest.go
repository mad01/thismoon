// Package firstlaunchtest warms a freshly written executable before a test
// times it. macOS checks each new executable on its first exec, and under a
// full-repo `go test ./...` that check can take seconds and outlast a test's
// short deadline; later launches of the same file take milliseconds.
package firstlaunchtest

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// budget bounds the warm-up launch: long enough for a check queued behind
// dozens of fresh test binaries, short enough that a hung script still fails.
const budget = 30 * time.Second

// pipeGrace is how long Warm waits for the output pipe to close once the
// launch has exited or been killed. A child the launch left running holds
// the pipe open, and the budget's kill reaches only the launch itself.
const pipeGrace = time.Second

// Warm runs path once with args, outside any test deadline, and fails t
// with the combined output if it does not exit 0 within the budget, or if a
// child it started still holds the output after pipeGrace. A nil env
// inherits the test's environment; any other env replaces it whole.
func Warm(t testing.TB, path string, args, env []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = pipeGrace
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("first launch of %s: %v: %s", path, err, out)
	}
}
