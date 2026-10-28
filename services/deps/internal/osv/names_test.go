package osv

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
)

// exportZip packs records the way OSV's data export does: one JSON file per
// advisory, named after its id.
func exportZip(t *testing.T, records ...[]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rec := range records {
		var v struct{ ID string }
		if err := json.Unmarshal(rec, &v); err != nil {
			t.Fatalf("record %s: %v", rec, err)
		}
		w, err := zw.Create(v.ID + ".json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(rec); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testdataRecords returns every advisory record in testdata.
func testdataRecords(t *testing.T) [][]byte {
	t.Helper()
	paths, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	records := make([][]byte, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, raw)
	}
	return records
}

// nameLookups returns the package names the querybatch requests asked for.
func nameLookups(reqs []batchRequest) []string {
	var out []string
	for _, r := range reqs {
		for _, q := range r.Queries {
			if q.Package != nil {
				out = append(out, q.Package.Name)
			}
		}
	}
	return out
}

// TestCheckActionAsksForOSVSpelling: OSV files Azure/setup-kubectl with a
// capital A and matches names case-sensitively, so the action written as
// azure/setup-kubectl, as its README spells it, is asked for as written and
// in OSV's spelling. The range filter and the fix version match it too.
func TestCheckActionAsksForOSVSpelling(t *testing.T) {
	srv, bodies := tjStub(t)

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{
		{
			Name:      "azure/setup-kubectl",
			Ecosystem: "GitHub Actions",
			Version:   "v2.0.0",
			Commit:    "5555555555555555555555555555555555555555",
		},
		{
			Name:      "TJ-Actions/Changed-Files",
			Ecosystem: "GitHub Actions",
			Version:   "v45.0.7",
			Commit:    tjCommit,
		},
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !slices.Equal(ids(got[0]), []string{"GHSA-p756-rfxh-x63h"}) {
		t.Fatalf("azure/setup-kubectl advisories = %v, want GHSA-p756-rfxh-x63h", ids(got[0]))
	}
	if got[0][0].FixedVersion != "3" {
		t.Errorf("GHSA-p756-rfxh-x63h fix = %q, want 3", got[0][0].FixedVersion)
	}
	if want := []string{"CVE-2025-30066", "GHSA-mrrh-fwg8-r2c3"}; !slices.Equal(ids(got[1]), want) {
		t.Errorf("TJ-Actions/Changed-Files advisories = %v, want %v", ids(got[1]), want)
	}
	want := []string{
		"azure/setup-kubectl", "Azure/setup-kubectl",
		"TJ-Actions/Changed-Files", "tj-actions/changed-files",
	}
	if names := nameLookups(bodies()); !slices.Equal(names, want) {
		t.Errorf(
			"name lookups = %v, want each name as written, then OSV's spellings %v",
			names,
			want,
		)
	}
}

// TestCheckActionAsksForTheWrittenName: the export can lag OSV. When it holds
// only another spelling of an action, an advisory OSV files under the name as
// written is still found.
func TestCheckActionAsksForTheWrittenName(t *testing.T) {
	export := exportZip(t, []byte(
		`{"id":"GHSA-0000-0000-0000","affected":[{"package":`+
			`{"ecosystem":"GitHub Actions","name":"TJ-Actions/changed-files"}}]}`,
	))
	srv, _ := tjStubWithExport(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(export)
	})

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
		t.Errorf("advisories = %v, want %v", ids(got[0]), want)
	}
}

// TestCheckActionWithoutNameList: a name list that can't be read must not
// pass for clean. Check still returns every finding, asking for each name as
// written, beside an *ActionNamesError.
func TestCheckActionWithoutNameList(t *testing.T) {
	srv, bodies := tjStubWithExport(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{{
		Name:      "tj-actions/changed-files",
		Ecosystem: "GitHub Actions",
		Version:   "v45.0.7",
		Commit:    tjCommit,
	}})
	var namesErr *ActionNamesError
	if !errors.As(err, &namesErr) {
		t.Fatalf("Check error = %v, want an *ActionNamesError", err)
	}
	if want := []string{"CVE-2025-30066", "GHSA-mrrh-fwg8-r2c3"}; !slices.Equal(ids(got[0]), want) {
		t.Errorf("advisories = %v, want %v beside the error", ids(got[0]), want)
	}
	if names := nameLookups(bodies()); !slices.Equal(names, []string{"tj-actions/changed-files"}) {
		t.Errorf("name lookups = %v, want the name as written", names)
	}
}

// TestCheckReadsNameListOncePerCall: the list is read once however many
// actions a check covers, and not at all when it covers none.
func TestCheckReadsNameListOncePerCall(t *testing.T) {
	export := exportZip(t, testdataRecords(t)...)
	var hits atomic.Int32
	srv, _ := tjStubWithExport(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(export)
	})
	c := NewWithURL(srv.URL)
	action := Query{Name: "a/b", Ecosystem: "GitHub Actions", Commit: tjCommit}

	if _, err := c.Check(context.Background(), []Query{action, action}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if _, err := c.Check(context.Background(), []Query{
		{Name: "left-pad", Ecosystem: "npm", Version: "1.0.0"},
	}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("name list read %d times, want once", n)
	}
}

// TestCheckActionAsksForLongNames: OSV files a few actions under a longer
// name than owner/repo, the name discovery gives every row. The row is asked
// for by each of them, and what they find is its own. GHSA-r79c-pqj3-577x is
// filed under super-linter/super-linter and its /slim sub-action, so both
// lookups find it and it is listed once, range-filtered and with its fix.
// GHSA-hw6r-g8gj-2987 is filed only under a URL to a pytorch sub-action.
func TestCheckActionAsksForLongNames(t *testing.T) {
	srv, bodies := tjStub(t)

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{
		{
			Name:      "super-linter/super-linter",
			Ecosystem: "GitHub Actions",
			Version:   "v8.3.0",
			Commit:    "6666666666666666666666666666666666666666",
		},
		{
			Name:      "pytorch/pytorch",
			Ecosystem: "GitHub Actions",
			Version:   "v2.1.0",
			Commit:    "7777777777777777777777777777777777777777",
		},
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !slices.Equal(ids(got[0]), []string{"GHSA-r79c-pqj3-577x"}) {
		t.Errorf("super-linter advisories = %v, want GHSA-r79c-pqj3-577x once", ids(got[0]))
	} else if got[0][0].FixedVersion != "8.3.1" {
		t.Errorf("GHSA-r79c-pqj3-577x fix = %q, want 8.3.1", got[0][0].FixedVersion)
	}
	if !slices.Equal(ids(got[1]), []string{"GHSA-hw6r-g8gj-2987"}) {
		t.Errorf("pytorch/pytorch advisories = %v, want GHSA-hw6r-g8gj-2987", ids(got[1]))
	}
	want := []string{
		"super-linter/super-linter", "super-linter/super-linter/slim",
		"pytorch/pytorch", pytorchFilterAction,
	}
	if names := nameLookups(bodies()); !slices.Equal(names, want) {
		t.Errorf("name lookups = %v, want each owner/repo, then its longer names %v", names, want)
	}
}

// TestActionRangeUnderLongName: a range filed under a sub-action's path or a
// URL to it judges the owner/repo row's version, and its fix is the row's.
func TestActionRangeUnderLongName(t *testing.T) {
	q := Query{Name: "Acme/Tool", Ecosystem: "GitHub Actions", Version: "v2.1.0"}
	for _, name := range []string{
		"acme/tool/sub",
		"https://github.com/acme/tool/.github/actions/sub",
	} {
		var v vuln
		rec := fmt.Sprintf(`{"id":"GHSA-1","affected":[{"package":`+
			`{"ecosystem":"GitHub Actions","name":%q},"ranges":[{"type":"ECOSYSTEM",`+
			`"events":[{"introduced":"0"},{"fixed":"2.0.0"}]}]}]}`, name)
		if err := json.Unmarshal([]byte(rec), &v); err != nil {
			t.Fatal(err)
		}
		if v.affects(q) {
			t.Errorf("%s: %s %s, past the fix, reads as affected", name, q.Name, q.Version)
		}
		if fix := v.fixedFor(q); fix != "2.0.0" {
			t.Errorf("%s: fix = %q, want 2.0.0", name, fix)
		}
	}
}

// TestActionRangeIgnoresOtherEcosystems: a record can list the action's repo
// under another ecosystem too: a GIT entry by URL, which actionRepo cuts to
// owner/repo, with commit ranges, or a Packagist package of the same name.
// Only the GitHub Actions entry judges the row's version and gives its fix.
func TestActionRangeIgnoresOtherEcosystems(t *testing.T) {
	q := Query{Name: "acme/tool", Ecosystem: "GitHub Actions", Version: "v2.1.0"}
	var v vuln
	rec := `{"id":"GHSA-1","affected":[` +
		`{"package":{"ecosystem":"GIT","name":"https://github.com/acme/tool"},` +
		`"ranges":[{"type":"GIT","events":[{"introduced":"0"},` +
		`{"fixed":"8888888888888888888888888888888888888888"}]}]},` +
		`{"package":{"ecosystem":"Packagist","name":"acme/tool"},` +
		`"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"},{"fixed":"9.0.0"}]}]},` +
		`{"package":{"ecosystem":"GitHub Actions","name":"acme/tool"},` +
		`"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"},{"fixed":"2.0.0"}]}]}]}`
	if err := json.Unmarshal([]byte(rec), &v); err != nil {
		t.Fatal(err)
	}
	if v.affects(q) {
		t.Errorf("%s %s, past the GitHub Actions fix, reads as affected", q.Name, q.Version)
	}
	if fix := v.fixedFor(q); fix != "2.0.0" {
		t.Errorf("fix = %q, want 2.0.0 from the GitHub Actions entry", fix)
	}
}

// exportRecord is an export record for one advisory on one package.
func exportRecord(id, ecosystem, name string) []byte {
	return fmt.Appendf(nil,
		`{"id":%q,"affected":[{"package":{"ecosystem":%q,"name":%q}}]}`,
		id, ecosystem, name)
}

// TestParseActionNamesFoldsLongNames: a sub-action's path, or a github.com URL
// to one, is listed under its owner/repo, ignoring case, and so is a name
// with a trailing slash. A name it can't read an owner/repo from for sure (a
// URL that doesn't start https://github.com/, fewer than two segments, an
// empty segment) is listed under its own name, as any other name is: such a
// name is missed, never filed under the wrong row.
func TestParseActionNamesFoldsLongNames(t *testing.T) {
	const (
		slim   = "super-linter/super-linter/slim"
		widget = "HTTPS://GitHub.com/Acme/Widget/.github/actions/build"
	)
	names, err := parseActionNames(exportZip(t,
		exportRecord("GHSA-1", "GitHub Actions", "super-linter/super-linter"),
		exportRecord("GHSA-2", "GitHub Actions", slim),
		exportRecord("GHSA-3", "GitHub Actions", pytorchFilterAction),
		exportRecord("GHSA-4", "GitHub Actions", widget),
		exportRecord("GHSA-5", "GitHub Actions", "https://gitlab.com/acme/tool/sub"),
		exportRecord("GHSA-6", "GitHub Actions", "https://github.com/acme"),
		exportRecord("GHSA-7", "GitHub Actions", "acme"),
		exportRecord("GHSA-8", "GitHub Actions", "acme/gadget/"),
		exportRecord("GHSA-9", "GitHub Actions", "acme//tool"),
		exportRecord("GHSA-10", "GitHub Actions", "http://github.com/acme/tool/sub"),
	))
	if err != nil {
		t.Fatalf("parseActionNames: %v", err)
	}
	want := actionNames{
		"super-linter/super-linter":        {"super-linter/super-linter", slim},
		"pytorch/pytorch":                  {pytorchFilterAction},
		"acme/widget":                      {widget},
		"https://gitlab.com/acme/tool/sub": {"https://gitlab.com/acme/tool/sub"},
		"https://github.com/acme":          {"https://github.com/acme"},
		"acme":                             {"acme"},
		"acme/gadget":                      {"acme/gadget/"},
		"acme//tool":                       {"acme//tool"},
		"http://github.com/acme/tool/sub":  {"http://github.com/acme/tool/sub"},
	}
	if !maps.EqualFunc(names, want, slices.Equal) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if got, want := names.spellings("Super-Linter/Super-Linter"), []string{
		"Super-Linter/Super-Linter", "super-linter/super-linter", slim,
	}; !slices.Equal(got, want) {
		t.Errorf("spellings in another case = %v, want %v", got, want)
	}
}

func TestParseActionNames(t *testing.T) {
	names, err := parseActionNames(exportZip(t,
		exportRecord("GHSA-1", "GitHub Actions", "Azure/setup-kubectl"),
		exportRecord("GHSA-2", "GitHub Actions", "Azure/setup-kubectl"),
		exportRecord("GHSA-3", "GitHub Actions", "azure/Setup-Kubectl"),
		exportRecord("GHSA-4", "npm", "AZURE/SETUP-KUBECTL"),
	))
	if err != nil {
		t.Fatalf("parseActionNames: %v", err)
	}
	want := []string{"azure/setup-kubectl", "Azure/setup-kubectl", "azure/Setup-Kubectl"}
	if got := names.spellings("azure/setup-kubectl"); !slices.Equal(got, want) {
		t.Errorf("spellings = %v, want %v (npm names left out)", got, want)
	}
	want = []string{"Azure/setup-kubectl", "azure/Setup-Kubectl"}
	if got := names.spellings("Azure/setup-kubectl"); !slices.Equal(got, want) {
		t.Errorf("spellings of an OSV spelling = %v, want %v (no repeat)", got, want)
	}
	if got := names.spellings("acme/unknown"); !slices.Equal(got, []string{"acme/unknown"}) {
		t.Errorf("spellings of a name with no advisory = %v, want it as written", got)
	}

	var broken bytes.Buffer
	zw := zip.NewWriter(&broken)
	w, err := zw.Create("GHSA-1.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`{"id":"GHSA-1","affected":[`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseActionNames(broken.Bytes()); err == nil {
		t.Error("an export with an unreadable record parsed without error")
	}
}
