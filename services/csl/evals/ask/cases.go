package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultRepo           = "mad01/thismoon"
	defaultMaxFiles       = 5
	defaultMaxAnswerChars = 1500
	splitTrain            = "train"
	splitTest             = "test"
)

// Case is one question over a corpus that a lexical search selects, with
// the facts a correct answer must carry and the ones it must not invent.
type Case struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	// Split is train (the default) or test. The held-out split is read
	// once, at the end of a tuning round.
	Split string `yaml:"split"`
	// Repo is the csl repo name the query runs in.
	Repo string `yaml:"repo"`
	// Query is the zoekt query whose matching files, in rank order, become
	// the corpus. File narrows it with a path regex, as --file does.
	Query string `yaml:"query"`
	File  string `yaml:"file"`
	// Files is the explicit override: repo-relative paths read as given,
	// with no search. A case sets Query or Files.
	Files []string `yaml:"files"`
	// Anchors are paths the resolved corpus must include, so a query that
	// drifts away from the file the facts live in fails loudly.
	Anchors []string `yaml:"anchors"`
	// MaxFiles caps the corpus at the top-ranked files.
	MaxFiles int `yaml:"max_files"`
	// MaxAnswerChars is the compression budget. An answer over it fails,
	// which is what rejects a worker that echoes the corpus back.
	MaxAnswerChars int `yaml:"max_answer_chars"`
	// Expected are regexes the answer must match, one fact each.
	Expected []string `yaml:"expected"`
	// Forbidden are regexes a correct answer never matches: the names and
	// values a worker invents when it stops reading.
	Forbidden []string `yaml:"forbidden"`
	// Oracle is the hand-written ideal answer. It is the sign-off document
	// and the grading sanity check, so it must score full marks.
	Oracle string `yaml:"oracle"`
	Notes  string `yaml:"notes"`

	// Question is the body below the frontmatter.
	Question string `yaml:"-"`
	Path     string `yaml:"-"`

	expected  []*regexp.Regexp
	forbidden []*regexp.Regexp
}

var idRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

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
		c, err := parseCase(data)
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

// parseCase decodes one case file: a "---" fenced YAML frontmatter, then
// the question. Unknown keys fail the load, so a typo cannot silently
// disable a check.
func parseCase(raw []byte) (Case, error) {
	front, body, err := splitFrontmatter(raw)
	if err != nil {
		return Case{}, err
	}
	var c Case
	dec := yaml.NewDecoder(bytes.NewReader(front))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Case{}, fmt.Errorf("frontmatter: %w", err)
	}
	c.Question = strings.TrimSpace(string(body))
	if err := c.validate(); err != nil {
		return Case{}, err
	}
	return c, nil
}

func splitFrontmatter(raw []byte) (front, body []byte, err error) {
	const fence = "---\n"
	if !bytes.HasPrefix(raw, []byte(fence)) {
		return nil, nil, errors.New("missing frontmatter fence")
	}
	rest := raw[len(fence):]
	end := bytes.Index(rest, []byte("\n"+fence))
	if end < 0 {
		return nil, nil, errors.New("unterminated frontmatter")
	}
	return rest[:end], rest[end+1+len(fence):], nil
}

func (c *Case) validate() error {
	if !idRE.MatchString(c.ID) {
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", c.ID)
	}
	switch c.Split {
	case "":
		c.Split = splitTrain
	case splitTrain, splitTest:
	default:
		return fmt.Errorf("split %q must be train or test", c.Split)
	}
	if c.Repo == "" {
		c.Repo = defaultRepo
	}
	if c.Query == "" && len(c.Files) == 0 {
		return errors.New("a case needs a query or a files list")
	}
	if c.MaxFiles == 0 {
		c.MaxFiles = defaultMaxFiles
	}
	if c.MaxAnswerChars == 0 {
		c.MaxAnswerChars = defaultMaxAnswerChars
	}
	if c.MaxFiles < 0 || c.MaxAnswerChars < 0 {
		return errors.New("max_files and max_answer_chars must be positive")
	}
	if len(c.Expected) == 0 {
		return errors.New("expected must list at least one fact")
	}
	if strings.TrimSpace(c.Oracle) == "" {
		return errors.New("oracle must hold the ideal answer")
	}
	if c.Question == "" {
		return errors.New("the body must hold the question")
	}
	var err error
	if c.expected, err = compileAll(c.Expected); err != nil {
		return fmt.Errorf("expected: %w", err)
	}
	if c.forbidden, err = compileAll(c.Forbidden); err != nil {
		return fmt.Errorf("forbidden: %w", err)
	}
	return nil
}

func compileAll(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// selectCases applies the -filter substring and the -split choice.
func selectCases(cases []Case, filter, split string) ([]Case, error) {
	switch split {
	case "all", splitTrain, splitTest:
	default:
		return nil, fmt.Errorf("split %q must be train, test or all", split)
	}
	var out []Case
	for _, c := range cases {
		if filter != "" && !strings.Contains(c.ID, filter) {
			continue
		}
		if split != "all" && c.Split != split {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, errors.New("no cases selected")
	}
	return out, nil
}
