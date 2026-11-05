package cli

import (
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Exit codes. docs/cli.md documents them; keep the two in step.
const (
	exitOK           = 0
	exitError        = 1
	exitUsage        = 2
	exitNotFound     = 3
	exitExists       = 4
	exitInvalidQuery = 5
	exitReadOnly     = 6
	exitConflict     = 7
)

// usageError marks a bad flag or argument (exit code 2).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usageErrorf(format string, args ...any) error {
	return &usageError{err: fmt.Errorf(format, args...)}
}

// reportedError marks a failure whose result the command already wrote to
// stdout (doctor's report), so the boundary adds no JSON error envelope.
type reportedError struct{ err error }

func (e *reportedError) Error() string { return e.err.Error() }
func (e *reportedError) Unwrap() error { return e.err }

// errorEnvelope is the JSON written to stdout for a failed command under -o json.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// classify maps an error to its exit code and JSON error code.
func classify(err error) (int, string) {
	var usage *usageError
	switch {
	case errors.Is(err, vault.ErrAmbiguous):
		return exitUsage, "ambiguous"
	case errors.As(err, &usage),
		errors.Is(err, vault.ErrInvalidPath),
		errors.Is(err, vault.ErrUnknownTier),
		errors.Is(err, vault.ErrUnknownVault),
		errors.Is(err, vault.ErrImmutableID):
		return exitUsage, "usage"
	case errors.Is(err, config.ErrInvalid):
		return exitUsage, "invalid_config"
	case errors.Is(err, vault.ErrNotFound):
		return exitNotFound, "not_found"
	case errors.Is(err, vault.ErrExists):
		return exitExists, "already_exists"
	case errors.Is(err, search.ErrInvalidQuery):
		return exitInvalidQuery, "invalid_query"
	case errors.Is(err, vault.ErrReadOnly):
		return exitReadOnly, "read_only"
	case errors.Is(err, vault.ErrConflict):
		return exitConflict, "conflict"
	}
	return exitError, "error"
}

// report prints err once as "brain <cmd>: <message>" on stderr and, under
// -o json, as an error envelope on stdout. It returns the exit code.
func (a *app) report(cmd *cobra.Command, err error, stdout, stderr io.Writer) int {
	exit, code := classify(err)
	name := "brain"
	if cmd != nil {
		name = cmd.CommandPath()
	}
	msg := oneLine(err.Error())
	fmt.Fprintf(stderr, "%s: %s\n", name, msg)
	var reported *reportedError
	if a.output == outputJSON && !errors.As(err, &reported) {
		env := errorEnvelope{Error: errorBody{Code: code, Message: msg}}
		if werr := writeJSON(stdout, env); werr != nil {
			fmt.Fprintf(stderr, "%s: write JSON error: %s\n", name, werr)
		}
	}
	return exit
}

var lineBreaks = regexp.MustCompile(`\s*\n\s*`)

// oneLine joins a multi-line message (yaml and zoekt errors wrap) into one
// line so every failure is exactly one stderr line.
func oneLine(msg string) string {
	return lineBreaks.ReplaceAllString(strings.TrimSpace(msg), "; ")
}

// outputFromArgs finds the -o/--output value in raw arguments. run uses it
// when flag parsing failed before -o was read, so a JSON caller still gets
// the JSON error envelope. The last occurrence wins, as in flag parsing.
func outputFromArgs(args []string) string {
	out := ""
	for i, arg := range args {
		if arg == "--" {
			break
		}
		switch {
		case arg == "-o" || arg == "--output":
			if i+1 < len(args) {
				out = args[i+1]
			}
		case strings.HasPrefix(arg, "--output="):
			out = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-o="):
			out = strings.TrimPrefix(arg, "-o=")
		case strings.HasPrefix(arg, "-o") && !strings.HasPrefix(arg, "--"):
			out = strings.TrimPrefix(arg, "-o")
		}
	}
	return out
}

// refPath returns the path part of a note ref: rel for vault:rel, the ref
// itself for a bare path.
func refPath(ref string) string {
	if before, after, ok := strings.Cut(ref, ":"); ok && before != "" &&
		!strings.ContainsAny(before, `/\.`) {
		return after
	}
	return ref
}

// withNoteHint appends a way to find the right path to a not-found error for
// the vault-relative path rel. Other errors pass through unchanged.
func withNoteHint(err error, rel string) error {
	if !errors.Is(err, vault.ErrNotFound) {
		return err
	}
	tier := "<inbox|notes|outputs|MOCs>"
	if first, _, ok := strings.Cut(rel, "/"); ok {
		if t, terr := vault.ParseTier(first); terr == nil {
			tier = t.Dir()
		}
	}
	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	return fmt.Errorf(
		"%w (find it with: brain list --tier %s, or brain search 'f:%s')",
		err, tier, regexp.QuoteMeta(stem),
	)
}
