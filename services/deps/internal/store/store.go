// Package store is the single source of truth for the latest dependency scan:
// the discovered dependencies, the advisories flagged against them, and the set
// of advisory occurrences already notified. The serve process owns a Store; the
// scanner and HTTP handlers go through these methods, so there is exactly one
// writer and no file-lock contention.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// fileName is the JSON file under the workdir that holds the latest scan.
const fileName = "scan.json"

// Advisory is a single OSV advisory affecting a dependency version.
type Advisory struct {
	ID           string `json:"id"`
	Summary      string `json:"summary,omitempty"`
	Severity     string `json:"severity,omitempty"`
	FixedVersion string `json:"fixed_version,omitempty"`
}

// Dependency is one resolved external package found in a repo, with any
// advisories OSV reports for that exact version (empty = clean).
type Dependency struct {
	Ecosystem    string     `json:"ecosystem"` // OSV ecosystem: Go | npm | PyPI
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	Repo         string     `json:"repo"`          // repo root the dep was found in
	ManifestPath string     `json:"manifest_path"` // module/lockfile that declared it
	Direct       bool       `json:"direct"`        // direct vs transitive
	// Imported is true when the dependency contributes at least one package to
	// the main module's build (Go) — i.e. it is actually compiled in, not a
	// graph-only transitive. Ecosystems where reachability isn't computed (npm,
	// PyPI) leave it true. A flagged-but-not-imported dep is informational only:
	// it doesn't count as active and never notifies.
	Imported   bool       `json:"imported"`
	Advisories []Advisory `json:"advisories,omitempty"`
}

// Flagged reports whether the dependency has at least one advisory.
func (d Dependency) Flagged() bool { return len(d.Advisories) > 0 }

// Scan is the persisted shape: the dependencies from the last run, the notified
// set, and the user-resolved (acknowledged) set. ScannedAt is the time of the
// last *full* scan (per-repo rescans don't reset it, so the daily catch-up still
// fires). It is zero until the first scan completes.
type Scan struct {
	ScannedAt time.Time    `json:"scanned_at"`
	Deps      []Dependency `json:"deps"`
	Notified  []string     `json:"notified"`
	Resolved  []string     `json:"resolved"`
}

// Flag is one advisory against one dependency — the unit the notifier fires on
// and the user resolves.
type Flag struct {
	Dep      Dependency
	Advisory Advisory
}

// Key is the stable identity of this advisory-on-dependency, used for both the
// notified and resolved sets and passed back by the UI/MCP to resolve a finding.
func (f Flag) Key() string {
	return FlagKey(f.Dep.Ecosystem, f.Dep.Name, f.Dep.Version, f.Advisory.ID)
}

// FlagKey uniquely identifies an advisory occurrence so a fixed-then-
// reintroduced advisory still notifies, but a steady-state one does not re-
// notify each cycle. Resolving a finding keys on this too, so an acknowledged
// advisory auto-resurfaces once the package version changes (the key changes).
func FlagKey(eco, name, version, advisoryID string) string {
	return eco + ":" + name + "@" + version + ":" + advisoryID
}

// Store holds the latest scan and persists it atomically to disk.
type Store struct {
	mu       sync.Mutex
	path     string
	now      func() time.Time
	deps     []Dependency
	scanned  time.Time
	notified map[string]struct{}
	resolved map[string]struct{}
}

// New loads the store from <workdir>/scan.json, creating the directory if
// needed. A missing file yields an empty store (not an error).
func New(workdir string) (*Store, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}
	s := &Store{
		path:     filepath.Join(workdir, fileName),
		now:      func() time.Time { return time.Now().UTC() },
		notified: map[string]struct{}{},
		resolved: map[string]struct{}{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read store: %w", err)
	}
	var sc Scan
	if err := json.Unmarshal(raw, &sc); err != nil {
		return fmt.Errorf("parse store: %w", err)
	}
	s.deps = sc.Deps
	s.scanned = sc.ScannedAt
	for _, k := range sc.Notified {
		s.notified[k] = struct{}{}
	}
	for _, k := range sc.Resolved {
		s.resolved[k] = struct{}{}
	}
	return nil
}

// keys returns the sorted keys of a set, for stable persistence.
func keys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// save writes the whole store atomically (temp file + rename). Callers hold s.mu.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(Scan{
		ScannedAt: s.scanned,
		Deps:      s.deps,
		Notified:  keys(s.notified),
		Resolved:  keys(s.resolved),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit store: %w", err)
	}
	return nil
}

// Save replaces the whole dependency set with the result of a full scan, stamps
// the scan time, and persists. The notified and resolved sets are preserved.
func (s *Store) Save(deps []Dependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deps = deps
	s.scanned = s.now()
	return s.save()
}

// SaveRepo replaces only the dependencies belonging to repo, keeping every other
// repo's deps and — deliberately — the last full-scan time, so a per-repo rescan
// doesn't reset the daily catch-up clock.
func (s *Store) SaveRepo(repo string, deps []Dependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]Dependency, 0, len(s.deps))
	for _, d := range s.deps {
		if d.Repo != repo {
			kept = append(kept, d)
		}
	}
	s.deps = append(kept, deps...)
	return s.save()
}

// Snapshot returns a copy of the current scan.
func (s *Store) Snapshot() Scan {
	s.mu.Lock()
	defer s.mu.Unlock()
	deps := make([]Dependency, len(s.deps))
	copy(deps, s.deps)
	return Scan{
		ScannedAt: s.scanned,
		Deps:      deps,
		Notified:  keys(s.notified),
		Resolved:  keys(s.resolved),
	}
}

// Flagged returns every dependency that has at least one advisory.
func (s *Store) Flagged() []Dependency {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Dependency
	for _, d := range s.deps {
		if d.Flagged() {
			out = append(out, d)
		}
	}
	return out
}

// ResolvedItem is an acknowledged advisory occurrence parsed back from its key,
// for display once the occurrence is no longer active (the dependency moved past
// it). It carries only what the key encodes — no summary/severity/repo, since a
// fixed advisory is gone from the scan.
type ResolvedItem struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	ID        string `json:"id"`
	Key       string `json:"key"`
}

// parseKey splits a FlagKey (eco:name@version:advisoryID) back into parts. Module
// names, versions, ecosystems, and advisory IDs contain no ':' and the name/
// version split is the last '@', so the bounds are unambiguous.
func parseKey(k string) (ResolvedItem, bool) {
	eco, rest, ok := strings.Cut(k, ":")
	if !ok {
		return ResolvedItem{}, false
	}
	j := strings.LastIndexByte(rest, ':')
	if j < 0 {
		return ResolvedItem{}, false
	}
	nameVer, id := rest[:j], rest[j+1:]
	a := strings.LastIndexByte(nameVer, '@')
	if a < 0 {
		return ResolvedItem{}, false
	}
	return ResolvedItem{Ecosystem: eco, Name: nameVer[:a], Version: nameVer[a+1:], ID: id, Key: k}, true
}

// ResolvedFixed returns acknowledged advisory occurrences that no longer appear
// in the current scan — i.e. the dependency has since been updated past them.
// These are "resolved" in the strong sense (fixed by a bump), distinct from
// acknowledged advisories that are still present on a vulnerable version.
func (s *Store) ResolvedFixed() []ResolvedItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := map[string]struct{}{}
	for _, d := range s.deps {
		for _, a := range d.Advisories {
			active[FlagKey(d.Ecosystem, d.Name, d.Version, a.ID)] = struct{}{}
		}
	}
	out := make([]ResolvedItem, 0)
	for k := range s.resolved {
		if _, ok := active[k]; ok {
			continue
		}
		if item, ok := parseKey(k); ok {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// IsResolved reports whether the advisory occurrence with the given key has been
// acknowledged by the user.
func (s *Store) IsResolved(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.resolved[key]
	return ok
}

// Resolve acknowledges the given advisory-occurrence keys: they stop notifying
// and drop out of the active findings until the package version changes or a new
// advisory appears (either changes the key). Returns the number newly resolved.
func (s *Store) Resolve(keys []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	for _, k := range keys {
		if _, ok := s.resolved[k]; !ok {
			s.resolved[k] = struct{}{}
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	return added, s.save()
}

// PendingFlags returns each advisory-against-dependency that has neither been
// notified nor resolved — the set the notifier should fire on. It does not
// mutate the notified set; the caller marks the flags it delivered via
// MarkNotified, so a failed notification retries.
func (s *Store) PendingFlags() []Flag {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pending []Flag
	for _, d := range s.deps {
		if !d.Imported {
			// Graph-only transitive: in the module graph but not compiled in.
			// Not a real exposure — never notify on it.
			continue
		}
		for _, a := range d.Advisories {
			key := FlagKey(d.Ecosystem, d.Name, d.Version, a.ID)
			if _, ok := s.notified[key]; ok {
				continue
			}
			if _, ok := s.resolved[key]; ok {
				continue
			}
			pending = append(pending, Flag{Dep: d, Advisory: a})
		}
	}
	return pending
}

// MarkNotified records that the given flags were delivered, so they are not
// notified again next cycle.
func (s *Store) MarkNotified(flags []Flag) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range flags {
		s.notified[f.Key()] = struct{}{}
	}
	return s.save()
}
