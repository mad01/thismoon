package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Hello World", "hello-world"},
		{"  Trim-Me  ", "trim-me"},
		{"With Punctuation!! And...stuff", "with-punctuation-and-stuff"},
		{"CamelCase Foo", "camelcase-foo"},
		{"", "untitled"},
		{"-----", "untitled"},
		{"  --bob--  ", "bob"},
	}
	for _, tc := range tests {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseTier(t *testing.T) {
	good := map[string]Tier{
		"inbox": TierInbox, "INBOX": TierInbox, "  notes ": TierNotes,
		"outputs": TierOutputs, "output": TierOutputs,
		"MOCs": TierMOCs, "moc": TierMOCs,
	}
	for in, want := range good {
		got, err := ParseTier(in)
		if err != nil {
			t.Errorf("ParseTier(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseTier(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseTier("nope"); err == nil {
		t.Error("ParseTier(\"nope\") should error")
	}
}

func TestTierOf(t *testing.T) {
	dir := t.TempDir()
	for _, tier := range AllTiers {
		if err := os.MkdirAll(filepath.Join(dir, tier.Dir()), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := TierOf(dir, filepath.Join(dir, "inbox", "foo.md"))
	if err != nil || got != TierInbox {
		t.Errorf("TierOf inbox = (%v, %v)", got, err)
	}
	got, err = TierOf(dir, filepath.Join(dir, "notes", "sub", "foo.md"))
	if err != nil || got != TierNotes {
		t.Errorf("TierOf notes = (%v, %v)", got, err)
	}
	if _, err := TierOf(dir, "/elsewhere/foo.md"); err == nil {
		t.Error("TierOf outside vault should error")
	}
}

func fixtureVault(t *testing.T) *Vault {
	t.Helper()
	root := t.TempDir()
	for _, tier := range AllTiers {
		if err := os.MkdirAll(filepath.Join(root, tier.Dir()), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestWrite(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	path, err := v.Write(WriteOptions{
		Tier:  TierInbox,
		Title: "My Capture",
		Body:  "This is a capture.",
		Tags:  []string{"research/ai"},
		Now:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "inbox/my-capture.md") {
		t.Errorf("unexpected path: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"title: My Capture", "status: raw", "type: note", "2026-04-14", "This is a capture."} {
		if !strings.Contains(content, want) {
			t.Errorf("content missing %q:\n%s", want, content)
		}
	}

	// Collision should error.
	if _, err := v.Write(WriteOptions{Tier: TierInbox, Title: "My Capture", Body: "x", Now: now}); err == nil {
		t.Error("want collision error")
	}
}

func TestEdit(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	path, err := v.Write(
		WriteOptions{Tier: TierInbox, Title: "Edit Me", Body: "original\n", Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(24 * time.Hour)
	if _, err := v.Edit(path, EditOptions{Append: "appended", Now: later}); err != nil {
		t.Fatal(err)
	}
	fm, body, err := v.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "original") || !strings.Contains(string(body), "appended") {
		t.Errorf("body after append: %q", body)
	}
	if fm.Updated != "2026-04-15" {
		t.Errorf("Updated = %q, want 2026-04-15", fm.Updated)
	}
}

func TestPromote(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	// Seed: an inbox note and a second file that references it via wikilink.
	inboxPath, err := v.Write(WriteOptions{
		Tier: TierInbox, Title: "Raw Thought", Body: "some idea", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	refPath, err := v.Write(WriteOptions{
		Tier:  TierNotes,
		Title: "Reference",
		Body:  "See [[raw-thought]] and [[raw-thought#heading|aliased]].\n",
		Now:   now,
	})
	if err != nil {
		t.Fatal(err)
	}

	mocPath := filepath.Join(v.Root, "MOCs", "topics.md")
	res, err := v.Promote(inboxPath, PromoteOptions{
		MOC: mocPath,
		Now: now,
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !strings.HasSuffix(res.NewPath, "notes/raw-thought.md") {
		t.Errorf("NewPath = %s", res.NewPath)
	}
	if _, err := os.Stat(inboxPath); !os.IsNotExist(err) {
		t.Error("old inbox path should be gone")
	}
	fm, _, err := v.ReadFrontmatter(res.NewPath)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Status != "validated" {
		t.Errorf("Status = %q, want validated", fm.Status)
	}

	// Backlinks — the title hasn't changed so the stem "raw-thought" stays.
	// Verify the reference file still has its wikilinks intact (and wasn't
	// rewritten since new stem == old stem).
	refData, _ := os.ReadFile(refPath)
	if !strings.Contains(string(refData), "[[raw-thought]]") {
		t.Errorf("reference lost wikilink:\n%s", refData)
	}

	// MOC appended.
	mocData, err := os.ReadFile(mocPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mocData), "[[raw-thought|Raw Thought]]") {
		t.Errorf("MOC not appended:\n%s", mocData)
	}
}

func TestWriteAutoTagsOutputs(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	path, err := v.Write(WriteOptions{
		Tier:  TierOutputs,
		Title: "Synthesis",
		Body:  "x",
		Tags:  []string{"topic/x"},
		Now:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := v.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fm.HasTag("meta/output") {
		t.Errorf("output missing meta/output: %v", fm.Tags)
	}
	// Idempotent: a caller that already passes meta/output should not get a duplicate.
	path2, err := v.Write(WriteOptions{
		Tier:  TierOutputs,
		Title: "Already Tagged",
		Body:  "x",
		Tags:  []string{"meta/output", "topic/y"},
		Now:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	fm2, _, _ := v.ReadFrontmatter(path2)
	count := 0
	for _, tag := range fm2.Tags {
		if tag == "meta/output" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("meta/output appears %d times, want 1: %v", count, fm2.Tags)
	}
	// Non-output tiers must NOT get the tag.
	notePath, _ := v.Write(WriteOptions{Tier: TierNotes, Title: "Note", Body: "x", Now: now})
	noteFM, _, _ := v.ReadFrontmatter(notePath)
	if noteFM.HasTag("meta/output") {
		t.Errorf("non-output got meta/output: %v", noteFM.Tags)
	}
}

func TestPromoteRejectsBatchIndexMOCByName(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	inboxPath, err := v.Write(
		WriteOptions{Tier: TierInbox, Title: "Topic Note", Body: "x", Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"research-index-2026-134-reference-topics.md", "kickoff-q2-2026.md"} {
		mocPath := filepath.Join(v.Root, "MOCs", name)
		_, err := v.Promote(inboxPath, PromoteOptions{MOC: mocPath, Now: now})
		if err == nil {
			t.Fatalf("Promote with batch-index moc %q should error", name)
		}
		if !strings.Contains(err.Error(), "batch-provenance index") {
			t.Errorf("error for %q lacks expected message: %v", name, err)
		}
		// Source must remain in inbox so the user can retry with a topical MOC.
		if _, statErr := os.Stat(inboxPath); os.IsNotExist(statErr) {
			t.Fatalf("Promote moved the inbox note despite rejecting batch-index moc %q", name)
		}
	}
}

func TestPromoteRejectsBatchIndexMOCByTag(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	inboxPath, err := v.Write(
		WriteOptions{Tier: TierInbox, Title: "Topic Note", Body: "x", Now: now},
	)
	if err != nil {
		t.Fatal(err)
	}
	// A MOC whose name doesn't match the pattern but is tagged meta/index.
	mocPath := filepath.Join(v.Root, "MOCs", "topics-roster.md")
	if err := os.WriteFile(mocPath, []byte("---\ntitle: Roster\ntags:\n  - meta/index\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Promote(inboxPath, PromoteOptions{MOC: mocPath, Now: now}); err == nil {
		t.Fatal("Promote with meta/index-tagged moc should error")
	}
	if _, statErr := os.Stat(inboxPath); os.IsNotExist(statErr) {
		t.Fatal("Promote moved the inbox note despite rejecting meta/index-tagged moc")
	}
}

func TestPromoteAcceptsTopicalMOC(t *testing.T) {
	// Regression guard: a normal topical MOC name (and no meta/index tag)
	// must continue to be accepted.
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	inboxPath, _ := v.Write(
		WriteOptions{Tier: TierInbox, Title: "Concurrency Patterns", Body: "x", Now: now},
	)
	mocPath := filepath.Join(v.Root, "MOCs", "golang.md")
	res, err := v.Promote(inboxPath, PromoteOptions{MOC: mocPath, Now: now})
	if err != nil {
		t.Fatalf("Promote with topical moc: %v", err)
	}
	if res.MOCAppended == "" {
		t.Error("expected MOCAppended to be set on success")
	}
}

func TestIsBatchIndexMOC(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		path    string
		content string
		want    bool
	}{
		{"name research-index", filepath.Join(dir, "research-index-foo.md"), "", true},
		{"name kickoff", filepath.Join(dir, "kickoff-q2.md"), "", true},
		{"name uppercase", filepath.Join(dir, "Research-Index-Bar.md"), "", true},
		{"name plain topical", filepath.Join(dir, "golang.md"), "", false},
		{
			"tag meta/index",
			filepath.Join(dir, "roster.md"),
			"---\ntags:\n  - meta/index\n---\n",
			true,
		},
		{
			"tag unrelated",
			filepath.Join(dir, "topics.md"),
			"---\ntags:\n  - topic/foo\n---\n",
			false,
		},
		{"missing file plain name", filepath.Join(dir, "missing.md"), "", false},
	}
	for _, tc := range cases {
		if tc.content != "" {
			if err := os.WriteFile(tc.path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		got, err := isBatchIndexMOC(tc.path)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPromoteRenames(t *testing.T) {
	v := fixtureVault(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	inboxPath, _ := v.Write(WriteOptions{Tier: TierInbox, Title: "Old Name", Body: "x", Now: now})
	refPath, _ := v.Write(WriteOptions{
		Tier: TierNotes, Title: "Ref",
		Body: "Link: [[old-name]] and [[old-name#sec|x]] and [[old-name|alias]].\n",
		Now:  now,
	})
	res, err := v.Promote(inboxPath, PromoteOptions{NewTitle: "New Name", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.NewPath, "notes/new-name.md") {
		t.Errorf("new path: %s", res.NewPath)
	}
	refData, _ := os.ReadFile(refPath)
	if strings.Contains(string(refData), "old-name") {
		t.Errorf("reference still contains old-name:\n%s", refData)
	}
	if !strings.Contains(string(refData), "[[new-name]]") ||
		!strings.Contains(string(refData), "[[new-name#sec|x]]") ||
		!strings.Contains(string(refData), "[[new-name|alias]]") {
		t.Errorf("wikilinks not fully rewritten:\n%s", refData)
	}
}
