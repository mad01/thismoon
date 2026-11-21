package frame

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestEditQuery(t *testing.T) {
	n := tea.KeyPressMsg{Code: 'n', Text: "n"}
	ctrlN, altN := n, n
	ctrlN.Mod, altN.Mod = tea.ModCtrl, tea.ModAlt
	backspace := tea.KeyPressMsg{Code: tea.KeyBackspace}
	tests := []struct {
		name   string
		query  string
		key    tea.KeyPressMsg
		want   string
		wantOK bool
	}{
		{"types text", "lg", n, "lgn", true},
		{"types a space", "lgn", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, "lgn ", true},
		{"backspace deletes a whole rune", "café", backspace, "caf", true},
		{"backspace on empty", "", backspace, "", true},
		{"ctrl is not text", "lg", ctrlN, "lg", false},
		{"alt is not text", "lg", altN, "lg", false},
		{"enter is not text", "lg", tea.KeyPressMsg{Code: tea.KeyEnter}, "lg", false},
		{
			"a C1 control in key text becomes a space", "lg",
			tea.KeyPressMsg{Code: 0x9b, Text: "\u009b"},
			"lg ", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := EditQuery(tt.query, tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("EditQuery(%q) = %q, %v; want %q, %v",
					tt.query, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFooterEndsInStatus(t *testing.T) {
	var s Status
	s.Fail(errors.New("line 3: bad \x1b[31m input"))
	s.End(nil)

	got := ansi.Strip(Footer("lgn", s, "2/5", "following"))
	want := "  > lgn  2/5  following  input ended  1 error  line 3: bad  [31m input"
	if got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if !s.Ended() {
		t.Error("Ended = false after End")
	}
}

func TestCounts(t *testing.T) {
	cases := []struct {
		scopes []string
		want   string
	}{
		{nil, "2/5"},
		{[]string{"ignored"}, "ignored  2/5"},
		{[]string{"ignored", "inbox"}, "ignored, inbox  2/5"},
	}
	for _, tc := range cases {
		if got := ansi.Strip(Counts(tc.scopes, 2, 5)); got != tc.want {
			t.Errorf("Counts(%q, 2, 5) = %q, want %q", tc.scopes, got, tc.want)
		}
	}
}

func TestEndWithErrorFails(t *testing.T) {
	var s Status
	s.End(errors.New("read: broken pipe"))

	got := ansi.Strip(Footer("", s))
	if want := "1 error  input: read: broken pipe"; !strings.HasSuffix(got, want) {
		t.Errorf("footer = %q, want the read error counted and shown", got)
	}
}
