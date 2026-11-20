package secretscan

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// stagedEntry is a file a commit of the git index would add, copy, modify or
// rename: its repo-relative path, the id of the blob the index holds for it,
// and that blob's size once blobSizes has read it.
type stagedEntry struct {
	rel  string
	oid  string
	size int64
}

// stagedEntries lists the staged files ScanStaged scans, with their index
// blob ids, from one `git diff --cached --raw`. The diff filter is the one
// the file list has always used, so the set of scanned files is unchanged.
func stagedEntries(repoPath string) ([]stagedEntry, error) {
	cmd := exec.Command(
		"git",
		"-C",
		repoPath,
		"diff",
		"--cached",
		"--raw",
		"--no-abbrev",
		"--diff-filter=ACMR",
		"-z",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}
	return parseRawDiff(out)
}

// parseRawDiff parses `git diff --raw -z --no-abbrev` output. Each record is
// ":<src mode> <dst mode> <src oid> <dst oid> <status>" followed by one path,
// or by a source and a destination path for a copy or rename. An entry keeps
// the destination path and the destination (index) blob id.
func parseRawDiff(out []byte) ([]stagedEntry, error) {
	fields := strings.Split(string(out), "\000")
	var entries []stagedEntry
	for i := 0; i < len(fields); i++ {
		header := fields[i]
		if header == "" {
			continue
		}
		meta := strings.Fields(strings.TrimPrefix(header, ":"))
		if !strings.HasPrefix(header, ":") || len(meta) != 5 {
			return nil, fmt.Errorf("parse git diff --raw: unexpected record %q", header)
		}
		paths := 1
		if status := meta[4]; status[0] == 'R' || status[0] == 'C' {
			paths = 2
		}
		i += paths
		if i >= len(fields) || fields[i] == "" {
			return nil, fmt.Errorf("parse git diff --raw: record %q has no path", header)
		}
		entries = append(entries, stagedEntry{rel: fields[i], oid: meta[3]})
	}
	return entries, nil
}

// blobSizes sets each entry's size from one `git cat-file --batch-check`,
// which reads object headers only, so an oversize blob is never inflated. An
// object git cannot find fails the call, as an unreadable blob must (ADR 0001),
// and so does an object that is not a blob: a staged gitlink names a commit,
// whose raw bytes are not the file content the scan promises to read.
func blobSizes(repoPath string, entries []stagedEntry) error {
	cmd := exec.Command("git", "-C", repoPath, "cat-file", "--batch-check")
	cmd.Stdin = strings.NewReader(oidLines(entries))
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git cat-file --batch-check: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(entries) {
		return fmt.Errorf(
			"git cat-file --batch-check: %d results for %d staged files",
			len(lines), len(entries),
		)
	}
	for i, line := range lines {
		// "<oid> <type> <size>", or "<oid> missing" for an absent object.
		meta := strings.Fields(line)
		if len(meta) != 3 || meta[0] != entries[i].oid {
			return fmt.Errorf("git cat-file --batch-check :%s: %q", entries[i].rel, line)
		}
		if meta[1] != "blob" {
			return fmt.Errorf("staged :%s is a %s, not a file", entries[i].rel, meta[1])
		}
		size, err := strconv.ParseInt(meta[2], 10, 64)
		if err != nil {
			return fmt.Errorf("parse staged size for :%s: %w", entries[i].rel, err)
		}
		entries[i].size = size
	}
	return nil
}

// oidLines renders the entries' blob ids as cat-file batch input, one per line.
func oidLines(entries []stagedEntry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.oid)
		b.WriteByte('\n')
	}
	return b.String()
}

// blobStream reads the content of the requested blobs, in request order, out
// of one `git cat-file --batch` process, so ScanStaged scans each blob as it
// arrives rather than holding every staged file in memory at once.
type blobStream struct {
	cmd       *exec.Cmd
	out       *bufio.Reader
	stderr    bytes.Buffer
	remaining int
}

// openBlobStream starts the cat-file process for entries. With no entries it
// starts nothing, and next must not be called.
func openBlobStream(repoPath string, entries []stagedEntry) (*blobStream, error) {
	b := &blobStream{remaining: len(entries)}
	if len(entries) == 0 {
		return b, nil
	}
	b.cmd = exec.Command("git", "-C", repoPath, "cat-file", "--batch")
	b.cmd.Stdin = strings.NewReader(oidLines(entries))
	b.cmd.Stderr = &b.stderr
	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch: %w", err)
	}
	if err := b.cmd.Start(); err != nil {
		return nil, fmt.Errorf("git cat-file --batch: %w", err)
	}
	b.out = bufio.NewReaderSize(stdout, 64*1024)
	return b, nil
}

// next returns the content of the next blob, which must be e's. Each record
// is "<oid> <type> <size>\n", the content, then "\n".
func (b *blobStream) next(e stagedEntry) ([]byte, error) {
	header, err := b.out.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch :%s: %w", e.rel, err)
	}
	meta := strings.Fields(header)
	if len(meta) != 3 || meta[0] != e.oid {
		return nil, fmt.Errorf("git cat-file --batch :%s: %q", e.rel, strings.TrimSpace(header))
	}
	size, err := strconv.ParseInt(meta[2], 10, 64)
	if err != nil || size != e.size {
		return nil, fmt.Errorf("git cat-file --batch :%s: size %q, want %d", e.rel, meta[2], e.size)
	}
	content := make([]byte, size+1)
	if _, err := io.ReadFull(b.out, content); err != nil {
		return nil, fmt.Errorf("git cat-file --batch :%s: %w", e.rel, err)
	}
	if content[size] != '\n' {
		return nil, fmt.Errorf("git cat-file --batch :%s: record not terminated", e.rel)
	}
	b.remaining--
	return content[:size], nil
}

// close ends the cat-file process. A stream closed before every blob was
// read kills the process, since the caller already has the error that
// stopped it; a stream read to the end reports how git exited.
func (b *blobStream) close() error {
	if b.cmd == nil {
		return nil
	}
	if b.remaining > 0 {
		_ = b.cmd.Process.Kill()
		_ = b.cmd.Wait()
		return nil
	}
	if err := b.cmd.Wait(); err != nil {
		return fmt.Errorf(
			"git cat-file --batch: %w: %s",
			err, strings.TrimSpace(b.stderr.String()),
		)
	}
	return nil
}
