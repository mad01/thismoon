package vault

import (
	"path/filepath"
	"slices"
	"testing"
)

// recorder is a Committer that keeps every commit it is asked for.
type recorder struct {
	messages []string
	paths    [][]string
}

func (r *recorder) Commit(message string, paths []string) error {
	r.messages = append(r.messages, message)
	r.paths = append(r.paths, slices.Clone(paths))
	return nil
}

func (r *recorder) last(t *testing.T) (string, []string) {
	t.Helper()
	if len(r.messages) == 0 {
		t.Fatal("no commit")
	}
	return r.messages[len(r.messages)-1], r.paths[len(r.paths)-1]
}

func TestEveryChangeCommitsItsPaths(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"inbox/old.md":   "---\ntitle: Old\nstatus: raw\nexpires_at: 2026-10-01\n---\nold\n",
		"inbox/topic.md": "---\ntitle: Topic\nstatus: raw\n---\nbody\n",
		"notes/link.md":  "---\ntitle: Link\n---\nsee [[topic]]\n",
	})
	rec := &recorder{}
	v.Committer = rec

	path, err := v.Write(WriteOptions{Tier: TierNotes, Title: "Fresh", Now: revokeDay})
	if err != nil {
		t.Fatal(err)
	}
	if msg, paths := rec.last(t); msg != "brain: write notes/fresh.md" ||
		!slices.Equal(paths, []string{"notes/fresh.md"}) {
		t.Errorf("write committed %q %v", msg, paths)
	}
	if _, err := v.Edit(path, EditOptions{Append: "more", Now: revokeDay}); err != nil {
		t.Fatal(err)
	}
	if msg, _ := rec.last(t); msg != "brain: edit notes/fresh.md" {
		t.Errorf("edit committed %q", msg)
	}

	if _, err := v.Promote(filepath.Join(v.Root, "inbox", "topic.md"),
		PromoteOptions{NewTitle: "Topic Two", Now: revokeDay}); err != nil {
		t.Fatal(err)
	}
	msg, paths := rec.last(t)
	want := []string{"notes/topic-two.md", "inbox/topic.md", "notes/link.md"}
	if msg != "brain: promote notes/topic-two.md" || !slices.Equal(paths, want) {
		t.Errorf("promote committed %q %v, want %v", msg, paths, want)
	}

	if _, err := v.ArchiveExpired(revokeDay, false); err != nil {
		t.Fatal(err)
	}
	msg, paths = rec.last(t)
	if msg != "brain: archive inbox/old.md" ||
		!slices.Equal(paths, []string{"inbox/old.md", ".brain/archive/inbox/old.md"}) {
		t.Errorf("archive committed %q %v", msg, paths)
	}

	n := len(rec.messages)
	if _, err := v.ArchiveExpired(revokeDay, true); err != nil {
		t.Fatal(err)
	}
	if len(rec.messages) != n {
		t.Errorf("a dry run committed %q", rec.messages[n:])
	}
}
