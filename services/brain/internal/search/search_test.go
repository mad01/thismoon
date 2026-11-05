package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// vaultFile describes a fixture note written into the test vault.
type vaultFile struct {
	rel     string
	content string
}

// fixtureVault writes a small set of notes across tiers and returns the vault root.
func fixtureVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := []vaultFile{
		{
			rel: "inbox/raw-thought.md",
			content: `---
title: Raw Thought
tags: [idea, inbox]
type: note
status: draft
---
A sudden observation about zoekt: sparse trigrams make filters fast.
`,
		},
		{
			rel: "notes/topic/goroutines.md",
			content: `---
title: Goroutines
tags: [go, concurrency]
type: note
status: stable
---
# Goroutines

Cheap to spawn, scheduled cooperatively. Use channels to synchronize.
`,
		},
		{
			rel: "outputs/talk-draft.md",
			content: `---
title: Talk Draft
tags: [talk]
type: output
status: in-progress
---
Slides about channels and select statements.
`,
		},
		{
			rel: "MOCs/concurrency.md",
			content: `---
title: Concurrency MOC
tags: [moc, concurrency]
---
- [[goroutines]]
- [[talk-draft]]
`,
		},
	}

	for _, f := range files {
		abs := filepath.Join(dir, f.rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}

	// Write some noise we expect the walker to skip.
	for _, skip := range []string{".obsidian/config.json", "attachments/pic.png", ".trash/old.md"} {
		abs := filepath.Join(dir, skip)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte("ignore me"), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}

	return dir
}

// indexFixture builds a zoekt index for the fixture vault and returns the
// vault root + index dir.
func indexFixture(t *testing.T) (string, string) {
	t.Helper()
	vaultRoot := fixtureVault(t)
	indexDir := t.TempDir()
	if err := IndexVault(Target{Name: "v", Root: vaultRoot, IndexDir: indexDir}, nil); err != nil {
		t.Fatalf("IndexVault: %v", err)
	}
	return vaultRoot, indexDir
}

// ---------- state roundtrip ----------

func TestIndexState_SaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	state := &IndexState{
		Fingerprint: "abc123",
		IndexedAt:   time.Now().UTC().Truncate(time.Second),
		Files: map[string]FileState{
			"notes/a.md": {Size: 42, ModTime: time.Unix(1700000000, 0).UTC()},
		},
	}

	if err := state.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.Fingerprint != state.Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", loaded.Fingerprint, state.Fingerprint)
	}
	fs, ok := loaded.Files["notes/a.md"]
	if !ok {
		t.Fatal("missing file entry after roundtrip")
	}
	if fs.Size != 42 {
		t.Errorf("Size = %d, want 42", fs.Size)
	}
}

func TestLoadState_MissingFile(t *testing.T) {
	state, err := LoadState(t.TempDir())
	if err != nil {
		t.Fatalf("LoadState on empty dir: %v", err)
	}
	if state == nil || state.Files == nil {
		t.Fatal("expected non-nil state with initialized Files map")
	}
}

// ---------- fingerprint ----------

func TestComputeFingerprint_StableAcrossRuns(t *testing.T) {
	vaultRoot := fixtureVault(t)

	fp1, files1, err := ComputeFingerprint(vaultRoot)
	if err != nil {
		t.Fatalf("ComputeFingerprint: %v", err)
	}
	fp2, files2, err := ComputeFingerprint(vaultRoot)
	if err != nil {
		t.Fatalf("ComputeFingerprint 2nd: %v", err)
	}
	if fp1 != fp2 {
		t.Errorf("fingerprint not stable: %q vs %q", fp1, fp2)
	}
	if len(files1) != len(files2) {
		t.Errorf("file count differs: %d vs %d", len(files1), len(files2))
	}
	// Non-markdown / ignored files should not appear.
	for k := range files1 {
		if strings.HasPrefix(k, ".obsidian/") || strings.HasPrefix(k, "attachments/") ||
			strings.HasPrefix(k, ".trash/") {
			t.Errorf("unexpected ignored path in fingerprint: %q", k)
		}
	}
	// Every tier's note should be present.
	for _, want := range []string{"inbox/raw-thought.md", "notes/topic/goroutines.md", "outputs/talk-draft.md", "MOCs/concurrency.md"} {
		if _, ok := files1[want]; !ok {
			t.Errorf("missing expected file in fingerprint: %q", want)
		}
	}
}

func TestComputeFingerprint_ChangesOnMtime(t *testing.T) {
	vaultRoot := fixtureVault(t)

	fp1, _, err := ComputeFingerprint(vaultRoot)
	if err != nil {
		t.Fatalf("ComputeFingerprint: %v", err)
	}

	// Bump mtime on one file.
	target := filepath.Join(vaultRoot, "notes/topic/goroutines.md")
	future := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(target, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	fp2, _, err := ComputeFingerprint(vaultRoot)
	if err != nil {
		t.Fatalf("ComputeFingerprint 2nd: %v", err)
	}
	if fp1 == fp2 {
		t.Errorf("fingerprint unchanged after mtime bump: %q", fp1)
	}
}

// TestComputeFingerprint_ChangesOnSubsecondMtime pins nanosecond mtimes:
// a same-size edit within the second of the last one changes the
// fingerprint.
func TestComputeFingerprint_ChangesOnSubsecondMtime(t *testing.T) {
	vaultRoot := fixtureVault(t)
	target := filepath.Join(vaultRoot, "notes/topic/goroutines.md")
	second := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, second, second.Add(100*time.Millisecond)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	fp1, _, err := ComputeFingerprint(vaultRoot)
	if err != nil {
		t.Fatalf("ComputeFingerprint: %v", err)
	}
	if err := os.Chtimes(target, second, second.Add(600*time.Millisecond)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	fp2, _, err := ComputeFingerprint(vaultRoot)
	if err != nil {
		t.Fatalf("ComputeFingerprint 2nd: %v", err)
	}
	if fp1 == fp2 {
		t.Errorf("fingerprint unchanged after a sub-second mtime change: %q", fp1)
	}
}

func TestIsStale(t *testing.T) {
	state := &IndexState{Fingerprint: "abc", Files: map[string]FileState{}}
	if IsStale(state, "abc") {
		t.Error("IsStale(abc, abc) = true, want false")
	}
	if !IsStale(state, "def") {
		t.Error("IsStale(abc, def) = false, want true")
	}
	if !IsStale(&IndexState{Files: map[string]FileState{}}, "anything") {
		t.Error("IsStale(empty, anything) = false, want true")
	}
	if !IsStale(nil, "x") {
		t.Error("IsStale(nil, x) = false, want true")
	}
}

// ---------- search ----------

func TestSearch_BasicQuery(t *testing.T) {
	_, indexDir := indexFixture(t)
	ctx := context.Background()

	matches, err := Search(ctx, indexDir, SearchOptions{Pattern: "goroutines", Limit: 50})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match for 'goroutines'")
	}

	seen := map[string]bool{}
	for _, m := range matches {
		seen[m.Path] = true
	}
	if !seen["notes/topic/goroutines.md"] {
		t.Errorf("expected hit in notes/topic/goroutines.md, got paths: %v", pathsOf(matches))
	}
}

func TestSearch_TierFilter(t *testing.T) {
	_, indexDir := indexFixture(t)
	ctx := context.Background()

	// "note" appears in multiple frontmatters; restrict to inbox only.
	matches, err := Search(ctx, indexDir, SearchOptions{
		Pattern: "observation",
		Tier:    vault.TierInbox,
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected matches from inbox tier")
	}
	for _, m := range matches {
		if !strings.HasPrefix(m.Path, "inbox/") {
			t.Errorf("match outside inbox tier: %q", m.Path)
		}
	}

	// A term that only exists in notes/ should return nothing under tier:inbox.
	matches, err = Search(ctx, indexDir, SearchOptions{
		Pattern: "cooperatively",
		Tier:    vault.TierInbox,
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("Search inbox/cooperatively: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("tier:inbox should exclude notes tier, got paths: %v", pathsOf(matches))
	}
}

func TestSearch_LangMarkdown(t *testing.T) {
	_, indexDir := indexFixture(t)
	ctx := context.Background()

	matches, err := Search(ctx, indexDir, SearchOptions{
		Pattern: "channels",
		Lang:    "markdown",
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected markdown matches for 'channels'")
	}
	for _, m := range matches {
		if !strings.HasSuffix(m.Path, ".md") && !strings.HasSuffix(m.Path, ".mdx") {
			t.Errorf("non-markdown match slipped through: %q", m.Path)
		}
	}
}

func TestSearch_NoResults(t *testing.T) {
	_, indexDir := indexFixture(t)

	matches, err := Search(context.Background(), indexDir, SearchOptions{
		Pattern: "xyznonexistent42",
		Limit:   50,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for nonsense pattern, got %d", len(matches))
	}
}

// ---------- count ----------

func TestCount_GroupByTier(t *testing.T) {
	_, indexDir := indexFixture(t)

	results, total, err := Count(context.Background(), indexDir, CountOptions{
		Pattern: "title:",
		GroupBy: "tier",
	})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total == 0 {
		t.Error("expected non-zero total")
	}

	groups := map[string]int{}
	for _, r := range results {
		groups[r.Group] = r.Count
	}
	for _, want := range []string{"inbox", "notes", "outputs", "MOCs"} {
		if groups[want] == 0 {
			t.Errorf("expected non-zero count for tier %q, groups = %+v", want, groups)
		}
	}
}

// ---------- validate ----------

func TestValidateQuery(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		wantValid bool
		wantHint  string
	}{
		{name: "simple valid", pattern: "Hello", wantValid: true},
		{name: "regex valid", pattern: "func\\s+\\w+", wantValid: true},
		{
			name:      "unmatched paren",
			pattern:   "func (Walk",
			wantValid: false,
			wantHint:  "Escape special regex",
		},
		{name: "boolean", pattern: "Hello OR Add", wantValid: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ValidateQuery(tc.pattern)
			if info.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v (Error: %q)", info.Valid, tc.wantValid, info.Error)
			}
			if tc.wantValid && info.Parsed == "" {
				t.Error("expected non-empty Parsed for valid query")
			}
			if !tc.wantValid && info.Error == "" {
				t.Error("expected non-empty Error for invalid query")
			}
			if tc.wantHint != "" && !strings.Contains(info.Hint, tc.wantHint) {
				t.Errorf("Hint = %q, want substring %q", info.Hint, tc.wantHint)
			}
		})
	}
}

// ---------- buildQueryString ----------

func TestBuildQueryString(t *testing.T) {
	tests := []struct {
		name string
		opts SearchOptions
		want string
	}{
		{
			name: "pattern only",
			opts: SearchOptions{Pattern: "Hello"},
			want: "Hello",
		},
		{
			name: "tier filter",
			opts: SearchOptions{Pattern: "foo", Tier: vault.TierNotes},
			want: "foo file:^notes/",
		},
		{
			name: "all filters",
			opts: SearchOptions{
				Pattern:       "query",
				Tier:          vault.TierInbox,
				File:          "daily",
				Lang:          "markdown",
				CaseSensitive: true,
			},
			want: "query file:^inbox/ file:daily lang:markdown case:yes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildQueryString(tc.opts)
			if got != tc.want {
				t.Errorf("buildQueryString() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------- helpers ----------

func pathsOf(matches []Match) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Path)
	}
	return out
}
