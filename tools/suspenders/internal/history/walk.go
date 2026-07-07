// Package history walks and rewrites a repository's commit history. The
// walker attributes every added line to the commit that introduced it; the
// rewriter streams git fast-export through a string/email transform into
// git fast-import.
package history

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// maxLineBytes is the longest diff or message line the walker will process.
// Longer lines make the walk fail closed rather than being silently skipped.
const maxLineBytes = 8 << 20

// Commit identifies one commit on the walked ref. IsTag marks a synthetic
// Commit standing in for an annotated tag object (Hash is the tagged object,
// Subject the tag name); such records only ever reach OnMessage.
type Commit struct {
	Hash        string
	AuthorName  string
	AuthorEmail string
	Subject     string
	Message     string
	IsTag       bool
}

// Visitor receives walk events. Nil callbacks are skipped. A callback error
// aborts the walk.
type Visitor struct {
	// OnMessage is called once per commit with the full commit message.
	OnMessage func(c Commit, message string) error
	// OnAddedLine is called for every line added by a commit, with the
	// line's number in the post-image of the file.
	OnAddedLine func(c Commit, path string, lineNum int, line string) error
	// OnNewFile is called for every path a commit adds, renames to, or
	// copies to, so file-name rules can run on it.
	OnNewFile func(c Commit, path string) error
}

// ResolveRef picks the ref to walk. An explicit branch wins; otherwise the
// remote default branch (origin/HEAD), then origin/main, origin/master,
// local main, master, and finally HEAD.
func ResolveRef(repoPath, branch string) (string, error) {
	if branch != "" {
		if !refExists(repoPath, branch) {
			return "", fmt.Errorf("ref %q does not exist", branch)
		}
		return branch, nil
	}

	out, err := gitOutput(repoPath, "symbolic-ref", "-q", "refs/remotes/origin/HEAD")
	if err == nil {
		if ref := strings.TrimPrefix(strings.TrimSpace(out), "refs/remotes/"); ref != "" {
			return ref, nil
		}
	}
	for _, ref := range []string{"origin/main", "origin/master", "main", "master", "HEAD"} {
		if refExists(repoPath, ref) {
			return ref, nil
		}
	}
	return "", fmt.Errorf("no walkable ref found in %s", repoPath)
}

func refExists(repoPath, ref string) bool {
	err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}").
		Run()
	return err == nil
}

func gitOutput(repoPath string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", repoPath}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// Walk visits every commit reachable from ref, oldest first, so a line is
// attributed to the earliest commit that introduced it. Annotated tag
// messages, which git log never reaches, are scanned after the commits.
func Walk(repoPath, ref string, v Visitor) error {
	commits, err := loadCommits(repoPath, ref)
	if err != nil {
		return err
	}
	if v.OnMessage != nil {
		for _, c := range commits {
			if err := v.OnMessage(c, c.Message); err != nil {
				return err
			}
		}
		if err := walkTags(repoPath, v); err != nil {
			return err
		}
	}
	if v.OnAddedLine == nil && v.OnNewFile == nil {
		return nil
	}
	byHash := make(map[string]Commit, len(commits))
	for _, c := range commits {
		byHash[c.Hash] = c
	}
	return walkDiffs(repoPath, ref, byHash, v)
}

// loadCommits reads commit metadata and messages for every commit on ref,
// ordered oldest first. Records are NUL-separated; fields are separated by
// \x01, which cannot appear in git author fields.
func loadCommits(repoPath, ref string) ([]Commit, error) {
	out, err := gitOutput(
		repoPath, "log", ref, "--reverse", "-z",
		"--format=%H%x01%an%x01%ae%x01%s%x01%B",
	)
	if err != nil {
		return nil, err
	}

	var commits []Commit
	for record := range strings.SplitSeq(out, "\x00") {
		record = strings.TrimPrefix(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x01", 5)
		if len(fields) != 5 {
			return nil, fmt.Errorf("malformed git log record: %q", record)
		}
		commits = append(commits, Commit{
			Hash:        fields[0],
			AuthorName:  fields[1],
			AuthorEmail: fields[2],
			Subject:     fields[3],
			Message:     fields[4],
		})
	}
	return commits, nil
}

// walkTags feeds every annotated tag's message through OnMessage, attributed
// to the tagged object. git log only visits commits, so a secret pasted into
// a tag message would otherwise be invisible to scan and never collected by
// clean. Lightweight tags carry no message of their own and are skipped.
func walkTags(repoPath string, v Visitor) error {
	out, err := gitOutput(
		repoPath, "for-each-ref", "refs/tags",
		"--format=%(objecttype) %(objectname) %(refname:short)",
	)
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 || fields[0] != "tag" {
			continue
		}
		c, message, err := readAnnotatedTag(repoPath, fields[1], fields[2])
		if err != nil {
			return err
		}
		if err := v.OnMessage(c, message); err != nil {
			return err
		}
	}
	return nil
}

// readAnnotatedTag reads an annotated tag object and returns a Commit
// standing in for it plus the tag message. The Commit is attributed to the
// tagged object so findings group with that object's other findings.
func readAnnotatedTag(repoPath, obj, name string) (Commit, string, error) {
	out, err := gitOutput(repoPath, "cat-file", "tag", obj)
	if err != nil {
		return Commit{}, "", err
	}
	header, message, _ := strings.Cut(out, "\n\n")
	c := Commit{Hash: obj, Subject: "tag " + name, IsTag: true}
	for hline := range strings.SplitSeq(header, "\n") {
		switch {
		case strings.HasPrefix(hline, "object "):
			c.Hash = strings.TrimSpace(strings.TrimPrefix(hline, "object "))
		case strings.HasPrefix(hline, "tagger "):
			c.AuthorName, c.AuthorEmail = parseIdent(strings.TrimPrefix(hline, "tagger "))
		}
	}
	return c, message, nil
}

// parseIdent splits a git identity line "Name <email> ts tz" into name and
// email. Trailing timestamp fields are ignored.
func parseIdent(s string) (name, email string) {
	lb := strings.IndexByte(s, '<')
	rb := strings.IndexByte(s, '>')
	if lb < 0 || rb < lb {
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(s[:lb]), s[lb+1 : rb]
}

// walkDiffs streams one `git log -p` over the whole history and parses it,
// attributing added lines and new files to their commit. \x01 marks the
// commit sentinel line; diff content never starts with it.
func walkDiffs(repoPath, ref string, commits map[string]Commit, v Visitor) error {
	cmd := exec.Command(
		"git", "-C", repoPath, "log", ref,
		"--reverse", "-p", "-U0", "--diff-filter=ACMRT", "--no-color",
		// first-parent diffs merge commits so conflict-resolution content
		// (which exists in neither parent) is visible; without this, merge
		// commits emit no diff at all. T catches symlink<->file typechanges,
		// which git splits into synthetic delete+add pairs.
		"--diff-merges=first-parent",
		"--format=%x01%H",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("git log -p pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git log -p: %w", err)
	}

	parseErr := parseDiffStream(bufio.NewReaderSize(stdout, 64*1024), commits, v)
	waitErr := cmd.Wait()
	if parseErr != nil {
		return parseErr
	}
	if waitErr != nil {
		return fmt.Errorf("git log -p: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func parseDiffStream(r *bufio.Reader, commits map[string]Commit, v Visitor) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxLineBytes)

	var (
		current     Commit
		haveCommit  bool
		currentPath string
		diffGitPath string
		newLine     int
		pendingNew  bool
		inHunk      bool
	)

	emitNewFile := func(path string) error {
		if v.OnNewFile == nil || path == "" {
			return nil
		}
		return v.OnNewFile(current, path)
	}

	// flushPendingNew emits a new-file event for a file whose "+++" line
	// never arrived: binary and empty new files carry no such line, so the
	// path is recovered from the "diff --git" header instead. Called when the
	// diff for the file ends (next diff, next commit, or stream end).
	flushPendingNew := func() error {
		if !pendingNew {
			return nil
		}
		pendingNew = false
		return emitNewFile(diffGitPath)
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "\x01"):
			if err := flushPendingNew(); err != nil {
				return err
			}
			c, ok := commits[line[1:]]
			if !ok {
				return fmt.Errorf("diff stream references unknown commit %q", line[1:])
			}
			current, haveCommit = c, true
			currentPath = ""
			diffGitPath = ""
			inHunk = false

		case !haveCommit:
			continue

		case strings.HasPrefix(line, "diff --git "):
			if err := flushPendingNew(); err != nil {
				return err
			}
			currentPath = ""
			diffGitPath = parseDiffGitNewPath(line)
			inHunk = false

		case strings.HasPrefix(line, "@@ "):
			// Hunk headers cannot be confused with content: added lines
			// inside a hunk always carry a "+" prefix first.
			start, err := parseHunkNewStart(line)
			if err != nil {
				return err
			}
			newLine = start
			inHunk = true

		// Inside a hunk, everything is content. Header-looking lines such
		// as "+++ i;" (an added "++ i;") must not be parsed as headers.
		case inHunk && strings.HasPrefix(line, "+"):
			if v.OnAddedLine != nil && currentPath != "" {
				if err := v.OnAddedLine(current, currentPath, newLine, line[1:]); err != nil {
					return err
				}
			}
			newLine++

		case inHunk && strings.HasPrefix(line, " "):
			newLine++

		case inHunk:
			// Removed lines and "\ No newline at end of file" markers.

		case strings.HasPrefix(line, "new file mode "):
			pendingNew = true

		case strings.HasPrefix(line, "rename to "):
			if err := emitNewFile(parseDiffPath(strings.TrimPrefix(line, "rename to "))); err != nil {
				return err
			}

		case strings.HasPrefix(line, "copy to "):
			if err := emitNewFile(parseDiffPath(strings.TrimPrefix(line, "copy to "))); err != nil {
				return err
			}

		case strings.HasPrefix(line, "+++ "):
			currentPath = parseTargetPath(line)
			if pendingNew {
				pendingNew = false
				if err := emitNewFile(currentPath); err != nil {
					return err
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("parse diff stream: %w", err)
	}
	return flushPendingNew()
}

// parseDiffGitNewPath recovers the added file's path from a
// "diff --git a/PATH b/PATH" header. For a newly added file the two paths are
// identical, so the b-side is the second half of the remainder; this is the
// only path source for binary and empty new files, whose diffs carry no
// "+++" line. C-quoted paths are unquoted.
func parseDiffGitNewPath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if strings.HasPrefix(rest, "\"") {
		if idx := strings.LastIndex(rest, " \""); idx >= 0 {
			if unquoted, err := strconv.Unquote(rest[idx+1:]); err == nil {
				return strings.TrimPrefix(unquoted, "b/")
			}
		}
		return ""
	}
	// Unquoted symmetric form "a/PATH b/PATH": len = 5 + 2*len(PATH).
	if len(rest) >= 5 && (len(rest)-5)%2 == 0 {
		return strings.TrimPrefix(rest[3+(len(rest)-5)/2:], "b/")
	}
	return ""
}

// parseTargetPath extracts the post-image path from a "+++ b/<path>" line.
// Returns "" for /dev/null (pure deletions).
func parseTargetPath(line string) string {
	raw := strings.TrimSuffix(strings.TrimPrefix(line, "+++ "), "\t")
	if raw == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(parseDiffPath(raw), "b/")
}

// parseDiffPath undoes git's C-style quoting of unusual path names.
func parseDiffPath(raw string) string {
	raw = strings.TrimSuffix(raw, "\t")
	if strings.HasPrefix(raw, "\"") {
		if unquoted, err := strconv.Unquote(raw); err == nil {
			return unquoted
		}
	}
	return raw
}

// parseHunkNewStart extracts the post-image start line from a hunk header
// "@@ -a,b +c,d @@". With d == 0 git reports the line before the hunk; that
// is harmless because such hunks contain no added lines.
func parseHunkNewStart(line string) (int, error) {
	_, rest, found := strings.Cut(line, " +")
	if !found {
		return 0, fmt.Errorf("malformed hunk header: %q", line)
	}
	if end := strings.IndexAny(rest, ", "); end >= 0 {
		rest = rest[:end]
	}
	start, err := strconv.Atoi(rest)
	if err != nil {
		return 0, fmt.Errorf("malformed hunk header %q: %w", line, err)
	}
	return start, nil
}
