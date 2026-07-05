package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxStaleAge = 24 * time.Hour

type Store struct {
	path   string
	mu     sync.RWMutex
	saveMu sync.Mutex
	data   Data
}

// Data is the on-disk store payload: cached PR state keyed by host/owner/repo.
type Data struct {
	Repos map[string]*RepoState `json:"repos"`
}

type RepoState struct {
	PRs             []CachedPR     `json:"prs"`
	ReviewDecisions map[int]string `json:"review_decisions,omitempty"`
	FetchedAt       time.Time      `json:"fetched_at"`
	Error           string         `json:"error,omitempty"`
	Archived        bool           `json:"archived,omitempty"`
}

type CachedPR struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	State        string    `json:"state"`
	Draft        bool      `json:"draft,omitempty"`
	HTMLURL      string    `json:"html_url"`
	Author       string    `json:"author"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	HeadRef      string    `json:"head_ref"`
	HeadSHA      string    `json:"head_sha"`
	BaseRef      string    `json:"base_ref"`
	Additions    int       `json:"additions,omitempty"`
	Deletions    int       `json:"deletions,omitempty"`
	ChangedFiles int       `json:"changed_files,omitempty"`
	Labels       []string  `json:"labels,omitempty"`
	TicketID     string    `json:"ticket_id,omitempty"`
}

var ticketRe = regexp.MustCompile(`(?i)\b([A-Z]{2,10}-\d+)\b`)

func ExtractTicketID(title, branch string) string {
	if m := ticketRe.FindStringSubmatch(title); len(m) > 1 {
		return strings.ToUpper(m[1])
	}
	if m := ticketRe.FindStringSubmatch(branch); len(m) > 1 {
		return strings.ToUpper(m[1])
	}
	return ""
}

func Load(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: Data{Repos: make(map[string]*RepoState)},
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read store: %w", err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return s, nil
	}
	if s.data.Repos == nil {
		s.data.Repos = make(map[string]*RepoState)
	}
	s.pruneExpired()
	return s, nil
}

func (s *Store) Get(key string) *RepoState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data.Repos[key]
	if !ok {
		return nil
	}
	cp := *v
	return &cp
}

func (s *Store) Put(key string, state *RepoState) {
	s.mu.Lock()
	s.data.Repos[key] = state
	s.mu.Unlock()
}

func (s *Store) Snapshot() map[string]*RepoState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*RepoState, len(s.data.Repos))
	for k, v := range s.data.Repos {
		cp := *v
		out[k] = &cp
	}
	return out
}

func (s *Store) PruneStale(validKeys map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.data.Repos {
		if !validKeys[k] {
			delete(s.data.Repos, k)
		}
	}
}

func (s *Store) pruneExpired() {
	now := time.Now()
	pruned := 0
	for k, rs := range s.data.Repos {
		if now.Sub(rs.FetchedAt) > maxStaleAge {
			delete(s.data.Repos, k)
			pruned++
		}
	}
	if pruned > 0 {
		log.Printf("pr: pruned %d expired repos from cache", pruned)
	}
}

func (s *Store) Save() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.RLock()
	raw, err := json.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	return writeFileAtomic(s.path, raw)
}

// writeFileAtomic writes data to path via a temp file + rename so a crash
// mid-write never leaves a truncated store. The temp file is removed on any
// error.
func writeFileAtomic(path string, data []byte) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp store: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write store: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync store: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename store: %w", err)
	}
	return nil
}
