// Package outline checks a component doc's headings against the shared
// outline: the title shape, known second-level sections only, in outline
// order, and every required section present. It is pure: it reads a doc's
// text and returns the deviations, one Problem each.
package outline

import (
	"fmt"
	"strings"
)

// Title rules a Spec can name for a doc's first-level heading.
const (
	// TitleNameComma wants "# <name>, <purpose>": the component name,
	// optionally followed by a parenthesised short form, then a comma.
	TitleNameComma = "name-comma-purpose"
	// TitleName wants "# <name>" and nothing else.
	TitleName = "name"
)

// Problem kinds.
const (
	KindTitle     = "title"
	KindUnknown   = "unknown"
	KindOrder     = "order"
	KindMissing   = "missing"
	KindDuplicate = "duplicate"
)

// Spec is the shared outline for one kind of doc (CLAUDE.md or README.md).
type Spec struct {
	Title    string    `yaml:"title"    json:"title"`
	Sections []Section `yaml:"sections" json:"sections"`
}

// Section is one second-level heading the outline allows, in outline order.
type Section struct {
	Heading  string `yaml:"heading"  json:"heading"`
	Required bool   `yaml:"required" json:"required"`
}

// Problem is one way a doc deviates from its Spec.
type Problem struct {
	Kind    string `json:"kind"`
	Heading string `json:"heading"`
}

// String renders the problem for a check detail line.
func (p Problem) String() string { return fmt.Sprintf("%s: %s", p.Kind, p.Heading) }

// Check returns every deviation of doc from spec. component is the
// component's directory name, which the title rule matches against. An
// empty result means the doc follows the outline.
func Check(doc, component string, spec Spec) []Problem {
	title, sections := Headings(doc)
	var problems []Problem
	if !titleOK(title, component, spec.Title) {
		problems = append(problems, Problem{Kind: KindTitle, Heading: title})
	}
	problems = append(problems, sectionProblems(sections, spec.Sections)...)
	return problems
}

// Headings returns the first first-level heading and every second-level
// heading of a markdown doc, in order. Lines inside fenced code blocks are
// skipped, so a shell comment in a fence never counts as a heading.
func Headings(doc string) (title string, sections []string) {
	inFence := false
	for line := range strings.Lines(doc) {
		line = strings.TrimRight(line, "\r\n")
		if isFence(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# ") && title == "":
			title = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "## "):
			sections = append(sections, strings.TrimSpace(line[3:]))
		}
	}
	return title, sections
}

func isFence(line string) bool {
	t := strings.TrimLeft(line, " ")
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

func titleOK(title, component, rule string) bool {
	switch rule {
	case TitleName:
		return title == component
	case TitleNameComma:
		rest, ok := strings.CutPrefix(title, component)
		if !ok {
			return false
		}
		if strings.HasPrefix(rest, " (") {
			_, rest, ok = strings.Cut(rest, ")")
			if !ok {
				return false
			}
		}
		return strings.HasPrefix(rest, ", ") && len(strings.TrimSpace(rest[2:])) > 0
	default:
		return title != ""
	}
}

// sectionProblems flags unknown and duplicate headings, headings out of
// outline order, and required headings that never appear. Order problems
// are the known headings outside the longest run that is already in
// outline order, so one misplaced section counts once.
func sectionProblems(found []string, allowed []Section) []Problem {
	index := make(map[string]int, len(allowed))
	for i, s := range allowed {
		index[s.Heading] = i
	}
	var problems []Problem
	seen := map[string]bool{}
	var known []string
	for _, h := range found {
		_, ok := index[h]
		switch {
		case !ok:
			problems = append(problems, Problem{Kind: KindUnknown, Heading: h})
		case seen[h]:
			problems = append(problems, Problem{Kind: KindDuplicate, Heading: h})
		default:
			seen[h] = true
			known = append(known, h)
		}
	}
	for _, h := range outOfOrder(known, index) {
		problems = append(problems, Problem{Kind: KindOrder, Heading: h})
	}
	for _, s := range allowed {
		if s.Required && !seen[s.Heading] {
			problems = append(problems, Problem{Kind: KindMissing, Heading: s.Heading})
		}
	}
	return problems
}

// outOfOrder returns the headings outside one longest increasing
// subsequence of their outline positions.
func outOfOrder(known []string, index map[string]int) []string {
	n := len(known)
	if n == 0 {
		return nil
	}
	length := make([]int, n)
	prev := make([]int, n)
	best := 0
	for i := range n {
		length[i], prev[i] = 1, -1
		for j := range i {
			if index[known[j]] < index[known[i]] && length[j]+1 > length[i] {
				length[i], prev[i] = length[j]+1, j
			}
		}
		if length[i] > length[best] {
			best = i
		}
	}
	inRun := make([]bool, n)
	for i := best; i >= 0; i = prev[i] {
		inRun[i] = true
	}
	var out []string
	for i, h := range known {
		if !inRun[i] {
			out = append(out, h)
		}
	}
	return out
}
