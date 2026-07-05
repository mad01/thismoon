package store

import (
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
	if err := st.Save(deps); err != nil {
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
	if err := st.Save([]Dependency{flaggedDep("a", "GHSA-1", "GHSA-2")}); err != nil {
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
	if err := st2.Save([]Dependency{flaggedDep("a", "GHSA-1", "GHSA-2")}); err != nil {
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
	if err := st.Save([]Dependency{graphOnly, flaggedDep("a", "GHSA-1")}); err != nil {
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
	if err := st.Save([]Dependency{dep}); err != nil {
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
	}); err != nil {
		t.Fatal(err)
	}
	fullScanAt := st.Snapshot().ScannedAt

	// Rescan only /r1 with a different dep set.
	if err := st.SaveRepo("/r1", []Dependency{
		{Ecosystem: "Go", Name: "a", Version: "2", Repo: "/r1"},
		{Ecosystem: "Go", Name: "c", Version: "1", Repo: "/r1"},
	}); err != nil {
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
	if err := st.Save([]Dependency{flaggedDep("a", "GHSA-1")}); err != nil {
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
