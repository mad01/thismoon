package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/search"
)

// TestOutputSearchResults_ContentMergesBlocks: content mode prints one header
// per file, nearby hits as one block with each line once, `--` between gaps,
// and a blank line between files; a match line carries its column and a
// sym: hit keeps its kind marker.
func TestOutputSearchResults_ContentMergesBlocks(t *testing.T) {
	prevMode, prevJSON, prevToon := searchOutputModeFlag, searchJSONFlag, searchToonFlag
	t.Cleanup(func() {
		searchOutputModeFlag, searchJSONFlag, searchToonFlag = prevMode, prevJSON, prevToon
	})
	searchOutputModeFlag, searchJSONFlag, searchToonFlag = "content", false, false

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	err := outputSearchResults(cmd, []search.Match{
		{
			Repo: "org/repo", File: "a.go", Line: 5, Column: 1, Text: "five",
			Before: "three\nfour\n", After: "six\nseven\n",
		},
		{
			Repo: "org/repo", File: "a.go", Line: 7, Column: 3, Text: "seven",
			Before: "five\nsix\n", After: "eight\nnine\n",
		},
		{
			Repo:   "org/repo",
			File:   "a.go",
			Line:   20,
			Column: 1,
			Text:   "func Twenty() {",
			Kind:   "function",
		},
		{
			Repo:   "org/repo",
			File:   "b.go",
			Line:   1,
			Column: 1,
			Text:   "\tindented",
			Before: "",
			After:  "",
		},
	})
	if err != nil {
		t.Fatalf("outputSearchResults: %v", err)
	}
	want := "org/repo/a.go\n" +
		"3-three\n" +
		"4-four\n" +
		"5:1:five\n" +
		"6-six\n" +
		"7:3:seven\n" +
		"8-eight\n" +
		"9-nine\n" +
		"--\n" +
		"20:1:func Twenty() {  kind=function\n" +
		"\n" +
		"org/repo/b.go\n" +
		"1:1:\tindented\n"
	if got := buf.String(); got != want {
		t.Errorf("content output differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestOutputSearchResults_FilesUnchanged: files_with_matches still lists each
// repo/path once, untouched by the content-mode block merge.
func TestOutputSearchResults_FilesUnchanged(t *testing.T) {
	prevMode, prevJSON, prevToon := searchOutputModeFlag, searchJSONFlag, searchToonFlag
	t.Cleanup(func() {
		searchOutputModeFlag, searchJSONFlag, searchToonFlag = prevMode, prevJSON, prevToon
	})
	searchOutputModeFlag, searchJSONFlag, searchToonFlag = "files_with_matches", false, false

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	err := outputSearchResults(cmd, []search.Match{
		{Repo: "org/repo", File: "a.go", Line: 5, Text: "five", After: "six\n"},
		{Repo: "org/repo", File: "a.go", Line: 7, Text: "seven"},
		{Repo: "org/repo", File: "b.go", Line: 1, Text: "one"},
	})
	if err != nil {
		t.Fatalf("outputSearchResults: %v", err)
	}
	if got, want := buf.String(), "org/repo/a.go\norg/repo/b.go\n"; got != want {
		t.Errorf("files output = %q, want %q", got, want)
	}
}
