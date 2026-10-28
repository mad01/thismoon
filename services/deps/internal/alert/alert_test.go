package alert

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRunWithTimeoutKillsHungCommand: a command that outlives the timeout is
// killed and comes back as a deadline error, well before it would exit. It
// runs /bin/sleep in place of osascript, so no banner fires.
func TestRunWithTimeoutKillsHungCommand(t *testing.T) {
	start := time.Now()
	err := runWithTimeout(100*time.Millisecond, "/bin/sleep", "30")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("returned after %s, want the kill at the timeout", took)
	}
}

// TestRunWithTimeoutPassesExitStatus: a command that finishes in time returns
// its own result, with no deadline error.
func TestRunWithTimeoutPassesExitStatus(t *testing.T) {
	if err := runWithTimeout(osascriptTimeout, "/usr/bin/true"); err != nil {
		t.Errorf("true: err = %v, want nil", err)
	}
	err := runWithTimeout(osascriptTimeout, "/usr/bin/false")
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("false: err = %v, want its exit status", err)
	}
}
