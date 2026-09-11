package finder

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mad01/thismoon/kit/repofind"
)

const numWorkers = 32

// Walk scans the given directories for git repositories and returns a list of Repos.
// Repo names are extracted by reading .git/config directly (no subprocess).
// Uses a concurrent BFS: each directory is checked for .git via ReadDir.
// If found, the repo is recorded and no deeper descent occurs.
// Otherwise, subdirectories are queued for further scanning.
func Walk(dirs []string) ([]Repo, error) {
	work := make(chan string, 4096)
	type result struct {
		path   string
		name   string
		remote string
		host   string
	}
	results := make(chan result, 256)

	// Track in-flight work to know when to stop
	var inflight sync.WaitGroup

	// Seed initial directories before starting the closer goroutine
	for _, dir := range dirs {
		dir = repofind.ExpandHome(dir)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		inflight.Add(1)
		work <- dir
	}

	// Workers: read a directory, check for .git, queue children
	var workerWg sync.WaitGroup
	for range numWorkers {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for dir := range work {
				entries, err := os.ReadDir(dir)
				if err != nil {
					inflight.Done()
					continue
				}

				// Check if this directory is a git repo
				isRepo := false
				for _, e := range entries {
					if e.Name() == ".git" {
						isRepo = true
						break
					}
				}

				if isRepo {
					name, remote, host := repoInfo(dir)
					if name != "" {
						results <- result{path: dir, name: name, remote: remote, host: host}
					}
					inflight.Done()
					continue
				}

				// Not a repo — queue subdirectories for scanning
				for _, e := range entries {
					if !e.IsDir() || e.Name()[0] == '.' {
						continue
					}
					inflight.Add(1)
					work <- filepath.Join(dir, e.Name())
				}
				inflight.Done()
			}
		}()
	}

	// Close work channel when all in-flight items are done
	go func() {
		inflight.Wait()
		close(work)
	}()

	// Collect results until workers finish
	go func() {
		workerWg.Wait()
		close(results)
	}()

	seen := make(map[string]bool)
	var repos []Repo
	for r := range results {
		if !seen[r.path] {
			seen[r.path] = true
			repos = append(repos, Repo{Name: r.name, Path: r.path, Remote: r.remote, Host: r.host})
		}
	}
	return repos, nil
}

// FilteredWalk is like Walk but applies a host allowlist after discovery.
// When allowedHosts is non-empty, only repos whose Host field matches one of
// the listed values are returned. Repos with no remote, or a remote whose host
// cannot be parsed, are always excluded when the allowlist is non-empty.
// When allowedHosts is empty, FilteredWalk behaves identically to Walk.
func FilteredWalk(dirs []string, allowedHosts []string) ([]Repo, error) {
	kept, _, err := FilteredWalkReport(dirs, allowedHosts)
	return kept, err
}

// FilteredWalkReport is FilteredWalk that also hands back what it removed.
// The first return is what callers index and search; the second carries every
// repo the allowlist rejected, each with the rule and a reason naming the
// setting, so a surface can answer "csl cannot see my repo" instead of
// leaving it silently missing. An empty allowedHosts drops nothing.
func FilteredWalkReport(dirs, allowedHosts []string) ([]Repo, []Dropped, error) {
	repos, err := Walk(dirs)
	if err != nil {
		return nil, nil, err
	}
	if len(allowedHosts) == 0 {
		return repos, nil, nil
	}
	allowed := make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		allowed[h] = true
	}
	kept := make([]Repo, 0, len(repos))
	var dropped []Dropped
	for _, r := range repos {
		switch {
		case allowed[r.Host]:
			kept = append(kept, r)
		case r.Remote == "":
			dropped = append(dropped, Dropped{Repo: r, Kind: DropNoRemote})
		default:
			// A remote with no parseable host (a local path, say) is a host
			// mismatch, not a missing remote; Reason says which.
			dropped = append(dropped, Dropped{Repo: r, Kind: DropHost})
		}
	}
	return kept, dropped, nil
}

// Inspect resolves a single repo by its absolute working-tree path.
// Returns an error if the path is not a directory or has no .git entry.
// Use this instead of Walk when you already know the repo path (e.g. from a git hook).
func Inspect(path string) (Repo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Repo{}, err
	}
	if !info.IsDir() {
		return Repo{}, &os.PathError{Op: "inspect", Path: path, Err: os.ErrInvalid}
	}
	gitInfo, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil || (!gitInfo.IsDir() && !gitInfo.Mode().IsRegular()) {
		return Repo{}, &os.PathError{Op: "inspect", Path: path, Err: os.ErrNotExist}
	}
	name, remote, host := repoInfo(path)
	if name == "" {
		return Repo{}, &os.PathError{Op: "inspect", Path: path, Err: os.ErrInvalid}
	}
	return Repo{Name: name, Path: path, Remote: remote, Host: host}, nil
}

// repoInfo resolves a repo's identity (name, remote URL, host) from its .git
// entry, reading files directly (no subprocess).
//
// For a normal clone .git is a directory and the origin remote is read from
// .git/config. For a linked git WORKTREE .git is a FILE pointing at the repo's
// shared git dir; the origin lives in the common config there and the working
// tree sits on its own branch. A worktree is named "org/repo@branch" so it
// resolves the same remote/host as its primary clone — which keeps it past the
// host allowlist (FilteredWalk drops empty-host repos) — while staying a
// distinct entry, since zoekt keys repos by Name and several worktrees of one
// clone would otherwise collide.
func repoInfo(dir string) (name, remote, host string) {
	gitPath := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitPath); err == nil && !info.IsDir() {
		if n, r, h, ok := worktreeInfo(dir, gitPath); ok {
			return n, r, h
		}
	}
	raw := readOriginURL(filepath.Join(dir, ".git", "config"))
	if raw != "" {
		return ParseRemote(raw), raw, ParseHost(raw)
	}
	// Fallback to directory name — no remote info available
	return filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir), "", ""
}

// worktreeInfo resolves identity for a linked worktree whose .git is a pointer
// file ("gitdir: <repo>/.git/worktrees/<id>"). The origin remote lives in the
// shared config at the common git dir (located via the worktree's "commondir"
// file); the checked-out branch comes from the worktree's own HEAD. The name is
// "org/repo@branch", or "org/repo@<id>" when HEAD is detached, so multiple
// worktrees of one clone never collide on name. ok is false when the pointer
// cannot be followed to an origin URL, so the caller falls back to the
// directory-name heuristic.
func worktreeInfo(dir, gitFile string) (name, remote, host string, ok bool) {
	gitDir := readGitdirPointer(gitFile)
	if gitDir == "" {
		return "", "", "", false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	commonDir := resolveCommonDir(gitDir)
	raw := readOriginURL(filepath.Join(commonDir, "config"))
	if raw == "" {
		return "", "", "", false
	}
	suffix := worktreeBranch(filepath.Join(gitDir, "HEAD"))
	if suffix == "" {
		suffix = filepath.Base(gitDir) // worktree id — unique per repo
	}
	return ParseRemote(raw) + "@" + suffix, raw, ParseHost(raw), true
}

// readGitdirPointer returns the path from a worktree's ".git" pointer file
// ("gitdir: <path>"), or "" when the file is not a well-formed pointer.
func readGitdirPointer(gitFile string) string {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return ""
	}
	return strings.TrimSpace(rest)
}

// resolveCommonDir returns the shared git dir for a worktree git dir. A
// worktree's private git dir holds a "commondir" file whose (usually relative)
// value points at the main repo's git dir, where the shared config lives. When
// the file is absent the git dir is its own common dir.
func resolveCommonDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}
	common := strings.TrimSpace(string(data))
	if common == "" {
		return gitDir
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return filepath.Clean(common)
}

// worktreeBranch returns the short branch name from a HEAD file
// ("ref: refs/heads/<branch>"), or "" when HEAD is detached (a raw sha).
func worktreeBranch(headPath string) string {
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "ref:")
	if !ok {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(ref), "refs/heads/")
}

// readOriginURL parses a git config file to extract the URL of [remote "origin"].
func readOriginURL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if inOrigin {
			if strings.HasPrefix(line, "[") {
				return "" // next section, no url found
			}
			if strings.HasPrefix(line, "url = ") {
				return strings.TrimPrefix(line, "url = ")
			}
		}
	}
	return ""
}
