package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Case is one labeled passage. Label is the expected verdict for the scored
// buckets and "unknown" for the ambiguous bucket, which the run reports but
// never grades.
type Case struct {
	ID        string
	Label     string
	Bucket    string
	Split     string
	Source    string
	License   string
	Generator string
	Notes     string
	Words     int
	Text      string
	Path      string
}

var (
	validLabels  = map[string]bool{"likely_human": true, "likely_ai": true, "unknown": true}
	validBuckets = map[string]bool{"human": true, "ai": true, "hard": true, "ambiguous": true}
)

// Scored reports whether the case contributes to the headline metrics.
func (c Case) Scored() bool { return c.Label != "unknown" }

// loadCases reads every *.md under dir, sorted by id.
func loadCases(dir string) ([]Case, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no case files under %s", dir)
	}
	var cases []Case
	seen := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		c, err := parseCase(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if other, dup := seen[c.ID]; dup {
			return nil, fmt.Errorf("%s: id %q already used by %s", path, c.ID, other)
		}
		seen[c.ID] = path
		c.Path = path
		cases = append(cases, c)
	}
	slices.SortFunc(cases, func(a, b Case) int { return strings.Compare(a.ID, b.ID) })
	return cases, nil
}

// parseCase decodes one case file: a "---" fenced frontmatter of key: value
// lines, then the passage. The word count is recomputed from the body so a
// stale frontmatter value cannot mislead the report.
func parseCase(raw string) (Case, error) {
	front, body, err := splitFrontmatter(raw)
	if err != nil {
		return Case{}, err
	}
	fields := map[string]string{}
	for line := range strings.SplitSeq(front, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	c := Case{
		ID:        fields["id"],
		Label:     fields["label"],
		Bucket:    fields["bucket"],
		Split:     fields["split"],
		Source:    fields["source"],
		License:   fields["license"],
		Generator: fields["generator"],
		Notes:     fields["notes"],
		Text:      strings.TrimSpace(body),
	}
	c.Words = len(strings.Fields(c.Text))
	if c.Split == "" {
		c.Split = "train"
	}
	return c, validateCase(c)
}

func validateCase(c Case) error {
	switch {
	case c.ID == "":
		return errors.New("missing id")
	case !validLabels[c.Label]:
		return fmt.Errorf("label %q: want likely_human, likely_ai, or unknown", c.Label)
	case !validBuckets[c.Bucket]:
		return fmt.Errorf("bucket %q: want human, ai, hard, or ambiguous", c.Bucket)
	case (c.Bucket == "ambiguous") != (c.Label == "unknown"):
		return errors.New("label unknown and bucket ambiguous must go together")
	case c.Split != "train" && c.Split != "test":
		return fmt.Errorf("split %q: want train or test (or omit for train)", c.Split)
	case c.Words < 20:
		return fmt.Errorf("passage has %d words, want at least 20", c.Words)
	}
	return nil
}

// splitFrontmatter returns the text between the opening "---" line and the
// closing one, and everything after the closing fence.
func splitFrontmatter(raw string) (front, body string, err error) {
	raw = strings.TrimPrefix(raw, string(rune(0xFEFF)))
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		return "", "", errors.New("file must start with a --- frontmatter fence")
	}
	front, body, ok = strings.Cut(rest, "\n---\n")
	if !ok {
		return "", "", errors.New("frontmatter has no closing --- fence")
	}
	return front, body, nil
}

// writeReview prints the sign-off document: a table of every case, then
// each passage in a fence longer than any backtick run inside it.
func writeReview(w io.Writer, cases []Case) {
	counts := map[string]int{}
	for _, c := range cases {
		counts[c.Bucket]++
	}
	fmt.Fprintf(
		w,
		"# Judge eval cases\n\n%d cases: %d human, %d ai, %d hard, %d ambiguous (unscored).\n\n",
		len(cases),
		counts["human"],
		counts["ai"],
		counts["hard"],
		counts["ambiguous"],
	)
	fmt.Fprintln(w, "| id | bucket | split | label | words | source or generator |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, c := range cases {
		origin := c.Source
		if c.Generator != "" {
			origin = c.Generator
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %d | %s |\n",
			c.ID, c.Bucket, c.Split, c.Label, c.Words, cell(origin))
	}
	for _, c := range cases {
		fmt.Fprintf(w, "\n### %s\n\n", c.ID)
		fmt.Fprintf(
			w,
			"bucket %s, label %s, %d words. Source: %s.",
			c.Bucket,
			c.Label,
			c.Words,
			c.Source,
		)
		if c.License != "" {
			fmt.Fprintf(w, " License: %s.", c.License)
		}
		if c.Generator != "" {
			fmt.Fprintf(w, " Generator: %s.", c.Generator)
		}
		if c.Notes != "" {
			fmt.Fprintf(w, " Notes: %s", c.Notes)
		}
		fence := fenceFor(c.Text)
		fmt.Fprintf(w, "\n\n%s\n%s\n%s\n", fence, c.Text, fence)
	}
}

// fenceFor returns a backtick fence one longer than the longest run of
// backticks in text, and never shorter than three.
func fenceFor(text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

// cell escapes a value for a markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}
