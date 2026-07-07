// Package transform renders text into bionic reading format (first ~50% of each
// word bold) while preserving markdown structure.
package transform

import (
	"slices"
	"strings"
	"unicode"
)

// Bionic converts text to bionic reading format: the first ~50% of each word
// is wrapped in markdown bold (**...**). Numbers, code blocks, existing bold
// markers, and link URLs are preserved.
func Bionic(text string) string {
	lines := strings.Split(text, "\n")
	inCodeBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}
		if trimmed == "" {
			continue
		}
		lines[i] = bionicLine(line)
	}
	return strings.Join(lines, "\n")
}

func bionicLine(line string) string {
	runes := []rune(line)
	var out strings.Builder
	out.Grow(len(line) * 2)

	i := 0
	inBold := false
	inItalic := false

	for i < len(runes) {
		// Inline code: skip until closing backtick
		if runes[i] == '`' {
			j := i + 1
			for j < len(runes) && runes[j] != '`' {
				j++
			}
			if j < len(runes) {
				j++
			}
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}

		// Bold markers ** or __
		if i+1 < len(runes) &&
			((runes[i] == '*' && runes[i+1] == '*') || (runes[i] == '_' && runes[i+1] == '_')) {
			inBold = !inBold
			out.WriteRune(runes[i])
			out.WriteRune(runes[i+1])
			i += 2
			continue
		}

		// Italic markers: single * or _ (must check after bold **)
		if (runes[i] == '*' || runes[i] == '_') && !isWordChar(runes[i]) {
			inItalic = !inItalic
			out.WriteRune(runes[i])
			i++
			continue
		}

		// Link URL portion: ](...) — track balanced parens for URLs with parens
		if runes[i] == ']' && i+1 < len(runes) && runes[i+1] == '(' {
			j := i + 2
			depth := 1
			for j < len(runes) && depth > 0 {
				switch runes[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}

		// Header prefix: # at start of line
		if i == 0 && runes[i] == '#' {
			j := i
			for j < len(runes) && runes[j] == '#' {
				j++
			}
			for j < len(runes) && runes[j] == ' ' {
				j++
			}
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}

		// Word: sequence of letters (possibly with apostrophes)
		if isWordChar(runes[i]) {
			j := i
			for j < len(runes) && isWordChar(runes[j]) {
				j++
			}
			word := runes[i:j]
			if inBold || inItalic || !hasLetter(word) {
				out.WriteString(string(word))
			} else {
				out.WriteString(boldWord(word))
			}
			i = j
			continue
		}

		out.WriteRune(runes[i])
		i++
	}

	return out.String()
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || r == '\''
}

func hasLetter(runes []rune) bool {
	return slices.ContainsFunc(runes, unicode.IsLetter)
}

func boldWord(word []rune) string {
	n := (len(word) + 1) / 2
	return "**" + string(word[:n]) + "**" + string(word[n:])
}
