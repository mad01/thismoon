package scanner

import "strings"

// Finding represents a single secret detected in a file.
type Finding struct {
	File     string
	Line     int
	Column   int
	Commit   string // introducing commit hash; set only by history scans
	Rule     Rule
	RawMatch string // the matched text, unredacted (not included in output)
	Match    string // the matched text, redacted
	Context  string // the full line with the match redacted in-place
}

// Redact masks the middle of a secret so it is safe to display.
// For strings longer than 12 characters it shows the first 4 and last 4
// characters with asterisks in between. For shorter strings it shows the
// first 2 characters and masks the rest.
func Redact(s string) string {
	if len(s) == 0 {
		return s
	}
	if len(s) <= 12 {
		if len(s) <= 2 {
			return strings.Repeat("*", len(s))
		}
		return s[:2] + strings.Repeat("*", len(s)-2)
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}
