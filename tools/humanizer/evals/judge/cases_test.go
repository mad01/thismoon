package main

import (
	"bytes"
	"strings"
	"testing"
)

const sampleCase = "---\n" +
	"id: human-sample\n" +
	"label: likely_human\n" +
	"bucket: human\n" +
	"source: https://example.com/readme#intro, commit abc (2019-01-02)\n" +
	"generator: \"\"\n" +
	"words: 999\n" +
	"notes: a colon: inside the notes\n" +
	"---\n" +
	"One two three four five six seven eight nine ten eleven twelve thirteen fourteen " +
	"fifteen sixteen seventeen eighteen nineteen twenty twenty-one.\n"

func TestParseCase(t *testing.T) {
	c, err := parseCase(sampleCase)
	if err != nil {
		t.Fatalf("parseCase() error = %v", err)
	}
	if c.ID != "human-sample" || c.Label != "likely_human" || c.Bucket != "human" {
		t.Errorf("fields = %+v", c)
	}
	if !strings.HasPrefix(c.Source, "https://example.com/readme#intro, commit abc") {
		t.Errorf("source lost its colon: %q", c.Source)
	}
	if c.Notes != "a colon: inside the notes" {
		t.Errorf("notes = %q", c.Notes)
	}
	if c.Generator != "" {
		t.Errorf("generator = %q, want empty after quote stripping", c.Generator)
	}
	if c.Words != 21 {
		t.Errorf("words = %d, want 21 (recomputed, not the frontmatter's 999)", c.Words)
	}
	if !c.Scored() {
		t.Error("human case should be scored")
	}
}

func TestParseCaseErrors(t *testing.T) {
	cases := map[string]string{
		"no fence":       "id: x\nlabel: likely_ai\n",
		"unclosed fence": "---\nid: x\nlabel: likely_ai\nbucket: ai\n",
		"bad label":      strings.Replace(sampleCase, "likely_human", "human", 1),
		"bad bucket":     strings.Replace(sampleCase, "bucket: human", "bucket: people", 1),
		"unknown not ambig": strings.Replace(
			sampleCase,
			"label: likely_human",
			"label: unknown",
			1,
		),
		"too short": strings.SplitN(sampleCase, "---\nOne", 2)[0] + "---\nOne two three.\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCase(raw); err == nil {
				t.Fatal("parseCase() succeeded, want error")
			}
		})
	}
}

func TestFenceFor(t *testing.T) {
	if got := fenceFor("plain text"); got != "```" {
		t.Errorf("fenceFor(plain) = %q", got)
	}
	if got := fenceFor("has a ``` fence inside"); got != "````" {
		t.Errorf("fenceFor(with fence) = %q", got)
	}
}

func TestWriteReview(t *testing.T) {
	c, err := parseCase(sampleCase)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writeReview(&buf, []Case{c})
	out := buf.String()
	for _, want := range []string{"| human-sample | human | train | likely_human | 21 |", "### human-sample", "```\nOne two"} {
		if !strings.Contains(out, want) {
			t.Errorf("review missing %q:\n%s", want, out)
		}
	}
}
