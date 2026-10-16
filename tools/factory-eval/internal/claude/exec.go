package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Output is what one call left behind.
type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Wall     time.Duration
}

// Exec runs the real claude binary.
type Exec struct {
	// Bin is the claude binary to run, "claude" from PATH when empty.
	Bin string
}

// interruptGrace is how long a cancelled session gets after SIGINT to end
// its turn and print its result before it is killed.
const interruptGrace = 30 * time.Second

// Run executes inv and measures its wall clock. A non-zero exit is not an
// error: the report on stdout says what happened. Run errors only when the
// process cannot start or ctx ends the call.
func (e Exec) Run(ctx context.Context, inv Invocation) (Output, error) {
	bin := e.Bin
	if bin == "" {
		bin = "claude"
	}
	cmd := exec.CommandContext(ctx, bin, inv.Args...)
	cmd.Dir = inv.Dir
	cmd.Stdin = strings.NewReader(inv.Stdin)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = interruptGrace
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	out := Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Wall: time.Since(start)}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		out.ExitCode = -1
		return out, fmt.Errorf("claude: run stopped: %w", ctx.Err())
	case errors.As(err, &exitErr):
		out.ExitCode = exitErr.ExitCode()
		return out, nil
	case err != nil:
		return out, fmt.Errorf("claude: run %s: %w", bin, err)
	}
	return out, nil
}

// Version runs `claude --version` and parses it.
func (e Exec) Version(ctx context.Context) ([3]int, string, error) {
	bin := e.Bin
	if bin == "" {
		bin = "claude"
	}
	raw, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return [3]int{}, "", fmt.Errorf("claude: %s --version: %w", bin, err)
	}
	v, err := ParseVersion(string(raw))
	return v, strings.TrimSpace(string(raw)), err
}
