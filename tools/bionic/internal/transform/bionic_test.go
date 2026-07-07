package transform

import "testing"

func TestBoldWord(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a", "**a**"},
		{"to", "**t**o"},
		{"the", "**th**e"},
		{"word", "**wo**rd"},
		{"hello", "**hel**lo"},
		{"infrastructure", "**infrast**ructure"},
	}
	for _, tt := range tests {
		got := boldWord([]rune(tt.in))
		if got != tt.want {
			t.Errorf("boldWord(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBionic(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "simple sentence",
			in:   "The quick brown fox",
			want: "**Th**e **qui**ck **bro**wn **fo**x",
		},
		{
			name: "preserves numbers",
			in:   "May 2026 was rough with 10 incidents",
			want: "**Ma**y 2026 **wa**s **rou**gh **wi**th 10 **incid**ents",
		},
		{
			name: "preserves code blocks",
			in:   "text\n```\ncode here\n```\nmore text",
			want: "**te**xt\n```\ncode here\n```\n**mo**re **te**xt",
		},
		{
			name: "preserves inline code",
			in:   "run `kubectl get pods` now",
			want: "**ru**n `kubectl get pods` **no**w",
		},
		{
			name: "preserves existing bold",
			in:   "**INCIDENT-23639** was bad",
			want: "**INCIDENT-23639** **wa**s **ba**d",
		},
		{
			name: "preserves link URLs",
			in:   "see [review doc](https://example.com) for details",
			want: "**se**e [**rev**iew **do**c](https://example.com) **fo**r **deta**ils",
		},
		{
			name: "preserves headers",
			in:   "## The Big Picture",
			want: "## **Th**e **Bi**g **Pict**ure",
		},
		{
			name: "empty line passthrough",
			in:   "first\n\nsecond",
			want: "**fir**st\n\n**sec**ond",
		},
		{
			name: "italic markers preserved",
			in:   "an *italic* word",
			want: "**a**n *italic* **wo**rd",
		},
		{
			name: "underscore italic preserved",
			in:   "an _italic_ word",
			want: "**a**n _italic_ **wo**rd",
		},
		{
			name: "apostrophe in word",
			in:   "don't stop",
			want: "**don**'t **st**op",
		},
		{
			name: "single character word",
			in:   "I am a test",
			want: "**I** **a**m **a** **te**st",
		},
		{
			name: "unicode accented chars",
			in:   "café résumé naïve",
			want: "**ca**fé **rés**umé **naï**ve",
		},
		{
			name: "blockquote",
			in:   "> quoted text here",
			want: "> **quo**ted **te**xt **he**re",
		},
		{
			name: "URL with parens",
			in:   "see [wiki](https://en.wikipedia.org/wiki/Bionic_(technique)) here",
			want: "**se**e [**wi**ki](https://en.wikipedia.org/wiki/Bionic_(technique)) **he**re",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "multiple code blocks",
			in:   "before\n```\nfirst\n```\nmiddle\n```\nsecond\n```\nafter",
			want: "**bef**ore\n```\nfirst\n```\n**mid**dle\n```\nsecond\n```\n**aft**er",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Bionic(tt.in)
			if got != tt.want {
				t.Errorf("Bionic(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}
