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

// Kind classifies a finding, and the kind alone decides how deps surfaces it.
// The set is closed: a producer picks one of the constants below. An empty
// kind (a scan.json written before kinds existed, or a producer that left it
// unset) is derived from the finding's id by KindForID.
type Kind string

const (
	// KindVuln is a known vulnerability from an advisory database. It counts as
	// active and notifies in the coalesced summary banner, but only while the
	// dependency is compiled in (Dependency.Imported).
	KindVuln Kind = "vuln"
	// KindMalicious is a version known to be malicious: an OSV MAL- record, or
	// a version another producer's rule marks as such. It counts as active and
	// fires its own banner even when the dependency is not compiled in,
	// because its install scripts may already have run.
	KindMalicious Kind = "malicious"
	// KindSignal is a warning sign that is not an advisory. It shows on the
	// page only: it never notifies and never counts as active, though it can
	// still be resolved by key. Its Advisory.ID is a rule id with no ':' in it
	// (such as deps-movable-ref), so the FlagKey stays parseable.
	KindSignal Kind = "signal"
)

// KindForID classifies an OSV advisory by its id. OSV serves the OpenSSF
// malicious-packages database as MAL- records, GitHub's malware advisories
// included, so the prefix alone marks a malicious version. It needs no fetched
// record, so a finding whose record failed to load keeps its kind, and a
// finding saved before kinds existed loads with the kind it would get today.
func KindForID(id string) Kind {
	if strings.HasPrefix(id, "MAL-") {
		return KindMalicious
	}
	return KindVuln
}

// Alerts reports whether a finding of kind k is a real exposure on a
// dependency that is, or is not, compiled in: one that notifies when new and
// counts as active until resolved. It is the one rule PendingFlags and the API
// counts share; the web page mirrors it.
func (k Kind) Alerts(imported bool) bool {
	switch k {
	case KindMalicious:
		return true
	case KindSignal:
		return false
	default:
		return imported
	}
}

// Advisory is one finding against a dependency version: an OSV advisory, or a
// signal a deps rule raised. Kind says which; ID is the advisory id or the
// rule id.
type Advisory struct {
	ID           string `json:"id"`
	Kind         Kind   `json:"kind"`
	Summary      string `json:"summary,omitempty"`
	Severity     string `json:"severity,omitempty"`
	FixedVersion string `json:"fixed_version,omitempty"`
}

// Dependency is one resolved external package found in a repo, with the
// findings OSV, its registry or a discoverer raised against that exact version
// (empty = clean).
type Dependency struct {
	Ecosystem    string `json:"ecosystem"` // OSV ecosystem: Go | npm | PyPI
	Name         string `json:"name"`
	Version      string `json:"version"`
	Repo         string `json:"repo"`          // repo root the dep was found in
	ManifestPath string `json:"manifest_path"` // module/lockfile that declared it
	Direct       bool   `json:"direct"`        // direct vs transitive
	// Imported is true when the dependency contributes at least one package to
	// the main module's build (Go) — i.e. it is actually compiled in, not a
	// graph-only transitive. Ecosystems where reachability isn't computed (npm,
	// PyPI) leave it true. A vulnerability on a not-imported dep is
	// informational only: it doesn't count as active and never notifies. A
	// malicious finding ignores this gate (see Kind.Alerts).
	Imported   bool       `json:"imported"`
	Advisories []Advisory `json:"advisories,omitempty"`
	// RegistryUnknown is true when the last check's registry lookup for this
	// version got no answer. Its registry findings are then the ones the check
	// before found, kept so that an outage neither drops a finding nor makes a
	// resolved one read as fixed, and whether the registry withdrew or yanked
	// the version since is unknown rather than clean.
	RegistryUnknown bool `json:"registry_unknown,omitempty"`
	// OtherIndex is true when the manifest points its package manager at a
	// source besides the public registry (a requirements.txt with
	// --index-url, say). The version may come from there, so the public
	// registry not serving it is no evidence it was withdrawn.
	OtherIndex bool `json:"other_index,omitempty"`
	// NameFromFile is true for a vendored file whose npm name comes from its
	// file name rather than a jsDelivr npm match: a file that matches nothing
	// published, or one published on GitHub. Its bytes aren't known to be the
	// npm package of that name, so the registry is never asked about it.
	NameFromFile bool `json:"name_from_file,omitempty"`
}

// Flagged reports whether the dependency has at least one advisory.
func (d Dependency) Flagged() bool { return len(d.Advisories) > 0 }

// DiscoveryError is one repo and ecosystem whose discovery failed in the last
// scan, so none of its dependencies were checked. Serve persists these with the
// scan; the API, MCP tools, doctor, event, and web page read them back, so a
// failed `go list` can't pass for a clean scan.
type DiscoveryError struct {
	Repo      string `json:"repo"`
	Ecosystem string `json:"ecosystem"`
	Error     string `json:"error"`
}

// Scan is the persisted shape: the dependencies from the last run, the notified
// set, and the user-resolved (acknowledged) set. ScannedAt is the time of the
// last *full* scan (per-repo rescans don't reset it, so the daily catch-up still
// fires). It is zero until the first scan completes. DiscoveryErrors lists the
// repo and ecosystem pairs the last scan could not read.
type Scan struct {
	ScannedAt       time.Time        `json:"scanned_at"`
	Deps            []Dependency     `json:"deps"`
	DiscoveryErrors []DiscoveryError `json:"discovery_errors,omitempty"`
	Notified        []string         `json:"notified"`
	Resolved        []string         `json:"resolved"`
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
	discErrs []DiscoveryError
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

// Read loads the scan persisted under workdir without creating or writing
// anything, for read-only callers such as doctor. A missing file yields an
// empty Scan.
func Read(workdir string) (Scan, error) {
	return readScan(filepath.Join(workdir, fileName))
}

func readScan(path string) (Scan, error) {
	var sc Scan
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return sc, nil
	}
	if err != nil {
		return sc, fmt.Errorf("read store: %w", err)
	}
	if err := json.Unmarshal(raw, &sc); err != nil {
		return sc, fmt.Errorf("parse store: %w", err)
	}
	defaultKinds(sc.Deps)
	return sc, nil
}

// defaultKinds sets every empty Advisory.Kind in deps from its id with
// KindForID, in place. A scan.json written before kinds existed then loads a
// MAL- finding as malicious, so it surfaces and notifies like one found today.
func defaultKinds(deps []Dependency) {
	for i := range deps {
		for j := range deps[i].Advisories {
			if a := &deps[i].Advisories[j]; a.Kind == "" {
				a.Kind = KindForID(a.ID)
			}
		}
	}
}

func (s *Store) load() error {
	sc, err := readScan(s.path)
	if err != nil {
		return err
	}
	s.deps = sc.Deps
	s.discErrs = sc.DiscoveryErrors
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
		ScannedAt:       s.scanned,
		Deps:            s.deps,
		DiscoveryErrors: s.discErrs,
		Notified:        keys(s.notified),
		Resolved:        keys(s.resolved),
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

// Save replaces the whole dependency set and the discovery errors with the
// result of a full scan, stamps the scan time, and persists. The notified and
// resolved sets are preserved.
func (s *Store) Save(deps []Dependency, discErrs []DiscoveryError) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defaultKinds(deps)
	s.deps = deps
	s.discErrs = discErrs
	s.scanned = s.now()
	return s.save()
}

// SaveRepo replaces only the dependencies and discovery errors belonging to
// repo. It keeps every other repo's, and deliberately keeps the last full-scan
// time too, so a per-repo rescan doesn't reset the daily catch-up clock.
func (s *Store) SaveRepo(repo string, deps []Dependency, discErrs []DiscoveryError) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defaultKinds(deps)
	kept := make([]Dependency, 0, len(s.deps))
	for _, d := range s.deps {
		if d.Repo != repo {
			kept = append(kept, d)
		}
	}
	s.deps = append(kept, deps...)
	keptErrs := make([]DiscoveryError, 0, len(s.discErrs))
	for _, e := range s.discErrs {
		if e.Repo != repo {
			keptErrs = append(keptErrs, e)
		}
	}
	s.discErrs = append(keptErrs, discErrs...)
	return s.save()
}

// DiscoveryErrors returns a copy of the repo and ecosystem pairs the last scan
// could not read.
func (s *Store) DiscoveryErrors() []DiscoveryError {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]DiscoveryError(nil), s.discErrs...)
}

// RegistryUnknown counts the dependencies whose registry lookup got no answer
// in the check that last saved them.
func (s *Store) RegistryUnknown() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range RegistryUnknownByEcosystem(s.deps) {
		n += c
	}
	return n
}

// RegistryUnknownByEcosystem counts, per ecosystem, the dependencies in deps
// whose registry lookup got no answer. An ecosystem with none is left out, so
// an empty map means every lookup was answered. Store.RegistryUnknown is its
// total; doctor reads it from the persisted Scan.
func RegistryUnknownByEcosystem(deps []Dependency) map[string]int {
	out := map[string]int{}
	for _, d := range deps {
		if d.RegistryUnknown {
			out[d.Ecosystem]++
		}
	}
	return out
}

// Snapshot returns a copy of the current scan.
func (s *Store) Snapshot() Scan {
	s.mu.Lock()
	defer s.mu.Unlock()
	deps := make([]Dependency, len(s.deps))
	copy(deps, s.deps)
	return Scan{
		ScannedAt:       s.scanned,
		Deps:            deps,
		DiscoveryErrors: append([]DiscoveryError(nil), s.discErrs...),
		Notified:        keys(s.notified),
		Resolved:        keys(s.resolved),
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
// names, versions, ecosystems, advisory IDs and signal rule ids contain no ':'
// and the name/version split is the last '@', so the bounds are unambiguous.
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
	return ResolvedItem{
		Ecosystem: eco,
		Name:      nameVer[:a],
		Version:   nameVer[a+1:],
		ID:        id,
		Key:       k,
	}, true
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
// notified nor resolved: the set the notifier should fire on. Only findings
// whose kind Alerts qualify: a signal never notifies, and neither does a
// vulnerability on a graph-only dependency (in the module graph but not
// compiled in). It does not mutate the notified set; the caller marks the
// flags it delivered via MarkNotified, so a failed notification retries.
func (s *Store) PendingFlags() []Flag {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pending []Flag
	for _, d := range s.deps {
		for _, a := range d.Advisories {
			if !a.Kind.Alerts(d.Imported) {
				continue
			}
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
