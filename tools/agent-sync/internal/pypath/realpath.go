package pypath

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RealPath is Path.resolve(), which is os.path.realpath(p, strict=False) in
// Python 3.13: an absolute path with every symlink resolved as far as the
// file system goes. A name that cannot be looked up is kept as it is, and so
// is a link in a loop. The loop follows posixpath.realpath step for step.
func RealPath(p string) string {
	var rest nameStack
	pending := rest.push(p)
	path := "/"
	if !strings.HasPrefix(p, "/") {
		path = workingDir()
	}
	// seen maps a link to its resolution, nil while it is being resolved.
	seen := map[string]*string{}
	for pending > 0 {
		top := rest.pop()
		if top.resolved {
			resolved := path
			seen[rest.pop().name] = &resolved
			continue
		}
		pending--
		switch top.name {
		case "", ".":
			continue
		case "..":
			if path = path[:strings.LastIndex(path, "/")]; path == "" {
				path = "/"
			}
			continue
		}
		next := strings.TrimSuffix(path, "/") + "/" + top.name
		if !isSymlink(next) {
			path = next
			continue
		}
		if done, ok := seen[next]; ok {
			path = next
			if done != nil {
				path = *done
			}
			continue
		}
		target, err := os.Readlink(next)
		if err != nil {
			path = next
			continue
		}
		if strings.HasPrefix(target, "/") {
			path = "/"
		}
		seen[next] = nil
		rest = append(rest, nameEntry{name: next}, nameEntry{resolved: true})
		pending += rest.push(target)
	}
	return path
}

// isSymlink reports whether path is a symlink; one that cannot be looked up
// is not.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
}

// nameStack is realpath's stack of names still to walk, top last. A resolved
// entry says the link entry below it now resolves to the current path.
type nameStack []nameEntry

type nameEntry struct {
	name     string
	resolved bool
}

// push adds the names of p so that its first name is on top, and returns
// how many it added.
func (s *nameStack) push(p string) int {
	names := strings.Split(p, "/")
	for i := len(names) - 1; i >= 0; i-- {
		*s = append(*s, nameEntry{name: names[i]})
	}
	return len(names)
}

func (s *nameStack) pop() nameEntry {
	top := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return top
}

// workingDir is os.getcwd(): the physical working directory, free of
// symlinks, which os.Getwd does not promise.
func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "/"
	}
	if physical, err := filepath.EvalSymlinks(wd); err == nil {
		return physical
	}
	return wd
}
