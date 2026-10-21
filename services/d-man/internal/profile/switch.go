package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/d-man/internal/config"
)

// fileMode is the mode of every file Switch writes. Nothing here is secret,
// and the user-run d-man list and config commands read the routes file.
const fileMode = 0o644

// Switch writes the active routes file at routes for profile name, then
// records name as active. home replaces a leading ~/ in games_dir.
//
// It reads BaseFile and BlocklistFile(name) from the directory holding
// routes. The block list is required for every profile but Default, so a
// misspelled name fails instead of silently blocking nothing. The assembled
// file must validate before it replaces the live one: a bad input changes
// nothing, and the daemon keeps serving the last good routes.
func Switch(routes, name, home string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	dir := filepath.Dir(routes)
	base, err := os.ReadFile(filepath.Join(dir, BaseFile))
	if err != nil {
		return fmt.Errorf("read base routes: %w", err)
	}
	blocklist, err := readBlocklist(dir, name)
	if err != nil {
		return err
	}
	out := Render(base, blocklist, home)
	if _, err := config.Parse(out, routes); err != nil {
		return fmt.Errorf("profile %s would write invalid routes, %s left unchanged: %w",
			name, routes, err)
	}
	if err := writeFileAtomic(routes, out); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, RecordFile), []byte(name+"\n"))
}

// Current returns the active profile recorded beside routes, or Default when
// nothing is recorded.
func Current(routes string) (string, error) {
	path := filepath.Join(filepath.Dir(routes), RecordFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default, nil
	}
	if err != nil {
		return "", fmt.Errorf("read active profile: %w", err)
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return Default, nil
	}
	if err := CheckName(name); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return name, nil
}

// available lists the profiles a switch accepts in dir, sorted: Default plus
// every name with a block-list file. It only feeds an error message, so a
// directory it cannot read just lists Default.
func available(dir string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, BlocklistFile("*")))
	names := []string{Default}
	for _, m := range matches {
		name := strings.TrimSuffix(
			strings.TrimPrefix(filepath.Base(m), blocklistPrefix), blocklistSuffix)
		if CheckName(name) == nil {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// readBlocklist returns the block list for profile name in dir. A missing
// file is an error for every profile but Default, which then blocks nothing.
func readBlocklist(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, BlocklistFile(name))
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, fs.ErrNotExist) && name == Default:
		return []byte{}, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("profile %q has no block list: %s does not exist (profiles: %s)",
			name, path, strings.Join(available(dir), ", "))
	default:
		return nil, fmt.Errorf("read block list: %w", err)
	}
}

// writeFileAtomic replaces path with data through a temp file in the same
// directory and a rename, so the daemon's file watch never reads a half
// written file. A symlink at path is replaced by a regular file, not
// followed.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // a no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, fileMode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
