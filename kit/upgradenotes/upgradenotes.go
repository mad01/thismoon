// Package upgradenotes parses docs/UPGRADING.md into entries and checks them
// against the form that page describes: a dated heading, then Change, Run,
// Verify and Since, in that order.
//
// Entries older than FormDate predate the form. They keep their own shape and
// are checked only for what every entry has always carried: a dated heading,
// a Change that names a pull request, and a Verify.
//
// The package reads text and returns values. It opens no file and runs no
// command, so a test, a doctor check or a later command can all use it.
package upgradenotes

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// FormDate is the first day an entry must follow the full form. Entries dated
// before it are legacy.
const FormDate = "2026-11-14"

// Values a Since item takes besides a component and a version.
const (
	sinceUnreleased = "unreleased"
	sinceMainOnly   = "main only"
)

// The labels of an entry, in the order an entry carries them.
const (
	labelChange = "Change"
	labelRun    = "Run"
	labelVerify = "Verify"
	labelSince  = "Since"
)

const dateLayout = "2006-01-02"

var (
	entryLabels  = []string{labelChange, labelRun, labelVerify, labelSince}
	entryHeading = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}) ([^:]+): (.+)$`)
	pullRequest  = regexp.MustCompile(`#\d+`)
	sinceVersion = regexp.MustCompile(`^\S+ \d+\.\d+\.\d+$`)
)

// Entry is one upgrade note: what changed, what to run on a machine that
// already has the tools, how to see that it worked, and which release carries
// the change.
type Entry struct {
	Date       string // YYYY-MM-DD from the heading; empty when the heading is malformed
	Components string // the heading text between the date and the colon
	Title      string
	Change     string
	Run        string
	Verify     string
	Since      string
	Line       int // 1-based line of the entry heading

	labels []string // labels in the order the entry carries them
}

// Problem is one place where a page departs from the form.
type Problem struct {
	Line    int
	Message string
}

// String formats the problem as "line N: message".
func (p Problem) String() string {
	return fmt.Sprintf("line %d: %s", p.Line, p.Message)
}

// Parse reads an upgrade page. Every "## " heading opens an entry; text before
// the first one is the page's introduction. Inside an entry, a line that
// starts with a label and a colon opens that field, and the lines after it
// continue the field.
func Parse(doc string) []Entry {
	var (
		entries []Entry
		entry   *Entry
		field   *string
	)
	for i, line := range strings.Split(doc, "\n") {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			entries = append(entries, newEntry(heading, i+1))
			entry, field = &entries[len(entries)-1], nil
			continue
		}
		if entry != nil {
			field = appendLine(entry, field, line)
		}
	}
	for i := range entries {
		trimFields(&entries[i])
	}
	return entries
}

// newEntry builds an entry from its heading text. A heading that does not
// read "date components: title" keeps the whole text as the title and an
// empty date, which Check reports.
func newEntry(heading string, line int) Entry {
	m := entryHeading.FindStringSubmatch(strings.TrimSpace(heading))
	if m == nil {
		return Entry{Title: strings.TrimSpace(heading), Line: line}
	}
	return Entry{Date: m[1], Components: m[2], Title: m[3], Line: line}
}

// appendLine adds one body line to the entry. A line that opens a label
// switches the field; any other line continues the field in progress. It
// returns the field later lines continue.
func appendLine(entry *Entry, field *string, line string) *string {
	for _, label := range entryLabels {
		rest, ok := strings.CutPrefix(line, label+":")
		if !ok {
			continue
		}
		entry.labels = append(entry.labels, label)
		field = entry.field(label)
		*field += strings.TrimSpace(rest)
		return field
	}
	if field != nil {
		*field += "\n" + line
	}
	return field
}

// field returns the entry field a label fills.
func (e *Entry) field(label string) *string {
	switch label {
	case labelChange:
		return &e.Change
	case labelRun:
		return &e.Run
	case labelVerify:
		return &e.Verify
	default:
		return &e.Since
	}
}

func trimFields(e *Entry) {
	for _, label := range entryLabels {
		f := e.field(label)
		*f = strings.TrimSpace(*f)
	}
}

// Check returns every problem in a parsed page, in line order. No problems
// means the page follows the form.
func Check(entries []Entry) []Problem {
	var problems []Problem
	newer := ""
	for _, entry := range entries {
		problems = append(problems, checkEntry(entry, newer)...)
		if entry.Date != "" {
			newer = entry.Date
		}
	}
	return problems
}

// checkEntry checks one entry. newer is the date of the entry above it, empty
// for the first: the page runs newest first, so no entry is dated after it.
func checkEntry(entry Entry, newer string) []Problem {
	var problems []Problem
	add := func(format string, args ...any) {
		problems = append(
			problems,
			Problem{Line: entry.Line, Message: fmt.Sprintf(format, args...)},
		)
	}
	if _, err := time.Parse(dateLayout, entry.Date); err != nil {
		add("heading %q must read \"## YYYY-MM-DD components: title\"", entry.Title)
		return problems
	}
	if newer != "" && entry.Date > newer {
		add("entry %q is dated %s, after the entry above it (%s); newest goes first",
			entry.Title, entry.Date, newer)
	}
	if entry.Date >= FormDate && !slices.Equal(entry.labels, entryLabels) {
		add(
			"entry %q must carry Change:, Run:, Verify: and Since: once each, in that order; found %v",
			entry.Title,
			entry.labels,
		)
		return problems
	}
	for _, label := range requiredLabels(entry) {
		if *entry.field(label) == "" {
			add("entry %q has no text for %s:", entry.Title, label)
		}
	}
	if entry.Change != "" && !pullRequest.MatchString(entry.Change) {
		add("entry %q: Change: must name the pull request, such as #500", entry.Title)
	}
	if entry.Since != "" {
		if bad := badSinceItems(entry.Since); len(bad) > 0 {
			add("entry %q: Since: items must be \"unreleased\", \"main only\" or a component "+
				"and version such as \"belt 2.12.0\"; found %q", entry.Title, bad)
		}
	}
	return problems
}

// requiredLabels returns the labels that must have text: all four from
// FormDate on, and the two every legacy entry carries before it.
func requiredLabels(entry Entry) []string {
	if entry.Date >= FormDate {
		return entryLabels
	}
	return []string{labelChange, labelVerify}
}

// badSinceItems returns the comma-separated items of a Since value that name
// neither a state nor a component with a version. A component may also carry
// a state, as in "speak unreleased".
func badSinceItems(since string) []string {
	var bad []string
	for _, item := range strings.Split(strings.TrimSuffix(since, "."), ",") {
		item = strings.TrimSpace(item)
		state := item
		if _, rest, ok := strings.Cut(item, " "); ok && rest == sinceUnreleased {
			state = rest
		}
		if state == sinceUnreleased || state == sinceMainOnly || sinceVersion.MatchString(item) {
			continue
		}
		bad = append(bad, item)
	}
	return bad
}
