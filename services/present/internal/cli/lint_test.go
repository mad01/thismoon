package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/limits"
	"github.com/mad01/thismoon/services/present/internal/lint"
	"github.com/mad01/thismoon/services/present/internal/store"
)

const (
	faultyDoc = `{"summary":"Short.","sections":[{"h":"Results","blocks":[{"t":"stat","value":"3","label":"x"}]}]}`
	cleanDoc  = `{"summary":"Short.","sections":[{"h":"The cache cut p99 latency by 40 percent","blocks":[]}]}`
)

// lintFlags sets the lint command's flags for one test and puts them back.
func lintFlags(t *testing.T, file string, asJSON, strict bool) {
	t.Helper()
	prev := []any{flagLintFile, flagLintJSON, flagLintStrict}
	flagLintFile, flagLintJSON, flagLintStrict = file, asJSON, strict
	t.Cleanup(func() {
		flagLintFile = prev[0].(string)
		flagLintJSON, flagLintStrict = prev[1].(bool), prev[2].(bool)
	})
}

func runLintCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	lintCmd.SetOut(&out)
	t.Cleanup(func() { lintCmd.SetOut(nil) })
	err := runLint(lintCmd, args)
	return out.String(), err
}

// argsFile writes a present_create argument object with content as the
// Doc string the tool takes.
func argsFile(t *testing.T, doc string) string {
	t.Helper()
	data, err := json.Marshal(map[string]string{"title": "T", "content": doc})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "args.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLintFilePrintsOneLinePerFinding(t *testing.T) {
	lintFlags(t, argsFile(t, faultyDoc), false, false)
	out, err := runLintCmd(t)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	want := `content.sections[0].h [heading.label] section "Results": the heading is a label`
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 1 ||
		!strings.HasPrefix(lines[0], want) {
		t.Errorf("printed %q, want one line opening %q", out, want)
	}
}

func TestLintCleanPagePrintsNothing(t *testing.T) {
	lintFlags(t, argsFile(t, cleanDoc), false, true)
	out, err := runLintCmd(t)
	if err != nil || out != "" {
		t.Errorf("clean page with --strict: printed %q, err %v; want nothing", out, err)
	}
}

func TestLintJSONAndStrict(t *testing.T) {
	lintFlags(t, argsFile(t, faultyDoc), true, true)
	out, err := runLintCmd(t)
	if err == nil || !strings.Contains(err.Error(), "1 warning") {
		t.Errorf("--strict with a finding: err = %v, want a count of 1 warning", err)
	}
	var r lint.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("--json output: %v\n%s", err, out)
	}
	if len(r.Warnings) != 1 || r.Counts[limits.HeadingLabel] != 1 ||
		len(r.Counts) != len(limits.Warnings()) {
		t.Errorf("report = %+v", r)
	}
}

func TestLintReadsStdin(t *testing.T) {
	lintFlags(t, "-", false, false)
	lintCmd.SetIn(strings.NewReader(`{"title":"T","deck":` + faultyDoc + `}`))
	t.Cleanup(func() { lintCmd.SetIn(nil) })
	out, err := runLintCmd(t)
	if err != nil || !strings.HasPrefix(out, "deck.sections[0].h [heading.label]") {
		t.Errorf("stdin: printed %q, err %v", out, err)
	}
}

func TestLintStoredPage(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	withDoc, err := st.Create(t.Context(), store.Draft{
		Title: "T", Content: "<p>x</p>", Doc: []byte(faultyDoc),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>`x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	prev := flagWorkdir
	flagWorkdir = dir
	t.Cleanup(func() { flagWorkdir = prev })
	lintFlags(t, "", false, false)

	if out, err := runLintCmd(t, withDoc.ID); err != nil ||
		!strings.Contains(out, "[heading.label]") {
		t.Errorf("stored page: printed %q, err %v", out, err)
	}
	if out, err := runLintCmd(t, legacy.ID); err != nil || out != "" {
		t.Errorf("legacy page: printed %q, err %v; want nothing", out, err)
	}
	if _, err := runLintCmd(t, "missing0000"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing page: err = %v, want ErrNotFound", err)
	}
}

func TestLintNeedsOneTarget(t *testing.T) {
	lintFlags(t, "", false, false)
	if _, err := runLintCmd(t); !errors.Is(err, errLintTarget) {
		t.Errorf("no target: err = %v", err)
	}
	lintFlags(t, "args.json", false, false)
	if _, err := runLintCmd(t, "abc"); !errors.Is(err, errLintTarget) {
		t.Errorf("two targets: err = %v", err)
	}
}

func TestLimitsPrintsTheReference(t *testing.T) {
	var out bytes.Buffer
	limitsCmd.SetOut(&out)
	t.Cleanup(func() { limitsCmd.SetOut(nil) })
	if err := runLimits(limitsCmd, nil); err != nil {
		t.Fatal(err)
	}
	if out.String() != lint.Reference() {
		t.Error("present limits must print the limits reference")
	}

	prev := flagLimitsJSON
	flagLimitsJSON = true
	t.Cleanup(func() { flagLimitsJSON = prev })
	out.Reset()
	if err := runLimits(limitsCmd, nil); err != nil {
		t.Fatal(err)
	}
	var table []limits.Limit
	if err := json.Unmarshal(out.Bytes(), &table); err != nil || len(table) != len(limits.All()) {
		t.Errorf("--json: %d entries, err %v", len(table), err)
	}
}
