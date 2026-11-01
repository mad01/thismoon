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

// CSLReadFile reads, from a csl_read tool response, the file it returned as
// csl names it (org/repo/path) and the whole file's line count. Claude Code
// hands an MCP tool's response over as its content blocks, and the first
// text block is csl's output. The default text form opens with the header
// line `org/repo/path lines A-B of N`; the json form is an object with
// repo, path and total_lines. Any other shape, another response format
// included, returns ("", 0), which callers read as unknown.
func CSLReadFile(raw json.RawMessage) (name string, totalLines int) {
	text, ok := firstTextBlock(raw)
	if !ok {
		return "", 0
	}
	if strings.HasPrefix(text, "{") {
		return cslReadJSON(text)
	}
	header, _, _ := strings.Cut(text, "\n")
	m := cslReadHeader.FindStringSubmatch(header)
	if m == nil {
		return "", 0
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0
	}
	return m[1], n
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

// cslReadJSON reads csl_read's json form: repo and path joined the way the
// text header joins them, and total_lines.
func cslReadJSON(text string) (name string, totalLines int) {
	var out struct {
		Repo       string `json:"repo"`
		Path       string `json:"path"`
		TotalLines int    `json:"total_lines"`
	}
	if json.Unmarshal([]byte(text), &out) != nil || out.Repo == "" || out.Path == "" {
		return "", 0
	}
	return out.Repo + "/" + out.Path, max(out.TotalLines, 0)
}
