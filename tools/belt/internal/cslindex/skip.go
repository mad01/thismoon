package cslindex

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/cslignore"
)

// nulScanChunk is how much of a file one read takes while looking for a
// NUL byte and counting lines.
const nulScanChunk = 32 << 10

// Skipped reports whether a csl_ask call scoped to the file at rel (a slash
// path below the working tree root) cannot answer from it. Two sets of
// rules decide. csl's walk keeps a file out of the index, name and all, when
// a directory on the way is one the walk never enters or a symlink, when the
// root's .cslignore matches the path, and when the file is not a regular
// file or is over cslignore.SizeMax bytes. zoekt keeps every other file at
// least by name, so the call's f: query finds it, and csl_ask reads it from
// disk; csl_ask refuses it only when it holds a NUL byte anywhere. Every
// hidden directory counts as left out: csl enters only an allowed few, and a
// file there only when git tracks it, which belt cannot tell without running
// git. A file that cannot be read counts as left out too.
func Skipped(root, rel string) bool {
	_, ok := AskLines(root, rel)
	return !ok
}

// AskLines returns the lines csl_ask counts in the file at rel, the count a
// call's max_lines must cover, from the same read that looks for a NUL
// byte. ok is false when Skipped reports the file.
func AskLines(root, rel string) (lines int, ok bool) {
	if skippedPath(root, rel) {
		return 0, false
	}
	return contentLines(filepath.Join(root, filepath.FromSlash(rel)))
}

// skippedPath applies the walk's path rules: kit/cslignore's directory
// names, symlinked directories (the walk does not follow them), and the
// root's .cslignore.
func skippedPath(root, rel string) bool {
	segs := strings.Split(rel, "/")
	dir := root
	for _, seg := range segs[:len(segs)-1] {
		if cslignore.SkipDir(seg, nil) {
			return true
		}
		dir = filepath.Join(dir, seg)
		if st, err := os.Lstat(dir); err != nil || !st.IsDir() {
			return true
		}
	}
	return cslignore.Load(root).Match(rel)
}

// contentLines applies the walk's file rules and csl_ask's binary check,
// and counts the lines of a file that passes both.
func contentLines(file string) (int, bool) {
	st, err := os.Lstat(file)
	if err != nil || !st.Mode().IsRegular() || st.Size() > cslignore.SizeMax {
		return 0, false
	}
	return scanText(file)
}

// scanText counts file's lines the way csl_ask does, ok false when it holds
// a NUL byte or cannot be read. It reads a chunk at a time and at most
// cslignore.SizeMax bytes, should the file have grown since its size was
// checked.
func scanText(file string) (int, bool) {
	f, err := os.Open(file)
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	r := io.LimitReader(f, cslignore.SizeMax)
	buf := make([]byte, nulScanChunk)
	var count lineCount
	for {
		n, err := r.Read(buf)
		if bytes.IndexByte(buf[:n], 0) >= 0 {
			return 0, false
		}
		count.add(buf[:n])
		if errors.Is(err, io.EOF) {
			return count.lines(), true
		}
		if err != nil {
			return 0, false
		}
	}
}

// lineCount counts lines chunk by chunk with csl_ask's rule: trailing
// newlines are dropped first, then the rest splits at each newline, so a
// file ending in one has no empty last line and an empty file is one line.
type lineCount struct {
	newlines int // every newline seen
	trailing int // the newlines at the end of what was seen
}

func (c *lineCount) add(chunk []byte) {
	c.newlines += bytes.Count(chunk, []byte{'\n'})
	body := bytes.TrimRight(chunk, "\n")
	if len(body) == 0 {
		c.trailing += len(chunk)
		return
	}
	c.trailing = len(chunk) - len(body)
}

func (c *lineCount) lines() int { return c.newlines - c.trailing + 1 }
