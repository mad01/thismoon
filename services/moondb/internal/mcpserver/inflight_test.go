package mcpserver

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
)

// The fixture is weeks old, so since reaches back to it.
const fixtureSince = "2026-10-01T09:00:00Z"

func TestInflightRoundTrip(t *testing.T) {
	cs := session(t, noChecks)
	var out inflightOutput
	call(t, cs, "moondb_inflight", map[string]any{
		"worktree": servertest.Worktree, "exclude_session": servertest.OldSession,
		"since": fixtureSince,
	}, &out)
	if len(out.Files) != 2 || out.Files[0].Path != servertest.PathA {
		t.Fatalf("files = %+v, want %s first of two", out.Files, servertest.PathA)
	}
	if !strings.HasPrefix(out.Summary, "2 files in "+servertest.Worktree) ||
		!strings.Contains(out.Summary, `"build the server" (`+servertest.NewSession+`)`) {
		t.Errorf("summary = %q, want the count and the newest edit's session", out.Summary)
	}
}

func TestInflightEmptyListsNoFiles(t *testing.T) {
	cs := session(t, noChecks)
	var out inflightOutput
	call(t, cs, "moondb_inflight", map[string]any{
		"worktree": servertest.Worktree, "exclude_session": servertest.NewSession,
		"since": fixtureSince,
	}, &out)
	if out.Files == nil || len(out.Files) != 0 || !strings.HasPrefix(out.Summary, "no files in") {
		t.Errorf("out = %+v, want an empty list and a none summary", out)
	}
}
