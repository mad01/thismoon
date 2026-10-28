package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/api"
	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/jsdelivr"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/regcheck"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// banner is one notification the fake notifier received.
type banner struct {
	title, body string
}

// fakeNotifier records every Notify call instead of shelling out to osascript.
type fakeNotifier struct {
	calls []banner
	fail  bool
}

func (f *fakeNotifier) Notify(title, body string) error {
	if f.fail {
		return errContext
	}
	f.calls = append(f.calls, banner{title: title, body: body})
	return nil
}

var errContext = context.Canceled // any non-nil error

// fakeChecker returns a fixed advisory for one named package.
type fakeChecker struct {
	flagName string
}

func (f fakeChecker) Check(_ context.Context, queries []osv.Query) ([][]store.Advisory, error) {
	out := make([][]store.Advisory, len(queries))
	for i, q := range queries {
		if q.Name == f.flagName {
			out[i] = []store.Advisory{{ID: "GHSA-1", FixedVersion: "9.9.9"}}
		}
	}
	return out, nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCheckAttachesAdvisories(t *testing.T) {
	deps := []store.Dependency{
		{Ecosystem: "Go", Name: "clean", Version: "1.0.0"},
		{Ecosystem: "Go", Name: "bad", Version: "1.0.0"},
	}
	got, _, err := Check(context.Background(), deps, fakeChecker{flagName: "bad"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Flagged() {
		t.Error("clean dep should not be flagged")
	}
	if !got[1].Flagged() {
		t.Error("bad dep should be flagged")
	}
}

func TestNotifyFiresOncePerNewFlag(t *testing.T) {
	st := newStore(t)
	if err := st.Save([]store.Dependency{{
		Ecosystem: "Go", Name: "bad", Version: "1.0.0", Imported: true,
		Advisories: []store.Advisory{{ID: "GHSA-1"}, {ID: "GHSA-2"}},
	}}, nil); err != nil {
		t.Fatal(err)
	}

	// Two advisories coalesce into ONE notification covering both.
	n := &fakeNotifier{}
	delivered, err := Notify(st, n)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 2 || len(n.calls) != 1 {
		t.Fatalf(
			"first notify: delivered=%d calls=%d, want 2 covered / 1 banner",
			delivered,
			len(n.calls),
		)
	}

	// Second pass: nothing new, so no notification at all.
	delivered, err = Notify(st, n)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 0 || len(n.calls) != 1 {
		t.Fatalf("second notify: delivered=%d calls=%d, want 0/1", delivered, len(n.calls))
	}
}

func TestNotifyRetriesOnFailure(t *testing.T) {
	st := newStore(t)
	if err := st.Save([]store.Dependency{{
		Ecosystem: "Go", Name: "bad", Version: "1.0.0", Imported: true,
		Advisories: []store.Advisory{{ID: "GHSA-1"}},
	}}, nil); err != nil {
		t.Fatal(err)
	}

	// Delivery fails: the flag must stay pending (not marked notified).
	if _, err := Notify(st, &fakeNotifier{fail: true}); err != nil {
		t.Fatal(err)
	}
	if got := len(st.PendingFlags()); got != 1 {
		t.Fatalf("after failed notify, pending = %d, want 1 (retryable)", got)
	}

	// A working notifier then delivers it.
	n := &fakeNotifier{}
	if _, err := Notify(st, n); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 1 {
		t.Fatalf("retry delivered %d, want 1", len(n.calls))
	}
}

// stubEco is a discover.Ecosystem returning fixed packages or a fixed error.
type stubEco struct {
	name string
	pkgs []discover.Package
	err  error
}

func (s stubEco) Name() string { return s.name }

func (s stubEco) Discover(string, discover.Options) ([]discover.Package, error) {
	return s.pkgs, s.err
}

// TestCheckSavesDiscoveryErrors is the failing-ecosystem fixture: Go discovery
// fails the way it does under a launchd PATH without go, npm works. The scan
// keeps the npm result and persists the Go failure for the API, doctor and page.
func TestCheckSavesDiscoveryErrors(t *testing.T) {
	st := newStore(t)
	goErr := `go list in /r1: exec: "go": executable file not found in $PATH`
	e := &Engine{
		Prepare: func() (Plan, error) {
			return Plan{Repos: []string{"/r1"}}, nil
		},
		Ecosystems: []discover.Ecosystem{
			stubEco{name: "Go", err: errors.New(goErr)},
			stubEco{name: "npm", pkgs: []discover.Package{
				{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Imported: true},
			}},
		},
		Checker: fakeChecker{},
		Store:   st,
	}

	deps, err := e.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(deps) != 1 || deps[0].Name != "lodash" {
		t.Errorf("deps = %+v, want the npm package kept", deps)
	}
	got := st.DiscoveryErrors()
	want := store.DiscoveryError{Repo: "/r1", Ecosystem: "Go", Error: goErr}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("saved discovery errors = %+v, want [%+v]", got, want)
	}
}

func TestScanCompleteEventWarnsOnDiscoveryErrors(t *testing.T) {
	clean := []store.Dependency{{Ecosystem: "npm", Name: "lodash", Version: "4.17.21"}}

	ev := scanCompleteEvent(1, clean, nil, 0)
	if ev.Level != "info" || strings.Contains(ev.Title, "discovery") || ev.Message != "" {
		t.Errorf("clean scan event = %+v, want info with no discovery note", ev)
	}

	ev = scanCompleteEvent(1, clean, []store.DiscoveryError{
		{Repo: "/r1", Ecosystem: "Go", Error: "boom"},
	}, 0)
	if ev.Level != "warn" {
		t.Errorf("level = %q, want warn when discovery failed", ev.Level)
	}
	if !strings.HasSuffix(ev.Title, ", 1 discovery error(s)") {
		t.Errorf("title = %q, want the discovery error count", ev.Title)
	}
	if ev.Message != "Go in /r1: boom" {
		t.Errorf("message = %q, want each failure named", ev.Message)
	}
	if ev.Tags["discovery_errors"] != "1" {
		t.Errorf("discovery_errors tag = %q, want 1", ev.Tags["discovery_errors"])
	}
}

// widget is the one repo the end-to-end engines below scan.
const widget = "/home/u/code/widget"

// osvStub stands in for api.osv.dev. querybatch answers each query with the
// ids batch lists for its package name; GET /v1/vulns/{id} serves records[id],
// or fails with 502 when id has no record.
func osvStub(t *testing.T, batch map[string][]string, records map[string]string) *osv.Client {
	t.Helper()
	type vulnID struct {
		ID string `json:"id"`
	}
	type result struct {
		Vulns []vulnID `json:"vulns"`
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Queries []struct {
				Package struct {
					Name string `json:"name"`
				} `json:"package"`
			} `json:"queries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode querybatch: %v", err)
		}
		results := make([]result, len(req.Queries))
		for i, q := range req.Queries {
			for _, id := range batch[q.Package.Name] {
				results[i].Vulns = append(results[i].Vulns, vulnID{ID: id})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /v1/vulns/{id}", func(w http.ResponseWriter, r *http.Request) {
		rec, ok := records[r.PathValue("id")]
		if !ok {
			http.Error(w, "upstream timeout", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(rec))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return osv.NewWithURL(srv.URL)
}

// npmEngine scans widget for the given npm packages against checker.
func npmEngine(
	st *store.Store,
	checker Checker,
	n *fakeNotifier,
	pkgs ...discover.Package,
) *Engine {
	return &Engine{
		Prepare: func() (Plan, error) {
			return Plan{Repos: []string{widget}}, nil
		},
		Ecosystems: []discover.Ecosystem{stubEco{name: "npm", pkgs: pkgs}},
		Checker:    checker,
		Notifier:   n,
		Store:      st,
	}
}

func npmPkg(name, version string) discover.Package {
	return discover.Package{Ecosystem: "npm", Name: name, Version: version, Imported: true}
}

const leftPadRecord = `{"id":"GHSA-xxxx","summary":"left-pad pads left",
	"database_specific":{"severity":"HIGH"}}`

// TestMaliciousFindingRanksFirstWithItsOwnBanner is the chalk@5.6.1 fixture end
// to end: OSV returns MAL-2025-46969 (the record in osv's testdata), which is
// saved as kind malicious, listed before an ordinary HIGH advisory, and fires
// its own banner naming the package, version and repo, apart from the summary.
func TestMaliciousFindingRanksFirstWithItsOwnBanner(t *testing.T) {
	malRecord, err := os.ReadFile("../osv/testdata/MAL-2025-46969.json")
	if err != nil {
		t.Fatal(err)
	}
	checker := osvStub(t,
		map[string][]string{"left-pad": {"GHSA-xxxx"}, "chalk": {"MAL-2025-46969"}},
		map[string]string{"GHSA-xxxx": leftPadRecord, "MAL-2025-46969": string(malRecord)},
	)
	st, n := newStore(t), &fakeNotifier{}
	e := npmEngine(st, checker, n, npmPkg("left-pad", "1.2.0"), npmPkg("chalk", "5.6.1"))

	if err := e.CheckAndNotify(context.Background()); err != nil {
		t.Fatalf("CheckAndNotify: %v", err)
	}

	flagged := api.Flagged(api.Enrich(st.Flagged(), st.IsResolved))
	if len(flagged) != 2 || flagged[0].Name != "chalk" {
		t.Fatalf("flagged = %+v, want chalk first of 2", flagged)
	}
	if got := flagged[0].Advisories[0].Kind; got != store.KindMalicious {
		t.Errorf("chalk kind = %q, want %q", got, store.KindMalicious)
	}

	if len(n.calls) != 2 {
		t.Fatalf("banners = %+v, want a malicious banner and a summary", n.calls)
	}
	mal := n.calls[0]
	if mal.title != "deps: malicious package chalk@5.6.1" {
		t.Errorf("malicious banner title = %q", mal.title)
	}
	for _, want := range []string{"chalk@5.6.1 in widget", "installed", "rotate the credentials"} {
		if !strings.Contains(mal.body, want) {
			t.Errorf("malicious banner body = %q, want it to contain %q", mal.body, want)
		}
	}
	summary := n.calls[1]
	if strings.Contains(summary.title+summary.body, "chalk") {
		t.Errorf("summary banner %+v repeats the malicious finding", summary)
	}
	if !strings.Contains(summary.body, "left-pad") {
		t.Errorf("summary banner body = %q, want left-pad", summary.body)
	}
}

// TestCheckSavesWhenOneRecordFetchFails: a failed GET /v1/vulns/{id} no longer
// aborts the scan. It saves, the unfetched finding keeps its id, and the other
// finding is intact.
func TestCheckSavesWhenOneRecordFetchFails(t *testing.T) {
	checker := osvStub(t,
		map[string][]string{"left-pad": {"GHSA-xxxx"}, "evil": {"GHSA-gone"}},
		map[string]string{"GHSA-xxxx": leftPadRecord},
	)
	st := newStore(t)
	e := npmEngine(
		st,
		checker,
		&fakeNotifier{},
		npmPkg("evil", "1.0.0"),
		npmPkg("left-pad", "1.2.0"),
	)

	if _, err := e.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	sc := st.Snapshot()
	if sc.ScannedAt.IsZero() {
		t.Fatal("scan was not saved")
	}
	byName := map[string][]store.Advisory{}
	for _, d := range sc.Deps {
		byName[d.Name] = d.Advisories
	}
	gone := store.Advisory{ID: "GHSA-gone", Kind: store.KindVuln}
	if got := byName["evil"]; len(got) != 1 || got[0] != gone {
		t.Errorf("evil advisories = %+v, want [%+v]", got, gone)
	}
	intact := store.Advisory{
		ID: "GHSA-xxxx", Kind: store.KindVuln,
		Summary: "left-pad pads left", Severity: "HIGH",
	}
	if got := byName["left-pad"]; len(got) != 1 || got[0] != intact {
		t.Errorf("left-pad advisories = %+v, want [%+v]", got, intact)
	}
}

// TestSignalNeverNotifiesOrCountsActive: a signal on a compiled-in dep stays
// off every alerting surface: no banner, no active count, no flagged tally in
// the scan event. It is still listed, so it can be resolved by key.
func TestSignalNeverNotifiesOrCountsActive(t *testing.T) {
	st := newStore(t)
	deps := []store.Dependency{{
		Ecosystem: "npm", Name: "left-pad", Version: "1.2.0", Repo: widget, Imported: true,
		Advisories: []store.Advisory{{ID: "deps-movable-ref", Kind: store.KindSignal}},
	}}
	if err := st.Save(deps, nil); err != nil {
		t.Fatal(err)
	}

	n := &fakeNotifier{}
	delivered, err := Notify(st, n)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 0 || len(n.calls) != 0 {
		t.Errorf("notify delivered %d in %+v, want no banner for a signal", delivered, n.calls)
	}

	flagged := api.Flagged(api.Enrich(st.Flagged(), st.IsResolved))
	if len(flagged) != 1 {
		t.Fatalf("flagged = %+v, want the signal listed", flagged)
	}
	if got := api.ActiveCount(flagged); got != 0 {
		t.Errorf("active count = %d, want 0", got)
	}
	if ev := scanCompleteEvent(1, deps, nil, 0); ev.Tags["flagged"] != "0" || ev.Level != "info" {
		t.Errorf("scan event = %+v, want info with 0 flagged", ev)
	}
}

// TestCheckKeepsDiscovererFindings: a finding a discoverer attached survives the
// checker on both the full and the per-repo path, beside the checker's own
// finding, and checking again lists each finding once.
func TestCheckKeepsDiscovererFindings(t *testing.T) {
	signal := store.Advisory{ID: "deps-movable-ref", Kind: store.KindSignal}
	vuln := store.Advisory{ID: "GHSA-1", Kind: store.KindVuln, FixedVersion: "9.9.9"}
	pkg := npmPkg("left-pad", "1.2.0")
	pkg.Advisories = []store.Advisory{signal}
	st := newStore(t)
	e := npmEngine(st, fakeChecker{flagName: "left-pad"}, &fakeNotifier{}, pkg)

	ctx := context.Background()
	full := func() error { _, err := e.Check(ctx); return err }
	repo := func() error { _, err := e.CheckRepo(ctx, "widget"); return err }
	runs := []struct {
		name  string
		check func() error
	}{
		{"full check", full},
		{"full check again", full},
		{"repo check", repo},
	}
	for _, run := range runs {
		if err := run.check(); err != nil {
			t.Fatalf("%s: %v", run.name, err)
		}
		deps := st.Snapshot().Deps
		if len(deps) != 1 {
			t.Fatalf("%s: saved %d deps, want 1", run.name, len(deps))
		}
		got := deps[0].Advisories
		if len(got) != 2 || got[0] != signal || got[1] != vuln {
			t.Errorf("%s: advisories = %+v, want [%+v %+v]", run.name, got, signal, vuln)
		}
	}

	// Check itself is idempotent on deps it already annotated.
	deps := st.Snapshot().Deps
	for range 2 {
		var err error
		if deps, _, err = Check(ctx, deps, fakeChecker{flagName: "left-pad"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := deps[0].Advisories; len(got) != 2 {
		t.Errorf("re-checked advisories = %+v, want the signal and GHSA-1 once each", got)
	}
}

// preKindMaliciousScan is a scan.json written before findings had a kind,
// holding a MAL- finding on a dep that is not compiled in. It was never
// notified.
const preKindMaliciousScan = `{
  "scanned_at": "2026-10-20T09:00:00Z",
  "deps": [{
    "ecosystem": "npm", "name": "chalk", "version": "5.6.1",
    "repo": "/home/u/code/widget", "manifest_path": "/home/u/code/widget/package-lock.json",
    "imported": false,
    "advisories": [{"id": "MAL-2025-46969"}]
  }],
  "notified": [],
  "resolved": []
}`

// TestPreKindMaliciousFindingLoadsAsMalicious: a MAL- finding saved before
// kinds existed loads as malicious, so it fires the malicious banner (not the
// summary) despite the dep not being compiled in, under the key it always had.
func TestPreKindMaliciousFindingLoadsAsMalicious(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scan.json"), []byte(preKindMaliciousScan), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Snapshot().Deps[0].Advisories[0].Kind; got != store.KindMalicious {
		t.Fatalf("loaded kind = %q, want %q", got, store.KindMalicious)
	}

	n := &fakeNotifier{}
	for range 2 {
		if _, err := Notify(st, n); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.calls) != 1 || n.calls[0].title != "deps: malicious package chalk@5.6.1" {
		t.Fatalf("banners = %+v, want one malicious banner for chalk@5.6.1", n.calls)
	}
	want := []string{"npm:chalk@5.6.1:MAL-2025-46969"}
	if got := st.Snapshot().Notified; !slices.Equal(got, want) {
		t.Errorf("notified = %v, want %v (the key is unchanged)", got, want)
	}
}

// TestMaliciousBannerNamesEachPackageOnce: one malicious version pinned in two
// repos, plus three more, makes one banner that names a package with all its
// repos and caps the list.
func TestMaliciousBannerNamesEachPackageOnce(t *testing.T) {
	flag := func(name, repo string) store.Flag {
		return store.Flag{
			Dep:      store.Dependency{Name: name, Version: "1.0.0", Repo: repo},
			Advisory: store.Advisory{ID: "MAL-1", Kind: store.KindMalicious},
		}
	}
	title, body := maliciousBanner([]store.Flag{
		flag("a", "/c/one"), flag("a", "/c/two"), flag("b", "/c/one"),
		flag("c", "/c/one"), flag("d", "/c/one"),
	})
	if title != "deps: 4 malicious packages" {
		t.Errorf("title = %q", title)
	}
	if !strings.HasPrefix(body, "a@1.0.0 in one, two; b@1.0.0 in one; c@1.0.0 in one; +1 more.") {
		t.Errorf("body = %q", body)
	}
}

// npmTool is one npm: tool in a scratch mise install: the version mise.lock
// pins, and the further "name@version" packages its tree resolves.
type npmTool struct {
	name    string
	version string
	tree    []string
}

// writeMise writes a mise.lock pinning tools, and an aube-lock.yaml install
// for each, under dir. It returns the Mise that reads them.
func writeMise(t *testing.T, dir string, tools ...npmTool) discover.Mise {
	t.Helper()
	installs := filepath.Join(dir, "installs")
	var lock strings.Builder
	for _, tool := range tools {
		fmt.Fprintf(&lock, "[[tools.%q]]\nversion = %q\n\n", "npm:"+tool.name, tool.version)
		aube := fmt.Sprintf("lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n"+
			"      %s: {version: %s}\npackages:\n", tool.name, tool.version)
		for _, p := range append([]string{tool.name + "@" + tool.version}, tool.tree...) {
			aube += "  " + p + ": {}\n"
		}
		versionDir := filepath.Join(installs, "npm-"+tool.name, tool.version)
		if err := os.MkdirAll(versionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(versionDir, "aube-lock.yaml"), aube)
	}
	lockPath := filepath.Join(dir, "mise.lock")
	writeFile(t, lockPath, lock.String())
	return discover.Mise{LockPath: lockPath, InstallRoot: installs}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// inventory renders the store's dependencies as sorted "repo name@version".
func inventory(st *store.Store) []string {
	var out []string
	for _, d := range st.Snapshot().Deps {
		out = append(out, d.Repo+" "+d.Name+"@"+d.Version)
	}
	slices.Sort(out)
	return out
}

// TestCheckCoversMiseTools: a full check reports the installed mise tools
// beside the repos, each under its label, and checks them against OSV. A mise
// row is imported, so its vulnerability counts as active instead of hiding as
// not compiled in.
func TestCheckCoversMiseTools(t *testing.T) {
	st := newStore(t)
	m := writeMise(t, t.TempDir(),
		npmTool{name: "yarn", version: "1.22.22"},
		npmTool{name: "left-pad", version: "1.3.0", tree: []string{"pad-core@2.0.0"}},
	)
	e := &Engine{
		Prepare: func() (Plan, error) {
			return Plan{Repos: []string{"/r1"}, Mise: m}, nil
		},
		Ecosystems: []discover.Ecosystem{stubEco{name: "npm", pkgs: []discover.Package{
			{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Imported: true},
		}}},
		Checker: fakeChecker{flagName: "pad-core"},
		Store:   st,
	}
	if _, err := e.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	want := []string{
		"/r1 lodash@4.17.21",
		"mise:npm:left-pad left-pad@1.3.0",
		"mise:npm:left-pad pad-core@2.0.0",
		"mise:npm:yarn yarn@1.22.22",
	}
	if got := inventory(st); !slices.Equal(got, want) {
		t.Errorf("inventory = %q, want %q", got, want)
	}
	flagged := st.Flagged()
	if len(flagged) != 1 || flagged[0].Name != "pad-core" ||
		flagged[0].Repo != "mise:npm:left-pad" {
		t.Errorf("flagged = %+v, want pad-core under mise:npm:left-pad", flagged)
	}
	if got := api.ActiveCount(api.Enrich(flagged, st.IsResolved)); got != 1 {
		t.Errorf("active count = %d, want 1: every mise row must be imported", got)
	}
}

// TestCheckRepoRescansOneMiseTool: a rescan by label replaces only that
// tool's packages and keeps the full-scan time. A label that names no
// installed tool, or a lock that no longer parses, fails the rescan and leaves
// the store as it was.
func TestCheckRepoRescansOneMiseTool(t *testing.T) {
	st := newStore(t)
	dir := t.TempDir()
	m := writeMise(t, dir,
		npmTool{name: "yarn", version: "1.22.22"},
		npmTool{name: "left-pad", version: "1.3.0"},
	)
	e := &Engine{
		Prepare: func() (Plan, error) { return Plan{Mise: m}, nil },
		Checker: fakeChecker{},
		Store:   st,
	}
	if _, err := e.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	scannedAt := st.Snapshot().ScannedAt

	// Both tools upgrade on disk; only yarn is rescanned.
	writeMise(t, dir,
		npmTool{name: "yarn", version: "1.22.23"},
		npmTool{name: "left-pad", version: "1.3.1"},
	)
	if _, err := e.CheckRepo(context.Background(), "mise:npm:yarn"); err != nil {
		t.Fatalf("CheckRepo: %v", err)
	}
	want := []string{"mise:npm:left-pad left-pad@1.3.0", "mise:npm:yarn yarn@1.22.23"}
	if got := inventory(st); !slices.Equal(got, want) {
		t.Errorf("after the yarn rescan, inventory = %q, want %q", got, want)
	}
	if !st.Snapshot().ScannedAt.Equal(scannedAt) {
		t.Error("a per-tool rescan must not change the full-scan time")
	}

	_, err := e.CheckRepo(context.Background(), "mise:npm:absent")
	if err == nil || !strings.Contains(err.Error(), "not among the installed mise tools") {
		t.Errorf(
			"rescan of an unknown label: err = %v, want not among the installed mise tools",
			err,
		)
	}
	writeFile(t, m.LockPath, "[[tools\n")
	_, err = e.CheckRepo(context.Background(), "mise:npm:yarn")
	if err == nil || !strings.Contains(err.Error(), m.LockPath) {
		t.Errorf("rescan with a broken lock: err = %v, want it to name %s", err, m.LockPath)
	}
	if got := inventory(st); !slices.Equal(got, want) {
		t.Errorf("failed rescans changed the inventory to %q, want %q", got, want)
	}
}

// fakeRegistry answers each query with the Result listed for its package
// name; a name it does not list gets a zero Result (nothing to say).
type fakeRegistry map[string]regcheck.Result

func (f fakeRegistry) Check(_ context.Context, queries []regcheck.Query) []regcheck.Result {
	out := make([]regcheck.Result, len(queries))
	for i, q := range queries {
		out[i] = f[q.Name]
	}
	return out
}

var (
	withdrawn = store.Advisory{
		ID: regcheck.RuleNPMWithdrawn, Kind: store.KindMalicious, Summary: "npm no longer serves chalk@5.6.1",
	}
	deprecated = store.Advisory{
		ID: regcheck.RuleNPMDeprecated, Kind: store.KindSignal, Summary: "Deprecated on npm: use pad-start",
	}
)

// registryEngine scans widget for chalk@5.6.1 and left-pad@1.2.0. OSV flags
// chalk as MAL-2025-46969 and left-pad as GHSA-xxxx; reg is the registry.
func registryEngine(t *testing.T, st *store.Store, n *fakeNotifier, reg Registry) *Engine {
	t.Helper()
	malRecord, err := os.ReadFile("../osv/testdata/MAL-2025-46969.json")
	if err != nil {
		t.Fatal(err)
	}
	checker := osvStub(t,
		map[string][]string{"left-pad": {"GHSA-xxxx"}, "chalk": {"MAL-2025-46969"}},
		map[string]string{"GHSA-xxxx": leftPadRecord, "MAL-2025-46969": string(malRecord)},
	)
	e := npmEngine(st, checker, n, npmPkg("chalk", "5.6.1"), npmPkg("left-pad", "1.2.0"))
	e.Registry = reg
	return e
}

// findingIDs maps each saved dep's name to its finding ids, in order.
func findingIDs(st *store.Store) map[string][]string {
	out := map[string][]string{}
	for _, d := range st.Snapshot().Deps {
		for _, a := range d.Advisories {
			out[d.Name] = append(out[d.Name], a.ID)
		}
	}
	return out
}

// TestRegistryFindingsJoinOSVFindings: the registry's findings sit beside
// OSV's on the same dependency. A withdrawn version joins the malicious
// banner, which still names the package once, and a deprecation stays off
// every banner.
func TestRegistryFindingsJoinOSVFindings(t *testing.T) {
	st, n := newStore(t), &fakeNotifier{}
	e := registryEngine(t, st, n, fakeRegistry{
		"chalk":    {Findings: []store.Advisory{withdrawn}},
		"left-pad": {Findings: []store.Advisory{deprecated}},
	})

	if err := e.CheckAndNotify(context.Background()); err != nil {
		t.Fatalf("CheckAndNotify: %v", err)
	}

	want := map[string][]string{
		"chalk":    {"MAL-2025-46969", regcheck.RuleNPMWithdrawn},
		"left-pad": {"GHSA-xxxx", regcheck.RuleNPMDeprecated},
	}
	if got := findingIDs(st); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("findings = %v, want %v", got, want)
	}
	if len(n.calls) != 2 || n.calls[0].title != "deps: malicious package chalk@5.6.1" {
		t.Fatalf("banners = %+v, want one malicious banner for chalk, then the summary", n.calls)
	}
	if n.calls[1].title != "deps: 1 new advisory(ies) across 1 package(s)" {
		t.Errorf("summary banner title = %q, want only left-pad's GHSA counted", n.calls[1].title)
	}
	if got := st.RegistryUnknown(); got != 0 {
		t.Errorf("registry unknown = %d, want 0 after every lookup answered", got)
	}
}

// TestUnreachableRegistryDropsNoFindingAndShowsNothingFixed is the outage
// case: after a check that found a withdrawn chalk and a deprecated left-pad,
// both acknowledged, the registry goes unreachable. Full and per-repo checks
// still save, keep OSV's findings and the registry's earlier ones, mark the
// deps unknown, show nothing as fixed, and fire no banner. Once the registry
// answers again, its answer replaces what was kept.
func TestUnreachableRegistryDropsNoFindingAndShowsNothingFixed(t *testing.T) {
	st, n := newStore(t), &fakeNotifier{}
	e := registryEngine(t, st, n, fakeRegistry{
		"chalk":    {Findings: []store.Advisory{withdrawn}},
		"left-pad": {Findings: []store.Advisory{deprecated}},
	})
	ctx := context.Background()
	if err := e.CheckAndNotify(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Resolve([]string{
		store.FlagKey("npm", "chalk", "5.6.1", regcheck.RuleNPMWithdrawn),
		store.FlagKey("npm", "left-pad", "1.2.0", regcheck.RuleNPMDeprecated),
	}); err != nil {
		t.Fatal(err)
	}
	before := findingIDs(st)
	banners := len(n.calls)

	e.Registry = downRegistry(t)
	runs := []struct {
		name  string
		check func() error
	}{
		{"full check", func() error { return e.CheckAndNotify(ctx) }},
		{"repo check", func() error { _, err := e.CheckRepo(ctx, "widget"); return err }},
	}
	for _, run := range runs {
		if err := run.check(); err != nil {
			t.Fatalf("%s with the registry down: %v", run.name, err)
		}
		if got := findingIDs(st); !maps.EqualFunc(got, before, slices.Equal) {
			t.Errorf("%s: findings = %v, want every one kept: %v", run.name, got, before)
		}
		if fixed := st.ResolvedFixed(); len(fixed) != 0 {
			t.Errorf("%s: resolved as fixed = %+v, want none", run.name, fixed)
		}
		if got := st.RegistryUnknown(); got != 2 {
			t.Errorf("%s: registry unknown = %d, want both deps", run.name, got)
		}
		if _, err := e.Notify(); err != nil || len(n.calls) != banners {
			t.Errorf(
				"%s: banners = %+v (err %v), want no new one",
				run.name,
				n.calls[banners:],
				err,
			)
		}
	}

	e.Registry = fakeRegistry{"chalk": {Findings: []store.Advisory{withdrawn}}}
	if _, err := e.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if got := findingIDs(st)["left-pad"]; !slices.Equal(got, []string{"GHSA-xxxx"}) {
		t.Errorf("left-pad findings = %v, want the deprecation gone once npm answers", got)
	}
	if got := st.RegistryUnknown(); got != 0 {
		t.Errorf("registry unknown = %d, want 0 once the registry answers", got)
	}
}

// shortRegistry answers no query at all.
type shortRegistry struct{}

func (shortRegistry) Check(context.Context, []regcheck.Query) []regcheck.Result { return nil }

// TestCheckRegistryShortAnswerIsUnknown: a registry that returns fewer
// results than queries leaves the rest unknown, never clean.
func TestCheckRegistryShortAnswerIsUnknown(t *testing.T) {
	prev := []store.Dependency{{
		Ecosystem: "npm", Name: "chalk", Version: "5.6.1",
		Advisories: []store.Advisory{withdrawn},
	}}
	deps := []store.Dependency{{Ecosystem: "npm", Name: "chalk", Version: "5.6.1"}}
	got := CheckRegistry(context.Background(), deps, shortRegistry{}, prev)
	if !got[0].RegistryUnknown || len(got[0].Advisories) != 1 || got[0].Advisories[0] != withdrawn {
		t.Errorf("dep = %+v, want unknown with the withdrawn finding kept", got[0])
	}
}

// TestKeepRegistryMergesRowsFromDifferentChecks: after a per-repo check, two
// repos' rows for one version can disagree. The version keeps the findings
// one row has, even when an earlier row has none, and stays unknown when any
// row is.
func TestKeepRegistryMergesRowsFromDifferentChecks(t *testing.T) {
	prev := []store.Dependency{
		{Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Repo: "/a", RegistryUnknown: true},
		{
			Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Repo: "/b",
			Advisories: []store.Advisory{withdrawn},
		},
	}
	deps := []store.Dependency{
		{Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Repo: "/a"},
		{Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Repo: "/c"},
	}
	for _, d := range keepRegistry(deps, prev) {
		if !d.RegistryUnknown || len(d.Advisories) != 1 || d.Advisories[0] != withdrawn {
			t.Errorf("%s row = %+v, want unknown with the withdrawn finding", d.Repo, d)
		}
	}
}

func TestScanCompleteEventWarnsOnUnknownRegistry(t *testing.T) {
	deps := []store.Dependency{
		{Ecosystem: "npm", Name: "chalk", Version: "5.6.2", RegistryUnknown: true},
		{Ecosystem: "npm", Name: "lodash", Version: "4.17.21"},
	}
	ev := scanCompleteEvent(1, deps, nil, 0)
	if ev.Level != "warn" || ev.Tags["registry_unknown"] != "1" {
		t.Errorf("event = %+v, want warn with registry_unknown 1", ev)
	}
	if !strings.HasSuffix(ev.Title, ", 1 registry lookup(s) unanswered") {
		t.Errorf("title = %q, want the unanswered count", ev.Title)
	}
}

// downRegistry is a real registry client whose hosts refuse every connection.
func downRegistry(t *testing.T) *regcheck.Client {
	t.Helper()
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	return regcheck.NewWithEndpoints(regcheck.Endpoints{
		NPM: down.URL, PyPI: down.URL, GoProxy: down.URL,
	})
}

// TestScanKeepsRegistryFindingsForALaterOutage: a scan asks no registry, so it
// keeps each version's registry findings. A check during a registry outage
// after it still has them to keep, and shows nothing as fixed.
func TestScanKeepsRegistryFindingsForALaterOutage(t *testing.T) {
	st, n := newStore(t), &fakeNotifier{}
	e := registryEngine(t, st, n, fakeRegistry{
		"chalk":    {Findings: []store.Advisory{withdrawn}},
		"left-pad": {Findings: []store.Advisory{deprecated}},
	})
	ctx := context.Background()
	if err := e.CheckAndNotify(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Resolve([]string{
		store.FlagKey("npm", "chalk", "5.6.1", regcheck.RuleNPMWithdrawn),
		store.FlagKey("npm", "left-pad", "1.2.0", regcheck.RuleNPMDeprecated),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Scan(); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"chalk":    {regcheck.RuleNPMWithdrawn},
		"left-pad": {regcheck.RuleNPMDeprecated},
	}
	if got := findingIDs(st); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("after scan: findings = %v, want the registry's kept: %v", got, want)
	}

	e.Registry = downRegistry(t)
	if _, err := e.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if got := findingIDs(st)["chalk"]; !slices.Contains(got, regcheck.RuleNPMWithdrawn) {
		t.Errorf("chalk findings = %v, want the withdrawn finding kept through the outage", got)
	}
	if fixed := st.ResolvedFixed(); len(fixed) != 0 {
		t.Errorf("resolved as fixed = %+v, want none", fixed)
	}
}

// TestScanKeepsRegistryUnknown: a version whose last lookup went unanswered
// stays unknown through a scan, which asks no registry either.
func TestScanKeepsRegistryUnknown(t *testing.T) {
	st, n := newStore(t), &fakeNotifier{}
	e := registryEngine(t, st, n, downRegistry(t))
	if _, err := e.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Scan(); err != nil {
		t.Fatal(err)
	}
	if got := st.RegistryUnknown(); got != 2 {
		t.Errorf("registry unknown after scan = %d, want both deps still unknown", got)
	}
}

// TestCheckRegistryKeepsOtherIndexPinsApart: during an outage a pin from a
// manifest that names another index keeps only its own registry findings,
// never a withdrawn finding that a plain pin of the same version carried, and
// the query tells the registry which pins those are.
func TestCheckRegistryKeepsOtherIndexPinsApart(t *testing.T) {
	gone := store.Advisory{ID: regcheck.RulePyPIWithdrawn, Kind: store.KindMalicious}
	prev := []store.Dependency{
		{Ecosystem: "PyPI", Name: "core", Version: "0.1.0", Advisories: []store.Advisory{gone}},
		{Ecosystem: "PyPI", Name: "core", Version: "0.1.0", OtherIndex: true},
	}
	deps := []store.Dependency{
		{Ecosystem: "PyPI", Name: "core", Version: "0.1.0", Repo: "/a"},
		{Ecosystem: "PyPI", Name: "core", Version: "0.1.0", Repo: "/b", OtherIndex: true},
	}
	reg := &recordingRegistry{}
	got := CheckRegistry(context.Background(), deps, reg, prev)
	if len(got[0].Advisories) != 1 || got[0].Advisories[0] != gone {
		t.Errorf("plain pin = %+v, want the withdrawn finding kept", got[0])
	}
	if len(got[1].Advisories) != 0 || !got[1].RegistryUnknown {
		t.Errorf("other-index pin = %+v, want unknown with no finding", got[1])
	}
	if len(reg.queries) != 2 || reg.queries[0].OtherIndex || !reg.queries[1].OtherIndex {
		t.Errorf("queries = %+v, want OtherIndex passed through per pin", reg.queries)
	}
}

// TestCheckRegistryAsksForThePublishedVersion: the registry is asked about
// the version as its registry publishes it. An npm build label is cut, and a
// GitHub Action goes without its commit.
func TestCheckRegistryAsksForThePublishedVersion(t *testing.T) {
	const sha = "a284dc1814e3fd07f2e34267fc8f81227ed29fb8"
	deps := []store.Dependency{
		{Ecosystem: "npm", Name: "cytoscape", Version: "3.31.0+build.7"},
		{
			Ecosystem: discover.ActionsEcosystem,
			Name:      "tj-actions/changed-files",
			Version:   "v45.0.7+" + sha,
		},
		{Ecosystem: "PyPI", Name: "torch", Version: "2.1.0+cu118"},
	}
	reg := &recordingRegistry{}
	CheckRegistry(context.Background(), deps, reg, nil)
	want := []string{"3.31.0", "v45.0.7", "2.1.0+cu118"}
	for i, q := range reg.queries {
		if q.Version != want[i] {
			t.Errorf("%s query version = %q, want %q", q.Name, q.Version, want[i])
		}
	}
	if deps[0].Version != "3.31.0+build.7" {
		t.Errorf("stored version = %q, want it unchanged", deps[0].Version)
	}
}

// hashLookup answers a file's sha256 with the published file listed for it;
// any other hash matches nothing.
type hashLookup map[string]jsdelivr.File

func (h hashLookup) Lookup(_ context.Context, sum string) (jsdelivr.File, bool, error) {
	f, found := h[sum]
	return f, found, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// withdrawingRegistry records its queries and answers every one with npm's
// withdrawn finding, as npm answers for utils@0.1.0 (unpublished in 2012).
type withdrawingRegistry struct{ queries []regcheck.Query }

func (r *withdrawingRegistry) Check(_ context.Context, qs []regcheck.Query) []regcheck.Result {
	r.queries = append(r.queries, qs...)
	out := make([]regcheck.Result, len(qs))
	for i := range out {
		out[i].Findings = []store.Advisory{withdrawn}
	}
	return out
}

// TestRegistryNeverAsksAboutAVendoredFileNamedByItsFileName: a vendored file
// named from its file name is never sent to npm, whether it matched nothing
// (a first-party static/utils-0.1.0.js) or matched on GitHub
// (static/core-1.0.23.js). npm withdrew unrelated packages of those names,
// and that must not read as a malicious file or fire a banner. A file named
// by an npm hash match is still asked.
func TestRegistryNeverAsksAboutAVendoredFileNamedByItsFileName(t *testing.T) {
	root := t.TempDir()
	static := filepath.Join(root, "static")
	if err := os.MkdirAll(static, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"utils-0.1.0.js":          "export const utils = {};\n",
		"core-1.0.23.js":          "/* core 1.0.23, from GitHub */\n",
		"cytoscape-3.31.0.min.js": "/* cytoscape 3.31.0, as published */\n",
	}
	for name, body := range files {
		writeFile(t, filepath.Join(static, name), body)
	}
	lookup := hashLookup{
		sha256Hex([]byte(files["core-1.0.23.js"])): {
			Type: "gh", Name: "someone/core", Version: "v1.0.23",
		},
		sha256Hex([]byte(files["cytoscape-3.31.0.min.js"])): {
			Type: "npm", Name: "cytoscape", Version: "3.31.0",
		},
	}
	st, n, reg := newStore(t), &fakeNotifier{}, &withdrawingRegistry{}
	e := &Engine{
		Prepare:    func() (Plan, error) { return Plan{Repos: []string{root}}, nil },
		Ecosystems: []discover.Ecosystem{discover.VendoredJS{Lookup: lookup}},
		Checker:    fakeChecker{},
		Registry:   reg,
		Notifier:   n,
		Store:      st,
	}

	if err := e.CheckAndNotify(context.Background()); err != nil {
		t.Fatalf("CheckAndNotify: %v", err)
	}

	if len(reg.queries) != 1 || reg.queries[0].Name != "cytoscape" {
		t.Errorf("registry queries = %+v, want cytoscape's alone", reg.queries)
	}
	want := map[string][]string{
		"utils":     {"deps-vendored-modified"},
		"cytoscape": {regcheck.RuleNPMWithdrawn},
	}
	if got := findingIDs(st); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("findings = %v, want %v", got, want)
	}
	if len(st.Snapshot().Deps) != 3 || st.RegistryUnknown() != 0 {
		t.Errorf("deps = %+v, want 3 rows, none unknown", st.Snapshot().Deps)
	}
	if len(n.calls) != 1 || n.calls[0].title != "deps: malicious package cytoscape@3.31.0" {
		t.Errorf("banners = %+v, want the malicious banner for cytoscape alone", n.calls)
	}
}

// TestFileNamedVendoredRowTakesNoEarlierRegistryState: a vendored row named
// by its file name takes neither the withdrawn finding nor the unknown mark
// an earlier build saved for it, on a scan or on a check during an outage,
// and the registry is never asked about it.
func TestFileNamedVendoredRowTakesNoEarlierRegistryState(t *testing.T) {
	const version = "0.1.0+sha256.0123456789ab"
	prev := []store.Dependency{{
		Ecosystem: "npm", Name: "utils", Version: version,
		Advisories: []store.Advisory{withdrawn}, RegistryUnknown: true,
	}}
	row := func() []store.Dependency {
		return []store.Dependency{
			{Ecosystem: "npm", Name: "utils", Version: version, NameFromFile: true},
		}
	}
	reg := &recordingRegistry{}
	got := map[string]store.Dependency{
		"scan":  keepRegistry(row(), prev)[0],
		"check": CheckRegistry(context.Background(), row(), reg, prev)[0],
	}
	for path, d := range got {
		if len(d.Advisories) != 0 || d.RegistryUnknown {
			t.Errorf("%s: row = %+v, want no registry finding and not unknown", path, d)
		}
	}
	if reg.queries != nil {
		t.Errorf("registry queries = %+v, want none", reg.queries)
	}
}

// recordingRegistry records its queries and answers none of them.
type recordingRegistry struct{ queries []regcheck.Query }

func (r *recordingRegistry) Check(_ context.Context, qs []regcheck.Query) []regcheck.Result {
	r.queries = qs
	out := make([]regcheck.Result, len(qs))
	for i := range out {
		out[i].Err = errors.New("registry down")
	}
	return out
}
