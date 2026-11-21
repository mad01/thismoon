package input

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

func TestReadSkipsBlankLinesAndKeepsGoing(t *testing.T) {
	in := strings.Join([]string{
		`{"type":"clear"}`,
		"",
		"   ",
		`not json`,
		`{"type":"remove","id":"acme/api#12"}`,
	}, "\n")
	var got []tea.Msg
	Read(strings.NewReader(in), func(m tea.Msg) { got = append(got, m) })

	if len(got) != 4 {
		t.Fatalf("Read sent %d messages, want 3 lines and an end: %#v", len(got), got)
	}
	if m, ok := got[0].(LineMsg); !ok || m.Line != 1 || m.Message != (protocol.Clear{}) {
		t.Errorf("message 1 = %#v, want line 1 decoded as clear", got[0])
	}
	if m, ok := got[1].(LineMsg); !ok || m.Line != 4 || !errors.Is(m.Err, protocol.ErrBadJSON) {
		t.Errorf("message 2 = %#v, want line 4 with ErrBadJSON", got[1])
	}
	if m, ok := got[2].(LineMsg); !ok || m.Line != 5 ||
		m.Message != (protocol.Remove{ID: "acme/api#12"}) {
		t.Errorf("message 3 = %#v, want line 5 decoded as remove", got[2])
	}
	if m, ok := got[3].(EndMsg); !ok || m.Err != nil {
		t.Errorf("last message = %#v, want EndMsg without an error", got[3])
	}
}

func TestReadSkipsOverlongLineAndKeepsGoing(t *testing.T) {
	in := strings.Repeat("x", maxLineBytes+1) + "\n" + `{"type":"clear"}`
	var got []tea.Msg
	Read(strings.NewReader(in), func(m tea.Msg) { got = append(got, m) })

	if len(got) != 3 {
		t.Fatalf("Read sent %d messages, want the skipped line, the next and an end", len(got))
	}
	if m, ok := got[0].(LineMsg); !ok || m.Line != 1 || !errors.Is(m.Err, ErrLineTooLong) ||
		m.Err.Error() != "longer than 16 MiB, skipped" {
		t.Errorf("message 1 = %+v, want line 1 with ErrLineTooLong", got[0])
	}
	if m, ok := got[1].(LineMsg); !ok || m.Line != 2 || m.Message != (protocol.Clear{}) {
		t.Errorf("message 2 = %#v, want line 2 decoded as clear", got[1])
	}
	if m, ok := got[2].(EndMsg); !ok || m.Err != nil {
		t.Errorf("last message = %#v, want EndMsg without an error", got[2])
	}
}

func TestReadCapLeavesOutTheLineEnding(t *testing.T) {
	// A clear message padded with white space inside the object to n bytes.
	clearOf := func(n int) string {
		const open, end = `{"type":"clear"`, "}"
		return open + strings.Repeat(" ", n-len(open)-len(end)) + end
	}
	cases := []struct {
		name    string
		in      string
		tooLong bool
	}{
		{"at the cap with a newline", clearOf(maxLineBytes) + "\n", false},
		{"at the cap with CRLF", clearOf(maxLineBytes) + "\r\n", false},
		{"at the cap at the end of input", clearOf(maxLineBytes), false},
		{"one byte over", clearOf(maxLineBytes+1) + "\n", true},
		{"one byte over at the end of input", clearOf(maxLineBytes + 1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []tea.Msg
			Read(strings.NewReader(tc.in), func(m tea.Msg) { got = append(got, m) })
			if len(got) != 2 {
				t.Fatalf("Read sent %d messages, want one line and an end", len(got))
			}
			m, ok := got[0].(LineMsg)
			if !ok {
				t.Fatalf("message 1 = %T, want a LineMsg", got[0])
			}
			if tc.tooLong && !errors.Is(m.Err, ErrLineTooLong) {
				t.Errorf("line error = %v, want ErrLineTooLong", m.Err)
			}
			if !tc.tooLong && (m.Err != nil || m.Message != (protocol.Clear{})) {
				t.Errorf("line = %v, %v, want it decoded as clear", m.Message, m.Err)
			}
		})
	}
}
