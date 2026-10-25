package query

import (
	"strings"
	"unicode"
)

// The rules below decide which lines count as the same line and which are
// too common to attribute to any one edit. They are deliberately small;
// tune them against real sessions, not against intuition.

// normalize makes a line comparable across edits: leading and trailing
// whitespace dropped and inner runs of whitespace collapsed to one space,
// so a re-indented or re-aligned line still matches.
func normalize(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

// fillerWords are the words a line can be made of and still say nothing
// about which edit wrote it.
var fillerWords = map[string]bool{
	"begin": true, "break": true, "continue": true, "default": true, "do": true,
	"done": true, "elif": true, "else": true, "end": true, "err": true, "esac": true,
	"false": true, "fi": true, "finally": true, "if": true, "nil": true, "none": true,
	"null": true, "pass": true, "return": true, "then": true, "true": true, "try": true,
}

// trivial reports whether a normalized line is too common to attribute:
// empty, punctuation only, filler words only (a lone return, "} else {",
// "if err != nil {", "return nil, err"), or an import, include or package
// line.
func trivial(line string) bool {
	lower := strings.ToLower(line)
	if importLine(lower) {
		return true
	}
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	for _, w := range words {
		if !fillerWords[w] {
			return false
		}
	}
	return true
}

// importLine reports whether a lowercased line pulls in another module or
// names the file's package.
func importLine(lower string) bool {
	switch {
	case lower == "import",
		strings.HasPrefix(lower, "import "),
		strings.HasPrefix(lower, "import("):
		return true
	case strings.HasPrefix(lower, "#include"), strings.HasPrefix(lower, "package "):
		return true
	case strings.HasPrefix(lower, "from ") && strings.Contains(lower, " import "):
		return true
	}
	return false
}
