package patch

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// fixture is a before/after pair with the hunks the harness reports for it,
// shaped like structuredPatch: three lines of context, jsdiff's marker for a
// missing final newline.
type fixture struct {
	name   string
	before string
	after  string
	hunks  []Hunk
}

var fixtures = []fixture{
	{
		name:   "edit in the middle",
		before: "a\nb\nc\nd\ne\nf\ng\n",
		after:  "a\nb\nc\nD\ne\nf\ng\n",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 7, NewStart: 1, NewLines: 7,
			Lines: []string{" a", " b", " c", "-d", "+D", " e", " f", " g"},
		}},
	},
	{
		name:   "two hunks with a gap",
		before: "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n",
		after:  "one\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\ntwelve\nthirteen\n",
		hunks: []Hunk{
			{
				OldStart: 1, OldLines: 4, NewStart: 1, NewLines: 4,
				Lines: []string{"-1", "+one", " 2", " 3", " 4"},
			},
			{
				OldStart: 9, OldLines: 4, NewStart: 9, NewLines: 5,
				Lines: []string{" 9", " 10", " 11", "-12", "+twelve", "+thirteen"},
			},
		},
	},
	{
		name:   "hunk ends at end of file",
		before: "a\nb\nc\nd\n",
		after:  "a\nb\nc\nd\ne\n",
		hunks: []Hunk{{
			OldStart: 2, OldLines: 3, NewStart: 2, NewLines: 4,
			Lines: []string{" b", " c", " d", "+e"},
		}},
	},
	{
		name:   "create from empty, start 0",
		before: "",
		after:  "package main\n\nfunc main() {}\n",
		hunks: []Hunk{{
			OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 3,
			Lines: []string{"+package main", "+", "+func main() {}"},
		}},
	},
	{
		name:   "create from empty, start 1",
		before: "",
		after:  "x\ny\n",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 0, NewStart: 1, NewLines: 2,
			Lines: []string{"+x", "+y"},
		}},
	},
	{
		name:   "remove everything",
		before: "a\nb\n",
		after:  "",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0,
			Lines: []string{"-a", "-b"},
		}},
	},
	{
		name:   "old side has no final newline",
		before: "a\nb",
		after:  "a\nb\nc\n",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3,
			Lines: []string{" a", "-b", `\ No newline at end of file`, "+b", "+c"},
		}},
	},
	{
		name:   "new side has no final newline",
		before: "a\nb\n",
		after:  "a\nB",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []string{" a", "-b", "+B", `\ No newline at end of file`},
		}},
	},
	{
		name:   "neither side has a final newline",
		before: "a\nb\nc",
		after:  "A\nb\nc",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []string{"-a", "+A", " b", " c", `\ No newline at end of file`},
		}},
	},
	{
		name:   "crlf file, hunks without carriage returns",
		before: "a\r\nb\r\nc\r\n",
		after:  "a\r\nB\r\nb2\r\nc\r\n",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4,
			Lines: []string{" a", "-b", "+B", "+b2", " c"},
		}},
	},
	{
		name:   "crlf file, hunks with carriage returns",
		before: "a\r\nb\r\nc\r\n",
		after:  "a\r\nB\r\nc\r\n",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []string{" a\r", "-b\r", "+B\r", " c\r"},
		}},
	},
	{
		name:   "crlf create",
		before: "",
		after:  "x\r\ny\r\n",
		hunks: []Hunk{{
			OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2,
			Lines: []string{"+x\r", "+y\r"},
		}},
	},
	{
		name:   "mixed endings keep untouched lines as they were",
		before: "a\r\nb\r\nc\n",
		after:  "a\r\nB\r\nc\n",
		hunks: []Hunk{{
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []string{" a", "-b", "+B", " c"},
		}},
	},
}

// hunksOf returns the hunks of the named fixture.
func hunksOf(t *testing.T, name string) []Hunk {
	t.Helper()
	for _, f := range fixtures {
		if f.name == name {
			return f.hunks
		}
	}
	t.Fatalf("no fixture %q", name)
	return nil
}

func TestApply(t *testing.T) {
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			got, err := Apply([]byte(f.before), f.hunks)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if string(got) != f.after {
				t.Errorf("Apply = %q, want %q", got, f.after)
			}
		})
	}
}

func TestReverse(t *testing.T) {
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			got, err := Reverse([]byte(f.after), f.hunks)
			if err != nil {
				t.Fatalf("Reverse: %v", err)
			}
			if string(got) != f.before {
				t.Errorf("Reverse = %q, want %q", got, f.before)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			after, err := Apply([]byte(f.before), f.hunks)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			back, err := Reverse(after, f.hunks)
			if err != nil {
				t.Fatalf("Reverse: %v", err)
			}
			if string(back) != f.before {
				t.Errorf("Reverse(Apply(b)) = %q, want %q", back, f.before)
			}
		})
	}
}

func TestApplyMismatch(t *testing.T) {
	edit := hunksOf(t, "edit in the middle")
	tests := []struct {
		name   string
		before string
		hunks  []Hunk
		want   MismatchError
	}{
		{
			name:   "context differs",
			before: "a\nb\nX\nd\ne\nf\ng\n",
			hunks:  edit,
			want:   MismatchError{Hunk: 0, Line: 3, Expected: "c\n", Got: "X\n"},
		},
		{
			name:   "removed line differs",
			before: "a\nb\nc\nzz\ne\nf\ng\n",
			hunks:  edit,
			want:   MismatchError{Hunk: 0, Line: 4, Expected: "d\n", Got: "zz\n"},
		},
		{
			name:   "content ends early",
			before: "a\nb\nc\nd\n",
			hunks:  edit,
			want:   MismatchError{Hunk: 0, Line: 5, Expected: "e\n", Got: ""},
		},
		{
			name:   "second hunk fails",
			before: "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n99\n",
			hunks:  hunksOf(t, "two hunks with a gap"),
			want:   MismatchError{Hunk: 1, Line: 12, Expected: "12\n", Got: "99\n"},
		},
		{
			name:   "marker says no newline, content has one",
			before: "a\nb\n",
			hunks:  hunksOf(t, "old side has no final newline"),
			want:   MismatchError{Hunk: 0, Line: 2, Expected: "b", Got: "b\n"},
		},
		{
			name:   "hunk ends the file, content goes on",
			before: "a\nb\nc\nd\nextra\n",
			hunks:  hunksOf(t, "hunk ends at end of file"),
			want:   MismatchError{Hunk: 0, Line: 5, Expected: "", Got: "extra\n"},
		},
		{
			name:   "content lacks the newline the hunk expects",
			before: "a\nb\nc\nd",
			hunks:  hunksOf(t, "hunk ends at end of file"),
			want:   MismatchError{Hunk: 0, Line: 4, Expected: "d\n", Got: "d"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Apply([]byte(tt.before), tt.hunks)
			var mm *MismatchError
			if !errors.As(err, &mm) {
				t.Fatalf("Apply error = %v, want *MismatchError", err)
			}
			if *mm != tt.want {
				t.Errorf("mismatch = %+v, want %+v", *mm, tt.want)
			}
		})
	}
}

func TestReverseMismatch(t *testing.T) {
	// The added line is not in the content, so this is not the patch's result.
	_, err := Reverse([]byte("a\nb\nc\nd\ne\nf\ng\n"), hunksOf(t, "edit in the middle"))
	var mm *MismatchError
	if !errors.As(err, &mm) {
		t.Fatalf("Reverse error = %v, want *MismatchError", err)
	}
	want := MismatchError{Hunk: 0, Line: 4, Expected: "D\n", Got: "d\n"}
	if *mm != want {
		t.Errorf("mismatch = %+v, want %+v", *mm, want)
	}
}

func TestApplyRejectsOverlappingHunks(t *testing.T) {
	hunks := []Hunk{
		{OldStart: 2, OldLines: 2, NewStart: 2, NewLines: 2, Lines: []string{" b", "-c", "+C"}},
		{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []string{"-a", "+A"}},
	}
	_, err := Apply([]byte("a\nb\nc\n"), hunks)
	var mm *MismatchError
	if err == nil || errors.As(err, &mm) {
		t.Fatalf("Apply error = %v, want an ordering error", err)
	}
}

func TestParse(t *testing.T) {
	raw := `[{"oldStart":3,"oldLines":3,"newStart":3,"newLines":4,` +
		`"lines":[" x","-y","+Y","+z"," w"]}]`
	got, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Hunk{{
		OldStart: 3, OldLines: 3, NewStart: 3, NewLines: 4,
		Lines: []string{" x", "-y", "+Y", "+z", " w"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
}

func TestParseNull(t *testing.T) {
	got, err := Parse([]byte("null"))
	if err != nil || got != nil {
		t.Errorf("Parse(null) = %v, %v; want nil, nil", got, err)
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"not json", `{"oldStart":`},
		{"not an array", `{"oldStart":1}`},
		{
			"count disagrees",
			`[{"oldStart":1,"oldLines":2,"newStart":1,"newLines":1,"lines":["-a","+b"]}]`,
		},
		{
			"unknown prefix",
			`[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":["*a"]}]`,
		},
		{"empty line", `[{"oldStart":1,"oldLines":0,"newStart":1,"newLines":0,"lines":[""]}]`},
		{
			"marker first",
			`[{"oldStart":0,"oldLines":0,"newStart":0,"newLines":0,"lines":["\\ x"]}]`,
		},
		{
			"old lines at line 0",
			`[{"oldStart":0,"oldLines":1,"newStart":0,"newLines":0,"lines":["-a"]}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.raw)); err == nil {
				t.Errorf("Parse(%s) succeeded, want an error", tt.raw)
			}
		})
	}
}

func TestDetectEOL(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"", ""},
		{"no newline", ""},
		{"a\nb\n", "lf"},
		{"a\r\nb\r\n", "crlf"},
		{"a\r\nb\n", "mixed"},
		{"a\rb", ""},
	}
	for _, tt := range tests {
		if got := DetectEOL([]byte(tt.content)); got != tt.want {
			t.Errorf("DetectEOL(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestAddedLines(t *testing.T) {
	got := AddedLines(
		slices.Concat(
			hunksOf(t, "two hunks with a gap"),
			hunksOf(t, "crlf file, hunks with carriage returns"),
		),
	)
	want := []string{"one", "twelve", "thirteen", "B"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AddedLines = %q, want %q", got, want)
	}
}

func TestTouches(t *testing.T) {
	hunks := slices.Concat(hunksOf(t, "two hunks with a gap"), hunksOf(t, "remove everything"))
	tests := []struct {
		line int
		want bool
	}{
		{0, false},
		{1, true},
		{4, true},
		{5, false},
		{8, false},
		{9, true},
		{13, true},
		{14, false},
	}
	for _, tt := range tests {
		if got := Touches(hunks, tt.line); got != tt.want {
			t.Errorf("Touches(%d) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
