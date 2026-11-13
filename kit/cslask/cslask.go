// Package cslask holds the line limits of one csl_ask call and the
// max_lines a call needs to read one file. csl_ask reads each file whole or
// skips it, so a call scoped to a long file must raise max_lines to cover
// it. csl's ask package applies the limits, and csl_read's truncated note
// and belt's large-read guard and read-size hint size their calls with
// MaxLines, so no ready call asks for more than csl_ask accepts.
package cslask

const (
	// DefaultMaxLines is the line budget of a call that names no max_lines.
	DefaultMaxLines = 6000
	// MaxLinesCeiling is the most max_lines a call may ask for.
	MaxLinesCeiling = 20000
)

// linesStep is what MaxLines rounds up to, so the number reads as a budget
// rather than a measurement.
const linesStep = 100

// MaxLines returns the max_lines a csl_ask call needs to read a file of
// lines lines whole. 0 means the default budget covers it, so the call
// needs none. Past the default the count is rounded up to linesStep, never
// past the ceiling. ok is false past the ceiling, where no call can read
// the file.
func MaxLines(lines int) (maxLines int, ok bool) {
	switch {
	case lines > MaxLinesCeiling:
		return 0, false
	case lines <= DefaultMaxLines:
		return 0, true
	}
	return min((lines+linesStep-1)/linesStep*linesStep, MaxLinesCeiling), true
}
