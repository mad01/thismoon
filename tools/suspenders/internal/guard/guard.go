// Package guard blocks commits that reference internal repository names.
// It scans workspace directories to collect org/repo names, then checks
// staged diffs for case-insensitive matches.
package guard

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/repo"
)

// Finding represents a blocked name found by a guard check. Staged-diff
// findings carry only Match; working-tree findings also locate the hit.
type Finding struct {
	Match string
	File  string // repo-relative path; empty for staged-diff findings
	Line  int    // 1-based; 0 for staged-diff findings
}

// SkippedFile records a tracked file CheckDir did not read, with the reason
// (too large, unreadable). A skip is surfaced, never silent: a green check
// must not hide files the guard never scanned.
type SkippedFile struct {
	Path   string
	Reason string
}

// Guard checks staged diffs for internal repository name references.
type Guard struct {
	cfg config.GuardConfig

	// OnSkip, when set, is called for each file CheckDir skips instead of
	// scanning it. Set it once before checking. When nil, skips are logged to
	// stderr so they are never silent.
	OnSkip func(SkippedFile)
}

// New creates a Guard from the given config.
func New(cfg config.GuardConfig) *Guard {
	return &Guard{cfg: cfg}
}

// reportSkip surfaces a file the guard did not read. It routes to OnSkip when
// set, otherwise to stderr, so a skip is never dropped silently.
func (g *Guard) reportSkip(path, reason string) {
	sk := SkippedFile{Path: path, Reason: reason}
	if g.OnSkip != nil {
		g.OnSkip(sk)
		return
	}
	fmt.Fprintf(os.Stderr, "suspenders: skipped %s (%s)\n", path, reason)
}

// CollectNames discovers internal repo names from workspace directories,
// filters the allowlist, and appends blocked words. Returns deduplicated names.
func (g *Guard) CollectNames() ([]string, error) {
	var expandedDirs []string
	for _, d := range g.cfg.WorkspaceDirs {
		expandedDirs = append(expandedDirs, config.ExpandPath(d))
	}

	repos, err := repo.Find(expandedDirs, nil)
	if err != nil {
		return nil, fmt.Errorf("discover repos: %w", err)
	}

	allowed := make(map[string]bool, len(g.cfg.Allowlist))
	for _, a := range g.cfg.Allowlist {
		allowed[a] = true
	}

	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if name == "" || allowed[name] || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}

	for _, r := range repos {
		add(r.Name)
		add(filepath.Base(r.Path))
	}

	for _, w := range g.cfg.BlockedWords {
		add(w)
	}

	sort.Strings(names)
	return names, nil
}

// Matcher matches blocked names in arbitrary content, case-insensitively.
type Matcher struct {
	pattern *regexp.Regexp
}

// NewMatcher collects the blocked names and compiles them into a Matcher.
// Returns nil (and no error) when there are no names to match.
func (g *Guard) NewMatcher() (*Matcher, error) {
	names, err := g.CollectNames()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	// Longest name first: Go regexp alternation is leftmost-first, so
	// overlapping entries (docs.example.net vs docs.example.net/runbook)
	// must try the more specific string first. Cleanup then replaces whole
	// references, not fragments.
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	escaped := make([]string, len(names))
	for i, n := range names {
		escaped[i] = namePattern(n)
	}
	pattern, err := regexp.Compile("(?i)(" + strings.Join(escaped, "|") + ")")
	if err != nil {
		return nil, fmt.Errorf("compile pattern: %w", err)
	}
	return &Matcher{pattern: pattern}, nil
}

// namePattern converts a blocked name into its regex form. Entries are
// matched literally except `*`, which matches a run of non-whitespace
// characters, so `*.example.net` blocks every subdomain and the whole
// matched reference (scheme and all) becomes one cleanup replacement.
// Edges that are word characters get a \b guard so a short entry cannot
// match inside an ordinary word (a 3-letter host alias must not hit the
// middle of "higher"); wildcard and punctuation edges keep their reach.
func namePattern(name string) string {
	parts := strings.Split(name, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	pattern := strings.Join(parts, `\S*`)

	runes := []rune(name)
	if len(runes) > 0 && isWordChar(runes[0]) {
		pattern = `\b` + pattern
	}
	if len(runes) > 0 && isWordChar(runes[len(runes)-1]) {
		pattern += `\b`
	}
	return pattern
}

// isWordChar reports whether r counts as a word character for regexp \b
// (RE2 defines word boundaries over ASCII [0-9A-Za-z_]).
func isWordChar(r rune) bool {
	return r == '_' ||
		r >= '0' && r <= '9' ||
		r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z'
}

// Find returns the blocked-name matches in line, in original case.
func (m *Matcher) Find(line string) []string {
	return m.pattern.FindAllString(line, -1)
}

// Check runs the full guard: collects names and scans the staged diff in
// repoPath. Returns any findings.
func (g *Guard) Check(repoPath string) ([]Finding, error) {
	matcher, err := g.NewMatcher()
	if err != nil {
		return nil, err
	}
	if matcher == nil {
		return nil, nil
	}

	diffArgs := []string{"diff", "--cached", "--diff-filter=ACMR", "-U0", "--"}
	diffArgs = append(diffArgs, g.cfg.FilePatterns...)

	cmd := exec.Command("git", diffArgs...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}

	matched := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		for _, m := range matcher.Find(line) {
			matched[strings.ToLower(m)] = true
		}
	}

	var findings []Finding
	for m := range matched {
		findings = append(findings, Finding{Match: m})
	}
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Match < findings[j].Match
	})
	return findings, nil
}

// maxFileBytes mirrors the scanner's content cap: tracked files larger than
// this are skipped by CheckDir to bound work.
const maxFileBytes = 1 << 20

// CheckDir collects the blocked names and scans every tracked file in the
// working tree of repoPath, honoring the configured file patterns as git
// pathspecs. Binary files and files larger than maxFileBytes are skipped.
func (g *Guard) CheckDir(repoPath string) ([]Finding, error) {
	matcher, err := g.NewMatcher()
	if err != nil {
		return nil, err
	}
	if matcher == nil {
		return nil, nil
	}

	lsArgs := []string{"ls-files", "-z", "--"}
	lsArgs = append(lsArgs, g.cfg.FilePatterns...)
	cmd := exec.Command("git", lsArgs...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var findings []Finding
	for rel := range strings.SplitSeq(string(out), "\000") {
		if rel == "" {
			continue
		}
		abs := filepath.Join(repoPath, rel)
		info, err := os.Stat(abs)
		if err != nil {
			g.reportSkip(abs, "unreadable: "+err.Error())
			continue
		}
		if info.Size() > maxFileBytes {
			g.reportSkip(abs, fmt.Sprintf("exceeds %d bytes", maxFileBytes))
			continue
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", abs, err)
		}
		if isBinary(content) {
			continue
		}
		lineNum := 0
		for line := range strings.SplitSeq(string(content), "\n") {
			lineNum++
			matches := matcher.Find(line)
			if len(matches) == 0 {
				continue
			}
			// Dedup per line by exact string; distinct casings stay
			// distinct so each can become its own cleanup replacement.
			seen := make(map[string]bool, len(matches))
			for _, m := range matches {
				if seen[m] {
					continue
				}
				seen[m] = true
				findings = append(findings, Finding{Match: m, File: rel, Line: lineNum})
			}
		}
	}
	return findings, nil
}

// isBinary reports whether content looks binary (null byte in the first 512
// bytes), matching the scanner's heuristic.
func isBinary(content []byte) bool {
	header := content
	if len(header) > 512 {
		header = header[:512]
	}
	return bytes.IndexByte(header, 0) >= 0
}

// StagedFiles returns the list of staged file paths in the given repo.
func StagedFiles(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
