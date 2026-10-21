// Package apply holds the conventions every agent-sync section (settings,
// mods, mcp) shares, so the three behave alike under ralph:
//
//   - Three modes. Sync applies the change, Plan prints what Sync would do
//     and writes nothing, Check writes nothing and exits CodeDrift when Sync
//     would change anything. In Check the first stdout line is a one-line
//     summary, because `ralph doctor` shows only that line.
//   - Exit codes: CodeOK, CodeDrift, CodeError, CodeExternal.
//   - stderr lines start with "error: " or "warning: ", the prefixes the
//     Python sync scripts this tool replaces used.
//
// A command reports drift on stdout itself and then returns Exit(CodeDrift),
// which carries the code without a message. Any other error goes back up to
// Main, which prints it once as "error: <message>".
package apply

import (
	"errors"
	"fmt"
	"io"
)

// Mode is what a section command does with the change it computes.
type Mode int

// The three modes; see the package comment for their contracts.
const (
	Sync Mode = iota + 1
	Plan
	Check
)

// String returns the subcommand name for m.
func (m Mode) String() string {
	switch m {
	case Sync:
		return "sync"
	case Plan:
		return "plan"
	case Check:
		return "check"
	default:
		return fmt.Sprintf("Mode(%d)", int(m))
	}
}

// Writes reports whether m may change anything on disk.
func (m Mode) Writes() bool { return m == Sync }

// Process exit codes. Drift and errors share 1, which is what ralph treats
// as "not ok"; the separate names say which one a call site means.
const (
	CodeOK    = 0
	CodeDrift = 1 // Check found the live state differs from the desired state
	CodeError = 1 // bad config, unreadable data, or a failed write
	// CodeExternal reports an external command (the claude CLI) that failed;
	// the mcp section's Python predecessor used 2 for it.
	CodeExternal = 2
)

// ExitError carries an exit code for a run whose output is already
// printed. Main exits with Code and prints nothing more, unless Err is set.
type ExitError struct {
	Code int
	Err  error
}

// Error returns the wrapped message, or a placeholder for a bare code.
func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *ExitError) Unwrap() error { return e.Err }

// Exit returns an error that makes Main exit with code and print nothing.
func Exit(code int) error { return &ExitError{Code: code} }

// Fail returns an error that makes Main print "error: <err>" and exit with
// code. Use it for codes other than CodeError, which any plain error gets.
func Fail(code int, err error) error { return &ExitError{Code: code, Err: err} }

// Streams is where a section command writes: Out for the report (the
// summary line, the diff, what was written), Err for warnings.
type Streams struct {
	Out io.Writer
	Err io.Writer
}

// Warn prints a "warning: " line to w.
func Warn(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "warning: "+format+"\n", args...)
}

// Main turns a command's result into the process exit code, printing the
// error to stderr as "error: <message>" when there is one to print.
func Main(stderr io.Writer, err error) int {
	if err == nil {
		return CodeOK
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		if exit.Err != nil {
			fmt.Fprintf(stderr, "error: %v\n", exit.Err)
		}
		return exit.Code
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return CodeError
}
