// Package atomicfile writes files so that a reader never sees a half-written
// one. The bytes go to a temporary file with a unique name beside the target
// first, so two writers never share a temporary file; Write then renames it
// over the target and Create links it to a path that must not exist yet.
package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Write writes data to path with perm through a uniquely named temporary
// file in path's directory, then renames it over path. On failure the
// temporary file is removed and path is left as it was.
func Write(path string, data []byte, perm fs.FileMode) error {
	tmp, err := writeTemp(path, data, perm)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return discard(tmp, err)
	}
	return nil
}

// writeTemp writes data with perm to a new, uniquely named temporary file
// beside path and returns its name.
func writeTemp(path string, data []byte, perm fs.FileMode) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", discard(tmp, err)
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return "", discard(tmp, err)
	}
	if err := f.Close(); err != nil {
		return "", discard(tmp, err)
	}
	return tmp, nil
}

// discard removes the temporary file after a failed write and returns the
// write's error, joined with the removal's when that fails too.
func discard(tmp string, cause error) error {
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(cause, err)
	}
	return cause
}

// Create writes data to a new file at path with perm, complete or not at
// all, and fails with an error matching fs.ErrExist when path already
// exists. The bytes go to a temporary file first, which is then hard-linked
// to path: the link is the exclusive create, so of two writers racing for
// one path exactly one wins and a reader never sees a partial file.
func Create(path string, data []byte, perm fs.FileMode) error {
	tmp, err := writeTemp(path, data, perm)
	if err != nil {
		return err
	}
	if err := os.Link(tmp, path); err != nil {
		return discard(tmp, err)
	}
	return os.Remove(tmp)
}
