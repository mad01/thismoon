package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	deps "github.com/mad01/thismoon/services/deps"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// savedScan writes a completed full scan holding saved and errs to a fresh
// workdir and returns the workdir.
func savedScan(t *testing.T, saved []store.Dependency, errs []store.DiscoveryError) string {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(saved, errs); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiscoveryCleanCheck(t *testing.T) {
	run := func(dir string) doctor.Result {
		t.Helper()
		return doctor.Collect(context.Background(), []doctor.Check{discoveryClean(dir)}).Checks[0]
	}

	if got := run(t.TempDir()); got.Status != doctor.StatusSkipped {
		t.Errorf("no scan yet: status = %q, want skipped", got.Status)
	}
	if got := run(savedScan(t, nil, nil)); got.Status != doctor.StatusOK {
		t.Errorf("clean scan: status = %q (%s), want ok", got.Status, got.Detail)
	}

	got := run(savedScan(t, nil, []store.DiscoveryError{
		{Repo: "/r1", Ecosystem: "Go", Error: `exec: "go": executable file not found in $PATH`},
	}))
	if got.Status != doctor.StatusFail {
		t.Fatalf("discovery errors saved: status = %q, want fail", got.Status)
	}
	for _, want := range []string{"1 repo/ecosystem", "Go in /r1", "executable file not found"} {
		if !strings.Contains(got.Detail, want) {
			t.Errorf("detail = %q, want it to contain %q", got.Detail, want)
		}
	}
}

// TestRegistryAnsweredCheck runs the check through both doctor surfaces: the
// report the deps_doctor MCP tool returns (doctor.Collect) and the line `deps
// doctor` prints (doctor.Run).
func TestRegistryAnsweredCheck(t *testing.T) {
	answered := store.Dependency{
		Ecosystem: "Go",
		Name:      "golang.org/x/net",
		Version:   "0.62.0",
		Repo:      "/r1",
	}
	unanswered := func(eco, name, version string) store.Dependency {
		return store.Dependency{
			Ecosystem:       eco,
			Name:            name,
			Version:         version,
			Repo:            "/r2",
			RegistryUnknown: true,
		}
	}

	tests := []struct {
		name       string
		dir        func(t *testing.T) string
		wantStatus string
		wantLine   string
	}{
		{
			name:       "no scan yet",
			dir:        func(t *testing.T) string { return t.TempDir() },
			wantStatus: doctor.StatusSkipped,
			wantLine:   "ok registry-answered (skipped: no scan has completed yet)",
		},
		{
			name: "every lookup answered",
			dir: func(t *testing.T) string {
				return savedScan(t, []store.Dependency{answered}, nil)
			},
			wantStatus: doctor.StatusOK,
			wantLine:   "ok registry-answered",
		},
		{
			name: "lookups unanswered",
			dir: func(t *testing.T) string {
				return savedScan(t, []store.Dependency{
					answered,
					unanswered("npm", "chalk", "5.6.2"),
					unanswered("PyPI", "requests", "2.32.3"),
					unanswered("npm", "debug", "4.4.1"),
				}, nil)
			},
			wantStatus: doctor.StatusFail,
			wantLine:   "FAIL registry-answered: last check got no registry answer for 3 dep(s) (PyPI 1, npm 2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := []doctor.Check{registryAnswered(tt.dir(t))}

			report := doctor.Collect(context.Background(), checks)
			if res := report.Checks[0]; res.Status != tt.wantStatus {
				t.Errorf("report status = %q (%s), want %q", res.Status, res.Detail, tt.wantStatus)
			}
			if wantOK := tt.wantStatus != doctor.StatusFail; report.OK != wantOK {
				t.Errorf("report ok = %v, want %v", report.OK, wantOK)
			}

			var out bytes.Buffer
			err := doctor.Run(context.Background(), &out, deps.Facts(), checks)
			if wantErr := tt.wantStatus == doctor.StatusFail; (err != nil) != wantErr {
				t.Errorf("run error = %v, want error %v", err, wantErr)
			}
			line, _, _ := strings.Cut(out.String(), "\n")
			if !strings.HasPrefix(line, tt.wantLine) {
				t.Errorf("printed line = %q, want prefix %q", line, tt.wantLine)
			}
		})
	}
}

// TestDoctorChecksIncludeRegistryAnswered: `deps doctor` and the deps_doctor
// MCP tool both run doctorChecks, so the check must be in that one list.
func TestDoctorChecksIncludeRegistryAnswered(t *testing.T) {
	for _, c := range doctorChecks(context.Background()) {
		if c.Name == "registry-answered" {
			return
		}
	}
	t.Error("doctorChecks has no registry-answered check")
}
