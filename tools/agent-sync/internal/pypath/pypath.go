// Package pypath reproduces the parts of Python's path handling the sync
// scripts' output depends on, so the sections that port them print the
// same paths and the same errors: pathlib's normalizing and joining,
// Path.expanduser, Path.resolve (os.path.realpath in Python 3.13), a sorted
// Path.glob, and the text of an OSError.
//
// Paths are plain strings with "/" separators, as on macOS and Linux.
package pypath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// Clean normalizes p the way pathlib does: repeated slashes and "."
// segments go, ".." stays, and an empty path is ".". Exactly two leading
// slashes are kept, as POSIX allows.
func Clean(p string) string {
	root := ""
	if strings.HasPrefix(p, "/") {
		root = "/"
		if strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "///") {
			root = "//"
		}
	}
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		if seg != "" && seg != "." {
			parts = append(parts, seg)
		}
	}
	if out := root + strings.Join(parts, "/"); out != "" {
		return out
	}
	return "."
}

// Join is pathlib's base / rel, normalized: an absolute rel replaces base,
// and a base that is the root adds no slash of its own.
func Join(base, rel string) string {
	if strings.HasPrefix(rel, "/") || base == "" {
		return Clean(rel)
	}
	return Clean(strings.TrimSuffix(base, "/") + "/" + rel)
}

// ExpandUser is Path(p).expanduser(): p normalized, with a leading "~"
// segment replaced by the home directory ($HOME, or the user database when
// $HOME is unset) and a leading "~user" by that user's. A user that does not
// exist is an error, as it is in Python.
func ExpandUser(p string) (string, error) {
	p = Clean(p)
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	first, rest, _ := strings.Cut(p, "/")
	home, err := homeDir(first[1:])
	if err != nil {
		return "", err
	}
	if home == "" {
		home = "/"
	}
	return Join(home, rest), nil
}

// homeDir returns name's home directory, or the current user's for "",
// with trailing slashes removed.
func homeDir(name string) (string, error) {
	var u *user.User
	var err error
	switch home, ok := os.LookupEnv("HOME"); {
	case name == "" && ok:
		return strings.TrimRight(home, "/"), nil
	case name == "":
		u, err = user.Current()
	default:
		u, err = user.Lookup(name)
	}
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return strings.TrimRight(u.HomeDir, "/"), nil
}

// Glob is sorted(Path(dir).glob(pattern)) for a pattern with no "/": the
// entries of dir whose name matches, hidden ones included, in name order. A
// dir that is missing or cannot be read has none.
func Glob(dir, pattern string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if ok, _ := filepath.Match(pattern, e.Name()); ok {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths
}

// ErrorText renders err the way str(exc) renders an OSError, e.g.
// "[Errno 2] No such file or directory: '/path'". An error that carries no
// errno keeps its own message.
func ErrorText(err error) string {
	var pathErr *fs.PathError
	var errno syscall.Errno
	if !errors.As(err, &pathErr) || !errors.As(pathErr.Err, &errno) {
		return err.Error()
	}
	// Go spells strerror in lower case; C capitalizes the first letter.
	msg := errno.Error()
	if msg != "" {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
	}
	return fmt.Sprintf("[Errno %d] %s: %s", int(errno), msg, pyjson.Repr(pathErr.Path))
}
