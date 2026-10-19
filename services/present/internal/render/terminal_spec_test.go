package render

import "testing"

// FindTerminalSpec is where serve reads what a session runs, so it must
// find a block wherever a Doc can hold one, in either rendition, and say
// so plainly when the page declares no such session.
func TestFindTerminalSpec(t *testing.T) {
	block := Block{
		T: "terminal", Session: "demo", Cmd: " claude\n", Cwd: "~/code ",
		Prefill: " explain this repo", TerminalRows: 24,
	}
	want := TerminalSpec{Cmd: "claude", Cwd: "~/code", Prefill: "explain this repo"}
	inColumn := Block{T: "columns", Columns: [][]Block{{{T: "p", Text: "x"}}, {block}}}
	other := Block{T: "terminal", Session: "other", Cmd: "htop"}
	plain := terminalDoc(Block{T: "p", Text: "no terminal here"})
	both := []Doc{terminalDoc(block), terminalDoc(other)}
	var none TerminalSpec

	cases := []struct {
		name    string
		session string
		docs    []Doc
		want    TerminalSpec
		found   bool
	}{
		{"top-level block", "demo", []Doc{terminalDoc(other, block)}, want, true},
		{"block in a column", "demo", []Doc{terminalDoc(inColumn)}, want, true},
		{"deck only", "demo", []Doc{plain, terminalDoc(block)}, want, true},
		{"another session", "other", both, TerminalSpec{Cmd: "htop"}, true},
		{"undeclared session", "nope", both, none, false},
		{"session differs only in case", "Demo", both, none, false},
		{"no docs", "demo", nil, none, false},
		{"non-terminal block", "demo", []Doc{plain}, none, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, found := FindTerminalSpec(c.session, c.docs...)
			if got != c.want || found != c.found {
				t.Errorf("FindTerminalSpec(%q) = %v, %v; want %v, %v",
					c.session, got, found, c.want, c.found)
			}
		})
	}
}
