// Package ask is csl's bulk reader: a question over the files a lexical
// search selected, answered by a cheap hosted worker model, so the files
// never enter the calling agent's context and only the answer comes back.
//
// It owns the worker's instructions and message format, the corpus caps,
// the worker calls, and their list price. The search that selects the files
// stays with the search surfaces (internal/mcpserver runs it the way
// csl_search_multi does), and the backend comes from kit/llm. The csl_ask
// eval (services/csl/evals/ask) imports the prompt and the message format
// from here, so a measured result stays tied to the wording the tool sends.
package ask

import (
	"fmt"
	"strings"
)

// SystemPrompt is the worker's instructions. The eval records a hash of it
// with every run, so a change here is a change the eval should measure.
const SystemPrompt = `You are a precise code analyst. You receive one or more source files, each inside a <file path="..."> tag with every line prefixed by its line number, and one question. Answer from the files alone.

Rules:
- Output structured bullets only. No greeting, no preamble, no summary, no closing line.
- Lead every bullet with the exact identifier, value, or path:line it is about.
- Cite a line number only as the number printed at the start of that line.
- When the files do not contain the answer, say so in one bullet. Never invent a name, value, or line.
- Include only what the question asks for, and keep it short.`

// File is one file as the worker reads it: the path it cites the file by,
// and the file's lines without their line breaks.
type File struct {
	Path  string
	Lines []string
}

// Message is the user message the worker reads: each file inside a tag
// naming its path and length, every line prefixed with its number, then the
// question. The prefix is what makes a path:line claim checkable.
func Message(question string, files []File) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "<file path=%q lines=\"%d\">\n", f.Path, len(f.Lines))
		for i, line := range f.Lines {
			fmt.Fprintf(&b, "%d: %s\n", i+1, line)
		}
		b.WriteString("</file>\n\n")
	}
	fmt.Fprintf(&b, "Question: %s\n", question)
	return b.String()
}

// SplitLines splits file content into the lines Message numbers. Trailing
// newlines are dropped first, so a file ending in one has no empty last
// line; an empty file is one empty line.
func SplitLines(data []byte) []string {
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}
