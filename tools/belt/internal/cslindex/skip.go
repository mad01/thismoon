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
// NUL byte.
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
	return skippedPath(root, rel) || skippedContent(filepath.Join(root, filepath.FromSlash(rel)))
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

// skippedContent applies the walk's file rules and csl_ask's binary check.
func skippedContent(file string) bool {
	st, err := os.Lstat(file)
	if err != nil || !st.Mode().IsRegular() || st.Size() > cslignore.SizeMax {
		return true
	}
	nul, err := hasNUL(file)
	return err != nil || nul
}

// hasNUL reports whether file holds a NUL byte, reading it a chunk at a
// time and at most cslignore.SizeMax bytes of it, should it have grown
// since its size was checked.
func hasNUL(file string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	r := io.LimitReader(f, cslignore.SizeMax)
	buf := make([]byte, nulScanChunk)
	for {
		n, err := r.Read(buf)
		if bytes.IndexByte(buf[:n], 0) >= 0 {
			return true, nil
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
