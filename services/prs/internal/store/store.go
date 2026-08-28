// Package store holds the polled PR state: an in-memory map guarded by a
// mutex, persisted as an append-only JSONL log under the workdir. Every
// change appends one complete RepoState record as one line; on load the
// newest record per repo wins (greater fetched_at, a tie goes to the later
// line), the same last-wins semantics kof and wire use. The serve process is
// the single writer. The log is a cache of upstream GitHub state — deleting
// it costs one cold poll cycle, nothing more.
package store

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// logFile is the JSONL log name under the workdir.
const logFile = "repos.jsonl"

// PR is one open pull request, the single model used by the poller, the API,
// the MCP tools, and the web page.
type PR struct {
	Host           string    `json:"host"`
	Repo           string    `json:"repo"` // org/name
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	Author         string    `json:"author"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Labels         []string  `json:"labels,omitempty"`
	ReviewDecision string    `json:"review_decision,omitempty"` // APPROVED | CHANGES_REQUESTED | ""
}

// RepoState is one repo's slice of the cache: its open PRs, when the last
// poll touched it, and the fetch error when that poll failed (the previous
// PRs are kept so a transient error never blanks the page). A Removed record
// is the tombstone for a repo that is no longer discovered locally.
type RepoState struct {
	Host      string    `json:"host"`
	Repo      string    `json:"repo"`
	PRs       []PR      `json:"prs,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	Error     string    `json:"error,omitempty"`
	Removed   bool      `json:"removed,omitempty"`
}

// Key is the log key for a repo: host/org/name.
func (r RepoState) Key() string { return r.Host + "/" + r.Repo }

// changed reports whether the poll-relevant content differs from prev —
// FetchedAt alone advancing is not a change worth a log line.
func (r RepoState) changed(prev RepoState) bool {
	a, b := r, prev
	a.FetchedAt, b.FetchedAt = time.Time{}, time.Time{}
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	return string(ra) != string(rb)
}

// Filter narrows and orders a List call. Zero values mean "no filter".
type Filter struct {
	Repo           string // exact org/name
	Author         string // exact login
	ReviewDecision string // exact review decision
	Sort           string // "newest" (default) or "oldest", by created_at
}

// SortValues returns the valid Sort values, for boundary validation and
// error messages.
func SortValues() []string { return []string{"newest", "oldest"} }

// ValidSort reports whether s names a known sort order.
func ValidSort(s string) bool { return slices.Contains(SortValues(), s) }

// Status summarizes the cache for /api/status and the doctor.
type Status struct {
	PolledAt time.Time   `json:"polled_at"`
	Repos    int         `json:"repos"`
	OpenPRs  int         `json:"open_prs"`
	Errors   []RepoError `json:"errors,omitempty"`
	Hosts    []string    `json:"hosts,omitempty"`
}

// RepoError names one repo whose last fetch failed.
type RepoError struct {
	Host  string `json:"host"`
	Repo  string `json:"repo"`
	Error string `json:"error"`
}

// Store is the mutex-guarded cache over the JSONL log.
type Store struct {
	mu       sync.Mutex
	path     string
	repos    map[string]RepoState // newest record per key, tombstones included
	polledAt time.Time
}

// New loads the log from workdir, creating the directory when missing, and
// compacts the file when appends have outgrown the live set. A mid-file
// parse error is fatal and names the file — the cache is disposable, but
// silently dropping it would hide the corruption.
func New(workdir string) (*Store, error) {
	path, err := prepareLog(workdir, logFile)
	if err != nil {
		return nil, err
	}
	repos, lines, err := scanLog(path)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, repos: repos}
	for _, st := range repos {
		if !st.Removed && st.FetchedAt.After(s.polledAt) {
			s.polledAt = st.FetchedAt
		}
	}
	if shouldCompact(lines, len(repos)) {
		if err := compactLog(path, repos); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// SetRepos records the outcome of one completed poll cycle: incoming states
// replace the live set, repos no longer discovered get tombstones, and only
// records whose content actually changed are appended — a quiet cycle where
// nothing moved costs zero log lines.
func (s *Store) SetRepos(states []RepoState, polledAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]bool, len(states))
	var appends []RepoState
	for _, st := range states {
		seen[st.Key()] = true
		prev, ok := s.repos[st.Key()]
		if !ok || st.changed(prev) {
			appends = append(appends, st)
		}
		s.repos[st.Key()] = st
	}
	for key, prev := range s.repos {
		if seen[key] || prev.Removed {
			continue
		}
		tomb := RepoState{Host: prev.Host, Repo: prev.Repo, FetchedAt: polledAt, Removed: true}
		appends = append(appends, tomb)
		s.repos[key] = tomb
	}
	s.polledAt = polledAt

	for _, st := range appends {
		if err := appendLog(s.path, st); err != nil {
			return err
		}
	}
	return nil
}

// List returns the PRs matching f, ordered by created_at (newest first
// unless f.Sort is "oldest"), with repo then number as the stable tiebreak.
func (s *Store) List(f Filter) []PR {
	s.mu.Lock()
	defer s.mu.Unlock()
	var prs []PR
	for _, st := range s.repos {
		if st.Removed {
			continue
		}
		for _, pr := range st.PRs {
			if f.Repo != "" && pr.Repo != f.Repo {
				continue
			}
			if f.Author != "" && pr.Author != f.Author {
				continue
			}
			if f.ReviewDecision != "" && pr.ReviewDecision != f.ReviewDecision {
				continue
			}
			prs = append(prs, pr)
		}
	}
	oldest := f.Sort == "oldest"
	sort.Slice(prs, func(i, j int) bool {
		if !prs[i].CreatedAt.Equal(prs[j].CreatedAt) {
			if oldest {
				return prs[i].CreatedAt.Before(prs[j].CreatedAt)
			}
			return prs[i].CreatedAt.After(prs[j].CreatedAt)
		}
		if prs[i].Repo != prs[j].Repo {
			return prs[i].Repo < prs[j].Repo
		}
		return prs[i].Number < prs[j].Number
	})
	return prs
}

// Facets returns the distinct repos and authors across every cached PR,
// sorted — the filter dropdown options, independent of the active filter.
func (s *Store) Facets() (repos, authors []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	repoSet := map[string]struct{}{}
	authorSet := map[string]struct{}{}
	for _, st := range s.repos {
		if st.Removed {
			continue
		}
		for _, pr := range st.PRs {
			repoSet[pr.Repo] = struct{}{}
			if pr.Author != "" {
				authorSet[pr.Author] = struct{}{}
			}
		}
	}
	return sortedKeys(repoSet), sortedKeys(authorSet)
}

// Status summarizes the cache: last poll time, repo and PR counts, hosts
// seen, and every repo whose last fetch failed.
func (s *Store) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{PolledAt: s.polledAt}
	hosts := map[string]struct{}{}
	for _, rs := range s.repos {
		if rs.Removed {
			continue
		}
		st.Repos++
		st.OpenPRs += len(rs.PRs)
		hosts[rs.Host] = struct{}{}
		if rs.Error != "" {
			st.Errors = append(st.Errors, RepoError{Host: rs.Host, Repo: rs.Repo, Error: rs.Error})
		}
	}
	st.Hosts = sortedKeys(hosts)
	sort.Slice(st.Errors, func(i, j int) bool {
		return st.Errors[i].Repo < st.Errors[j].Repo
	})
	return st
}

// Repo returns the cached state for key (host/org/name), for the poller's
// error-preserving fallback when a fetch fails. Tombstoned repos report as
// absent.
func (s *Store) Repo(key string) (RepoState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.repos[key]
	if !ok || st.Removed {
		return RepoState{}, false
	}
	return st, true
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, strings.Compare)
	return keys
}
