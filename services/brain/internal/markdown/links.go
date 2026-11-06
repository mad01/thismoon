package markdown

import (
	"regexp"
	"strings"
)

// Wikilink is an Obsidian [[Target|Alias#heading^block]] link.
type Wikilink struct {
	Target  string // "Target" (without heading/block)
	Heading string // optional #heading
	Block   string // optional ^block
	Alias   string // display text after |
	Raw     string // the full matched "[[...]]" substring
	Line    int    // 1-based line number in the source
	Column  int    // 1-based column
	Offset  int    // byte offset of the opening [[ in the source
}

// MDLink is a standard [text](url) markdown link.
type MDLink struct {
	Text   string
	URL    string
	Raw    string
	Line   int
	Column int
}

var (
	wikilinkRe = regexp.MustCompile(`\[\[([^\[\]]+?)\]\]`)
	// Matches [text](url) where text may contain brackets, but we keep this simple.
	mdlinkRe = regexp.MustCompile(`\[([^\]]+?)\]\(([^)]+?)\)`)
)

// ExtractWikilinks returns every wikilink in the content, skipping fenced code
// blocks (```...```) and inline code spans (`...`). Line/column are 1-based.
func ExtractWikilinks(content []byte) []Wikilink {
	out := []Wikilink{}
	for _, seg := range uncodedSegments(content) {
		for _, m := range wikilinkRe.FindAllSubmatchIndex(seg.bytes, -1) {
			raw := string(seg.bytes[m[0]:m[1]])
			inner := string(seg.bytes[m[2]:m[3]])
			wl := parseWikilinkInner(inner)
			wl.Raw = raw
			line, col := offsetToLineCol(content, seg.offset+m[0])
			wl.Line = line
			wl.Column = col
			wl.Offset = seg.offset + m[0]
			out = append(out, wl)
		}
	}
	return out
}

// ExtractMDLinks returns every [text](url) markdown link in the content,
// skipping code blocks and inline code spans.
func ExtractMDLinks(content []byte) []MDLink {
	out := []MDLink{}
	for _, seg := range uncodedSegments(content) {
		for _, m := range mdlinkRe.FindAllSubmatchIndex(seg.bytes, -1) {
			raw := string(seg.bytes[m[0]:m[1]])
			text := string(seg.bytes[m[2]:m[3]])
			url := string(seg.bytes[m[4]:m[5]])
			line, col := offsetToLineCol(content, seg.offset+m[0])
			out = append(out, MDLink{
				Text:   text,
				URL:    url,
				Raw:    raw,
				Line:   line,
				Column: col,
			})
		}
	}
	return out
}

func parseWikilinkInner(inner string) Wikilink {
	var wl Wikilink
	// Split on | for alias first (the rightmost |).
	target := inner
	if i := strings.Index(inner, "|"); i >= 0 {
		target = inner[:i]
		wl.Alias = strings.TrimSpace(inner[i+1:])
	}
	// Split off block (^) and heading (#).
	if i := strings.Index(target, "^"); i >= 0 {
		wl.Block = strings.TrimSpace(target[i+1:])
		target = target[:i]
	}
	if i := strings.Index(target, "#"); i >= 0 {
		wl.Heading = strings.TrimSpace(target[i+1:])
		target = target[:i]
	}
	wl.Target = strings.TrimSpace(target)
	return wl
}

type segment struct {
	bytes  []byte
	offset int // byte offset into the original content
}

// uncodedSegments splits content into the parts outside fenced code blocks and
// inline code spans. Fenced blocks are ``` on their own line; inline spans are
// backtick-delimited. A quick-and-dirty scanner that is good enough for the
// link-extraction use case.
func uncodedSegments(content []byte) []segment {
	var out []segment
	i := 0
	inFence := false
	start := 0
	for i < len(content) {
		// fenced code block: ``` at start of line (ignoring whitespace? keep simple — column 0)
		if !inFence && (i == 0 || content[i-1] == '\n') && bytesHasPrefixAt(content, i, "```") {
			if !inFence {
				out = appendSegment(out, content[start:i], start)
			}
			inFence = true
			// skip the fence line
			nl := indexByteAt(content, i, '\n')
			if nl < 0 {
				return out
			}
			i = nl + 1
			continue
		}
		if inFence && (i == 0 || content[i-1] == '\n') && bytesHasPrefixAt(content, i, "```") {
			nl := indexByteAt(content, i, '\n')
			if nl < 0 {
				nl = len(content) - 1
			}
			inFence = false
			i = nl + 1
			start = i
			continue
		}
		// inline code span: single or triple backticks — strip everything between matching
		if !inFence && content[i] == '`' {
			// flush before the backtick
			out = appendSegment(out, content[start:i], start)
			// count opening backticks
			j := i
			for j < len(content) && content[j] == '`' {
				j++
			}
			openLen := j - i
			open := content[i:j]
			// find closing run of exactly openLen backticks
			k := j
			for k < len(content) {
				if content[k] == '`' {
					// count
					l := k
					for l < len(content) && content[l] == '`' {
						l++
					}
					if l-k == openLen {
						i = l
						break
					}
					k = l
					continue
				}
				k++
			}
			if k >= len(content) {
				// unterminated inline — treat as plain text
				out = appendSegment(out, open, i)
				i = j
			}
			start = i
			continue
		}
		i++
	}
	if start < len(content) && !inFence {
		out = appendSegment(out, content[start:], start)
	}
	return out
}

func appendSegment(out []segment, b []byte, off int) []segment {
	if len(b) == 0 {
		return out
	}
	return append(out, segment{bytes: b, offset: off})
}

func bytesHasPrefixAt(content []byte, i int, prefix string) bool {
	if i+len(prefix) > len(content) {
		return false
	}
	return string(content[i:i+len(prefix)]) == prefix
}

func indexByteAt(content []byte, from int, c byte) int {
	for i := from; i < len(content); i++ {
		if content[i] == c {
			return i
		}
	}
	return -1
}

// offsetToLineCol returns 1-based line/column for a byte offset in content.
func offsetToLineCol(content []byte, offset int) (int, int) {
	line := 1
	col := 1
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
