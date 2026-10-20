package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

func TestDiscoveryCleanCheck(t *testing.T) {
	run := func(dir string) doctor.Result {
		t.Helper()
		return doctor.Collect(context.Background(), []doctor.Check{discoveryClean(dir)}).Checks[0]
	}
	saved := func(errs []store.DiscoveryError) string {
		t.Helper()
		dir := t.TempDir()
		st, err := store.New(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Save(nil, errs); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	if got := run(t.TempDir()); got.Status != doctor.StatusSkipped {
		t.Errorf("no scan yet: status = %q, want skipped", got.Status)
	}
	if got := run(saved(nil)); got.Status != doctor.StatusOK {
		t.Errorf("clean scan: status = %q (%s), want ok", got.Status, got.Detail)
	}

	got := run(saved([]store.DiscoveryError{
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
