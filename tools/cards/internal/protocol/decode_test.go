package protocol

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"
)

// plain builds the Line a plain-string LINE decodes to.
func plain(text string) Line { return Line{{Text: text, Color: ColorDefault}} }

func TestDecodeValid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Message
	}{
		{
			name: "header string",
			in:   `{"type":"header","text":"Open pull requests"}`,
			want: Header{Text: plain("Open pull requests")},
		},
		{
			name: "header spans",
			in:   `{"type":"header","text":[{"text":"3 ","bold":true},{"text":"open","color":"green"}]}`,
			want: Header{Text: Line{
				{Text: "3 ", Color: ColorDefault, Bold: true},
				{Text: "open", Color: ColorGreen},
			}},
		},
		{
			name: "header empty text",
			in:   `{"type":"header","text":""}`,
			want: Header{Text: plain("")},
		},
		{
			name: "minimal card",
			in:   `{"type":"card","id":"acme/api#12","lines":["Fix login redirect"]}`,
			want: Card{
				ID:    "acme/api#12",
				Lines: []Line{plain("Fix login redirect")},
				Tone:  ColorDefault,
			},
		},
		{
			name: "card without id",
			in:   `{"type":"card","lines":["build passed"]}`,
			want: Card{Lines: []Line{plain("build passed")}, Tone: ColorDefault},
		},
		{
			name: "full card with run action",
			in: `{"type":"card","id":"acme/api#12","lines":["Fix login redirect",` +
				`[{"text":"alice","color":"cyan"},{"text":" 2h","color":"dim"}],"CI green"],` +
				`"tone":"yellow","match":"login alice","at":"2026-11-21T10:30:00Z",` +
				`"actions":{"enter":{"run":["open","https://example.com/acme/api/pull/12"]}}}`,
			want: Card{
				ID: "acme/api#12",
				Lines: []Line{
					plain("Fix login redirect"),
					{{Text: "alice", Color: ColorCyan}, {Text: " 2h", Color: ColorDim}},
					plain("CI green"),
				},
				Tone:  ColorYellow,
				Match: "login alice",
				At:    time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC),
				Enter: &Action{Run: []string{"open", "https://example.com/acme/api/pull/12"}},
			},
		},
		{
			name: "card with emit action",
			in:   `{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"emit":"review"}}}`,
			want: Card{
				ID:    "a",
				Lines: []Line{plain("x")},
				Tone:  ColorDefault,
				Enter: &Action{Emit: "review"},
			},
		},
		{
			name: "card with letter actions and labels",
			in: `{"type":"card","id":"acme/api#12","lines":["x"],"actions":{` +
				`"enter":{"run":["open","https://example.com/acme/api/pull/12"],"label":"open"},` +
				`"x":{"run":["/usr/local/bin/mute","acme/api#12"],"label":"ignore"},` +
				`"a":{"emit":"approve"},"z":null}}`,
			want: Card{
				ID:    "acme/api#12",
				Lines: []Line{plain("x")},
				Tone:  ColorDefault,
				Enter: &Action{
					Run:   []string{"open", "https://example.com/acme/api/pull/12"},
					Label: "open",
				},
				Letters: map[string]*Action{
					"x": {Run: []string{"/usr/local/bin/mute", "acme/api#12"}, Label: "ignore"},
					"a": {Emit: "approve"},
				},
			},
		},
		{
			name: "null letter actions only",
			in:   `{"type":"card","id":"a","lines":["x"],"actions":{"x":null,"enter":null}}`,
			want: Card{ID: "a", Lines: []Line{plain("x")}, Tone: ColorDefault},
		},
		{
			name: "empty color is default",
			in:   `{"type":"card","id":"a","lines":[[{"text":"x","color":""}]],"tone":""}`,
			want: Card{ID: "a", Lines: []Line{plain("x")}, Tone: ColorDefault},
		},
		{
			name: "unknown fields ignored",
			in: `{"type":"card","id":"a","lines":["x"],"priority":5,` +
				`"actions":{"x":{"emit":"mute","confirm":true}}}`,
			want: Card{
				ID:      "a",
				Lines:   []Line{plain("x")},
				Tone:    ColorDefault,
				Letters: map[string]*Action{"x": {Emit: "mute"}},
			},
		},
		{
			name: "card with scope",
			in:   `{"type":"card","id":"acme/api#12","lines":["Fix login redirect"],"scope":"ignored"}`,
			want: Card{
				ID:    "acme/api#12",
				Lines: []Line{plain("Fix login redirect")},
				Tone:  ColorDefault,
				Scope: "ignored",
			},
		},
		{
			name: "remove",
			in:   `{"type":"remove","id":"acme/api#12"}`,
			want: Remove{ID: "acme/api#12"},
		},
		{
			name: "snapshot keeps order",
			in:   `{"type":"snapshot","cards":[{"id":"b","lines":["two"]},{"id":"a","lines":["one"]}]}`,
			want: Snapshot{Cards: []Card{
				{ID: "b", Lines: []Line{plain("two")}, Tone: ColorDefault},
				{ID: "a", Lines: []Line{plain("one")}, Tone: ColorDefault},
			}},
		},
		{
			name: "empty snapshot",
			in:   `{"type":"snapshot","cards":[]}`,
			want: Snapshot{Cards: []Card{}},
		},
		{
			name: "clear",
			in:   `{"type":"clear"}`,
			want: Clear{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode([]byte(tc.in))
			if err != nil {
				t.Fatalf("Decode(%s) error = %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Decode(%s) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr error
		// wantMsg is a fragment the error text must hold, so the case fails
		// when the line is rejected for some other reason.
		wantMsg string
	}{
		{"not JSON", `{"type":"card",`, ErrBadJSON, "unexpected end"},
		{"empty line", ``, ErrBadJSON, "unexpected end"},
		{"not an object", `["card"]`, ErrBadJSON, "cannot unmarshal"},
		{"id is a number", `{"type":"card","id":12,"lines":["x"]}`, ErrBadJSON, "id"},
		{
			"line is a number",
			`{"type":"card","id":"a","lines":[7]}`,
			ErrBadJSON,
			"string or an array of spans",
		},
		{
			"line is null",
			`{"type":"card","id":"a","lines":[null]}`,
			ErrBadJSON,
			"string or an array of spans",
		},
		{"missing type", `{"id":"a"}`, ErrUnknownType, `""`},
		{"unknown type", `{"type":"banner","text":"x"}`, ErrUnknownType, `"banner"`},
		{"header without text", `{"type":"header"}`, ErrInvalid, "text is required"},
		{"header null text", `{"type":"header","text":null}`, ErrInvalid, "text is required"},
		{"no lines", `{"type":"card","id":"a"}`, ErrInvalid, "got 0, want 1 to 3"},
		{"empty lines", `{"type":"card","id":"a","lines":[]}`, ErrInvalid, "got 0, want 1 to 3"},
		{
			"four lines",
			`{"type":"card","id":"a","lines":["1","2","3","4"]}`,
			ErrInvalid,
			"got 4, want 1 to 3",
		},
		{
			"unknown tone",
			`{"type":"card","id":"a","lines":["x"],"tone":"pink"}`,
			ErrInvalid,
			`tone: unknown color "pink"`,
		},
		{
			"unknown span color",
			`{"type":"card","id":"a","lines":["x",[{"text":"y","color":"orange"}]]}`,
			ErrInvalid, `line 2: span 1: unknown color "orange"`,
		},
		{
			"bright color",
			`{"type":"header","text":[{"text":"x","color":"bright-red"}]}`,
			ErrInvalid,
			"unknown color",
		},
		{"bad at", `{"type":"card","id":"a","lines":["x"],"at":"yesterday"}`, ErrInvalid, "at:"},
		{
			"action with both",
			`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"run":["true"],"emit":"x"}}}`,
			ErrInvalid, "not both",
		},
		{
			"action with neither",
			`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{}}}`,
			ErrInvalid, "set one of run and emit",
		},
		{
			"empty argv",
			`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"run":[]}}}`,
			ErrInvalid, "argv",
		},
		{
			"empty program name",
			`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"run":[""]}}}`,
			ErrInvalid, "argv",
		},
		{
			"empty emit name",
			`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"emit":""}}}`,
			ErrInvalid, "emit",
		},
		{
			"unknown action key",
			`{"type":"card","id":"a","lines":["x"],"actions":{"space":{"emit":"x"}}}`,
			ErrInvalid, `actions: unknown key "space"`,
		},
		{
			"capital letter key",
			`{"type":"card","id":"a","lines":["x"],"actions":{"X":{"emit":"x"}}}`,
			ErrInvalid, `actions: unknown key "X"`,
		},
		{
			"two-letter key",
			`{"type":"card","id":"a","lines":["x"],"actions":{"xy":{"emit":"x"}}}`,
			ErrInvalid, `actions: unknown key "xy"`,
		},
		{
			"bad key reported first in sorted order",
			`{"type":"card","id":"a","lines":["x"],"actions":{"tab":{"emit":"x"},"esc":{"emit":"x"}}}`,
			ErrInvalid, `actions: unknown key "esc"`,
		},
		{
			"letter action with neither",
			`{"type":"card","id":"a","lines":["x"],"actions":{"x":{"label":"ignore"}}}`,
			ErrInvalid, "actions: x: set one of run and emit",
		},
		{
			"label is a number",
			`{"type":"card","id":"a","lines":["x"],"actions":{"x":{"emit":"x","label":5}}}`,
			ErrBadJSON, "label",
		},
		{
			"scope is a number",
			`{"type":"card","id":"a","lines":["x"],"scope":7}`,
			ErrBadJSON, "scope",
		},
		{
			"snapshot card with a bad scope",
			`{"type":"snapshot","cards":[{"id":"a","lines":["x"],"scope":"Muted"}]}`,
			ErrInvalid, `card 1: scope: "Muted"`,
		},
		{"remove without id", `{"type":"remove"}`, ErrInvalid, "id is required"},
		{
			"snapshot with a bad card",
			`{"type":"snapshot","cards":[{"id":"a","lines":["x"]},{"id":"b","lines":[]}]}`,
			ErrInvalid, "card 2: lines",
		},
	}
	sentinels := []error{ErrBadJSON, ErrUnknownType, ErrInvalid}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode([]byte(tc.in))
			if err == nil {
				t.Fatalf("Decode(%s) = %#v, want error %v", tc.in, got, tc.wantErr)
			}
			for _, s := range sentinels {
				if is := errors.Is(err, s); is != (s == tc.wantErr) {
					t.Errorf("Decode(%s) error = %v; errors.Is(%v) = %t", tc.in, err, s, is)
				}
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("Decode(%s) error = %q, want it to mention %q", tc.in, err, tc.wantMsg)
			}
		})
	}
}

func TestDecodeScope(t *testing.T) {
	cases := []struct {
		scope string
		valid bool
	}{
		{"", true},
		{"i", true},
		{"ignored", true},
		{"needs-review", true},
		{"tier2", true},
		{"a-1-", true},
		{strings.Repeat("a", 32), true},
		{strings.Repeat("a", 33), false},
		{"Ignored", false},
		{"ignoreD", false},
		{"2nd", false},
		{"-x", false},
		{"snake_case", false},
		{"a:b", false},
		{"two words", false},
		{"tab\there", false},
		{"åsa", false},
	}
	for _, tc := range cases {
		t.Run(tc.scope, func(t *testing.T) {
			in := fmt.Sprintf(`{"type":"card","id":"a","lines":["x"],"scope":%q}`, tc.scope)
			got, err := Decode([]byte(in))
			if !tc.valid {
				if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "card: scope:") {
					t.Fatalf("Decode(%s) = %#v, %v; want an ErrInvalid scope error", in, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode(%s) error = %v", in, err)
			}
			if c := got.(Card); c.Scope != tc.scope {
				t.Errorf("Decode(%s) scope = %q, want %q", in, c.Scope, tc.scope)
			}
		})
	}
}

func TestDecodeReplacesControlCharacters(t *testing.T) {
	// Every escape-capable character class: ESC (a CSI sequence), CR, LF,
	// tab, NUL, BEL, DEL, and the C1 range (NEL, one-byte CSI, U+009F).
	// JSON escapes, decoded by Decode: each one is a single character.
	hostile := `\u001b[31mred\r\nnext\tcol\u0000\u0007\u007f\u0085\u009b2J\u009f end`
	clean := " [31mred" + "  " + "next" + " " + "col" + strings.Repeat(" ", 5) + "2J" + " " + " end"

	in := `{"type":"card","id":"acme/api#12\u001b","lines":["` + hostile + `",` +
		`[{"text":"` + hostile + `"}]],"match":"` + hostile + `",` +
		`"actions":{"enter":{"emit":"go\u001b","label":"` + hostile + `"},` +
		`"x":{"run":["mute","a\u001b"],"label":"ig\u009bnore"}}}`
	m, err := Decode([]byte(in))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	c := m.(Card)
	got := map[string]string{
		"id":          c.ID,
		"line 1":      c.Lines[0].Text(),
		"line 2 span": c.Lines[1][0].Text,
		"match":       c.Match,
		"emit":        c.Enter.Emit,
		"enter label": c.Enter.Label,
		"x label":     c.Letters["x"].Label,
		"x argv":      c.Letters["x"].Run[1],
	}
	// The id is a key and an argv is executed; neither is drawn, so both stay
	// as sent.
	want := map[string]string{
		"id":          "acme/api#12\x1b",
		"line 1":      clean,
		"line 2 span": clean,
		"match":       clean,
		"emit":        "go ",
		"enter label": clean,
		"x label":     "ig nore",
		"x argv":      "a\x1b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sanitized fields = %q, want %q", got, want)
	}

	h, err := Decode([]byte(`{"type":"header","text":"` + hostile + `"}`))
	if err != nil {
		t.Fatalf("Decode(header) error = %v", err)
	}
	if text := h.(Header).Text.Text(); text != clean {
		t.Errorf("header text = %q, want %q", text, clean)
	}

	r, err := Decode([]byte(`{"type":"remove","id":"acme/api#12\u001b"}`))
	if err != nil {
		t.Fatalf("Decode(remove) error = %v", err)
	}
	if id := r.(Remove).ID; id != c.ID {
		t.Errorf("remove id = %q, want %q to match the card id as sent", id, c.ID)
	}
}

func TestDecodeKeepsPrintableUnicode(t *testing.T) {
	const text = "Åsa fixed the  build ✓ 修复"
	m, err := Decode([]byte(`{"type":"card","id":"a","lines":["` + text + `"]}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got := m.(Card).Lines[0].Text(); got != text {
		t.Errorf("line text = %q, want %q", got, text)
	}
}

func TestDecodeKeepsRunArgvVerbatim(t *testing.T) {
	m, err := Decode([]byte(
		`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"run":["printf","a\tb\n"]}}}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	want := []string{"printf", "a\tb\n"}
	if got := m.(Card).Enter.Run; !reflect.DeepEqual(got, want) {
		t.Errorf("run argv = %q, want %q", got, want)
	}
}

func TestMatchText(t *testing.T) {
	cases := []struct {
		name string
		card Card
		want string
	}{
		{
			name: "match field wins",
			card: Card{Match: "login", Lines: []Line{plain("Fix login redirect")}},
			want: "login",
		},
		{
			name: "lines joined with spaces, spans without",
			card: Card{Lines: []Line{
				plain("Fix login"),
				{{Text: "alice"}, {Text: "@acme"}},
			}},
			want: "Fix login alice@acme",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.card.MatchText(); got != tc.want {
				t.Errorf("MatchText() = %q, want %q", got, tc.want)
			}
		})
	}
}

// FuzzDecode checks the two promises Decode makes on hostile input: it never
// panics, and a message it accepts carries no control character in any text
// it will draw.
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		`{"type":"header","text":[{"text":"\u001b]0;x\u0007","color":"red","bold":true}]}`,
		`{"type":"card","id":"a","lines":["\u009b31m",[{"text":"b"}]],"match":"\r"}`,
		`{"type":"card","id":"a","lines":["x"],"actions":{"enter":{"emit":"\n"}}}`,
		`{"type":"card","id":"a","lines":["x"],"actions":{"x":{"run":["a"],"label":"\u001b"}}}`,
		`{"type":"snapshot","cards":[{"id":"a","lines":["x"],"at":"2026-11-21T10:30:00+01:00"}]}`,
		`{"type":"remove","id":"\u007f"}`,
		`{"type":"clear"}`,
		`{"type":"card","lines":[null]}`,
		"\xff\xfe",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		m, err := Decode(line)
		if err != nil {
			return
		}
		for _, s := range drawnText(m) {
			if strings.ContainsFunc(s, unicode.IsControl) {
				t.Errorf("Decode(%q) kept a control character in %q", line, s)
			}
		}
	})
}

// drawnText lists every string in m that may reach the terminal.
func drawnText(m Message) []string {
	var out []string
	addLine := func(l Line) {
		for _, s := range l {
			out = append(out, s.Text)
		}
	}
	addCard := func(c Card) {
		out = append(out, c.Match)
		for _, l := range c.Lines {
			addLine(l)
		}
		if c.Enter != nil {
			out = append(out, c.Enter.Emit, c.Enter.Label)
		}
		for _, a := range c.Letters {
			out = append(out, a.Emit, a.Label)
		}
	}
	switch m := m.(type) {
	case Header:
		addLine(m.Text)
	case Card:
		addCard(m)
	case Snapshot:
		for _, c := range m.Cards {
			addCard(c)
		}
	}
	return out
}
