package cli

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "humanizer",
	Short: "Detect AI-writing patterns and profile voice",
	Long: `humanizer: a local CLI and MCP server that flags AI-writing patterns
and computes quantitative voice profiles for text samples.

Subcommands:
  detect   Scan text for AI-writing patterns.
  ste      Check an agent brief or report against the STE-lite rules.
  profile  Compute a voice profile (sentence stats, punctuation density).
  rules    List or explain the supported detection rules.
  mcp      Start the MCP stdio server for Claude Code.
  version  Print the build this binary came from.

Patterns are derived from Wikipedia's "Signs of AI writing" page via the
/humanizer skill. The detector is purely deterministic: rewrites are left
to the calling agent (Claude).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// pendingExitCode lets a command (e.g. lint) request a non-zero exit without
// returning an error, so findings don't print as an error message.
var pendingExitCode int

// exitError attaches a process exit code to an error a command returns.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

func (e *exitError) Unwrap() error { return e.err }

// withExitCode wraps a non-nil err so the process exits with code.
func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitError{code: code, err: err}
}

// ExitCode returns the process exit code for an error Execute returned: the
// code its command attached, else 1.
func ExitCode(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 1
}

// Execute runs the command tree. A command that found problems without
// failing sets pendingExitCode, and Execute exits with it.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return err
	}
	if pendingExitCode != 0 {
		os.Exit(pendingExitCode)
	}
	return nil
}
