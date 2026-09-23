package chunk

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"blank", "   \n  ", nil},
		{"single no punct", "hello there", []string{"hello there"}},
		{"two sentences", "One. Two.", []string{"One.", "Two."}},
		{"bang and question", "Wow! Really? Yes.", []string{"Wow!", "Really?", "Yes."}},
		{"decimal not split", "Pi is 3.14 today.", []string{"Pi is 3.14 today."}},
		{
			"newline is whitespace",
			"First line.\nSecond line.",
			[]string{"First line.", "Second line."},
		},
		{"trailing space", "Done. ", []string{"Done."}},
		{"collapses gap", "A.    B.", []string{"A.", "B."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitSentences(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitSentences(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSpeakableDropsQuotesAndCollapsesSpace(t *testing.T) {
	got := Speakable("Run `make test`,\n  then say “done”.")
	if want := "Run make test, then say done."; got != want {
		t.Errorf("Speakable = %q, want %q", got, want)
	}
}

// TestGroupFollowsTheRamp pins the part sizes: Live takes one sentence
// first, then fills parts up to each bound in turn, the last bound
// repeating.
func TestGroupFollowsTheRamp(t *testing.T) {
	sentence := strings.Repeat("x", 99) + "." // 100 characters
	pieces := make([]string, 12)
	for i := range pieces {
		pieces[i] = sentence
	}
	got := Group(pieces, Ramp{0, 250, 400})
	// 1 piece; 2 pieces (201 <= 250, 302 > 250); then 3 per part
	// (302 <= 400, 403 > 400), the last bound repeating.
	want := [][]int{{0}, {1, 2}, {3, 4, 5}, {6, 7, 8}, {9, 10, 11}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Group = %v, want %v", got, want)
	}
}

func TestGroupGivesAnOversizedPieceItsOwnPart(t *testing.T) {
	pieces := []string{"Short.", strings.Repeat("y", 300), "Tail."}
	got := Group(pieces, Ramp{250})
	want := [][]int{{0}, {1}, {2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Group = %v, want %v", got, want)
	}
}

func TestJoinTerminatesEachPiece(t *testing.T) {
	tests := []struct {
		name   string
		pieces []string
		want   string
	}{
		{"heading runs into nothing", []string{"Results", "It passed."}, "Results. It passed."},
		{"punctuation kept", []string{"Why?", "Because!", "Note:"}, "Why? Because! Note:"},
		{"closing bracket", []string{"(see below.)", "Next"}, "(see below.) Next."},
		{"blank pieces dropped", []string{" ", "One."}, "One."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Join(tc.pieces); got != tc.want {
				t.Errorf("Join(%q) = %q, want %q", tc.pieces, got, tc.want)
			}
		})
	}
}

// TestPiecesSplitsARunawaySentence: text with no sentence breaks (a long
// list pasted as one line, say) still comes out in parts a provider can
// answer.
func TestPiecesSplitsARunawaySentence(t *testing.T) {
	text := strings.TrimSpace(strings.Repeat("word ", 400)) // 1999 characters
	pieces := Pieces(text)
	if len(pieces) < 4 {
		t.Fatalf("got %d pieces, want the sentence split", len(pieces))
	}
	for i, p := range pieces {
		if n := utf8.RuneCountInString(p); n > MaxChars {
			t.Errorf("piece %d has %d characters, want at most %d", i, n, MaxChars)
		}
	}
	if got := strings.Join(pieces, " "); got != text {
		t.Error("pieces do not rejoin into the original text")
	}
}

func TestPartsStartWithOneSentence(t *testing.T) {
	got := Parts("Hello there. This is the second. And a third", Live)
	want := []string{"Hello there.", "This is the second. And a third."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parts = %q, want %q", got, want)
	}
}
