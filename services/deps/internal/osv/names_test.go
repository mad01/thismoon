package osv

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
// azure/setup-kubectl, as its README spells it, is asked for in OSV's
// spelling. The range filter and the fix version match it too.
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
	want := []string{"Azure/setup-kubectl", "tj-actions/changed-files"}
	if names := nameLookups(bodies()); !slices.Equal(names, want) {
		t.Errorf("name lookups = %v, want OSV's spellings %v", names, want)
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

func TestParseActionNames(t *testing.T) {
	record := func(id, ecosystem, name string) []byte {
		return fmt.Appendf(nil,
			`{"id":%q,"affected":[{"package":{"ecosystem":%q,"name":%q}}]}`,
			id, ecosystem, name)
	}
	names, err := parseActionNames(exportZip(t,
		record("GHSA-1", "GitHub Actions", "Azure/setup-kubectl"),
		record("GHSA-2", "GitHub Actions", "Azure/setup-kubectl"),
		record("GHSA-3", "GitHub Actions", "azure/Setup-Kubectl"),
		record("GHSA-4", "npm", "AZURE/SETUP-KUBECTL"),
	))
	if err != nil {
		t.Fatalf("parseActionNames: %v", err)
	}
	want := []string{"Azure/setup-kubectl", "azure/Setup-Kubectl"}
	if got := names.spellings("azure/setup-kubectl"); !slices.Equal(got, want) {
		t.Errorf("spellings = %v, want %v (npm names left out)", got, want)
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
