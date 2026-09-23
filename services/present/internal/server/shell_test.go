package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// The page shell is one file; shared mode rewrites the parts that only make
// sense on a machine with local .this sites and an index: the ⌘K control and
// the "← All" back link. Pin the markers the rewrite keys on as well, so an
// edit to shell.html cannot turn it into a silent no-op.
func TestPageShellPerMode(t *testing.T) {
	local := string(pageShell(ModeLocal))
	if !strings.Contains(local, `controls="cmdk,`) ||
		!strings.Contains(local, `back-label="← All"`) {
		t.Fatal("shell.html no longer carries the markers the shared rewrite keys on")
	}

	shared := string(pageShell(ModeShared))
	if strings.Contains(shared, "cmdk") {
		t.Error("shared shell still lists the cmdk control")
	}
	if !strings.Contains(shared, `controls="font,`) {
		t.Error("shared shell lost the controls that follow cmdk")
	}
	if !strings.Contains(shared, `back-label="← About"`) {
		t.Error("shared shell keeps the local back label")
	}
}

func TestSharedChromeHasNoSitePicker(t *testing.T) {
	f := setupShared(t)
	p, err := f.raw.Create(t.Context(), store.Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/p/" + p.ID} {
		code, body := get(t, f.ts.URL+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, code)
		}
		if strings.Contains(body, "cmdk") {
			t.Errorf("GET %s on a shared instance serves the cmdk control", path)
		}
	}

	ts, st := setup(t)
	local := createLocal(t, st, "Local")
	if _, body := get(t, ts.URL+"/p/"+local.ID); !strings.Contains(body, "cmdk") {
		t.Error("local page shell lost the cmdk control")
	}
}
