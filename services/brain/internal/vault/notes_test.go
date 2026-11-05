package vault

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// newTestVault creates a vault with the given vault-relative files.
func newTestVault(t *testing.T, files map[string]string) *Vault {
	t.Helper()
	root := t.TempDir()
	for _, tier := range AllTiers {
		if err := os.MkdirAll(filepath.Join(root, tier.Dir()), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

const provenanceNote = `---
owner: alex
title: Capture
classification: internal
sources:
  - url: https://example.com/a
    retrieved: 2026-03-01
status: raw
---
Body links [[other]].
`

func mustField(t *testing.T, key, yamlValue string) markdown.Field {
	t.Helper()
	node, err := markdown.ParseValue(yamlValue)
	if err != nil {
		t.Fatal(err)
	}
	return markdown.Field{Key: key, Value: node}
}

func TestEditAndPromoteKeepUnknownKeys(t *testing.T) {
	wantSources := []any{map[string]any{"url": "https://example.com/a", "retrieved": "2026-03-01"}}
	now := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		edit      func(t *testing.T) EditOptions
		wantExtra map[string]any
		wantEvid  string
	}{
		{
			name: "body edit",
			edit: func(*testing.T) EditOptions { return EditOptions{Append: "more", Now: now} },
			wantExtra: map[string]any{
				"owner": "alex", "classification": "internal", "sources": wantSources,
			},
		},
		{
			name: "frontmatter patch",
			edit: func(*testing.T) EditOptions {
				return EditOptions{FrontmatterPatch: &markdown.Frontmatter{Evidence: "B"}, Now: now}
			},
			wantExtra: map[string]any{
				"owner": "alex", "classification": "internal", "sources": wantSources,
			},
			wantEvid: "B",
		},
		{
			name: "set and unset",
			edit: func(t *testing.T) EditOptions {
				return EditOptions{
					Set: []markdown.Field{
						mustField(t, "owner", "sam"),
						mustField(t, "review", "{by: kim}"),
					},
					Unset: []string{"classification"},
					Now:   now,
				}
			},
			wantExtra: map[string]any{
				"owner": "sam", "sources": wantSources, "review": map[string]any{"by": "kim"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"inbox/capture.md": provenanceNote})
			src := filepath.Join(v.Root, "inbox", "capture.md")
			if _, err := v.Edit(src, tc.edit(t)); err != nil {
				t.Fatalf("Edit: %v", err)
			}
			res, err := v.Promote(src, PromoteOptions{Now: now})
			if err != nil {
				t.Fatalf("Promote: %v", err)
			}
			fm, _, err := v.ReadFrontmatter(res.NewPath)
			if err != nil {
				t.Fatal(err)
			}
			extra, err := fm.ExtraValues()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(extra, tc.wantExtra) {
				t.Errorf("extra = %#v, want %#v", extra, tc.wantExtra)
			}
			if fm.Status != "validated" || fm.Title != "Capture" || fm.Updated != "2026-04-02" {
				t.Errorf("typed fields = %+v", fm)
			}
			if fm.Evidence != tc.wantEvid {
				t.Errorf("evidence = %q, want %q", fm.Evidence, tc.wantEvid)
			}
		})
	}
}

func TestWriteSetUnset(t *testing.T) {
	v := newTestVault(t, nil)
	path, err := v.Write(WriteOptions{
		Tier:  TierOutputs,
		Title: "Report",
		Set: []markdown.Field{
			mustField(t, "tags", "[x]"),
			mustField(t, "sources", "[{url: https://x}]"),
			mustField(t, "status", "draft"),
		},
		Unset: []string{"created"},
		Now:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	fm, _, err := v.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Status != "draft" || fm.Created != "" || fm.Updated != "2026-04-02" {
		t.Errorf("typed fields = %+v", fm)
	}
	if !reflect.DeepEqual(fm.Tags, []string{"meta/output", "x"}) {
		t.Errorf("tags = %v, want meta/output kept first", fm.Tags)
	}
	extra, _ := fm.ExtraValues()
	if _, ok := extra["sources"]; !ok {
		t.Errorf("sources missing: %v", extra)
	}
}

func TestSentinelErrors(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"inbox/a.md": "---\ntitle: A\n---\n",
		"notes/a.md": "---\ntitle: A\n---\n",
	})
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			"read missing",
			func() error { _, err := v.Read("notes/nope.md", 0, 0); return err },
			ErrNotFound,
		},
		{"peek missing", func() error { _, err := v.Peek("nope.md", 0); return err }, ErrNotFound},
		{
			"backlinks missing",
			func() error { _, err := v.Backlinks("notes/nope.md"); return err },
			ErrNotFound,
		},
		{
			"title missing",
			func() error { _, err := v.FindByTitle("Nope"); return err },
			ErrNotFound,
		},
		{
			"edit missing",
			func() error {
				_, err := v.Edit(filepath.Join(v.Root, "notes/nope.md"), EditOptions{Body: "x"})
				return err
			},
			ErrNotFound,
		},
		{
			"promote missing",
			func() error { _, err := v.Promote(filepath.Join(v.Root, "nope.md"), PromoteOptions{}); return err },
			ErrNotFound,
		},
		{
			"write existing",
			func() error { _, err := v.Write(WriteOptions{Tier: TierNotes, Title: "A"}); return err },
			ErrExists,
		},
		{
			"promote onto existing",
			func() error { _, err := v.Promote(filepath.Join(v.Root, "inbox/a.md"), PromoteOptions{}); return err },
			ErrExists,
		},
		{"escape", func() error { _, err := v.Resolve("../x.md"); return err }, ErrInvalidPath},
		{
			"absolute",
			func() error { _, err := v.Resolve("/etc/passwd"); return err },
			ErrInvalidPath,
		},
		{"empty path", func() error { _, err := v.Resolve(""); return err }, ErrInvalidPath},
		{"unknown tier", func() error { _, err := ParseTier("bogus"); return err }, ErrUnknownTier},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestReadPeekBacklinks(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"notes/alpha.md": "---\ntitle: Alpha\naliases: [First]\nevidence: A\nowner: alex\n---\nline 1\nline 2\nline 3\n",
		"notes/beta.md":  "---\ntitle: Beta\nevidence: C\n---\nSee [[alpha]] and [[First|the first]].\n[[missing]]\n",
		"inbox/gamma.md": "---\ntitle: Gamma\n---\nAlso [[alpha#Heading]].\n",
	})

	note, err := v.Read("notes/alpha.md", 2, 3)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(note.BodyLines) != 2 || note.BodyLines[0] != (BodyLine{Line: 2, Text: "line 2"}) {
		t.Errorf("body lines = %+v", note.BodyLines)
	}
	if note.Frontmatter.Evidence != "A" || note.Frontmatter.Extra["owner"] != "alex" {
		t.Errorf("frontmatter = %+v", note.Frontmatter)
	}

	peek, err := v.Peek("notes/alpha.md", 1)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if !reflect.DeepEqual(peek.Preview, []string{"line 1"}) || peek.BacklinkCount != 2 {
		t.Errorf("peek = %+v, want 1 preview line and 2 backlinks", peek)
	}

	links, err := v.Backlinks("notes/alpha.md")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if links.NotePath != "notes/alpha.md" || links.Evidence != "A" {
		t.Errorf("note = %q evidence = %q", links.NotePath, links.Evidence)
	}
	inbound := map[string]string{}
	for _, in := range links.Inbound {
		inbound[in.Path] = in.Evidence
	}
	if !reflect.DeepEqual(inbound, map[string]string{"notes/beta.md": "C", "inbox/gamma.md": ""}) {
		t.Errorf("inbound = %+v", links.Inbound)
	}

	beta, err := v.Backlinks("notes/beta.md")
	if err != nil {
		t.Fatalf("Backlinks beta: %v", err)
	}
	// Lines count from the top of the file; an alias resolves like a stem.
	want := []Outbound{
		{Target: "alpha", Path: "notes/alpha.md", Line: 5, Evidence: "A"},
		{Target: "First", Path: "notes/alpha.md", Line: 5, Evidence: "A"},
		{Target: "missing", Line: 6, Dangling: true},
	}
	if !reflect.DeepEqual(beta.Outbound, want) {
		t.Errorf("outbound = %+v, want %+v", beta.Outbound, want)
	}

	rel, err := v.FindByTitle("first")
	if err != nil || rel != "notes/alpha.md" {
		t.Errorf("FindByTitle(first) = %q, %v", rel, err)
	}
}

func TestList(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"notes/old.md":   "---\ntitle: Old\ntags: [k8s]\nupdated: 2026-01-01\n---\n",
		"notes/new.md":   "---\ntitle: New\ntags: [k8s]\nupdated: 2026-03-01\n---\n",
		"notes/other.md": "---\ntitle: Other\nupdated: 2026-02-01\n---\n",
	})
	listing, err := v.List(ListOptions{Tier: TierNotes, Tag: "k8s", Limit: 1})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if listing.Tier != "notes" || listing.Total != 2 || len(listing.Notes) != 1 ||
		listing.Notes[0].Path != "notes/new.md" {
		t.Errorf("listing = %+v", listing)
	}
}
