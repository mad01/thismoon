package ingest

import (
	"reflect"
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

var (
	cwdLoc   = store.Location{Repo: repo, Worktree: worktree, Canonical: worktree, Branch: "main"}
	laterLoc = store.Location{Repo: "github.com/o/other", Worktree: worktree, Branch: "feat"}
)

// origin sets what a hook or an import says about where and how the
// session runs.
func origin(
	env spool.Envelope,
	loc store.Location,
	entrypoint string,
	tags ...string,
) spool.Envelope {
	env.Session.Location, env.Session.Entrypoint, env.Session.Tags = loc, entrypoint, tags
	return env
}

// TestSessionOriginFirstWins: the location and the entrypoint keep the first
// value an envelope brings, an envelope without them changes nothing, and
// the tags are the union of every envelope's.
func TestSessionOriginFirstWins(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(
		origin(
			editEnv(c, "s1", "p1", "tu1", 1, "a.go", "a\n", "a\n"),
			store.Location{},
			"cli",
			"eval",
		),
		c,
	)
	f.write(origin(openEnv(c, "s1", "p2", 2, "next"), cwdLoc, "sdk-cli", "ci", "eval"), c)
	f.write(origin(openEnv(c, "s1", "p3", 3, "and next"), laterLoc, "", "Bad"), c)
	f.write(openEnv(c, "s1", "p4", 4, "plain"), c)
	f.drain()

	s := f.session("s1")
	if s.Location != cwdLoc || s.Entrypoint != "cli" ||
		!slices.Equal(s.Tags, []string{"ci", "eval"}) {
		t.Errorf("session location %+v entrypoint %q tags %v; want %+v, the first %q and the union",
			s.Location, s.Entrypoint, s.Tags, cwdLoc, "cli")
	}
}

// TestSessionLocationFollowsCwd: a location counts only for the cwd the
// session keeps, so a later envelope from another directory leaves it
// empty.
func TestSessionLocationFollowsCwd(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(openEnv(c, "s1", "p1", 1, "start outside a repo"), c)
	elsewhere := origin(openEnv(c, "s1", "p2", 2, "moved"), laterLoc, "")
	elsewhere.Session.Cwd = "/wt/b"
	f.write(elsewhere, c)
	f.drain()

	if s := f.session("s1"); s.Cwd != worktree || s.Location != (store.Location{}) {
		t.Errorf(
			"session cwd %q location %+v; want %q with no location",
			s.Cwd,
			s.Location,
			worktree,
		)
	}
}

// TestBackfillFillsSessionOrigin: an import fills the location and the
// entrypoint a live session recorded before it kept them, and keeps its
// tags.
func TestBackfillFillsSessionOrigin(t *testing.T) {
	f := newFixture(t)
	c := chunkSet{}
	f.write(origin(openEnv(c, "s1", "p1", 1, "live"), store.Location{}, "", "eval"), c)
	f.drain()

	imported := origin(openEnv(c, "s1", "p1", 1, "live"), cwdLoc, "sdk-cli")
	imported.Backfill = true
	f.write(imported, c)
	f.drain()

	s := f.session("s1")
	want := store.Session{
		ID: "s1", Harness: "claude-code", Cwd: worktree, Location: cwdLoc, Entrypoint: "sdk-cli",
		Tags: []string{"eval"}, Title: "title s1", TitleSource: store.TitleGenerated,
		PromptLine: "live", StartedAt: at(1),
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("session = %+v\nwant %+v", s, want)
	}
}
