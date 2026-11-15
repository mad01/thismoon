package reply

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const one = `{"captures":[{"title":"t1","body":"b1","tags":["go"]}]}`
	tests := []struct {
		name    string
		reply   string
		want    Plan
		wantErr string
	}{
		{
			name:  "object alone",
			reply: one,
			want: Plan{
				Captures: []Capture{{Title: "t1", Body: "b1", Tags: []string{"go", "harvest"}}},
			},
		},
		{
			name:  "inside a code fence",
			reply: "```json\n" + one + "\n```\n",
			want: Plan{
				Captures: []Capture{{Title: "t1", Body: "b1", Tags: []string{"go", "harvest"}}},
			},
		},
		{
			name:  "prose around it",
			reply: "Here is the harvest.\n" + one + "\nThat is all.",
			want: Plan{
				Captures: []Capture{{Title: "t1", Body: "b1", Tags: []string{"go", "harvest"}}},
			},
		},
		{
			name:  "nothing to keep",
			reply: `{"captures":[],"dropped":[{"candidate":"a plan","reason":"work state"}]}`,
			want:  Plan{Dropped: []Drop{{Candidate: "a plan", Reason: "work state"}}},
		},
		{
			name:  "an empty object keeps nothing",
			reply: `{}`,
			want:  Plan{},
		},
		{
			name:    "no object",
			reply:   "captured: none",
			wantErr: ErrNoObject.Error(),
		},
		{
			name:    "an object that does not decode",
			reply:   `{"captures": [ {"title": "t" ]}`,
			wantErr: "reply: decode:",
		},
		{
			name:    "a closing brace before the opening one",
			reply:   "} nothing {",
			wantErr: ErrNoObject.Error(),
		},
		{
			name: "a capture needs a title and a body",
			reply: `{"captures":[{"title":"  ","body":"b"},{"title":"t","body":" \n"},
				{"title":"kept","body":"b"}]}`,
			want: Plan{
				Captures: []Capture{{Title: "kept", Body: "b", Tags: []string{"harvest"}}},
				Dropped: []Drop{
					{Candidate: "(untitled)", Reason: "no title"},
					{Candidate: "t", Reason: "no body"},
				},
			},
		},
		{
			name: "tags are lowercased, checked, deduplicated and capped",
			reply: `{"captures":[{"title":"t","body":"b",
				"tags":["Go","go","harvest","c++","two words","kit/llm","a-b","x","y"]}]}`,
			want: Plan{Captures: []Capture{{
				Title: "t", Body: "b", Tags: []string{"go", "kit/llm", "a-b", "harvest"},
			}}},
		},
		{
			name: "source only when it is an http or https URL",
			reply: `{"captures":[
				{"title":"a","body":"b","source":"https://example.com/page"},
				{"title":"c","body":"b","source":"http://example.com"},
				{"title":"d","body":"b","source":"file:///etc/passwd"},
				{"title":"e","body":"b","source":"--title=x"},
				{"title":"f","body":"b","source":"https://"}]}`,
			want: Plan{Captures: []Capture{
				{
					Title:  "a",
					Body:   "b",
					Tags:   []string{"harvest"},
					Source: "https://example.com/page",
				},
				{Title: "c", Body: "b", Tags: []string{"harvest"}, Source: "http://example.com"},
				{Title: "d", Body: "b", Tags: []string{"harvest"}},
				{Title: "e", Body: "b", Tags: []string{"harvest"}},
				{Title: "f", Body: "b", Tags: []string{"harvest"}},
			}},
		},
		{
			name:  "a title reads as one line",
			reply: `{"captures":[{"title":"a\nb\t c","body":"line 1\nline 2"}]}`,
			want: Plan{Captures: []Capture{
				{Title: "a b c", Body: "line 1\nline 2", Tags: []string{"harvest"}},
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.reply)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Parse() error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse() =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

func TestParseCapsTheCaptures(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"captures":[`)
	for i := range 7 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"title":"t` + string(rune('1'+i)) + `","body":"b"}`)
	}
	b.WriteString(`],"dropped":[{"candidate":"c","reason":"r"}]}`)

	got, err := Parse(b.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Captures) != MaxCaptures || got.Captures[4].Title != "t5" {
		t.Errorf("captures = %+v, want the first %d", got.Captures, MaxCaptures)
	}
	want := []Drop{
		{Candidate: "c", Reason: "r"},
		{Candidate: "t6", Reason: "over the cap of 5"},
		{Candidate: "t7", Reason: "over the cap of 5"},
	}
	if !reflect.DeepEqual(got.Dropped, want) {
		t.Errorf("dropped = %+v, want %+v", got.Dropped, want)
	}
}

func TestParseNoObjectIsTheSentinel(t *testing.T) {
	if _, err := Parse("no braces here"); !errors.Is(err, ErrNoObject) {
		t.Errorf("Parse() error = %v, want ErrNoObject", err)
	}
}
