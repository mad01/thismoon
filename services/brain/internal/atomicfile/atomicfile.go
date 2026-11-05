// Package atomicfile replaces a file's content with one rename. The bytes go
// to a temporary file with a unique name beside the target first, so a
// reader never sees a half-written file and two writers never share a
// temporary file.
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
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return discard(tmp, err)
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return discard(tmp, err)
	}
	if err := f.Close(); err != nil {
		return discard(tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return discard(tmp, err)
	}
	return nil
}

// discard removes the temporary file after a failed write and returns the
// write's error, joined with the removal's when that fails too.
func discard(tmp string, cause error) error {
	if err := os.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(cause, err)
	}
	return cause
}
