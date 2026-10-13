// Package progstatus reports what a program is doing to the terminal that
// runs it, through the Program Status Protocol (OSC 7501, revision 0.4,
// https://www.superlogical.com/rex/docs/build/program-status). A terminal
// that implements it (Ghostty, Rex, kitty) shows per tab whether the
// command is working, done, blocked on the user, or failed, and keeps that
// record after the process exits, so a long run that finished while the
// user was in another tab still reads as done or failed. Claude Code and pi
// already report this way; this package lets the tools here do the same.
//
// A report is one escape sequence, ESC ] 7501 ; key=value:key=value ESC \,
// written to stderr. A terminal that does not know the sequence drops it,
// which is why the package never asks the terminal whether it does: the
// spec's feature query (OSC 7501 ; ?) is optional, and answering it needs a
// raw-mode read of the tty with a deadline for a result that changes
// nothing. The only check is whether stderr is a terminal at all. A pipe, a
// file, CI, and go test get a Reporter that discards, so a tool run under
// `| jq` or inside a test never leaks escape bytes into its output.
//
// Every write is best effort: a write error is ignored, and every method is
// safe on a nil or zero Reporter, so a caller wires one in and never guards
// it. Two habits the spec asks for stay with the caller: report Idle when
// the user cancels, and send Done or Error before exiting, since the
// terminal shows the last state it saw. Run does both for the common case
// of one unit of work, and Steps turns a loop over a known count into
// progress reports, one per percent.
package progstatus

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-isatty"
)

// State is a Program Status Protocol state.
type State string

// The states a report can carry. Clear is the sixth value the protocol
// knows; Reporter.Clear sends it and callers never name it.
const (
	Idle    State = "idle"
	Working State = "working"
	Done    State = "done"
	Blocked State = "blocked"
	Error   State = "error"
)

// Kind says what a blocked program waits for.
type Kind string

// The kinds the protocol defines for a blocked report.
const (
	KindPermission Kind = "permission"
	KindQuestion   Kind = "question"
	KindAuth       Kind = "auth"
)

// Spec limits. The app value is ASCII only, so its cap is a byte count that
// never splits a character; the msg cap applies to the decoded UTF-8 and is
// cut on a rune boundary.
const (
	maxAppBytes = 32
	maxMsgBytes = 2048
)

// The sequence frame. ESC \ (ST) terminates it; BEL would also work but ST
// is what the spec shows.
const (
	prefix     = "\x1b]7501;"
	terminator = "\x1b\\"
)

// noProgress marks a report without a progress key.
const noProgress = -1

// Reporter emits reports for one app. The zero Reporter discards every
// report.
type Reporter struct {
	w   io.Writer
	app string
}

// New returns a Reporter for app that writes to stderr when stderr is a
// terminal, and a discarding Reporter otherwise (pipes, files, CI, go test).
// The go test case is checked explicitly, since a test run on a developer's
// machine can hold a real terminal on stderr.
func New(app string) *Reporter {
	if testing.Testing() || !IsTerminal(os.Stderr) {
		return NewWriter(nil, app)
	}
	return NewWriter(os.Stderr, app)
}

// NewWriter returns a Reporter that writes to w, for tests and callers that
// own a terminal handle. A nil w discards.
func NewWriter(w io.Writer, app string) *Reporter {
	return &Reporter{w: w, app: sanitizeApp(app)}
}

// Working reports that the program is busy, with msg as the detail.
func (r *Reporter) Working(msg string) {
	r.emit(Working, "", noProgress, msg)
}

// Progress reports that the program is busy and how far along it is.
// percent is clamped to 0-100.
func (r *Reporter) Progress(percent int, msg string) {
	r.emit(Working, "", max(0, min(100, percent)), msg)
}

// Blocked reports that the program waits for the user. kind says what for;
// an empty kind is allowed and omits the key.
func (r *Reporter) Blocked(kind Kind, msg string) {
	r.emit(Blocked, kind, noProgress, msg)
}

// Done reports that the program finished. Send it before exiting: the
// terminal keeps the last state it saw.
func (r *Reporter) Done(msg string) {
	r.emit(Done, "", noProgress, msg)
}

// Error reports that the program failed, with msg as the reason. Send it
// before exiting, like Done.
func (r *Reporter) Error(msg string) {
	r.emit(Error, "", noProgress, msg)
}

// Idle reports that the program is waiting for nothing in particular. The
// spec asks for it when the user cancels.
func (r *Reporter) Idle() {
	r.emit(Idle, "", noProgress, "")
}

// Clear removes the program's status from the terminal.
func (r *Reporter) Clear() {
	r.emit("clear", "", noProgress, "")
}

// Run reports Working with msg, runs fn, then Done with the same msg when fn
// returns nil, Idle when the run was cancelled (fn's error or ctx.Err()
// wraps context.Canceled: the user stopped it, so there is nothing to show),
// and Error with the error text otherwise. A deadline that ran out is a
// failure, not a cancel, so it reports Error. It returns fn's error unchanged.
func (r *Reporter) Run(ctx context.Context, msg string, fn func() error) error {
	r.Working(msg)
	err := fn()
	switch {
	case err == nil:
		r.Done(msg)
	case errors.Is(err, context.Canceled) || (ctx != nil && errors.Is(ctx.Err(), context.Canceled)):
		r.Idle()
	default:
		r.Error(err.Error())
	}
	return err
}

// Steps reports the progress of one run over a known number of steps as a
// percentage, writing only when the integer percent changes: a run over a
// few hundred repos sends at most a hundred reports rather than one per
// step. A step at zero percent is never written, since the Working report
// that opens the run already says that. A nil Steps, or one from a nil
// Reporter, reports nothing.
type Steps struct {
	r     *Reporter
	total int
	last  int
}

// Steps returns a progress tracker over total steps. A total of zero or
// less yields a tracker that reports nothing.
func (r *Reporter) Steps(total int) *Steps {
	return &Steps{r: r, total: total}
}

// Step reports that i of the total steps are complete, with msg as the
// detail, when that moves the integer percent.
func (s *Steps) Step(i int, msg string) {
	if s == nil || s.total <= 0 {
		return
	}
	pct := i * 100 / s.total
	if pct == s.last {
		return
	}
	s.last = pct
	s.r.Progress(pct, msg)
}

// emit writes one report and drops any write error. A nil or zero Reporter
// writes nothing.
func (r *Reporter) emit(state State, kind Kind, progress int, msg string) {
	if r == nil || r.w == nil {
		return
	}
	_, _ = r.w.Write(encode(state, kind, progress, r.app, msg))
}

// encode builds one report in the key order state, kind, progress, app,
// msg. Empty kind, app, and msg omit their key; progress is omitted when
// negative. The longest possible report (blocked, permission, a 32-byte
// app, a 2048-byte msg) is about 2.8 KB, inside the spec's 4096-byte cap,
// so no length guard is needed.
func encode(state State, kind Kind, progress int, app, msg string) []byte {
	b := make([]byte, 0, 64)
	b = append(b, prefix...)
	b = append(b, "state="...)
	b = append(b, state...)
	if kind != "" {
		b = append(b, ":kind="...)
		b = append(b, kind...)
	}
	if progress >= 0 {
		b = append(b, ":progress="...)
		b = strconv.AppendInt(b, int64(progress), 10)
	}
	if app != "" {
		b = append(b, ":app="...)
		b = append(b, app...)
	}
	if m := cleanMsg(msg); m != "" {
		b = append(b, ":msg="...)
		b = base64.StdEncoding.AppendEncode(b, []byte(m))
	}
	return append(b, terminator...)
}

// sanitizeApp keeps the bytes the spec allows in an app identifier,
// [A-Za-z0-9_.+-], and cuts the result at maxAppBytes. Anything else is
// dropped rather than escaped, so the caller's "my tool" becomes "mytool".
func sanitizeApp(app string) string {
	b := make([]byte, 0, min(len(app), maxAppBytes))
	for i := 0; i < len(app) && len(b) < maxAppBytes; i++ {
		if c := app[i]; isAppByte(c) {
			b = append(b, c)
		}
	}
	return string(b)
}

func isAppByte(c byte) bool {
	switch {
	case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		return true
	case c == '_', c == '.', c == '+', c == '-':
		return true
	}
	return false
}

// cleanMsg turns C0 and C1 control characters and DEL into spaces,
// collapses every run of spaces to one and trims the ends, then cuts the
// text at maxMsgBytes on a rune boundary. Mapping to a space rather than
// dropping keeps the word boundaries of a multi-line error: "denied\nhint:
// run csl doctor" reads "denied hint: run csl doctor". Invalid UTF-8 comes
// out as U+FFFD, since the range loop decodes and WriteRune re-encodes,
// which is what the spec's "base64 of UTF-8" wants.
func cleanMsg(msg string) string {
	var b strings.Builder
	b.Grow(len(msg))
	pending := false // a space is owed before the next kept rune
	for _, r := range msg {
		if r < 0x20 || (0x7f <= r && r <= 0x9f) || r == ' ' {
			pending = true
			continue
		}
		if pending && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pending = false
		b.WriteRune(r)
	}
	return truncateRunes(b.String(), maxMsgBytes)
}

// truncateRunes returns s cut to at most n bytes without splitting a rune.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// IsTerminal reports whether f is a terminal, a tty or a Cygwin or MSYS
// pty. New runs it on stderr; a command that draws its own progress on the
// same handle can run it too, so both decisions agree.
func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
