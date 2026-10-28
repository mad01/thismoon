package store

import (
	"os"
	"path/filepath"
	"testing"
)

func flaggedDep(name string, advIDs ...string) Dependency {
	d := Dependency{Ecosystem: "Go", Name: name, Version: "1.0.0", Repo: "/r", Imported: true}
	for _, id := range advIDs {
		d.Advisories = append(d.Advisories, Advisory{ID: id, FixedVersion: "1.0.1"})
	}
	return d
}

func TestSaveAndSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	deps := []Dependency{flaggedDep("a", "GHSA-1"), {Ecosystem: "Go", Name: "b", Version: "2.0.0"}}
	if err := st.Save(deps, nil); err != nil {
		t.Fatal(err)
	}

	// A fresh store over the same dir must read back what we saved.
	st2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	sc := st2.Snapshot()
	if len(sc.Deps) != 2 {
		t.Fatalf("reloaded %d deps, want 2", len(sc.Deps))
	}
	if sc.ScannedAt.IsZero() {
		t.Error("ScannedAt should be set after Save")
	}
	if got := len(st2.Flagged()); got != 1 {
		t.Errorf("Flagged() = %d, want 1", got)
	}
}

func TestPendingFlagsAndMarkNotified(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]Dependency{flaggedDep("a", "GHSA-1", "GHSA-2")}, nil); err != nil {
		t.Fatal(err)
	}

	// Both advisories are pending initially.
	pending := st.PendingFlags()
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}

	// Mark one delivered — only the other remains pending.
	if err := st.MarkNotified(pending[:1]); err != nil {
		t.Fatal(err)
	}
	pending = st.PendingFlags()
	if len(pending) != 1 {
		t.Fatalf("after mark, pending = %d, want 1", len(pending))
	}

	// The notified set survives a reload (so a steady-state flag isn't re-fired).
	st2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st2.Save([]Dependency{flaggedDep("a", "GHSA-1", "GHSA-2")}, nil); err != nil {
		t.Fatal(err)
	}
	if got := len(st2.PendingFlags()); got != 1 {
		t.Errorf("after reload+rescan, pending = %d, want 1 (notified persists)", got)
	}
}

func TestPendingFlagsSkipsNonImported(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A flagged dep that's graph-only (not compiled in) must never go pending —
	// it isn't a real exposure, so it doesn't notify.
	graphOnly := flaggedDep("g", "GHSA-9")
	graphOnly.Imported = false
	if err := st.Save([]Dependency{graphOnly, flaggedDep("a", "GHSA-1")}, nil); err != nil {
		t.Fatal(err)
	}
	pending := st.PendingFlags()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1 (graph-only dep excluded)", len(pending))
	}
	if pending[0].Dep.Name != "a" {
		t.Errorf("pending dep = %q, want the imported one (a)", pending[0].Dep.Name)
	}
}

func TestResolveHidesFromPendingAndPersists(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	dep := flaggedDep("a", "GHSA-1", "GHSA-2")
	if err := st.Save([]Dependency{dep}, nil); err != nil {
		t.Fatal(err)
	}

	key := FlagKey(dep.Ecosystem, dep.Name, dep.Version, "GHSA-1")
	added, err := st.Resolve([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("resolved %d, want 1", added)
	}
	if !st.IsResolved(key) {
		t.Error("key should be resolved")
	}
	// Resolved advisory drops out of PendingFlags (won't notify).
	if got := len(st.PendingFlags()); got != 1 {
		t.Fatalf("pending = %d, want 1 (GHSA-1 resolved, GHSA-2 still pending)", got)
	}
	// Resolving again is a no-op.
	if added, _ := st.Resolve([]string{key}); added != 0 {
		t.Errorf("re-resolve added %d, want 0", added)
	}
	// Resolved set survives a reload.
	st2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st2.IsResolved(key) {
		t.Error("resolved set should persist across reload")
	}
}

func TestSaveRepoMergesAndKeepsScanTime(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]Dependency{
		{Ecosystem: "Go", Name: "a", Version: "1", Repo: "/r1"},
		{Ecosystem: "Go", Name: "b", Version: "1", Repo: "/r2"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	fullScanAt := st.Snapshot().ScannedAt

	// Rescan only /r1 with a different dep set.
	if err := st.SaveRepo("/r1", []Dependency{
		{Ecosystem: "Go", Name: "a", Version: "2", Repo: "/r1"},
		{Ecosystem: "Go", Name: "c", Version: "1", Repo: "/r1"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	sc := st.Snapshot()
	if !sc.ScannedAt.Equal(fullScanAt) {
		t.Error("per-repo rescan must not change the full-scan time")
	}
	names := map[string]string{}
	for _, d := range sc.Deps {
		names[d.Name] = d.Version
	}
	if names["b"] != "1" {
		t.Error("/r2 dep b should be untouched")
	}
	if names["a"] != "2" || names["c"] != "1" {
		t.Errorf("/r1 deps should be replaced: %v", names)
	}
}

func TestResolvedFixedSplitsAckedFromFixed(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// One dep still flagged at v1.0.0 (GHSA-1), and we acknowledge it.
	if err := st.Save([]Dependency{flaggedDep("a", "GHSA-1")}, nil); err != nil {
		t.Fatal(err)
	}
	stillThere := FlagKey("Go", "a", "1.0.0", "GHSA-1")
	// And we acknowledged a finding on an old version of "b" that no longer
	// appears in the scan — the package was bumped past it.
	gone := FlagKey("Go", "b", "0.9.0", "GHSA-2")
	if _, err := st.Resolve([]string{stillThere, gone}); err != nil {
		t.Fatal(err)
	}

	fixed := st.ResolvedFixed()
	if len(fixed) != 1 {
		t.Fatalf("ResolvedFixed() = %d items, want 1 (only the gone one)", len(fixed))
	}
	got := fixed[0]
	if got.Name != "b" || got.Version != "0.9.0" || got.ID != "GHSA-2" || got.Ecosystem != "Go" {
		t.Errorf("parsed item = %+v, want b@0.9.0 GHSA-2 Go", got)
	}
}

func TestDiscoveryErrorsPersistAndMergePerRepo(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(nil, []DiscoveryError{
		{Repo: "/r1", Ecosystem: "Go", Error: "go list in /r1: exec: \"go\": executable file not found in $PATH"},
		{Repo: "/r2", Ecosystem: "npm", Error: "parse package-lock.json: unexpected EOF"},
	}); err != nil {
		t.Fatal(err)
	}

	// Read sees what serve saved, without going through a Store.
	sc, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.DiscoveryErrors) != 2 {
		t.Fatalf("Read discovery errors = %+v, want 2", sc.DiscoveryErrors)
	}

	// A per-repo rescan replaces only that repo's errors.
	reloaded, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.SaveRepo("/r1", nil, nil); err != nil {
		t.Fatal(err)
	}
	got := reloaded.DiscoveryErrors()
	if len(got) != 1 || got[0].Repo != "/r2" {
		t.Fatalf("after /r1 rescan, discovery errors = %+v, want only /r2's", got)
	}

	// A full scan replaces the whole set.
	if err := reloaded.Save(nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.DiscoveryErrors(); len(got) != 0 {
		t.Fatalf("after a clean full scan, discovery errors = %+v, want none", got)
	}
}

func TestReadMissingStoreIsEmptyAndWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	sc, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sc.ScannedAt.IsZero() || len(sc.Deps) != 0 {
		t.Errorf("Read of a missing store = %+v, want the zero Scan", sc)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Read created %s (stat err %v); it must stay read-only", dir, err)
	}
}

// preKindScan is a scan.json in the shape serve wrote before findings had a
// kind: no "kind" on any advisory. lodash's advisory was already notified, and
// a former finding on an older version was acknowledged.
const preKindScan = `{
  "scanned_at": "2026-10-20T09:00:00Z",
  "deps": [
    {
      "ecosystem": "npm", "name": "lodash", "version": "4.17.20",
      "repo": "/r", "manifest_path": "/r/package-lock.json",
      "direct": true, "imported": true,
      "advisories": [{"id": "GHSA-35jh-r3h4-6jhm", "severity": "HIGH", "fixed_version": "4.17.21"}]
    },
    {
      "ecosystem": "Go", "name": "golang.org/x/text", "version": "0.3.7",
      "repo": "/r", "manifest_path": "/r/go.mod", "imported": true
    }
  ],
  "notified": ["npm:lodash@4.17.20:GHSA-35jh-r3h4-6jhm"],
  "resolved": ["Go:golang.org/x/net@0.17.0:GO-2023-2102"]
}`

// TestLoadsScanWrittenBeforeKinds: a pre-change scan.json loads unchanged,
// each advisory taking the kind its id gives (a GHSA id reads as a
// vulnerability), and the notified and resolved sets still match their
// findings because the key format is the same.
func TestLoadsScanWrittenBeforeKinds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(preKindScan), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := New(dir)
	if err != nil {
		t.Fatalf("load pre-change scan.json: %v", err)
	}
	sc := st.Snapshot()
	if len(sc.Deps) != 2 || sc.ScannedAt.IsZero() {
		t.Fatalf(
			"loaded %d deps (scanned_at %v), want 2 and the saved time",
			len(sc.Deps),
			sc.ScannedAt,
		)
	}
	want := Advisory{
		ID: "GHSA-35jh-r3h4-6jhm", Kind: KindVuln,
		Severity: "HIGH", FixedVersion: "4.17.21",
	}
	if got := sc.Deps[0].Advisories; len(got) != 1 || got[0] != want {
		t.Errorf("lodash advisories = %+v, want [%+v]", got, want)
	}
	if pending := st.PendingFlags(); len(pending) != 0 {
		t.Errorf("pending = %+v, want none: the notified set must still match", pending)
	}
	if !st.IsResolved("Go:golang.org/x/net@0.17.0:GO-2023-2102") {
		t.Error("resolved set lost on load")
	}

	// The read-only path doctor uses sees the same kinds.
	read, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Deps[0].Advisories[0].Kind; got != KindVuln {
		t.Errorf("Read kind = %q, want %q", got, KindVuln)
	}
}

func TestKindForID(t *testing.T) {
	for id, want := range map[string]Kind{
		"MAL-2025-46969":      KindMalicious,
		"GHSA-35jh-r3h4-6jhm": KindVuln,
		"GO-2024-0001":        KindVuln,
		"PYSEC-2024-1":        KindVuln,
		"mal-lowercase":       KindVuln,
	} {
		if got := KindForID(id); got != want {
			t.Errorf("KindForID(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestEmptyKindDerivesFromID: an advisory saved with no kind takes its id's
// kind on load and on Save, on the store and on the read-only doctor path.
func TestEmptyKindDerivesFromID(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]Dependency{{
		Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Repo: "/r",
		Advisories: []Advisory{{ID: "MAL-2025-46969"}, {ID: "GHSA-9"}},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	read, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, deps := range map[string][]Dependency{
		"Save": st.Snapshot().Deps,
		"Read": read.Deps,
	} {
		got := deps[0].Advisories
		if got[0].Kind != KindMalicious || got[1].Kind != KindVuln {
			t.Errorf("%s kinds = %q, %q; want malicious, vuln", name, got[0].Kind, got[1].Kind)
		}
	}
}

// TestPendingFlagsFollowKind: a malicious finding notifies even on a dep that
// is not compiled in, a vulnerability there does not, and a signal never
// notifies on any dep.
func TestPendingFlagsFollowKind(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	graphOnly := Dependency{
		Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Repo: "/r",
		Advisories: []Advisory{
			{ID: "MAL-2025-46969", Kind: KindMalicious},
			{ID: "GHSA-9", Kind: KindVuln},
		},
	}
	compiledIn := Dependency{
		Ecosystem: "Go", Name: "a", Version: "1.0.0", Repo: "/r", Imported: true,
		Advisories: []Advisory{{ID: "deps-movable-ref", Kind: KindSignal}},
	}
	if err := st.Save([]Dependency{graphOnly, compiledIn}, nil); err != nil {
		t.Fatal(err)
	}
	pending := st.PendingFlags()
	if len(pending) != 1 || pending[0].Advisory.ID != "MAL-2025-46969" {
		t.Fatalf("pending = %+v, want only the malicious finding", pending)
	}
}

// TestSignalResolvesByKey: a signal's rule id makes an ordinary key, so it
// resolves and, once gone from the scan, parses back like any advisory.
func TestSignalResolvesByKey(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := FlagKey("npm", "@scope/pkg", "1.0.0", "deps-movable-ref")
	if _, err := st.Resolve([]string{key}); err != nil {
		t.Fatal(err)
	}
	fixed := st.ResolvedFixed()
	want := ResolvedItem{
		Ecosystem: "npm", Name: "@scope/pkg", Version: "1.0.0",
		ID: "deps-movable-ref", Key: key,
	}
	if len(fixed) != 1 || fixed[0] != want {
		t.Errorf("ResolvedFixed() = %+v, want [%+v]", fixed, want)
	}
}
