package scorecard

import (
	"encoding/json"
	"regexp"
	"strings"

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
	return headString(head, "file_path"), headRange.MatchString(head)
}

// inputHead is the head a subagent's transcript cut an input to; false
// when the input is not a string.
func inputHead(raw json.RawMessage) (string, bool) {
	var head string
	return head, json.Unmarshal(raw, &head) == nil
}

// headKeys match the string value of each input key a scorecard reads
// from a head: the file keys of fileTools, and csl_read's file and repo.
var headKeys = func() map[string]*regexp.Regexp {
	out := map[string]*regexp.Regexp{}
	for _, key := range []string{"file_path", "notebook_path", "file", "repo"} {
		out[key] = regexp.MustCompile(`"` + key + `":\s*("(?:[^"\\]|\\.)*")`)
	}
	return out
}()

// headString reads the string value of key from the head of a JSON
// object, "" when the head does not hold it whole or key is not one of
// headKeys.
func headString(head, key string) string {
	re, ok := headKeys[key]
	if !ok {
		return ""
	}
	m := re.FindStringSubmatch(head)
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

// touchedPath is a file a call read or edited, for joining hints to what
// the session did after them. Repo is the csl repo a csl_read named, ""
// for a path on disk.
type touchedPath struct {
	path, repo string
	call       Call
}

// fileTools are the tools whose input names one file on disk, by the key
// that holds it.
var fileTools = map[string]string{
	"Read": "file_path", "Edit": "file_path", "MultiEdit": "file_path",
	"Write": "file_path", "NotebookEdit": "notebook_path",
}

// touched lists the files the calls read or edited, in call order: the
// harness's file tools and csl_read. An input a subagent's transcript cut
// to its head is read for the keys the head still holds.
func touched(calls []Call) []touchedPath {
	var out []touchedPath
	for _, c := range calls {
		if c.IsError {
			continue
		}
		if key, ok := fileTools[c.Name]; ok {
			if p := inputString(c.Input, key); p != "" {
				out = append(out, touchedPath{path: p, call: c})
			}
			continue
		}
		if strings.HasPrefix(c.Name, "mcp__") && toolName(c.Name) == "csl_read" {
			if file := inputString(c.Input, "file"); file != "" {
				out = append(out, touchedPath{
					path: file, repo: inputString(c.Input, "repo"), call: c,
				})
			}
		}
	}
	return out
}

// inputString is the string value of key in a call's input, whole or cut
// to its head; "" when it holds none.
func inputString(raw json.RawMessage, key string) string {
	var in map[string]any
	if json.Unmarshal(raw, &in) == nil {
		s, _ := in[key].(string)
		return s
	}
	if head, ok := inputHead(raw); ok {
		return headString(head, key)
	}
	return ""
}
