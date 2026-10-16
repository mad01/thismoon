package outline

import (
	"reflect"
	"testing"
)

var claudeSpec = Spec{
	Title: TitleNameComma,
	Sections: []Section{
		{Heading: "Module layout", Required: true},
		{Heading: "How it works", Required: true},
		{Heading: "Commands"},
		{Heading: "Gotchas", Required: true},
		{Heading: "See also", Required: true},
	},
}

func TestCheckAcceptsADocOnTheOutline(t *testing.T) {
	doc := "# events, local event log\n\n## Module layout\n\n## How it works\n\n" +
		"## Gotchas\n\n## See also\n"
	if got := Check(doc, "events", claudeSpec); len(got) != 0 {
		t.Errorf("Check() = %v, want no problems", got)
	}
}

func TestCheckFlagsEachDeviationOnce(t *testing.T) {
	doc := "# events: local event log\n" +
		"## How it works\n## Module layout\n## Configuration\n## Commands\n" +
		"## Commands\n## Gotchas\n"
	want := []Problem{
		{Kind: KindTitle, Heading: "events: local event log"},
		{Kind: KindUnknown, Heading: "Configuration"},
		{Kind: KindDuplicate, Heading: "Commands"},
		{Kind: KindOrder, Heading: "Module layout"},
		{Kind: KindMissing, Heading: "See also"},
	}
	if got := Check(doc, "events", claudeSpec); !reflect.DeepEqual(got, want) {
		t.Errorf("Check() =\n %v\nwant\n %v", got, want)
	}
}

func TestHeadingsSkipFencedBlocks(t *testing.T) {
	doc := "# toss-bin\n```yaml\n# ~/.config/toss-bin/config.yaml\n## not a heading\n```\n" +
		"## Install\n~~~\n## also not\n~~~\n## Usage\n"
	title, sections := Headings(doc)
	if title != "toss-bin" {
		t.Errorf("title = %q, want toss-bin", title)
	}
	if want := []string{"Install", "Usage"}; !reflect.DeepEqual(sections, want) {
		t.Errorf("sections = %v, want %v", sections, want)
	}
}

func TestTitleRules(t *testing.T) {
	tests := []struct {
		title, component, rule string
		want                   bool
	}{
		{"keeper-of-facts (kof), assertion store", "keeper-of-facts", TitleNameComma, true},
		{"events, CLI + MCP", "events", TitleNameComma, true},
		{"events", "events", TitleNameComma, false},
		{"events,", "events", TitleNameComma, false},
		{"eventsd, log", "events", TitleNameComma, false},
		{"events", "events", TitleName, true},
		{"events, log", "events", TitleName, false},
	}
	for _, tt := range tests {
		if got := titleOK(tt.title, tt.component, tt.rule); got != tt.want {
			t.Errorf("titleOK(%q, %q, %q) = %v, want %v",
				tt.title, tt.component, tt.rule, got, tt.want)
		}
	}
}
