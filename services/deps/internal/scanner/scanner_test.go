package scanner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// fakeNotifier records every Notify call instead of shelling out to osascript.
type fakeNotifier struct {
	calls []string
	fail  bool
}

func (f *fakeNotifier) Notify(title, _ string) error {
	if f.fail {
		return errContext
	}
	f.calls = append(f.calls, title)
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
	got, err := Check(context.Background(), deps, fakeChecker{flagName: "bad"})
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
		Prepare: func() ([]string, discover.Options, error) {
			return []string{"/r1"}, discover.Options{}, nil
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
