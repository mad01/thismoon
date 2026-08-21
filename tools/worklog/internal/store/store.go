// Package store manages the on-disk worklog tree: one directory per work item
// (keyed by ticket id or topic slug), a CONTEXT.md contract per item, lazy
// per-repo note files, and a git history. The store is local-first; when a
// Remote is configured it also keeps origin pointed at that URL, clones it on
// a fresh machine, and pushes after each write. Which machine gets which
// upstream is the config's problem (profile-keyed), keeping the personal and
// work worlds in separate private repos.
package store

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrPush marks a write that was committed locally but could not be pushed to
// the configured upstream. The write succeeded; callers surface this as a
// warning, not a failure.
var ErrPush = errors.New("worklog: push failed")

// Remote is the store's resolved upstream: the git URL origin should point at
// and whether writes push. The zero value means local-only.
type Remote struct {
	URL  string
	Push bool
}

// Store is a worklog tree rooted at Root. Now is injectable for tests.
type Store struct {
	Root   string
	Now    func() time.Time
	Remote Remote
}

// DefaultRoot returns $WORKLOG_DIR, or ~/code/worklog.
func DefaultRoot() string {
	if d := os.Getenv("WORKLOG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "code", "worklog")
}

// New returns a Store rooted at root (DefaultRoot if empty).
func New(root string) *Store {
	if root == "" {
		root = DefaultRoot()
	}
	return &Store{Root: root, Now: time.Now}
}

func (s *Store) now() time.Time { return s.Now().UTC().Truncate(time.Second) }

func (s *Store) itemDir(key string) string { return filepath.Join(s.Root, key) }

// ItemDir returns the on-disk directory for a work item.
func (s *Store) ItemDir(key string) string { return s.itemDir(key) }

func (s *Store) contextPath(key string) string {
	return filepath.Join(s.itemDir(key), "CONTEXT.md")
}

func (s *Store) repoPath(key, repo string) string {
	return filepath.Join(s.itemDir(key), "repos", repo+".md")
}

// Exists reports whether a work item already has a CONTEXT.md.
func (s *Store) Exists(key string) bool {
	_, err := os.Stat(s.contextPath(key))
	return err == nil
}

// ensureGit makes the store directory a usable git repo: cloning the upstream
// when the directory is missing entirely (fresh machine), initializing
// otherwise, and reconciling origin with the configured remote either way. An
// existing local-only store therefore adopts a newly configured remote on its
// next write.
func (s *Store) ensureGit() error {
	// A failed bootstrap clone (offline, bad URL) degrades to a local repo:
	// the write must still succeed, and the push that follows reports the
	// remote problem as ErrPush.
	_ = s.EnsureCloned()
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".git")); err != nil {
		if err := s.git("init"); err != nil {
			return err
		}
	}
	return s.ensureOrigin()
}

// EnsureCloned clones the configured upstream when the store directory does
// not exist yet — the fresh-machine bootstrap. With no remote, or with the
// directory already present, it does nothing.
func (s *Store) EnsureCloned() error {
	if s.Remote.URL == "" {
		return nil
	}
	if _, err := os.Stat(s.Root); err == nil || !os.IsNotExist(err) {
		return err
	}
	cmd := exec.Command("git", "clone", s.Remote.URL, s.Root)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"git clone %s: %w: %s",
			s.Remote.URL,
			err,
			strings.TrimSpace(string(out)),
		)
	}
	return nil
}

// ensureOrigin points origin at the configured remote URL, adding or
// repointing it as needed. With no remote configured it leaves any manually
// added origin alone.
func (s *Store) ensureOrigin() error {
	if s.Remote.URL == "" {
		return nil
	}
	current, err := s.gitOut("remote", "get-url", "origin")
	if err != nil {
		return s.git("remote", "add", "origin", s.Remote.URL)
	}
	if current != s.Remote.URL {
		return s.git("remote", "set-url", "origin", s.Remote.URL)
	}
	return nil
}

// maybePush pushes the current branch to origin when the remote asks for it.
// Failures come back wrapped in ErrPush so callers can downgrade them to a
// warning — the local write already succeeded.
func (s *Store) maybePush() error {
	if s.Remote.URL == "" || !s.Remote.Push {
		return nil
	}
	branch, err := s.gitOut("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrPush, err)
	}
	if err := s.git("push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("%w: %s", ErrPush, err)
	}
	return nil
}

// Sync reconciles the store with its upstream: fast-forward pull, then push.
// A remote with no commits yet (nothing to pull from) is fine; anything else
// that blocks the pull — divergence, auth — is an error.
func (s *Store) Sync() error {
	if s.Remote.URL == "" {
		return fmt.Errorf("worklog: no remote configured for this machine")
	}
	if err := s.ensureGit(); err != nil {
		return err
	}
	branch, err := s.gitOut("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if err := s.git("pull", "--ff-only", "origin", branch); err != nil {
		if !strings.Contains(err.Error(), "couldn't find remote ref") {
			return err
		}
	}
	return s.git("push", "-u", "origin", branch)
}

func (s *Store) git(args ...string) error {
	_, err := s.gitOut(args...)
	return err
}

func (s *Store) gitOut(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", s.Root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"git %s: %w: %s",
			strings.Join(args, " "),
			err,
			strings.TrimSpace(string(out)),
		)
	}
	return strings.TrimSpace(string(out)), nil
}

// commit stages everything and commits. A no-op tree (nothing changed) is not
// an error — the caller's write already succeeded on disk. The store commits
// under a fixed synthetic identity so it works on machines (and CI runners)
// with no global git config — the history is local-only plumbing.
func (s *Store) commit(msg string) error {
	if err := s.git("add", "-A"); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", s.Root,
		"-c", "user.name=worklog", "-c", "user.email=worklog@localhost",
		"commit", "-m", msg)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns all items, newest-updated first. status and repo filter when set.
func (s *Store) List(status, repo string) ([]*Item, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []*Item
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !s.Exists(e.Name()) {
			continue
		}
		it, err := s.Load(e.Name())
		if err != nil {
			return nil, err
		}
		if status != "" && it.FM.Status != status {
			continue
		}
		if repo != "" && !contains(it.FM.Repos, repo) {
			continue
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].FM.Updated.After(items[j].FM.Updated)
	})
	return items, nil
}

// Search returns items whose CONTEXT.md or repo notes contain query (case
// -insensitive substring), active items first then by recency.
func (s *Store) Search(query string) ([]*Item, error) {
	items, err := s.List("", "")
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var hits []*Item
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.FM.Key), q) ||
			strings.Contains(strings.ToLower(it.Body), q) ||
			s.repoNotesMatch(it.FM.Key, q) {
			hits = append(hits, it)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		ai, aj := hits[i].FM.Status == "active", hits[j].FM.Status == "active"
		if ai != aj {
			return ai
		}
		return hits[i].FM.Updated.After(hits[j].FM.Updated)
	})
	return hits, nil
}

func (s *Store) repoNotesMatch(key, q string) bool {
	dir := filepath.Join(s.itemDir(key), "repos")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && strings.Contains(strings.ToLower(string(b)), q) {
			return true
		}
	}
	return false
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
