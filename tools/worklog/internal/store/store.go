// Package store manages the on-disk worklog tree: one directory per work item
// (keyed by ticket id or topic slug), a CONTEXT.md contract per item, lazy
// per-repo note files, and a local git history. There is no remote — the store
// is machine-local by design so internal references never leave the machine.
package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is a worklog tree rooted at Root. Now is injectable for tests.
type Store struct {
	Root string
	Now  func() time.Time
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

// ensureGit initializes the store directory as a local git repo on first use.
func (s *Store) ensureGit() error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".git")); err == nil {
		return nil
	}
	return s.git("init")
}

func (s *Store) git(args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", s.Root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"git %s: %w: %s",
			strings.Join(args, " "),
			err,
			strings.TrimSpace(string(out)),
		)
	}
	return nil
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
