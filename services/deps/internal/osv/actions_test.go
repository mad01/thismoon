package osv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// tjCommit is the commit tj-actions/changed-files tag v45.0.7 pointed to.
const tjCommit = "a284dc1814e3fd07f2e34267fc8f81227ed29fb8"

// tjStub is OSV as it answered for tj-actions/changed-files on 2026-10-28.
// The commit lookup finds CVE-2025-30066, whose git range holds the v45.0.7
// commit. The name lookup finds both advisories on the package: the March
// 2025 compromise (GHSA-mrrh-fwg8-r2c3, fixed in 46.0.1) and an older command
// injection (GHSA-mcph-m25j-8j63, fixed in 41). Like OSV, it matches names
// case-sensitively, so Azure/setup-kubectl finds GHSA-p756-rfxh-x63h (fixed
// in 3) and azure/setup-kubectl finds nothing. It also answers for the two
// actions OSV files under a longer name than owner/repo: GHSA-r79c-pqj3-577x
// under super-linter/super-linter and its /slim sub-action, and
// GHSA-hw6r-g8gj-2987 under a URL to a pytorch sub-action alone. The records
// in testdata are trimmed copies of the real ones, and the name list export
// is a zip of them. It records each querybatch body.
func tjStub(t *testing.T) (*httptest.Server, func() []batchRequest) {
	t.Helper()
	export := exportZip(t, testdataRecords(t)...)
	return tjStubWithExport(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(export)
	})
}

// stubNameHits is the advisories tjStub lists for each package name, matched
// case-sensitively as OSV does.
var stubNameHits = map[string][]string{
	"tj-actions/changed-files":       {"GHSA-mcph-m25j-8j63", "GHSA-mrrh-fwg8-r2c3"},
	"Azure/setup-kubectl":            {"GHSA-p756-rfxh-x63h"},
	"super-linter/super-linter":      {"GHSA-r79c-pqj3-577x"},
	"super-linter/super-linter/slim": {"GHSA-r79c-pqj3-577x"},
	pytorchFilterAction:              {"GHSA-hw6r-g8gj-2987"},
}

// pytorchFilterAction is the name OSV files GHSA-hw6r-g8gj-2987 under: a URL
// to a sub-action of pytorch/pytorch.
const pytorchFilterAction = "https://github.com/pytorch/pytorch/.github/actions/filter-test-configs"

// tjStubWithExport is tjStub with export serving the name list.
func tjStubWithExport(
	t *testing.T,
	export http.HandlerFunc,
) (*httptest.Server, func() []batchRequest) {
	t.Helper()
	var (
		mu     sync.Mutex
		bodies []batchRequest
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /GitHub%20Actions/all.zip", export)
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req batchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode querybatch: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, req)
		mu.Unlock()
		results := make([]map[string]any, len(req.Queries))
		for i, q := range req.Queries {
			var found []string
			switch {
			case q.Commit == tjCommit:
				found = []string{"CVE-2025-30066"}
			case q.Package != nil:
				found = stubNameHits[q.Package.Name]
			}
			vulns := make([]map[string]string, 0, len(found))
			for _, id := range found {
				vulns = append(vulns, map[string]string{"id": id})
			}
			results[i] = map[string]any{}
			if len(vulns) > 0 {
				results[i]["vulns"] = vulns
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /v1/vulns/{id}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := os.ReadFile("testdata/" + r.PathValue("id") + ".json")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(raw)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() []batchRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(bodies)
	}
}

// TestCheckActionByCommit is the tj-actions/changed-files v45.0.7 fixture: the
// commit lookup returns CVE-2025-30066, and of the two advisories on the
// package only the one whose range holds 45.0.7 is kept.
func TestCheckActionByCommit(t *testing.T) {
	srv, bodies := tjStub(t)

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{{
		Name:      "tj-actions/changed-files",
		Ecosystem: "GitHub Actions",
		Version:   "v45.0.7",
		Commit:    tjCommit,
	}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if want := []string{"CVE-2025-30066", "GHSA-mrrh-fwg8-r2c3"}; !slices.Equal(ids(got[0]), want) {
		t.Fatalf(
			"advisories = %v, want %v (GHSA-mcph-m25j-8j63 was fixed in 41)",
			ids(got[0]),
			want,
		)
	}
	cve, ghsa := got[0][0], got[0][1]
	if cve.Kind != store.KindVuln || cve.Summary == "" {
		t.Errorf("CVE-2025-30066 = %+v, want a vuln with the record's details", cve)
	}
	if ghsa.FixedVersion != "46.0.1" || ghsa.Severity != "HIGH" {
		t.Errorf("GHSA-mrrh-fwg8-r2c3 = %+v, want HIGH, fixed in 46.0.1", ghsa)
	}

	// OSV rejects a commit beside a version, and a version on the name lookup
	// would match nothing in this ecosystem.
	reqs := bodies()
	if len(reqs) != 1 || len(reqs[0].Queries) != 2 {
		t.Fatalf("querybatch requests = %+v, want one with two queries", reqs)
	}
	byCommit, byName := reqs[0].Queries[0], reqs[0].Queries[1]
	if byCommit.Commit != tjCommit || byCommit.Package != nil || byCommit.Version != "" {
		t.Errorf("commit lookup = %+v, want the commit alone", byCommit)
	}
	if byName.Package == nil || byName.Package.Ecosystem != "GitHub Actions" ||
		byName.Version != "" || byName.Commit != "" {
		t.Errorf("name lookup = %+v, want the package alone", byName)
	}
}

// TestCheckActionWithoutFullVersionKeepsEveryNameHit: a major-only label (v45)
// could be any 45.x.y, so every advisory on the package is kept rather than
// guessing.
func TestCheckActionWithoutFullVersionKeepsEveryNameHit(t *testing.T) {
	srv, _ := tjStub(t)
	for _, label := range []string{"v45", "", "main"} {
		got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{{
			Name:      "tj-actions/changed-files",
			Ecosystem: "GitHub Actions",
			Version:   label,
			Commit:    tjCommit,
		}})
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		want := []string{"CVE-2025-30066", "GHSA-mcph-m25j-8j63", "GHSA-mrrh-fwg8-r2c3"}
		if !slices.Equal(ids(got[0]), want) {
			t.Errorf("label %q: advisories = %v, want %v", label, ids(got[0]), want)
		}
	}
}

// TestCheckActionPastEveryFix: a version past both fixes keeps no name hit, and
// a clean commit adds nothing.
func TestCheckActionPastEveryFix(t *testing.T) {
	srv, _ := tjStub(t)
	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{{
		Name:      "tj-actions/changed-files",
		Ecosystem: "GitHub Actions",
		Version:   "v46.0.5",
		Commit:    "ed68ef82c095e0d48ec87eccea555d944a631a4c",
	}})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got[0]) != 0 {
		t.Errorf("advisories = %v, want none", ids(got[0]))
	}
}

func TestInRange(t *testing.T) {
	tests := []struct {
		name   string
		events []event
		ver    string
		in, ok bool
	}{
		{"below fix", []event{{Introduced: "0"}, {Fixed: "46.0.1"}}, "45.0.7", true, true},
		{"at fix", []event{{Introduced: "0"}, {Fixed: "46.0.1"}}, "46.0.1", false, true},
		{"short bound", []event{{Introduced: "0"}, {Fixed: "41"}}, "40.9.9", true, true},
		{
			"before introduced",
			[]event{{Introduced: "2.0.0"}, {Fixed: "2.3.0"}},
			"1.9.0",
			false,
			true,
		},
		{"last affected", []event{{Introduced: "0"}, {LastAffected: "4.1.0"}}, "4.1.0", true, true},
		{
			"past last affected",
			[]event{{Introduced: "0"}, {LastAffected: "4.1.0"}},
			"4.1.1",
			false,
			true,
		},
		{
			"second range",
			[]event{
				{Introduced: "1.0.0"},
				{Fixed: "1.5.0"},
				{Introduced: "2.0.0"},
				{Fixed: "2.3.0"},
			},
			"2.1.0", true, true,
		},
		{
			"between ranges",
			[]event{
				{Introduced: "1.0.0"},
				{Fixed: "1.5.0"},
				{Introduced: "2.0.0"},
				{Fixed: "2.3.0"},
			},
			"1.7.0", false, true,
		},
		{"unsorted", []event{{Fixed: "2.3.0"}, {Introduced: "2.0.0"}}, "2.1.0", true, true},
		// At one version a fixed sorts before an introduced in either record
		// order, so a range re-introduced at its own fix stays open.
		{"re-introduced at its fix", []event{
			{Introduced: "0"}, {Fixed: "1.0.0"}, {Introduced: "1.0.0"},
		}, "1.0.0", true, true},
		{"past re-introduced at its fix", []event{
			{Introduced: "0"}, {Fixed: "1.0.0"}, {Introduced: "1.0.0"},
		}, "1.5.0", true, true},
		{"before re-introduced at its fix", []event{
			{Introduced: "0"}, {Fixed: "1.0.0"}, {Introduced: "1.0.0"},
		}, "0.5.0", true, true},
		// An introduced and a fixed at one version then fail open: affected.
		{"introduced then fixed at one version", []event{
			{Introduced: "1.0.0"}, {Fixed: "1.0.0"},
		}, "1.0.0", true, true},
		{"fixed then introduced at one version", []event{
			{Fixed: "1.0.0"}, {Introduced: "1.0.0"},
		}, "1.0.0", true, true},
		{"before introduced and fixed at one version", []event{
			{Fixed: "1.0.0"}, {Introduced: "1.0.0"},
		}, "0.9.0", false, true},
		{"last affected then introduced at one version", []event{
			{LastAffected: "1.0.0"}, {Introduced: "1.0.0"},
		}, "1.0.0", true, true},
		{"past last affected then introduced at one version", []event{
			{LastAffected: "1.0.0"}, {Introduced: "1.0.0"},
		}, "1.0.1", false, true},
		{
			"bound not numeric",
			[]event{{Introduced: "0"}, {Fixed: "2.0.0-rc1"}},
			"1.0.0",
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver, ok := parseVersion(tt.ver)
			if !ok {
				t.Fatalf("parseVersion(%q) failed", tt.ver)
			}
			in, ok := inRange(tt.events, ver)
			if in != tt.in || ok != tt.ok {
				t.Errorf("inRange = %v, %v; want %v, %v", in, ok, tt.in, tt.ok)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	for s, want := range map[string]bool{
		"v45.0.7": true, "45.0.7": true,
		"v45": false, "v4.2": false, "main": false, "v2-beta": false, "1.2.3.4": false, "": false,
	} {
		if _, ok := parseVersion(s); ok != want {
			t.Errorf("parseVersion(%q) ok = %v, want %v", s, ok, want)
		}
	}
}
