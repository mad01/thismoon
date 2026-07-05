package server

import (
	"html"
	"strconv"
	"strings"
)

type DiffFile struct {
	Name   string
	Hunks  []DiffHunk
	Status string
}

type DiffHunk struct {
	Header string
	Lines  []DiffLine
}

type DiffLine struct {
	Type    string // "add", "del", "ctx", "header"
	Content string
	OldNum  int
	NewNum  int
}

func parseDiff(raw string) []DiffFile {
	var files []DiffFile
	var current *DiffFile
	var currentHunk *DiffHunk
	var oldLine, newLine int

	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			if current != nil {
				if currentHunk != nil {
					current.Hunks = append(current.Hunks, *currentHunk)
				}
				files = append(files, *current)
			}
			current = &DiffFile{}
			currentHunk = nil

		case strings.HasPrefix(line, "+++ b/"):
			if current != nil {
				current.Name = strings.TrimPrefix(line, "+++ b/")
			}

		case strings.HasPrefix(line, "--- a/"):
			// old file name, skip

		case strings.HasPrefix(line, "--- /dev/null"):
			if current != nil {
				current.Status = "added"
			}

		case strings.HasPrefix(line, "+++ /dev/null"):
			if current != nil {
				current.Status = "deleted"
			}

		case strings.HasPrefix(line, "@@"):
			if current != nil && currentHunk != nil {
				current.Hunks = append(current.Hunks, *currentHunk)
			}
			oldLine, newLine = parseHunkHeader(line)
			currentHunk = &DiffHunk{Header: line}

		case strings.HasPrefix(line, "+") && currentHunk != nil:
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type: "add", Content: line[1:], NewNum: newLine,
			})
			newLine++

		case strings.HasPrefix(line, "-") && currentHunk != nil:
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type: "del", Content: line[1:], OldNum: oldLine,
			})
			oldLine++

		case currentHunk != nil && len(line) > 0:
			content := line
			if len(content) > 0 && content[0] == ' ' {
				content = content[1:]
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type: "ctx", Content: content, OldNum: oldLine, NewNum: newLine,
			})
			oldLine++
			newLine++
		}
	}
	if current != nil {
		if currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
		}
		files = append(files, *current)
	}
	return files
}

func parseHunkHeader(line string) (oldStart, newStart int) {
	// @@ -oldStart,count +newStart,count @@ — count is omitted for 1.
	oldStart, newStart = 1, 1
	for _, p := range strings.Fields(line) {
		switch {
		case strings.HasPrefix(p, "-"):
			if n := parseRangeStart(p[1:]); n > 0 {
				oldStart = n
			}
		case strings.HasPrefix(p, "+"):
			if n := parseRangeStart(p[1:]); n > 0 {
				newStart = n
			}
		}
	}
	return
}

// parseRangeStart reads the start line from a hunk range field ("12,5" or "12").
func parseRangeStart(field string) int {
	if i := strings.IndexByte(field, ','); i >= 0 {
		field = field[:i]
	}
	n, _ := strconv.Atoi(field)
	return n
}

func renderDiffHTML(files []DiffFile) string {
	var b strings.Builder
	for _, f := range files {
		b.WriteString(`<div class="diff-file">`)
		b.WriteString(
			`<div class="diff-file-header" onclick="this.parentElement.classList.toggle('collapsed')">`,
		)
		b.WriteString(`<span class="diff-file-name">`)
		b.WriteString(html.EscapeString(f.Name))
		b.WriteString(`</span>`)
		b.WriteString(`<span class="diff-file-toggle">▼</span>`)
		b.WriteString(`</div>`)
		b.WriteString(`<div class="diff-file-body">`)
		for _, h := range f.Hunks {
			b.WriteString(`<div class="diff-hunk">`)
			b.WriteString(
				`<div class="diff-line diff-hunk-header"><span class="diff-ln"></span><span class="diff-ln"></span><span class="diff-code">`,
			)
			b.WriteString(html.EscapeString(h.Header))
			b.WriteString(`</span></div>`)
			for _, l := range h.Lines {
				cls := "diff-" + l.Type
				prefix := " "
				switch l.Type {
				case "add":
					prefix = "+"
				case "del":
					prefix = "-"
				}
				b.WriteString(`<div class="diff-line ` + cls + `">`)
				if l.OldNum > 0 {
					b.WriteString(`<span class="diff-ln">` + strconv.Itoa(l.OldNum) + `</span>`)
				} else {
					b.WriteString(`<span class="diff-ln"></span>`)
				}
				if l.NewNum > 0 {
					b.WriteString(`<span class="diff-ln">` + strconv.Itoa(l.NewNum) + `</span>`)
				} else {
					b.WriteString(`<span class="diff-ln"></span>`)
				}
				b.WriteString(
					`<span class="diff-code">` + prefix + html.EscapeString(l.Content) + `</span>`,
				)
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></div>`)
	}
	return b.String()
}
