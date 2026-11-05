package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// setupFixtureVault creates a temp vault, isolates HOME and the XDG and brain
// variables so testServer's config is <tmpHome>/.config/brain/config.yaml and
// the indexes land under <tmpHome>/.local/state/brain, and seeds the vault
// with a handful of markdown notes across tiers (with realistic frontmatter
// and [[wikilinks]] between them).
func setupFixtureVault(t *testing.T) string {
	t.Helper()

	vaultRoot := t.TempDir()
	tmpHome := testhome.Isolate(t)

	// Write the brain config so loadVault() resolves the vault to vaultRoot.
	cfgDir := filepath.Join(tmpHome, ".config", "brain")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir cfg: %v", err)
	}
	cfg := "vault: " + vaultRoot + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Tier dirs.
	for _, tier := range []string{"inbox", "notes", "outputs", "MOCs"} {
		if err := os.MkdirAll(filepath.Join(vaultRoot, tier), 0o755); err != nil {
			t.Fatalf("mkdir tier %s: %v", tier, err)
		}
	}

	// Seed notes. Keep frontmatter realistic — title, tags, type, status,
	// created/updated — and wire wikilinks between several of them.
	seeds := map[string]string{
		"inbox/raw-thought.md": `---
title: Raw Thought
tags:
  - ai/research
type: note
status: raw
created: 2026-04-10
updated: 2026-04-10
---
Quick capture.
See [[concept-alpha]] for more.
Another line about kubernetes.
`,
		"notes/concept-alpha.md": `---
title: Concept Alpha
aliases:
  - Alpha Concept
tags:
  - ai/research
type: note
status: validated
created: 2026-04-01
updated: 2026-04-12
---
Alpha is the primary concept in this research area.
It builds on foundational ideas from the 2020s.
Several follow-up notes extend it.
`,
		"notes/concept-beta.md": `---
title: Concept Beta
tags:
  - ai/research
type: note
status: validated
created: 2026-04-02
updated: 2026-04-13
---
Beta extends [[concept-alpha]] with a novel twist.
Related to concept-alpha and its aliases.
`,
		"outputs/2026-04-14-analysis.md": `---
title: Analysis 2026-04-14
tags:
  - ai/research
type: output
status: validated
created: 2026-04-14
updated: 2026-04-14
---
Synthesizes [[concept-alpha]] and [[concept-beta]] into an output.
`,
		"MOCs/ai-topics.md": `---
title: AI Topics
tags:
  - moc
type: moc
status: validated
created: 2026-04-01
updated: 2026-04-14
---
- [[concept-alpha]]
- [[concept-beta]]
`,
		// Evidence-tagged notes: one via frontmatter field, one via
		// evidence/* tag. Both should surface in tool outputs identically.
		"notes/graded-field.md": `---
title: Graded By Field
tags:
  - ai/research
type: note
status: validated
evidence: A
created: 2026-04-10
updated: 2026-04-13
---
A study with effect size g=0.42. See [[concept-alpha]].
`,
		"notes/graded-tag.md": `---
title: Graded By Tag
tags:
  - ai/research
  - evidence/D
type: note
status: validated
created: 2026-04-11
updated: 2026-04-13
---
Refuted claim; kept as do-not-cite record.
`,
	}

	for rel, content := range seeds {
		full := filepath.Join(vaultRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write seed %s: %v", rel, err)
		}
	}

	return vaultRoot
}

// testServer returns handlers loading the default config path, the one
// setupFixtureVault writes.
func testServer(t *testing.T) *handlers {
	t.Helper()
	path, err := brain.DefaultConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	return &handlers{configPath: path}
}

// ---------------------------------------------------------------------------
// brain_search
// ---------------------------------------------------------------------------

func TestBrainSearch_FilesMode(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleSearch(ctx, nil, searchInput{
		Query:      "concept",
		OutputMode: "files_with_matches",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	if out.OutputMode != "files_with_matches" {
		t.Errorf("OutputMode = %q", out.OutputMode)
	}
	if out.Total == 0 || len(out.Files) == 0 {
		t.Fatalf("expected file results, got %+v", out)
	}
	paths := make(map[string]bool, len(out.Files))
	for _, f := range out.Files {
		paths[f.Path] = true
	}
	// concept-alpha mentions "concept" in its body + tags + title; it must show.
	if !paths["notes/concept-alpha.md"] && !paths["notes/concept-beta.md"] {
		t.Errorf("expected concept notes in files, got %v", paths)
	}
}

func TestBrainSearch_ContentMode(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleSearch(ctx, nil, searchInput{
		Query:      "kubernetes",
		OutputMode: "content",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	if out.OutputMode != "content" {
		t.Errorf("OutputMode = %q", out.OutputMode)
	}
	if len(out.Lines) == 0 {
		t.Fatalf("expected line matches, got %+v", out)
	}
	found := false
	for _, l := range out.Lines {
		if l.Line > 0 && strings.Contains(strings.ToLower(l.Text), "kubernetes") {
			found = true
		}
	}
	if !found {
		t.Errorf("no matching line found; lines=%+v", out.Lines)
	}
}

func TestBrainSearch_TierFilter(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleSearch(ctx, nil, searchInput{
		Query:      "concept",
		Tier:       "inbox",
		OutputMode: "files_with_matches",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	for _, f := range out.Files {
		if !strings.HasPrefix(f.Path, "inbox/") {
			t.Errorf("tier=inbox but got path outside inbox: %s", f.Path)
		}
	}
}

func TestBrainSearch_TagFilter(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	// Search for a term that appears across many notes, then filter by tag.
	// Only notes whose frontmatter tags contain ai/research should remain.
	_, out, err := testServer(t).handleSearch(ctx, nil, searchInput{
		Query:      "concept",
		Tag:        "ai/research",
		OutputMode: "files_with_matches",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	if len(out.Files) == 0 {
		t.Fatalf("expected ai/research files")
	}
	for _, f := range out.Files {
		// MOCs/ai-topics.md has tag=moc only; it should not appear.
		if f.Path == "MOCs/ai-topics.md" {
			t.Errorf("tag filter didn't exclude MOC (tag=moc): %s", f.Path)
		}
	}
}

// ---------------------------------------------------------------------------
// brain_count
// ---------------------------------------------------------------------------

func TestBrainCount_GroupByTier(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleCount(ctx, nil, countInput{
		Query:   "concept",
		GroupBy: "tier",
	})
	if err != nil {
		t.Fatalf("handleCount: %v", err)
	}
	if out.Total == 0 {
		t.Fatalf("expected non-zero total")
	}
	if len(out.Groups) == 0 {
		t.Fatalf("expected groups, got %+v", out)
	}
	tiers := make(map[string]int)
	for _, g := range out.Groups {
		tiers[g.Group] = g.Count
	}
	if _, ok := tiers["notes"]; !ok {
		t.Errorf("expected 'notes' tier in groups, got %v", tiers)
	}
}

// ---------------------------------------------------------------------------
// brain_query_validate
// ---------------------------------------------------------------------------

func TestBrainQueryValidate(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	// Valid.
	_, ok, err := testServer(
		t,
	).handleQueryValidate(ctx, nil, queryValidateInput{Query: "hello world"})
	if err != nil {
		t.Fatalf("handleQueryValidate: %v", err)
	}
	if !ok.Valid {
		t.Errorf("expected Valid=true, got %+v", ok)
	}

	// Invalid regex — an unterminated character class fails the parser.
	_, bad, err := testServer(t).handleQueryValidate(ctx, nil, queryValidateInput{Query: "foo[bar"})
	if err != nil {
		t.Fatalf("handleQueryValidate invalid: %v", err)
	}
	if bad.Valid {
		t.Errorf("expected Valid=false for bad regex, got %+v", bad)
	}
	if bad.Error == "" {
		t.Errorf("expected error message, got %+v", bad)
	}
	if bad.Hint == "" {
		t.Errorf("expected hint for bad regex, got %+v", bad)
	}
}

// ---------------------------------------------------------------------------
// brain_read
// ---------------------------------------------------------------------------

func TestBrainRead_Full(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleRead(ctx, nil, readInput{Path: "notes/concept-alpha.md"})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if out.Frontmatter.Title != "Concept Alpha" {
		t.Errorf("Title = %q", out.Frontmatter.Title)
	}
	if len(out.BodyLines) < 3 {
		t.Errorf("expected at least 3 body lines, got %d", len(out.BodyLines))
	}
	if out.BodyLines[0].Line != 1 {
		t.Errorf("first body line number = %d, want 1", out.BodyLines[0].Line)
	}
}

func TestBrainRead_LineRange(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleRead(ctx, nil, readInput{
		Path:      "notes/concept-alpha.md",
		StartLine: 2,
		EndLine:   3,
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if len(out.BodyLines) != 2 {
		t.Fatalf("expected 2 lines, got %d (%+v)", len(out.BodyLines), out.BodyLines)
	}
	if out.BodyLines[0].Line != 2 || out.BodyLines[1].Line != 3 {
		t.Errorf("line numbers = %d,%d; want 2,3", out.BodyLines[0].Line, out.BodyLines[1].Line)
	}
}

func TestBrainRead_EscapeAttempt(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, _, err := testServer(t).handleRead(ctx, nil, readInput{Path: "../etc/passwd"})
	if err == nil {
		t.Fatal("expected error for path escape attempt")
	}
	if !strings.Contains(err.Error(), "escape") &&
		!strings.Contains(err.Error(), "vault-relative") {
		t.Errorf("error should mention escape/vault-relative, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// brain_peek
// ---------------------------------------------------------------------------

func TestBrainPeek(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handlePeek(ctx, nil, peekInput{
		Path:         "notes/concept-alpha.md",
		MaxBodyLines: 10,
	})
	if err != nil {
		t.Fatalf("handlePeek: %v", err)
	}
	if out.Frontmatter.Title != "Concept Alpha" {
		t.Errorf("Title = %q", out.Frontmatter.Title)
	}
	if len(out.Preview) == 0 {
		t.Errorf("expected preview lines")
	}
	// concept-alpha is linked from: inbox/raw-thought.md, notes/concept-beta.md,
	// outputs/2026-04-14-analysis.md, MOCs/ai-topics.md → 4 backlinks.
	if out.BacklinkCount < 3 {
		t.Errorf("BacklinkCount = %d, expected >= 3", out.BacklinkCount)
	}
}

// ---------------------------------------------------------------------------
// brain_list_tier
// ---------------------------------------------------------------------------

func TestBrainListTier(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleListTier(ctx, nil, listInput{Tier: "notes"})
	if err != nil {
		t.Fatalf("handleListTier: %v", err)
	}
	if out.Total != 4 {
		t.Errorf("Total = %d, want 4 (concept-alpha/beta + graded-field/tag)", out.Total)
	}
	// Tag filter should narrow to the four notes sharing ai/research.
	_, tagOut, err := testServer(
		t,
	).handleListTier(ctx, nil, listInput{Tier: "notes", Tag: "ai/research"})
	if err != nil {
		t.Fatalf("handleListTier tag: %v", err)
	}
	if tagOut.Total != 4 {
		t.Errorf("tag-filtered total = %d, want 4", tagOut.Total)
	}
	// Tag filter for a non-matching tag → 0.
	_, emptyOut, err := testServer(
		t,
	).handleListTier(ctx, nil, listInput{Tier: "notes", Tag: "nonexistent"})
	if err != nil {
		t.Fatalf("handleListTier empty: %v", err)
	}
	if emptyOut.Total != 0 {
		t.Errorf("nonexistent tag total = %d, want 0", emptyOut.Total)
	}
}

// ---------------------------------------------------------------------------
// brain_backlinks
// ---------------------------------------------------------------------------

func TestBrainBacklinks_ByPath(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(
		t,
	).handleBacklinks(ctx, nil, backlinksInput{Path: "notes/concept-alpha.md"})
	if err != nil {
		t.Fatalf("handleBacklinks: %v", err)
	}
	if out.NotePath != "notes/concept-alpha.md" {
		t.Errorf("NotePath = %q", out.NotePath)
	}

	inboundPaths := make(map[string]bool)
	for _, ib := range out.Inbound {
		inboundPaths[ib.Path] = true
	}
	for _, want := range []string{"inbox/raw-thought.md", "notes/concept-beta.md", "MOCs/ai-topics.md"} {
		if !inboundPaths[want] {
			t.Errorf("inbound missing %s; got %v", want, inboundPaths)
		}
	}

	// concept-alpha body has no wikilinks → outbound empty.
	if len(out.Outbound) != 0 {
		t.Errorf("expected no outbound from concept-alpha, got %+v", out.Outbound)
	}
}

func TestBrainGraphAroundANote(t *testing.T) {
	setupFixtureVault(t)
	_, g, err := testServer(t).handleGraph(context.Background(), nil, graphInput{
		Center: "notes/concept-beta.md",
	})
	if err != nil {
		t.Fatalf("handleGraph: %v", err)
	}
	// concept-beta links to concept-alpha; the output and the MOC link to it.
	want := map[string]bool{
		"default:notes/concept-beta.md -> default:notes/concept-alpha.md":         true,
		"default:outputs/2026-04-14-analysis.md -> default:notes/concept-beta.md": true,
		"default:MOCs/ai-topics.md -> default:notes/concept-beta.md":              true,
	}
	got := map[string]bool{}
	for _, e := range g.Edges {
		got[e.Source+" -> "+e.Target] = true
	}
	for edge := range want {
		if !got[edge] {
			t.Errorf("edges %v lack %s", got, edge)
		}
	}
	if len(g.Nodes) != 4 {
		t.Errorf("nodes = %+v, want concept-beta and its three neighbours", g.Nodes)
	}
}

func TestBrainBacklinks_ByTitle(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	// Lookup by frontmatter.title (exact, case-insensitive).
	_, out, err := testServer(t).handleBacklinks(ctx, nil, backlinksInput{Title: "Concept Beta"})
	if err != nil {
		t.Fatalf("handleBacklinks title: %v", err)
	}
	if out.NotePath != "notes/concept-beta.md" {
		t.Errorf("NotePath = %q", out.NotePath)
	}
	// concept-beta body links out to concept-alpha.
	foundAlpha := false
	for _, ob := range out.Outbound {
		if strings.EqualFold(ob.Target, "concept-alpha") {
			foundAlpha = true
			if ob.Path != "notes/concept-alpha.md" {
				t.Errorf("outbound concept-alpha path = %q", ob.Path)
			}
		}
	}
	if !foundAlpha {
		t.Errorf("expected outbound to concept-alpha; got %+v", out.Outbound)
	}

	// Lookup by alias.
	_, aliasOut, err := testServer(
		t,
	).handleBacklinks(ctx, nil, backlinksInput{Title: "Alpha Concept"})
	if err != nil {
		t.Fatalf("handleBacklinks alias: %v", err)
	}
	if aliasOut.NotePath != "notes/concept-alpha.md" {
		t.Errorf("alias lookup NotePath = %q", aliasOut.NotePath)
	}
}

// ---------------------------------------------------------------------------
// brain_write
// ---------------------------------------------------------------------------

func TestBrainWrite(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()

	_, out, err := testServer(t).handleWrite(ctx, nil, writeInput{
		Tier:  "inbox",
		Title: "Brand New Thought",
		Body:  "Fresh capture content.",
		Tags:  []string{"capture"},
	})
	if err != nil {
		t.Fatalf("handleWrite: %v", err)
	}
	if out.Path != "inbox/brand-new-thought.md" {
		t.Errorf("Path = %q", out.Path)
	}

	// Collision — writing the same title again must error.
	_, _, err = testServer(t).handleWrite(ctx, nil, writeInput{
		Tier:  "inbox",
		Title: "Brand New Thought",
		Body:  "second",
	})
	if err == nil {
		t.Fatal("expected collision error on duplicate write")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already exists, got %v", err)
	}
}

// TestBrainWriteDistinctFrom pins the CLI's duplicate refusal on brain_write
// and the distinct_from input that lets the write through.
func TestBrainWriteDistinctFrom(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()
	in := writeInput{Tier: "inbox", Title: "concept beta", Body: "a second take"}
	_, _, err := testServer(t).handleWrite(ctx, nil, in)
	if !errors.Is(err, vault.ErrPossibleDuplicate) ||
		!strings.Contains(err.Error(), "notes/concept-beta.md (same title)") {
		t.Fatalf("brain_write of a taken title = %v, want a possible duplicate", err)
	}
	in.DistinctFrom = []string{"notes/concept-beta.md"}
	_, out, err := testServer(t).handleWrite(ctx, nil, in)
	if err != nil || out.Path != "inbox/concept-beta.md" {
		t.Fatalf("brain_write with distinct_from = %+v, %v", out, err)
	}
}

// ---------------------------------------------------------------------------
// brain_edit
// ---------------------------------------------------------------------------

func TestBrainEdit_Append(t *testing.T) {
	vaultRoot := setupFixtureVault(t)
	ctx := context.Background()

	path := "notes/concept-alpha.md"
	full := filepath.Join(vaultRoot, filepath.FromSlash(path))
	before, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	_, editOut, err := testServer(t).handleEdit(ctx, nil, editInput{
		Path:   path,
		Append: "\nA new paragraph appended by test.\n",
	})
	if err != nil {
		t.Fatalf("handleEdit: %v", err)
	}
	if editOut.Path != path {
		t.Errorf("Path = %q", editOut.Path)
	}

	_, readOut, err := testServer(t).handleRead(ctx, nil, readInput{Path: path})
	if err != nil {
		t.Fatalf("handleRead after edit: %v", err)
	}
	var bodyText strings.Builder
	for _, ln := range readOut.BodyLines {
		bodyText.WriteString(ln.Text)
		bodyText.WriteByte('\n')
	}
	if !strings.Contains(bodyText.String(), "A new paragraph appended by test.") {
		t.Errorf("body missing appended text:\n%s", bodyText.String())
	}

	// Updated must advance — seed had updated: 2026-04-12. time.Now().UTC()
	// is guaranteed to be a later date (today), but compare by inequality to
	// be robust.
	if readOut.Frontmatter.Updated == "" {
		t.Error("Updated is empty after edit")
	}
	if readOut.Frontmatter.Updated == "2026-04-12" {
		t.Errorf("Updated did not change from seed (%q)", readOut.Frontmatter.Updated)
	}

	// File bytes should differ from original.
	after, _ := os.ReadFile(full)
	if string(before) == string(after) {
		t.Error("file content unchanged after edit")
	}
}

func TestBrainEdit_FrontmatterPatch(t *testing.T) {
	vaultRoot := setupFixtureVault(t)
	ctx := context.Background()

	path := "inbox/raw-thought.md"
	full := filepath.Join(vaultRoot, filepath.FromSlash(path))
	beforeBytes, _ := os.ReadFile(full)
	beforeBody := extractBody(beforeBytes)

	_, _, err := testServer(t).handleEdit(ctx, nil, editInput{
		Path: path,
		FrontmatterPatch: &frontmatterIn{
			Status: "validated",
			Tags:   []string{"ai/research", "reviewed"},
		},
	})
	if err != nil {
		t.Fatalf("handleEdit patch: %v", err)
	}

	_, readOut, err := testServer(t).handleRead(ctx, nil, readInput{Path: path})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if readOut.Frontmatter.Status != "validated" {
		t.Errorf("Status = %q, want validated", readOut.Frontmatter.Status)
	}
	tagSet := make(map[string]bool)
	for _, tag := range readOut.Frontmatter.Tags {
		tagSet[tag] = true
	}
	if !tagSet["reviewed"] || !tagSet["ai/research"] {
		t.Errorf("tags = %v, want both reviewed and ai/research", readOut.Frontmatter.Tags)
	}

	// Body must be untouched.
	afterBytes, _ := os.ReadFile(full)
	afterBody := extractBody(afterBytes)
	if beforeBody != afterBody {
		t.Errorf(
			"body changed by frontmatter patch:\nbefore: %q\nafter:  %q",
			beforeBody,
			afterBody,
		)
	}
}

// extractBody returns just the body portion of a markdown file with
// frontmatter, so tests can compare before/after bodies without the YAML.
func extractBody(raw []byte) string {
	s := string(raw)
	const fence = "---\n"
	if !strings.HasPrefix(s, fence) {
		return s
	}
	rest := s[len(fence):]
	end := strings.Index(rest, "\n"+fence[:3]+"\n") // "\n---\n"
	if end < 0 {
		return s
	}
	return rest[end+len("\n---\n"):]
}

// ---------------------------------------------------------------------------
// brain_promote
// ---------------------------------------------------------------------------

func TestBrainPromote(t *testing.T) {
	vaultRoot := setupFixtureVault(t)
	ctx := context.Background()

	mocRel := "MOCs/ai-topics.md"
	_, out, err := testServer(t).handlePromote(ctx, nil, promoteInput{
		Path: "inbox/raw-thought.md",
		MOC:  mocRel,
	})
	if err != nil {
		t.Fatalf("handlePromote: %v", err)
	}
	if out.NewPath != "notes/raw-thought.md" {
		t.Errorf("NewPath = %q, want notes/raw-thought.md", out.NewPath)
	}
	if out.OldPath != "inbox/raw-thought.md" {
		t.Errorf("OldPath = %q", out.OldPath)
	}

	// Old inbox file is gone.
	if _, err := os.Stat(filepath.Join(vaultRoot, "inbox/raw-thought.md")); !os.IsNotExist(err) {
		t.Errorf("old path should be gone, got err=%v", err)
	}

	// New note has status=validated.
	_, readOut, err := testServer(t).handleRead(ctx, nil, readInput{Path: out.NewPath})
	if err != nil {
		t.Fatalf("handleRead promoted: %v", err)
	}
	if readOut.Frontmatter.Status != "validated" {
		t.Errorf("Status = %q, want validated", readOut.Frontmatter.Status)
	}

	// MOC was appended (returned path).
	if out.MOCAppended == "" {
		t.Error("expected MOCAppended to be set")
	}
	mocData, err := os.ReadFile(filepath.Join(vaultRoot, mocRel))
	if err != nil {
		t.Fatalf("read MOC: %v", err)
	}
	if !strings.Contains(string(mocData), "raw-thought") {
		t.Errorf("MOC doesn't contain promoted note reference:\n%s", mocData)
	}
}

func TestBrainPromote_Rename(t *testing.T) {
	vaultRoot := setupFixtureVault(t)
	ctx := context.Background()

	// Seed: add a note in notes/ that references inbox stem "raw-thought".
	// (The fixture already wires inbox/raw-thought.md, notes/concept-beta.md,
	//  outputs/2026-04-14-analysis.md and MOCs/ai-topics.md, but only some
	//  of those actually reference raw-thought. concept-beta does NOT —
	//  we need one or more files that link [[raw-thought]] so the rename
	//  rewrite can be verified. inbox/raw-thought.md links to
	//  [[concept-alpha]], not itself. MOCs link to concept-alpha/beta.
	//  So add a dedicated referring note.)
	refRel := "notes/references-raw.md"
	refFull := filepath.Join(vaultRoot, filepath.FromSlash(refRel))
	refContent := `---
title: References Raw
tags: []
type: note
status: validated
created: 2026-04-10
updated: 2026-04-10
---
This links [[raw-thought]] and [[raw-thought|alias display]].
`
	if err := os.WriteFile(refFull, []byte(refContent), 0o644); err != nil {
		t.Fatalf("seed ref: %v", err)
	}

	_, out, err := testServer(t).handlePromote(ctx, nil, promoteInput{
		Path:     "inbox/raw-thought.md",
		NewTitle: "Refined Thought",
	})
	if err != nil {
		t.Fatalf("handlePromote rename: %v", err)
	}
	if out.NewPath != "notes/refined-thought.md" {
		t.Errorf("NewPath = %q, want notes/refined-thought.md", out.NewPath)
	}

	// The referencing file should have been rewritten: no more [[raw-thought]]
	// and a new [[refined-thought]] link.
	after, err := os.ReadFile(refFull)
	if err != nil {
		t.Fatalf("read ref after: %v", err)
	}
	if strings.Contains(string(after), "[[raw-thought") {
		t.Errorf("old wikilink still present in ref file:\n%s", after)
	}
	if !strings.Contains(string(after), "[[refined-thought]]") {
		t.Errorf("new wikilink missing in ref file:\n%s", after)
	}
	if !strings.Contains(string(after), "[[refined-thought|alias display]]") {
		t.Errorf("aliased wikilink not rewritten:\n%s", after)
	}

	// BacklinkRewrites should include the ref file.
	found := false
	for _, r := range out.BacklinkRewrites {
		if r == refRel {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s in BacklinkRewrites, got %v", refRel, out.BacklinkRewrites)
	}
}

// ---------------------------------------------------------------------------
// evidence field (trust tier) surfacing across tools
// ---------------------------------------------------------------------------

func TestEvidence_ListTier(t *testing.T) {
	setupFixtureVault(t)
	_, out, err := testServer(t).handleListTier(context.Background(), nil, listInput{Tier: "notes"})
	if err != nil {
		t.Fatalf("handleListTier: %v", err)
	}
	byPath := make(map[string]string, len(out.Notes))
	for _, n := range out.Notes {
		byPath[n.Path] = n.Evidence
	}
	if got := byPath["notes/graded-field.md"]; got != "A" {
		t.Errorf("graded-field evidence = %q, want A", got)
	}
	if got := byPath["notes/graded-tag.md"]; got != "D" {
		t.Errorf("graded-tag evidence = %q, want D (from tag)", got)
	}
	if got, ok := byPath["notes/concept-alpha.md"]; ok && got != "" {
		t.Errorf("concept-alpha evidence = %q, want empty (no evidence marker)", got)
	}
}

func TestEvidence_Read(t *testing.T) {
	setupFixtureVault(t)
	_, out, err := testServer(
		t,
	).handleRead(context.Background(), nil, readInput{Path: "notes/graded-field.md"})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if out.Frontmatter.Evidence != "A" {
		t.Errorf("Evidence = %q, want A", out.Frontmatter.Evidence)
	}
}

func TestEvidence_Peek(t *testing.T) {
	setupFixtureVault(t)
	_, out, err := testServer(
		t,
	).handlePeek(context.Background(), nil, peekInput{Path: "notes/graded-tag.md"})
	if err != nil {
		t.Fatalf("handlePeek: %v", err)
	}
	if out.Frontmatter.Evidence != "D" {
		t.Errorf("Evidence = %q, want D (from evidence/D tag)", out.Frontmatter.Evidence)
	}
}

func TestEvidence_Search_FilesMode(t *testing.T) {
	setupFixtureVault(t)
	_, out, err := testServer(t).handleSearch(context.Background(), nil, searchInput{
		Query:      "graded",
		OutputMode: "files_with_matches",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	evidenceByPath := make(map[string]string, len(out.Files))
	for _, f := range out.Files {
		evidenceByPath[f.Path] = f.Evidence
	}
	if got := evidenceByPath["notes/graded-field.md"]; got != "A" {
		t.Errorf("graded-field evidence = %q, want A", got)
	}
	if got := evidenceByPath["notes/graded-tag.md"]; got != "D" {
		t.Errorf("graded-tag evidence = %q, want D", got)
	}
}

func TestEvidence_Search_ContentMode(t *testing.T) {
	setupFixtureVault(t)
	_, out, err := testServer(t).handleSearch(context.Background(), nil, searchInput{
		Query:      "effect size",
		OutputMode: "content",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	if len(out.Lines) == 0 {
		t.Fatalf("no content-mode lines returned")
	}
	for _, l := range out.Lines {
		if l.Path == "notes/graded-field.md" && l.Evidence != "A" {
			t.Errorf("content-mode line %s:%d Evidence = %q, want A", l.Path, l.Line, l.Evidence)
		}
	}
}

func TestEvidence_Backlinks(t *testing.T) {
	setupFixtureVault(t)
	// graded-field.md links to [[concept-alpha]]. Ask for backlinks of
	// concept-alpha; inbound should include graded-field with Evidence=A.
	_, out, err := testServer(t).handleBacklinks(context.Background(), nil, backlinksInput{
		Path: "notes/concept-alpha.md",
	})
	if err != nil {
		t.Fatalf("handleBacklinks: %v", err)
	}
	foundInbound := false
	for _, ib := range out.Inbound {
		if ib.Path == "notes/graded-field.md" {
			foundInbound = true
			if ib.Evidence != "A" {
				t.Errorf("inbound[graded-field].Evidence = %q, want A", ib.Evidence)
			}
		}
	}
	if !foundInbound {
		t.Errorf("expected inbound from graded-field.md, got %+v", out.Inbound)
	}
	// concept-alpha itself has no evidence marker → Evidence empty.
	if out.Evidence != "" {
		t.Errorf("analyzed-note Evidence = %q, want empty", out.Evidence)
	}
}

func TestEvidence_Write_Field(t *testing.T) {
	setupFixtureVault(t)
	_, wOut, err := testServer(t).handleWrite(context.Background(), nil, writeInput{
		Tier:     "notes",
		Title:    "Direct Field",
		Body:     "Body here.",
		Tags:     []string{"ai/research"},
		Evidence: "B",
	})
	if err != nil {
		t.Fatalf("handleWrite: %v", err)
	}
	_, rOut, err := testServer(t).handleRead(context.Background(), nil, readInput{Path: wOut.Path})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if rOut.Frontmatter.Evidence != "B" {
		t.Errorf("Evidence after write = %q, want B", rOut.Frontmatter.Evidence)
	}
}

func TestEvidence_Edit_FrontmatterPatch(t *testing.T) {
	setupFixtureVault(t)
	// Patch the evidence field on concept-alpha (which had none).
	_, _, err := testServer(t).handleEdit(context.Background(), nil, editInput{
		Path: "notes/concept-alpha.md",
		FrontmatterPatch: &frontmatterIn{
			Evidence: "C",
		},
	})
	if err != nil {
		t.Fatalf("handleEdit: %v", err)
	}
	_, rOut, err := testServer(
		t,
	).handleRead(context.Background(), nil, readInput{Path: "notes/concept-alpha.md"})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if rOut.Frontmatter.Evidence != "C" {
		t.Errorf("Evidence after patch = %q, want C", rOut.Frontmatter.Evidence)
	}
}

func TestEvidence_Promote_Preserves(t *testing.T) {
	setupFixtureVault(t)
	// Create an inbox note with evidence, promote it, assert evidence survives.
	_, wOut, err := testServer(t).handleWrite(context.Background(), nil, writeInput{
		Tier:     "inbox",
		Title:    "Inbox With Evidence",
		Body:     "Captured body.",
		Evidence: "C",
	})
	if err != nil {
		t.Fatalf("handleWrite: %v", err)
	}
	_, pOut, err := testServer(
		t,
	).handlePromote(context.Background(), nil, promoteInput{Path: wOut.Path})
	if err != nil {
		t.Fatalf("handlePromote: %v", err)
	}
	if !strings.HasPrefix(pOut.NewPath, "notes/") {
		t.Fatalf("promoted path = %q, want under notes/", pOut.NewPath)
	}
	_, rOut, err := testServer(
		t,
	).handleRead(context.Background(), nil, readInput{Path: pOut.NewPath})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if rOut.Frontmatter.Evidence != "C" {
		t.Errorf("promoted Evidence = %q, want C", rOut.Frontmatter.Evidence)
	}
}
