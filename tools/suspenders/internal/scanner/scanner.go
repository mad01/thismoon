package scanner

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gobwas/glob"
)

// ErrFindingsFound is returned when scan finds secrets and --fail-on-findings is set.
var ErrFindingsFound = errors.New("scanner: secrets detected")

// inlineIgnoreMarker suppresses all findings on a line that contains it,
// e.g. `password = "example-not-real" // suspenders:ignore`.
const inlineIgnoreMarker = "suspenders:ignore"

// maxLineBytes is the longest single line the scanner will process. Lines
// beyond this (e.g. enormous minified bundles) make the scan fail closed
// rather than being silently skipped.
const maxLineBytes = 8 << 20

// maxFileBytes is the largest tracked file ScanDir reads for content scanning.
// Larger files (build artifacts, vendored blobs) are skipped to bound work;
// file-name rules still apply to them.
const maxFileBytes = 1 << 20

// Allowance represents a known-safe exact value that should be permitted.
type Allowance struct {
	Match       string
	Description string
	Paths       []glob.Glob
}

// SkippedFile records a tracked or staged file that was not content-scanned,
// with the reason it was skipped (too large, unreadable). A skip is surfaced,
// never silent: a green scan must not hide files the scanner never read.
type SkippedFile struct {
	Path   string
	Reason string
}

// Scanner holds the detection rules and optional per-repo ignore configuration.
type Scanner struct {
	Rules      []Rule
	FileRules  []FileRule
	Allowlist  []Allowance
	IgnoreFile string

	// OnSkip, when set, is called for each file ScanDir/ScanStaged skips
	// instead of content-scanning it. Set it once before scanning; if the
	// Scanner is shared across goroutines the callback must be safe for
	// concurrent use. When nil, skips are logged to stderr so they are never
	// silent.
	OnSkip func(SkippedFile)

	ignoreOnce sync.Once
	ignoreCfg  *IgnoreConfig
	ignoreErr  error
}

// reportSkip surfaces a file the scanner did not read. It routes to OnSkip
// when set, otherwise to stderr, so a skip is never dropped silently.
func (s *Scanner) reportSkip(path, reason string) {
	sk := SkippedFile{Path: path, Reason: reason}
	if s.OnSkip != nil {
		s.OnSkip(sk)
		return
	}
	fmt.Fprintf(os.Stderr, "suspenders: skipped %s (%s)\n", path, reason)
}

// New returns a Scanner using the provided rules and the default file rules.
func New(rules []Rule) *Scanner {
	return &Scanner{Rules: rules, FileRules: DefaultFileRules}
}

// NewWatchRule compiles a user-defined watch pattern into a Rule.
func NewWatchRule(id, description, pattern, severity string) (Rule, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Rule{}, fmt.Errorf("invalid watch pattern %q: %w", id, err)
	}
	if severity == "" {
		severity = "medium"
	}
	return Rule{
		ID:          id,
		Description: description,
		Pattern:     re,
		Severity:    severity,
	}, nil
}

// AddAllowance adds a known-safe value to the allowlist.
func (s *Scanner) AddAllowance(match, description string, paths []string) error {
	a := Allowance{Match: match, Description: description}
	for _, p := range paths {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return fmt.Errorf("invalid allowlist path glob %q: %w", p, err)
		}
		a.Paths = append(a.Paths, g)
	}
	s.Allowlist = append(s.Allowlist, a)
	return nil
}

func (s *Scanner) isAllowed(rawMatch, filePath string) bool {
	for _, a := range s.Allowlist {
		if a.Match != rawMatch {
			continue
		}
		if len(a.Paths) == 0 {
			return true
		}
		for _, g := range a.Paths {
			if g.Match(filePath) {
				return true
			}
		}
	}
	return false
}

// loadIgnoreOnce loads the per-repo ignore config exactly once and applies
// its watch rules and allowlist entries to the scanner.
func (s *Scanner) loadIgnoreOnce() (*IgnoreConfig, error) {
	s.ignoreOnce.Do(func() {
		if s.IgnoreFile == "" {
			return
		}
		ic, err := LoadIgnoreConfig(s.IgnoreFile)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err != nil {
			s.ignoreErr = err
			return
		}
		for _, a := range ic.Allowlist {
			_ = s.AddAllowance(a.Match, a.Description, a.Paths)
		}
		for _, w := range ic.Watch {
			r, err := NewWatchRule(w.ID, w.Description, w.Pattern, w.Severity)
			if err != nil {
				continue
			}
			s.Rules = append(s.Rules, r)
		}
		s.ignoreCfg = ic
	})
	return s.ignoreCfg, s.ignoreErr
}

// ScanFile scans a single file for secrets. It skips binary files detected by
// the presence of a null byte in the first 512 bytes.
func (s *Scanner) ScanFile(path string) ([]Finding, error) {
	ic, err := s.loadIgnoreOnce()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s.scanContent(path, content, ic)
}

func isBinary(content []byte) bool {
	header := content
	if len(header) > 512 {
		header = header[:512]
	}
	return bytes.IndexByte(header, 0) >= 0
}

// applicableRules filters the scanner's rules down to those that apply to
// the file at path, honoring per-rule ExcludeFiles basename globs.
func (s *Scanner) applicableRules(path string) []Rule {
	base := filepath.Base(path)
	rules := make([]Rule, 0, len(s.Rules))
	for _, rule := range s.Rules {
		excluded := false
		for _, pattern := range rule.ExcludeFiles {
			if ok, _ := filepath.Match(pattern, base); ok {
				excluded = true
				break
			}
		}
		if !excluded {
			rules = append(rules, rule)
		}
	}
	return rules
}

type span struct{ start, end int }

func overlaps(spans []span, m span) bool {
	for _, sp := range spans {
		if m.start < sp.end && sp.start < m.end {
			return true
		}
	}
	return false
}

// scanLine runs rules over a single line and returns its findings plus the
// matched spans. Context redaction is left to the caller. A line carrying
// the inline ignore marker yields no findings.
func (s *Scanner) scanLine(
	rules []Rule,
	path string,
	lineNum int,
	line string,
	ic *IgnoreConfig,
) ([]Finding, []span) {
	if strings.Contains(line, inlineIgnoreMarker) {
		return nil, nil
	}

	var findings []Finding
	var spans []span
	for _, rule := range rules {
		for _, m := range rule.Pattern.FindAllStringSubmatchIndex(line, -1) {
			matchSpan := span{m[0], m[1]}
			match := line[matchSpan.start:matchSpan.end]

			secret := match
			if len(m) >= 4 && m[2] >= 0 {
				secret = line[m[2]:m[3]]
			}
			if rule.SkipOverlapping && overlaps(spans, matchSpan) {
				continue
			}
			if rule.MinEntropy > 0 && shannonEntropy(secret) < rule.MinEntropy {
				continue
			}
			if rule.Filter != nil && !rule.Filter(secret) {
				continue
			}
			if s.isAllowed(match, path) {
				continue
			}

			finding := Finding{
				File:     path,
				Line:     lineNum,
				Column:   matchSpan.start + 1,
				Rule:     rule,
				RawMatch: match,
				Match:    Redact(match),
			}
			if ic != nil && ic.ShouldIgnore(finding) {
				continue
			}
			findings = append(findings, finding)
			spans = append(spans, matchSpan)
		}
	}
	return findings, spans
}

// redactContext masks every matched span in line so one finding's context
// never exposes another finding's raw secret. Redact is length-preserving,
// so overlapping spans are safe.
func redactContext(line string, spans []span) string {
	ctx := []byte(line)
	for _, sp := range spans {
		copy(ctx[sp.start:sp.end], Redact(line[sp.start:sp.end]))
	}
	return string(ctx)
}

// ScanLine scans a single line as if it were line lineNum of the file at
// path. Used for content that never touches the working tree, e.g. added
// diff lines and commit messages during a history scan.
func (s *Scanner) ScanLine(path string, lineNum int, line string) ([]Finding, error) {
	ic, err := s.loadIgnoreOnce()
	if err != nil {
		return nil, err
	}
	findings, spans := s.scanLine(s.applicableRules(path), path, lineNum, line, ic)
	if len(findings) == 0 {
		return nil, nil
	}
	ctx := redactContext(line, spans)
	for i := range findings {
		findings[i].Context = ctx
	}
	return findings, nil
}

// scanContent runs all applicable rules over content line by line.
// Binary content (null byte in the first 512 bytes) is skipped.
func (s *Scanner) scanContent(path string, content []byte, ic *IgnoreConfig) ([]Finding, error) {
	if isBinary(content) {
		return nil, nil
	}

	rules := s.applicableRules(path)

	var findings []Finding
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 64*1024), maxLineBytes)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Text()

		lineFindings, spans := s.scanLine(rules, path, lineNum, line, ic)
		if len(lineFindings) == 0 {
			continue
		}
		ctx := redactContext(line, spans)
		for i := range lineFindings {
			lineFindings[i].Context = ctx
		}
		findings = append(findings, lineFindings...)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return findings, nil
}

// checkFileRules returns file-level findings for a staged or tracked file,
// based on its name alone. rel is the repo-relative path, abs the display path.
func (s *Scanner) checkFileRules(rel, abs string, ic *IgnoreConfig) []Finding {
	base := filepath.Base(rel)
	var findings []Finding
	for _, fr := range s.FileRules {
		if !fr.Matches(base) {
			continue
		}
		if s.isAllowed(rel, rel) || s.isAllowed(rel, abs) {
			continue
		}
		finding := Finding{
			File:     abs,
			Rule:     Rule{ID: fr.ID, Description: fr.Description, Severity: fr.Severity},
			RawMatch: rel,
			Match:    rel,
			Context:  "sensitive file type; flagged by name regardless of content",
		}
		if ic != nil && ic.ShouldIgnore(finding) {
			continue
		}
		findings = append(findings, finding)
	}
	return findings
}

// CheckFileName runs the file-name rules against a repo-relative path that
// may not exist in the working tree, e.g. a file added in a past commit.
func (s *Scanner) CheckFileName(rel string) ([]Finding, error) {
	ic, err := s.loadIgnoreOnce()
	if err != nil {
		return nil, err
	}
	return s.checkFileRules(rel, rel, ic), nil
}

// ScanDir scans all git-tracked files in root. Uses `git ls-files` so
// .gitignore is respected automatically and untracked files are skipped.
func (s *Scanner) ScanDir(root string) ([]Finding, error) {
	ic, err := s.loadIgnoreOnce()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w", root, err)
	}

	var findings []Finding
	for rel := range strings.SplitSeq(string(out), "\000") {
		if rel == "" {
			continue
		}
		abs := filepath.Join(root, rel)
		findings = append(findings, s.checkFileRules(rel, abs, ic)...)

		info, err := os.Lstat(abs)
		if err != nil {
			s.reportSkip(abs, "unreadable: "+err.Error())
			continue
		}
		if !info.Mode().IsRegular() {
			s.reportSkip(abs, "not a regular file")
			continue
		}
		if info.Size() > maxFileBytes {
			s.reportSkip(abs, fmt.Sprintf("exceeds %d bytes", maxFileBytes))
			continue
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", abs, err)
		}
		ff, err := s.scanContent(abs, content, ic)
		if err != nil {
			return nil, err
		}
		findings = append(findings, ff...)
	}
	return findings, nil
}

// ScanStaged scans the files currently staged in the git index at repoPath.
// Content is read from the index (`git show :<path>`), not the working tree,
// so what gets scanned is exactly what would be committed. Errors fail the
// scan rather than silently passing a file.
func (s *Scanner) ScanStaged(repoPath string) ([]Finding, error) {
	ic, err := s.loadIgnoreOnce()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"git",
		"-C",
		repoPath,
		"diff",
		"--cached",
		"--name-only",
		"--diff-filter=ACMR",
		"-z",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}

	var findings []Finding
	for rel := range strings.SplitSeq(string(out), "\000") {
		if rel == "" {
			continue
		}
		abs := filepath.Join(repoPath, rel)
		findings = append(findings, s.checkFileRules(rel, abs, ic)...)

		// Bound work the same way ScanDir does, but read the size from the
		// index. A blob we cannot read fails closed (returns an error, per
		// ADR 0001); one that merely exceeds the cap is surfaced as a skip.
		size, err := stagedBlobSize(repoPath, rel)
		if err != nil {
			return nil, err
		}
		if size > maxFileBytes {
			s.reportSkip(abs, fmt.Sprintf("staged blob exceeds %d bytes", maxFileBytes))
			continue
		}

		content, err := stagedContent(repoPath, rel)
		if err != nil {
			return nil, err
		}
		ff, err := s.scanContent(abs, content, ic)
		if err != nil {
			return nil, err
		}
		findings = append(findings, ff...)
	}
	return findings, nil
}

// stagedBlobSize returns the byte size of a file's staged blob, read from the
// git index without materializing its content. Lets ScanStaged cap oversize
// files before buffering them.
func stagedBlobSize(repoPath, rel string) (int64, error) {
	cmd := exec.Command("git", "-C", repoPath, "cat-file", "-s", ":"+rel)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git cat-file -s :%s: %w", rel, err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse staged size for :%s: %w", rel, err)
	}
	return n, nil
}

// stagedContent reads a file's content from the git index.
func stagedContent(repoPath, rel string) ([]byte, error) {
	cmd := exec.Command("git", "-C", repoPath, "show", ":"+rel)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show :%s: %w", rel, err)
	}
	return out, nil
}
