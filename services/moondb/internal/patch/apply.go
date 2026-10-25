package patch

import (
	"bytes"
	"fmt"
	"strings"
)

// line is one line of a file: its text and its ending, which is "\n",
// "\r\n", or "" for a last line without a newline.
type line struct {
	text string
	eol  string
}

// op is one hunk line with its prefix split off and its trailing carriage
// return dropped, so it matches a line of either ending.
type op struct {
	kind byte
	text string
	// noEOL is set when a no-newline marker follows the line.
	noEOL bool
}

// split cuts content into lines, keeping each line's ending.
func split(content []byte) []line {
	var lines []line
	for len(content) > 0 {
		i := bytes.IndexByte(content, '\n')
		if i < 0 {
			lines = append(lines, line{text: string(content)})
			break
		}
		l := line{text: string(content[:i]), eol: "\n"}
		if strings.HasSuffix(l.text, "\r") {
			l.text, l.eol = l.text[:len(l.text)-1], "\r\n"
		}
		lines = append(lines, l)
		content = content[i+1:]
	}
	return lines
}

// shown is the line as a MismatchError reports it: its text, and "\n" when
// it ends in a newline of either kind.
func (l line) shown() string {
	if l.eol == "" {
		return l.text
	}
	return l.text + "\n"
}

func join(lines []line) []byte {
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l.text)
		b.WriteString(l.eol)
	}
	return b.Bytes()
}

// ops turns a validated hunk's lines into ops, folding each no-newline
// marker into the line before it.
func ops(h Hunk) []op {
	out := make([]op, 0, len(h.Lines))
	for _, l := range h.Lines {
		if l[0] == noNewlineMarker {
			out[len(out)-1].noEOL = true
			continue
		}
		out = append(out, op{kind: l[0], text: strings.TrimSuffix(l[1:], "\r")})
	}
	return out
}

// dominantEOL is the ending given to added lines: the more common ending in
// src, or, when src has none, CRLF if the hunks' added lines carry carriage
// returns and LF otherwise.
func dominantEOL(src []line, hunks []Hunk) string {
	var crlf, lf int
	for _, l := range src {
		switch l.eol {
		case "\r\n":
			crlf++
		case "\n":
			lf++
		}
	}
	if crlf+lf > 0 {
		if crlf > lf {
			return "\r\n"
		}
		return "\n"
	}
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l[0] == '+' && strings.HasSuffix(l, "\r") {
				return "\r\n"
			}
		}
	}
	return "\n"
}

// apply runs validated hunks over src in order. Each hunk's old side must
// match src at the line its OldStart names; lines between hunks are copied.
func apply(src []line, hunks []Hunk, eol string) ([]line, error) {
	out := make([]line, 0, len(src))
	pos := 0
	for i, h := range hunks {
		start := h.OldStart - 1
		if h.OldLines == 0 {
			// An insertion into an empty side: jsdiff states 0 or 1 for an
			// empty file, and the line after which to insert otherwise.
			start = min(h.OldStart, len(src))
		}
		if start < pos {
			return nil, fmt.Errorf(
				"patch: hunk %d starts at line %d, before line %d where hunk %d ended",
				i,
				start+1,
				pos+1,
				i-1,
			)
		}
		out = append(out, src[pos:start]...)
		pos = start
		hunkOps := ops(h)
		for _, o := range hunkOps {
			if o.kind == '+' {
				out = append(out, added(o, eol))
				continue
			}
			if err := match(src, pos, o); err != nil {
				err.Hunk = i
				return nil, err
			}
			if o.kind == ' ' {
				out = append(out, src[pos])
			}
			pos++
		}
		if endsFile(hunkOps) && pos < len(src) {
			return nil, &MismatchError{Hunk: i, Line: pos + 1, Got: src[pos].shown()}
		}
	}
	return append(out, src[pos:]...), nil
}

// endsFile reports a hunk whose trailing context is shorter than its
// leading context. Diff tools cut context short only at the ends of the
// file, so such a hunk ends at the end of the file; without this check an
// append at the end would fit content with extra lines after it.
func endsFile(hunkOps []op) bool {
	lead := 0
	for lead < len(hunkOps) && hunkOps[lead].kind == ' ' {
		lead++
	}
	if lead == len(hunkOps) {
		return false
	}
	trail := 0
	for hunkOps[len(hunkOps)-1-trail].kind == ' ' {
		trail++
	}
	return trail < lead
}

// match checks that src has the op's line at pos, ending included.
func match(src []line, pos int, o op) *MismatchError {
	want := o.text
	if !o.noEOL {
		want += "\n"
	}
	got := ""
	if pos < len(src) {
		got = src[pos].shown()
	}
	if got == want {
		return nil
	}
	return &MismatchError{Line: pos + 1, Expected: want, Got: got}
}

func added(o op, eol string) line {
	if o.noEOL {
		return line{text: o.text}
	}
	return line{text: o.text, eol: eol}
}
