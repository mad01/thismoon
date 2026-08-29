package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
)

var kofFacts = agentdoc.Facts{
	Name: "keeper-of-facts", Bin: "kof",
	Purpose: "evidence-pinned assertion store", HasDoctor: true,
}

func pass(name string) Check {
	return Check{Name: name, Run: func(context.Context) error { return nil }}
}

func fail(name, msg string) Check {
	return Check{Name: name, Run: func(context.Context) error { return errors.New(msg) }}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		checks  []Check
		want    string
		wantErr string
	}{
		{
			name:   "all pass",
			checks: []Check{pass("alpha"), pass("beta")},
			want: "ok alpha\nok beta\n" +
				"all checks passed; for semantics and gotchas run 'kof docs'\n",
		},
		{
			name:    "one fails",
			checks:  []Check{pass("alpha"), fail("beta", "boom")},
			want:    "ok alpha\nFAIL beta: boom\n",
			wantErr: "doctor: 1 of 2 checks failed",
		},
		{
			name:    "all fail",
			checks:  []Check{fail("alpha", "down"), fail("beta", "boom")},
			want:    "FAIL alpha: down\nFAIL beta: boom\n",
			wantErr: "doctor: 2 of 2 checks failed",
		},
		{
			name: "skip counts as pass",
			checks: []Check{
				pass("alpha"),
				{Name: "beta", Run: func(context.Context) error {
					return skip{"skipped: nothing to examine"}
				}},
			},
			want: "ok alpha\nok beta (skipped: nothing to examine)\n" +
				"all checks passed; for semantics and gotchas run 'kof docs'\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			err := Run(context.Background(), &b, kofFacts, tc.checks)
			if got := b.String(); got != tc.want {
				t.Errorf("Run output = %q, want %q", got, tc.want)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Run() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("Run() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestCollect(t *testing.T) {
	skipped := Check{Name: "gamma", Run: func(context.Context) error {
		return skip{"skipped: no store path configured"}
	}}

	tests := []struct {
		name   string
		checks []Check
		want   Report
	}{
		{
			name:   "no checks is a pass",
			checks: nil,
			want:   Report{OK: true, Checks: []Result{}},
		},
		{
			name:   "all pass",
			checks: []Check{pass("alpha"), pass("beta")},
			want: Report{OK: true, Checks: []Result{
				{Name: "alpha", Status: StatusOK},
				{Name: "beta", Status: StatusOK},
			}},
		},
		{
			name:   "skip counts as a pass and keeps its note",
			checks: []Check{pass("alpha"), skipped},
			want: Report{OK: true, Checks: []Result{
				{Name: "alpha", Status: StatusOK},
				{Name: "gamma", Status: StatusSkipped, Detail: "skipped: no store path configured"},
			}},
		},
		{
			name:   "one failure sinks the report",
			checks: []Check{pass("alpha"), fail("beta", "boom"), skipped},
			want: Report{OK: false, Checks: []Result{
				{Name: "alpha", Status: StatusOK},
				{Name: "beta", Status: StatusFail, Detail: "boom"},
				{Name: "gamma", Status: StatusSkipped, Detail: "skipped: no store path configured"},
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Collect(context.Background(), tc.checks)
			if got.OK != tc.want.OK {
				t.Errorf("Collect().OK = %v, want %v", got.OK, tc.want.OK)
			}
			if len(got.Checks) != len(tc.want.Checks) {
				t.Fatalf("Collect() = %+v, want %+v", got.Checks, tc.want.Checks)
			}
			for i, c := range got.Checks {
				if c != tc.want.Checks[i] {
					t.Errorf("Collect().Checks[%d] = %+v, want %+v", i, c, tc.want.Checks[i])
				}
			}
		})
	}
}

func TestReportMarshalsToJSON(t *testing.T) {
	report := Collect(context.Background(), []Check{pass("alpha"), fail("beta", "boom")})
	got, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal(Report) error: %v", err)
	}
	want := `{"ok":false,"checks":[{"name":"alpha","status":"ok"},` +
		`{"name":"beta","status":"fail","detail":"boom"}]}`
	if string(got) != want {
		t.Errorf("Marshal(Report) = %s, want %s", got, want)
	}
}

func TestServiceReachable(t *testing.T) {
	ok := httptest.NewServer(buildinfo.Info{Commit: "abc"}.Handler())
	defer ok.Close()
	broken := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer broken.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()

	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{name: "reachable", baseURL: ok.URL},
		{name: "non-200", baseURL: broken.URL, wantErr: "returned 500, want 200"},
		{name: "unreachable", baseURL: down.URL, wantErr: "service not reachable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := ServiceReachable(tc.baseURL)
			if c.Name != "service-reachable" {
				t.Errorf("Name = %q, want %q", c.Name, "service-reachable")
			}
			err := c.Run(context.Background())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Run() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Run() error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestVersionSkew(t *testing.T) {
	const built = "1111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const serving = "2222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	srv := httptest.NewServer(buildinfo.Info{Commit: serving}.Handler())
	defer srv.Close()
	blank := httptest.NewServer(buildinfo.Info{}.Handler())
	defer blank.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()

	tests := []struct {
		name        string
		baseURL     string
		localCommit string
		wantErr     string
		wantSkip    bool
	}{
		{name: "match", baseURL: srv.URL, localCommit: serving},
		{
			name: "mismatch", baseURL: srv.URL, localCommit: built,
			wantErr: "new binary on disk, old process serving; restart via t-man and re-check",
		},
		{name: "unknown local commit", baseURL: srv.URL, localCommit: "", wantSkip: true},
		{name: "unknown remote commit", baseURL: blank.URL, localCommit: built, wantSkip: true},
		{
			name: "service down", baseURL: down.URL, localCommit: built,
			wantErr: "cannot read",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := versionSkew(tc.baseURL, tc.localCommit)
			if c.Name != "version-skew" {
				t.Errorf("Name = %q, want %q", c.Name, "version-skew")
			}
			err := c.Run(context.Background())
			var s skip
			switch {
			case tc.wantSkip:
				if !errors.As(err, &s) {
					t.Errorf("Run() = %v, want a skip note", err)
				}
			case tc.wantErr == "":
				if err != nil {
					t.Errorf("Run() error = %v, want nil", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Run() error = %v, want it to contain %q", err, tc.wantErr)
				}
				if errors.As(err, &s) {
					t.Errorf("Run() = %v, is a skip, want a failure", err)
				}
			}
		})
	}
}

func TestStoreReadable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "assertions.jsonl")
	if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		path     string
		wantErr  string
		wantSkip bool
	}{
		{name: "readable directory", path: dir},
		{name: "readable file", path: file},
		{name: "missing path", path: filepath.Join(dir, "nope"), wantErr: "store not readable"},
		{name: "empty path skips", path: "", wantSkip: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := StoreReadable(tc.path)
			if c.Name != "store-readable" {
				t.Errorf("Name = %q, want %q", c.Name, "store-readable")
			}
			err := c.Run(context.Background())
			var s skip
			switch {
			case tc.wantSkip:
				if !errors.As(err, &s) || !strings.Contains(err.Error(), "skipped") {
					t.Errorf("Run() = %v, want a skipped note", err)
				}
			case tc.wantErr == "":
				if err != nil {
					t.Errorf("Run() error = %v, want nil", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Run() error = %v, want it to contain %q", err, tc.wantErr)
				}
			}
		})
	}
}

// TestSkip covers the exported constructor a component uses for a check that
// passed with something to say: it has to land as StatusSkipped in a report
// (so the MCP doctor tools carry the note) and render as an ok line with the
// note in the CLI output.
func TestSkip(t *testing.T) {
	checks := []Check{
		{Name: "unconfigured", Run: func(context.Context) error { return Skip("no config file yet") }},
	}

	report := Collect(context.Background(), checks)
	if !report.OK {
		t.Error("Collect().OK = false, want a skipped check to count as a pass")
	}
	if got := report.Checks[0].Status; got != StatusSkipped {
		t.Errorf("status = %q, want %q", got, StatusSkipped)
	}
	if got := report.Checks[0].Detail; got != "no config file yet" {
		t.Errorf("detail = %q, want the note", got)
	}

	var out bytes.Buffer
	if err := Run(context.Background(), &out, agentdoc.Facts{Bin: "csl"}, checks); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if want := "ok unconfigured (no config file yet)\n"; !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
}
