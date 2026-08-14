// Package repofind discovers git repositories on the local filesystem and
// parses their remotes. It is the shared git/file-tree layer under the
// suspenders pre-commit guard and the belt write-time firewall, so the two
// tools discover the same repos and derive the same org/repo names from
// them.
package repofind

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/gobwas/glob"
)

const workerCount = 32

var (
	sshRemoteRe   = regexp.MustCompile(`^[^@]+@[^:]+:(.+?)(?:\.git)?$`)
	httpsRemoteRe = regexp.MustCompile(`^https?://[^/]+/(.+?)(?:\.git)?$`)
)

// Repo represents a discovered git repository.
type Repo struct {
	Path   string // absolute path to repo root
	Name   string // org/repo extracted from remote
	Remote string // full remote URL
}

// Find walks all dirs concurrently, returning sorted git repos that do not
// match any exclude pattern.
func Find(dirs []string, excludes []string) ([]Repo, error) {
	patterns, err := compileGlobs(excludes)
	if err != nil {
		return nil, err
	}

	workCh := make(chan string, 256)
	resultCh := make(chan Repo, 256)

	var wg sync.WaitGroup
	for range workerCount {
		wg.Go(func() {
			for path := range workCh {
				r, ok := inspect(path)
				if !ok {
					continue
				}
				if matchesAny(r.Name, patterns) {
					continue
				}
				resultCh <- r
			}
		})
	}

	var walkErr error
	go func() {
		for _, dir := range dirs {
			expanded := ExpandHome(dir)
			if err := walkDir(expanded, workCh); err != nil {
				walkErr = fmt.Errorf("walk %s: %w", dir, err)
			}
		}
		close(workCh)
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Overlapping roots (~/code plus ~/code/src) discover the same repo
	// twice; keep the first.
	seen := make(map[string]bool)
	var repos []Repo
	for r := range resultCh {
		if seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		repos = append(repos, r)
	}

	if walkErr != nil {
		return repos, walkErr
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].Name < repos[j].Name
	})
	return repos, nil
}

// IsRepo reports whether path contains a .git directory or file.
func IsRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info != nil
}

// InsideWorkTree reports whether dir is anywhere inside a git working tree,
// including subdirectories of a repository (unlike IsRepo, which only checks
// for a .git at the path itself).
func InsideWorkTree(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// ParseRemote extracts "org/repo" from SSH or HTTPS git remote URLs.
// Falls back to "parentdir/repodir" when the URL cannot be parsed.
func ParseRemote(url string) string {
	url = strings.TrimSpace(url)

	if m := sshRemoteRe.FindStringSubmatch(url); len(m) == 2 {
		return m[1]
	}
	if m := httpsRemoteRe.FindStringSubmatch(url); len(m) == 2 {
		return m[1]
	}
	return ""
}

// ExpandHome expands a leading ~ to the user's home directory.
func ExpandHome(p string) string {
	if len(p) == 0 || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// walkDir sends all candidate repo root paths into workCh.
// It skips hidden directories (names starting with .) but not the root dir.
func walkDir(root string, workCh chan<- string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Swallow permission errors on individual entries.
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden dirs that are not the root itself.
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if IsRepo(path) {
			workCh <- path
			// Don't recurse into nested repos.
			return filepath.SkipDir
		}
		return nil
	})
}

// inspect reads the remote origin URL from .git/config and builds a Repo.
func inspect(path string) (Repo, bool) {
	remote, err := readRemote(path)
	if err != nil {
		remote = ""
	}
	name := ParseRemote(remote)
	if name == "" {
		// Fall back to parent-dir/repo-dir.
		parent := filepath.Base(filepath.Dir(path))
		name = parent + "/" + filepath.Base(path)
	}
	return Repo{
		Path:   path,
		Name:   name,
		Remote: remote,
	}, true
}

// readRemote parses .git/config and returns the [remote "origin"] url value.
func readRemote(repoPath string) (string, error) {
	f, err := os.Open(filepath.Join(repoPath, ".git", "config"))
	if err != nil {
		return "", err
	}
	defer f.Close()

	inOrigin := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if inOrigin {
			if strings.HasPrefix(line, "[") {
				break
			}
			if strings.HasPrefix(line, "url") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no remote origin in %s/.git/config", repoPath)
}

func compileGlobs(patterns []string) ([]glob.Glob, error) {
	compiled := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", p, err)
		}
		compiled = append(compiled, g)
	}
	return compiled, nil
}

func matchesAny(name string, patterns []glob.Glob) bool {
	for _, g := range patterns {
		if g.Match(name) {
			return true
		}
	}
	return false
}
