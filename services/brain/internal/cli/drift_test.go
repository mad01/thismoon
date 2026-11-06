package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/drift"
)

// TestDriftOnlineGuard pins that --online refuses a source on a local
// address unless --allow-local, and that --timeout and --allow-local need
// --online. The source is an httptest server, so nothing leaves the host.
func TestDriftOnlineGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("local service"))
	}))
	defer srv.Close()
	root := setupCLI(t, "")
	writeNote(t, root, "notes/local.md",
		"---\ntitle: Local\nsources:\n  - url: "+srv.URL+"/admin\n---\nA local source.\n")

	refused := runBrain(t, "", "drift", "notes/local.md", "--online", "-o", "json")
	r := decodeStrict[drift.Report](t, refused.stdout)
	if refused.code != exitOK || len(r.Notes) != 1 ||
		r.Notes[0].Findings[0].Kind != drift.KindUnresolved ||
		!strings.Contains(r.Notes[0].Findings[0].Message, "--allow-local") {
		t.Errorf("--online: exit %d, %+v", refused.code, r)
	}
	allowed := runBrain(t, "", "drift", "notes/local.md", "--online", "--allow-local",
		"--timeout", "30s", "-o", "json")
	if r := decodeStrict[drift.Report](t, allowed.stdout); allowed.code != exitOK ||
		len(r.Notes) != 0 {
		t.Errorf("--allow-local: exit %d, %+v", allowed.code, r)
	}

	for _, args := range [][]string{
		{"drift", "--timeout", "30s"},
		{"drift", "--allow-local"},
		{"drift", "--online", "--timeout", "0s"},
		{"stale", "--drift", "--allow-local"},
	} {
		if got := runBrain(t, "", args...); got.code != exitUsage {
			t.Errorf("%v: exit %d, want %d; stderr %q", args, got.code, exitUsage, got.stderr)
		}
	}
	// A 1ns budget runs out before the first fetch, so the fixture's
	// example.com source is never fetched.
	stale := runBrain(t, "", "stale", "--drift", "--online", "--allow-local", "--timeout", "1ns")
	if stale.code != exitOK {
		t.Errorf("stale with the online flags: exit %d, stderr %q", stale.code, stale.stderr)
	}
}
