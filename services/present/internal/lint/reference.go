package lint

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/services/present/internal/limits"
)

// notSeen is what the lint does not judge, stated in the reference so an
// author knows a clean run is not a verdict on the page.
var notSeen = []string{
	"whether a claim is true;",
	"whether a heading really is a claim: it catches labels only;",
	"whether the chart kind fits the data;",
	"anything that needs the drawn page, such as text size or overflow (the looks eval's probes measure that);",
	"what an `html` block holds.",
}

// Reference renders the limits reference the skill carries as
// references/limits.md: every limit in the table, warnings and refusals,
// with its number, scope, source, rule, and reason, then the lists the
// warnings use and what the lint does not see. `present limits` prints it,
// and TestSkillFilesAreCurrent keeps the file in the skill equal to it.
func Reference() string {
	var b strings.Builder
	b.WriteString("<!-- Generated from services/present/internal/limits by `make limits-doc` " +
		"in services/present. Do not edit by hand. -->\n\n")
	b.WriteString("# Limits\n\n")
	b.WriteString("Every number present holds a page to. A **warning** comes from the lint: " +
		"present_create, present_update, and present_patch_section return it in `warnings`, " +
		"and `present lint` prints it. The page is stored either way. A **refusal** comes from " +
		"the renderer: the page is not stored, and the error names the block and the rule.\n\n")
	b.WriteString("The source says how strong a number is: **study** is a measured result, " +
		"**guideline** a published style guide, **house** present's own number, and " +
		"**renderer** a fact of how present draws a page.\n\n")
	b.WriteString("## Warnings\n\nIn the order the lint reports them.\n\n")
	writeTable(&b, limits.Warnings())
	b.WriteString("\n" + warningTerms() + "\n")
	b.WriteString("\n## Reading a warning\n\n")
	b.WriteString(
		"A warning has three fields. `rule` is the rule id above. `where` is a path into " +
			"the tool's arguments, zero-based like a section patch's `index:N`: " +
			"`content.sections[2].blocks[0]` is the first block of the brief's third section, and " +
			"`content.sections[3..6]` is a run of sections. `message` names the place the way a " +
			"reader finds it, then the fault, the reason, and the fix. A slide number counts the " +
			"title slide as slide 1, the way the deck view does.\n\n",
	)
	b.WriteString(
		"Findings come by rule in the order of the table, then from the top of the page: " +
			"the title, the brief, then the deck. A section patch lists the patched section's " +
			"findings and counts the rest of the page in `elsewhere`.\n\n",
	)
	b.WriteString("## Refusals\n\n")
	var refusals []limits.Limit
	for _, l := range limits.All() {
		if l.Level == limits.Refuse {
			refusals = append(refusals, l)
		}
	}
	writeTable(&b, refusals)
	b.WriteString("\n## What the lint does not see\n\n")
	for _, s := range notSeen {
		b.WriteString("- " + s + "\n")
	}
	return b.String()
}

// writeTable writes limits as a table of their numbers, then one line per
// limit with the rule and its reason.
func writeTable(b *strings.Builder, ls []limits.Limit) {
	b.WriteString("| Id | Limit | Applies to | Source |\n|---|---|---|---|\n")
	for _, l := range ls {
		source := string(l.Source)
		if l.Basis != "" {
			source += " (" + l.Basis + ")"
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", l.ID, limitText(l), l.Scope, source)
	}
	b.WriteString("\n")
	for _, l := range ls {
		fmt.Fprintf(b, "- `%s`: %s %s\n", l.ID, l.Rule, l.Reason)
	}
}

// limitText is a limit's number and unit, or "none" for a rule without
// one.
func limitText(l limits.Limit) string {
	if l.Number == 0 {
		return "none"
	}
	return fmt.Sprintf("%d %s", l.Number, l.Unit)
}

// warningTerms spells out the lists the warnings read, from the same lists
// the checks use.
func warningTerms() string {
	quote := func(ws []string) string {
		out := make([]string, len(ws))
		for i, w := range ws {
			out[i] = "`" + w + "`"
		}
		return strings.Join(out, ", ")
	}
	return strings.Join([]string{
		"The label list for `heading.label`: " + strings.Join(labelHeadings, ", ") + ". " +
			"Exempt: a heading that opens with \"Next\" or \"References\", the page title, " +
			"a section divider slide (`layout: \"section\"`), and the ADHD mode's \"Do now\", " +
			"\"Now\", and \"Later\".",
		"",
		"A figure is a " + quote(figureKinds) + " block at the top of a section or in a column; " +
			"one inside `details` does not count, since the disclosure starts closed.",
		"",
		"A slide that holds only prose or a list is made of " + quote(textOnlyKinds) +
			" blocks, in columns too, or of nothing below its heading.",
		"",
		"A slide's words are the words of its blocks' text: paragraphs, lists, tables, " +
			"callouts, stats, quotes, captions, and chart titles. The labels drawn inside a " +
			"chart or diagram, an image's alt text, and the speaker notes do not count.",
	}, "\n")
}
