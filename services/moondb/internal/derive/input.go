package derive

import (
	"encoding/json"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/patch"
)

// Tools whose input names the text they replace.
const (
	toolEdit      = "Edit"
	toolMultiEdit = "MultiEdit"
)

// replacement is one swap an Edit or MultiEdit input asks for: Old becomes
// New once, or everywhere when All is set. An empty Old creates the file
// with New as its content.
type replacement struct {
	Old string `json:"old_string"`
	New string `json:"new_string"`
	All bool   `json:"replace_all"`
}

// replacements reads the swaps from an Edit or MultiEdit input in the order
// the tool applies them, or nil when the tool is another or its input does
// not carry them.
func replacements(tool string, raw json.RawMessage) []replacement {
	if len(raw) == 0 {
		return nil
	}
	switch tool {
	case toolEdit:
		var in struct {
			Old *string `json:"old_string"`
			New *string `json:"new_string"`
			All bool    `json:"replace_all"`
		}
		if json.Unmarshal(raw, &in) != nil || in.Old == nil || in.New == nil {
			return nil
		}
		return []replacement{{Old: *in.Old, New: *in.New, All: in.All}}
	case toolMultiEdit:
		var in struct {
			Edits []replacement `json:"edits"`
		}
		if json.Unmarshal(raw, &in) != nil || len(in.Edits) == 0 {
			return nil
		}
		return in.Edits
	default:
		return nil
	}
}

// creates reports a tool input whose swaps create their file: the first
// replaces nothing, which the tool allows only where no file exists. It
// reads the raw input, so an edit the person changed in the permission
// dialog, whose swaps load leaves out, still counts.
func creates(tool string, raw json.RawMessage) bool {
	reps := replacements(tool, raw)
	return len(reps) > 0 && reps[0].Old == ""
}

// outcome is how a swap met the tool's occurrence rule.
type outcome int

const (
	// swapped: the rule held and the swap was made.
	swapped outcome = iota
	// missing: the text to replace does not occur. A normalization the
	// harness applies before matching, such as quote styles, can explain
	// that, so it does not rule the content out.
	missing
	// refused: the text occurs more than once without replace_all, or an
	// empty text meets a file that is not empty. The tool refuses such a
	// call, so the content cannot be what it edited.
	refused
)

// replace applies the swaps to content in order, under the rule the tool
// enforces: each Old occurs exactly once, or at least once when All is
// set, in the content as the swaps before it left it. It stops at the
// first swap that breaks the rule and says how.
func replace(content []byte, reps []replacement) ([]byte, outcome) {
	t := lfView(content)
	for _, r := range reps {
		var o outcome
		if t.s, o = swap(t.s, r.Old, r.New, r.All); o != swapped {
			return nil, o
		}
	}
	return t.bytes(), swapped
}

// unreplace undoes the swaps on the content they produced, last first,
// under the same rule with New in the place of Old. Two kinds of swap
// cannot be undone: one that deleted its text leaves nothing to find, and
// one with replace_all cannot tell its New from the same text the content
// already held.
func unreplace(content []byte, reps []replacement) ([]byte, bool) {
	t := lfView(content)
	for i := len(reps) - 1; i >= 0; i-- {
		r := reps[i]
		switch {
		case r.New == "", r.All:
			return nil, false
		case r.Old == "":
			if t.s != r.New {
				return nil, false
			}
			t.s = ""
		default:
			var o outcome
			if t.s, o = swap(t.s, r.New, r.Old, false); o != swapped {
				return nil, false
			}
		}
	}
	return t.bytes(), true
}

// swap replaces from with to in s: its one occurrence, or every occurrence
// when all is set. An empty from fits only an empty s, which becomes to.
func swap(s, from, to string, all bool) (string, outcome) {
	if from == "" {
		if s != "" {
			return "", refused
		}
		return to, swapped
	}
	// strings.Count does not count overlapping matches: "aa" occurs once in "aaa".
	switch n := strings.Count(s, from); {
	case n == 0:
		return "", missing
	case n > 1 && !all:
		return "", refused
	case all:
		return strings.ReplaceAll(s, from, to), swapped
	default:
		return strings.Replace(s, from, to, 1), swapped
	}
}

// text is file content as the tool matches it. The tool input carries LF
// line endings, so a file whose every line ends in CRLF is matched on its
// LF view, and bytes gives the result the CRLF endings back.
type text struct {
	s    string
	crlf bool
}

func lfView(b []byte) text {
	if patch.DetectEOL(b) != "crlf" {
		return text{s: string(b)}
	}
	return text{s: strings.ReplaceAll(string(b), "\r\n", "\n"), crlf: true}
}

func (t text) bytes() []byte {
	if !t.crlf {
		return []byte(t.s)
	}
	lf := strings.ReplaceAll(t.s, "\r\n", "\n")
	return []byte(strings.ReplaceAll(lf, "\n", "\r\n"))
}
