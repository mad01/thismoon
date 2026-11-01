package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// searchLimit is how many matches one csl search may return. Files are
// deduplicated from them in rank order before the case's cap applies.
const searchLimit = 500

// CorpusFile is one file the worker reads. The lines are held for the
// message and for the line-claim check and stay out of the JSON.
type CorpusFile struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	Bytes int    `json:"bytes"`

	lines []string
}

// Corpus is the files a case's query selected, in rank order, capped.
type Corpus struct {
	Repo    string       `json:"repo"`
	Root    string       `json:"root"`
	Query   string       `json:"query,omitempty"`
	Matched int          `json:"matched_files"`
	Files   []CorpusFile `json:"files"`
}

// Lines is the line count over every file.
func (c Corpus) Lines() int {
	n := 0
	for _, f := range c.Files {
		n += f.Lines
	}
	return n
}

// Bytes is the byte count over every file.
func (c Corpus) Bytes() int {
	n := 0
	for _, f := range c.Files {
		n += f.Bytes
	}
	return n
}

func (c Corpus) paths() []string {
	out := make([]string, 0, len(c.Files))
	for _, f := range c.Files {
		out = append(out, f.Path)
	}
	return out
}

// lineText returns line n (1-based) of every file long enough to have one.
func (c Corpus) lineText(n int) []string {
	var out []string
	for _, f := range c.Files {
		if n >= 1 && n <= len(f.lines) {
			out = append(out, f.lines[n-1])
		}
	}
	return out
}

// dump is the corpus text with no question: the answer a worker that
// echoes its input would give, which the budget must reject.
func (c Corpus) dump() string {
	var b strings.Builder
	for _, f := range c.Files {
		b.WriteString(strings.Join(f.lines, "\n"))
		b.WriteByte('\n')
	}
	return b.String()
}

// buildMessage is what the worker reads: each file inside a tag naming its
// path and length, every line prefixed with its number, then the question.
// The prefix is what makes a line claim checkable.
func buildMessage(corpus Corpus, question string) string {
	var b strings.Builder
	for _, f := range corpus.Files {
		fmt.Fprintf(&b, "<file path=%q lines=\"%d\">\n", f.Path, f.Lines)
		for i, line := range f.lines {
			fmt.Fprintf(&b, "%d: %s\n", i+1, line)
		}
		b.WriteString("</file>\n\n")
	}
	fmt.Fprintf(&b, "Question: %s\n", question)
	return b.String()
}

type searchMatch struct {
	RepoPath string `json:"repo_path"`
	File     string `json:"file"`
}

type searchOutput struct {
	Matches []searchMatch `json:"matches"`
}

type repoEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// resolveCorpus turns a case into the files the worker reads. With a query
// it runs csl search --json and keeps the distinct files in the order
// zoekt ranked them; with a files list it resolves the repo root through
// csl repo and reads the paths as given. Files come from the checkout csl
// indexes, as csl_read reads them.
func resolveCorpus(ctx context.Context, c Case) (Corpus, error) {
	var (
		root  string
		paths []string
		err   error
	)
	if len(c.Files) > 0 {
		root, err = repoRoot(ctx, c.Repo)
		paths = c.Files
	} else {
		root, paths, err = searchFiles(ctx, c)
	}
	if err != nil {
		return Corpus{}, fmt.Errorf("%s: %w", c.ID, err)
	}
	corpus := Corpus{Repo: c.Repo, Root: root, Query: c.Query, Matched: len(paths)}
	if len(paths) > c.MaxFiles {
		paths = paths[:c.MaxFiles]
	}
	for _, p := range paths {
		f, err := readCorpusFile(root, p)
		if err != nil {
			return Corpus{}, fmt.Errorf("%s: %w", c.ID, err)
		}
		corpus.Files = append(corpus.Files, f)
	}
	for _, a := range c.Anchors {
		if !slices.Contains(corpus.paths(), a) {
			return Corpus{}, fmt.Errorf(
				"%s: anchor %s is not in the resolved corpus %v", c.ID, a, corpus.paths(),
			)
		}
	}
	return corpus, nil
}

func searchFiles(ctx context.Context, c Case) (root string, paths []string, err error) {
	args := []string{
		"search", c.Query,
		"--repo", "^" + regexp.QuoteMeta(c.Repo) + "$",
		"--json", "--limit", fmt.Sprint(searchLimit),
	}
	if c.File != "" {
		args = append(args, "--file", c.File)
	}
	out, err := runCSL(ctx, args...)
	if err != nil {
		return "", nil, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(out), []byte("{")) {
		return "", nil, fmt.Errorf("query %q matched no file in %s", c.Query, c.Repo)
	}
	var res searchOutput
	if err := json.Unmarshal(out, &res); err != nil {
		return "", nil, fmt.Errorf("csl search --json: %w", err)
	}
	seen := map[string]bool{}
	for _, m := range res.Matches {
		if seen[m.File] {
			continue
		}
		seen[m.File] = true
		if root == "" {
			root = m.RepoPath
		}
		paths = append(paths, m.File)
	}
	if len(paths) == 0 {
		return "", nil, fmt.Errorf("query %q matched no file in %s", c.Query, c.Repo)
	}
	return root, paths, nil
}

func repoRoot(ctx context.Context, repo string) (string, error) {
	out, err := runCSL(ctx, "repo", repo, "--json")
	if err != nil {
		return "", err
	}
	var entries []repoEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return "", fmt.Errorf("csl repo --json: %w", err)
	}
	for _, e := range entries {
		if e.Name == repo {
			return e.Path, nil
		}
	}
	return "", fmt.Errorf("csl repo %s: no exact match among %d candidates", repo, len(entries))
}

func runCSL(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "csl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf(
				"csl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()),
			)
		}
		return nil, fmt.Errorf("csl %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}

func readCorpusFile(root, rel string) (CorpusFile, error) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return CorpusFile{}, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	return CorpusFile{Path: rel, Lines: len(lines), Bytes: len(data), lines: lines}, nil
}
