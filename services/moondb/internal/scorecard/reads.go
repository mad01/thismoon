package scorecard

import (
	"encoding/json"
	"regexp"

	"github.com/mad01/thismoon/services/moondb"
)

// readTool is the harness's file read.
const readTool = "Read"

// readInput is the part of a Read call's input a scorecard reads. A range
// is an offset, a limit or a page range.
type readInput struct {
	FilePath string          `json:"file_path"`
	Offset   json.RawMessage `json:"offset"`
	Limit    json.RawMessage `json:"limit"`
	Pages    json.RawMessage `json:"pages"`
}

var (
	// numberedLine is a line of a Read result: its line number, a tab,
	// the text.
	numberedLine = regexp.MustCompile(`(?m)^\s*\d+\t`)
	// headRange reads the range keys of an input a subagent's transcript
	// cut to a string.
	headRange = regexp.MustCompile(`"(offset|limit|pages)":\s*[^n\s]`)
)

// parseReadInput returns the file a Read asked for and whether it asked
// for a range. An input cut to its head is read for the keys it still
// holds.
func parseReadInput(raw json.RawMessage) (path string, ranged bool) {
	var in readInput
	if json.Unmarshal(raw, &in) == nil {
		return in.FilePath, present(in.Offset) || present(in.Limit) || present(in.Pages)
	}
	head, ok := inputHead(raw)
	if !ok {
		return "", false
	}
	return headFilePath(head), headRange.MatchString(head)
}

// inputHead is the head a subagent's transcript cut an input to; false
// when the input is not a string.
func inputHead(raw json.RawMessage) (string, bool) {
	var head string
	return head, json.Unmarshal(raw, &head) == nil
}

// headFilePathKey matches the string value of a Read input's file_path in
// a head.
var headFilePathKey = regexp.MustCompile(`"file_path":\s*("(?:[^"\\]|\\.)*")`)

// headFilePath reads the file_path value from the head of a JSON object,
// "" when the head does not hold it whole.
func headFilePath(head string) string {
	m := headFilePathKey.FindStringSubmatch(head)
	if m == nil {
		return ""
	}
	var s string
	if json.Unmarshal([]byte(m[1]), &s) != nil {
		return ""
	}
	return s
}

// wholeFile reports whether a Read returned the whole file. The tool's own
// result says so when the transcript holds it: it started at line 1 and
// returned every line. Otherwise a read without an offset, a limit or a
// page range counts as whole.
func wholeFile(c Call, ranged bool) bool {
	if c.StartLine >= 0 && c.Lines >= 0 && c.TotalLines >= 0 {
		return c.StartLine <= 1 && c.Lines >= c.TotalLines
	}
	return !ranged
}

// readLines is how many lines a Read returned: the tool's own count, else
// the numbered lines of a result the transcript kept whole; -1 when
// neither says.
func readLines(c Call) int {
	switch {
	case c.Lines >= 0:
		return c.Lines
	case c.Answered && !c.Cut:
		return len(numberedLine.FindAllStringIndex(c.Result, -1))
	}
	return unknownLines
}

// readStats counts the Read calls among calls.
func readStats(calls []Call) ReadStats {
	s := ReadStats{LargeLines: moondb.LargeReadLines}
	for _, c := range calls {
		if c.Name != readTool {
			continue
		}
		s.Calls++
		if c.IsError {
			s.Errors++
			continue
		}
		path, ranged := parseReadInput(c.Input)
		whole := wholeFile(c, ranged)
		if whole {
			s.Unranged++
		}
		lines := readLines(c)
		if lines < 0 {
			s.Unsized++
			continue
		}
		s.Lines += lines
		if whole && lines >= moondb.LargeReadLines {
			s.Large++
		}
		if s.Largest == nil || lines > s.Largest.Lines {
			s.Largest = &ReadCall{
				Path: path, Lines: lines, Ranged: !whole, AgentID: c.AgentID,
				TotalLines: max(c.TotalLines, 0),
			}
		}
	}
	return s
}
