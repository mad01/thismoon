package hint

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/mcptool"
)

// ReadTotalLines reads the whole file's line count from a Read tool
// response: file.totalLines on a response of type "text". Any other shape
// returns 0, which callers read as unknown: an image, notebook or PDF read,
// a missing field, a field of another JSON type, or no JSON object at all.
func ReadTotalLines(raw json.RawMessage) int {
	var resp struct {
		Type string `json:"type"`
		File *struct {
			TotalLines int `json:"totalLines"`
		} `json:"file"`
	}
	if json.Unmarshal(raw, &resp) != nil || resp.Type != "text" || resp.File == nil {
		return 0
	}
	return max(resp.File.TotalLines, 0)
}

// cslReadOperation is the operation segment of csl's file-read MCP tool.
// The server segment is the consuming repo's registration name, so the
// check reads the operation alone.
const cslReadOperation = "csl_read"

// IsCSLRead reports whether tool is csl's csl_read MCP tool.
func IsCSLRead(tool string) bool {
	_, op, ok := mcptool.Split(tool)
	return ok && op == cslReadOperation
}

// cslReadHeader is the first line of csl_read's text response,
// `org/repo/path lines A-B of N`. The name runs up to the last " lines ",
// so a path holding a space still parses.
var cslReadHeader = regexp.MustCompile(`^(.+) lines \d+-\d+ of (\d+)$`)

// CSLFile names the file a csl_read returned. The text form's header joins
// repo and path into Name (org/repo/path); csl names a repo after its
// origin remote, which can hold more than one slash, so only the csl index
// can say where the repo ends. The json forms state Repo and Path apart,
// and they stay apart so no split can move that boundary. Either Name or
// Repo and Path are set, or none when the response was not understood.
type CSLFile struct {
	Name       string
	Repo, Path string
}

// Known reports whether the response named a file in either shape.
func (f CSLFile) Known() bool { return f.Name != "" || (f.Repo != "" && f.Path != "") }

// CSLReadFile reads, from a csl_read tool response, the file it returned
// and the whole file's line count. The text csl wrote is either the default
// text form, opening with the header line `org/repo/path lines A-B of N`,
// or the json form, an object with repo, path and total_lines. It parses
// from three shapes:
//
//   - content blocks, whose first text block holds either form. A text
//     form result arrives this way.
//   - a JSON string holding either form. For the json format csl also
//     returns structuredContent, and Claude Code records a result that
//     carries it as one string of JSON (seen in session transcripts, which
//     store a Read result in the shape its hook payload has).
//   - the json form's object itself, in case a hook receives it decoded.
//
// Any other shape, another response format included, returns a zero
// CSLFile and 0, which callers read as unknown.
func CSLReadFile(raw json.RawMessage) (CSLFile, int) {
	text, ok := responseText(raw)
	switch {
	case !ok:
		return cslReadObject(raw)
	case strings.HasPrefix(text, "{"):
		return cslReadObject([]byte(text))
	default:
		return cslReadText(text)
	}
}

// cslReadText reads the text form's header line.
func cslReadText(text string) (CSLFile, int) {
	header, _, _ := strings.Cut(text, "\n")
	m := cslReadHeader.FindStringSubmatch(header)
	if m == nil {
		return CSLFile{}, 0
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return CSLFile{}, 0
	}
	return CSLFile{Name: m[1]}, n
}

// responseText returns the text an MCP tool response carries: the response
// itself when it is a JSON string, else its first text block.
func responseText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	return firstTextBlock(raw)
}

// firstTextBlock returns the text of the first block of type "text" in an
// MCP tool response's content array.
func firstTextBlock(raw json.RawMessage) (string, bool) {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text, true
		}
	}
	return "", false
}

// cslReadObject reads csl_read's json form, from whichever shape carried
// it: repo and path as csl states them, and total_lines.
// An object without both names reads as unknown.
func cslReadObject(raw []byte) (CSLFile, int) {
	var out struct {
		Repo       string `json:"repo"`
		Path       string `json:"path"`
		TotalLines int    `json:"total_lines"`
	}
	if json.Unmarshal(raw, &out) != nil || out.Repo == "" || out.Path == "" {
		return CSLFile{}, 0
	}
	return CSLFile{Repo: out.Repo, Path: out.Path}, max(out.TotalLines, 0)
}
