package semantic

import (
	"bytes"
	"context"
	"encoding/gob"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// countingEmbedder wraps an Embedder to count Embed invocations.
type countingEmbedder struct {
	inner Embedder
	calls int
}

func (c *countingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	c.calls++
	return c.inner.Embed(ctx, texts)
}

func (c *countingEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	c.calls++
	return c.inner.EmbedQuery(ctx, query)
}

func (c *countingEmbedder) Dim() int { return c.inner.Dim() }

// fakeRepoTree writes a tiny source tree: one .go file with two funcs plus a
// node_modules file that indexing MUST skip. Returns the repo root.
func fakeRepoTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mainGo := "package main\n\nfunc Alpha() int { return 1 }\n\nfunc Beta() int { return 2 }\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	nm := filepath.Join(root, "node_modules", "dep")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nm, "index.go"), []byte("package dep\nfunc X() {}\n"), 0o644); err != nil {
		t.Fatalf("write node_modules file: %v", err)
	}
	return root
}

func TestIndexRepoSemantic(t *testing.T) {
	root := fakeRepoTree(t)
	repo := finder.Repo{Name: "x/y", Path: root}
	emb := &countingEmbedder{inner: newFakeEmbedder(8)}
	indexDir := t.TempDir()

	stats, err := IndexRepoSemantic(context.Background(), indexDir, repo, emb)
	if err != nil {
		t.Fatalf("IndexRepoSemantic: %v", err)
	}

	if stats.FilesScanned != 1 {
		t.Fatalf("FilesScanned = %d, want 1 (node_modules skipped)", stats.FilesScanned)
	}
	if stats.FilesEmbedded != 1 {
		t.Fatalf("FilesEmbedded = %d, want 1", stats.FilesEmbedded)
	}
	if stats.ChunksEmbedded < 2 {
		t.Fatalf("ChunksEmbedded = %d, want >= 2", stats.ChunksEmbedded)
	}

	// The store file exists and holds only main.go.
	store, err := LoadStore(StorePathForRepo(indexDir, repo))
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if store.RepoPath() != root {
		t.Fatalf("store RepoPath = %q, want %q", store.RepoPath(), root)
	}
	files := store.Files()
	if len(files) != 1 {
		t.Fatalf("store has %d files, want 1: %v", len(files), files)
	}
	if _, ok := files["main.go"]; !ok {
		t.Fatalf("main.go missing from store: %v", files)
	}
}

func TestIndexRepoSemanticIncremental(t *testing.T) {
	root := fakeRepoTree(t)
	repo := finder.Repo{Name: "x/y", Path: root}
	emb := &countingEmbedder{inner: newFakeEmbedder(8)}
	indexDir := t.TempDir()
	ctx := context.Background()

	if _, err := IndexRepoSemantic(ctx, indexDir, repo, emb); err != nil {
		t.Fatalf("first index: %v", err)
	}
	callsAfterFirst := emb.calls
	if callsAfterFirst == 0 {
		t.Fatalf("expected Embed to be called on first index")
	}

	// Second run, no changes: everything skipped, no embedding.
	stats, err := IndexRepoSemantic(ctx, indexDir, repo, emb)
	if err != nil {
		t.Fatalf("second index: %v", err)
	}
	if stats.FilesSkipped != 1 || stats.FilesEmbedded != 0 {
		t.Fatalf("incremental run: skipped=%d embedded=%d, want skipped=1 embedded=0", stats.FilesSkipped, stats.FilesEmbedded)
	}
	if emb.calls != callsAfterFirst {
		t.Fatalf("Embed called %d extra times on no-change run", emb.calls-callsAfterFirst)
	}

	// Modify the file: only it should be re-embedded.
	changed := "package main\n\nfunc Alpha() int { return 99 }\n\nfunc Beta() int { return 2 }\n\nfunc Gamma() int { return 3 }\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(changed), 0o644); err != nil {
		t.Fatalf("rewrite main.go: %v", err)
	}
	stats, err = IndexRepoSemantic(ctx, indexDir, repo, emb)
	if err != nil {
		t.Fatalf("third index: %v", err)
	}
	if stats.FilesEmbedded != 1 || stats.FilesSkipped != 0 {
		t.Fatalf("after change: embedded=%d skipped=%d, want embedded=1 skipped=0", stats.FilesEmbedded, stats.FilesSkipped)
	}
}

func TestIndexRepoSemanticRebuildsOnChunkerBump(t *testing.T) {
	root := fakeRepoTree(t)
	repo := finder.Repo{Name: "x/y", Path: root}
	emb := &countingEmbedder{inner: newFakeEmbedder(8)}
	indexDir := t.TempDir()
	ctx := context.Background()

	if _, err := IndexRepoSemantic(ctx, indexDir, repo, emb); err != nil {
		t.Fatalf("first index: %v", err)
	}

	// Rewrite the store as if an older chunker produced it. The file contents
	// are unchanged, so only the version mismatch can trigger a re-embed.
	storePath := StorePathForRepo(indexDir, repo)
	downgradeStoreVersion(t, storePath, chunkerVersion-1)

	stats, err := IndexRepoSemantic(ctx, indexDir, repo, emb)
	if err != nil {
		t.Fatalf("re-index after downgrade: %v", err)
	}
	if stats.FilesEmbedded != 1 || stats.FilesSkipped != 0 {
		t.Fatalf("chunker bump: embedded=%d skipped=%d, want embedded=1 skipped=0", stats.FilesEmbedded, stats.FilesSkipped)
	}

	// The rewritten store carries the current version: the next run skips again.
	stats, err = IndexRepoSemantic(ctx, indexDir, repo, emb)
	if err != nil {
		t.Fatalf("third index: %v", err)
	}
	if stats.FilesSkipped != 1 || stats.FilesEmbedded != 0 {
		t.Fatalf("post-rebuild run: skipped=%d embedded=%d, want skipped=1 embedded=0", stats.FilesSkipped, stats.FilesEmbedded)
	}
}

// downgradeStoreVersion rewrites a saved store's ChunkerVersion in place,
// simulating a store persisted by an older binary.
func downgradeStoreVersion(t *testing.T, path string, version int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	var snap persisted
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		t.Fatalf("decode store: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	snap.ChunkerVersion = version
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("rewrite store: %v", err)
	}
	if err := gob.NewEncoder(out).Encode(snap); err != nil {
		t.Fatalf("encode store: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close rewritten store: %v", err)
	}
}

func TestIndexRepoSemanticDeletesRemovedFiles(t *testing.T) {
	root := fakeRepoTree(t)
	repo := finder.Repo{Name: "x/y", Path: root}
	emb := newFakeEmbedder(8)
	indexDir := t.TempDir()
	ctx := context.Background()

	// Add a second source file, index, then remove it.
	extra := filepath.Join(root, "extra.go")
	if err := os.WriteFile(extra, []byte("package main\nfunc Z() {}\n"), 0o644); err != nil {
		t.Fatalf("write extra.go: %v", err)
	}
	if _, err := IndexRepoSemantic(ctx, indexDir, repo, emb); err != nil {
		t.Fatalf("index with extra: %v", err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatalf("remove extra.go: %v", err)
	}
	if _, err := IndexRepoSemantic(ctx, indexDir, repo, emb); err != nil {
		t.Fatalf("reindex after remove: %v", err)
	}

	store, err := LoadStore(StorePathForRepo(indexDir, repo))
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if _, ok := store.Files()["extra.go"]; ok {
		t.Fatalf("extra.go not pruned from store: %v", store.Files())
	}
}

func TestIsSkippedFile(t *testing.T) {
	skipped := []string{
		".build/arm64-apple-macosx/debug/ModuleCache/foo.pcm.timestamp",
		"DerivedData/App/Build/Intermediates/x.d",
		"Pods/Alamofire/Source/Session.swift",
		"Carthage/Checkouts/Dep/file.swift",
		"App/Assets.xcassets/AppIcon.appiconset/Contents.json",
		"MyApp.xcodeproj/xcuserdata/me.xcuserdatad/xcschemes/x.plist",
		"vendor/github.com/pkg/errors/errors.go",
		"node_modules/dep/index.ts",
		"target/debug/build/script-output.rs",
		"app/build/generated/source/Gen.java",
		"dist/bundle.js",
		"__pycache__/mod.cpython-312.pyc",
		"icons/logo.heic",
		"modules/Accessibility.swiftmodule",
		"App.xcodeproj/project.pbxproj",
		"App.xcworkspace/contents.xcworkspacedata",
		"Views/TimelineView.swift.backup",
		"Resources/Models/Qwen3-4bit/merges.txt",
		"Resources/Models/Qwen3-4bit/tokenizer_config.json",
		"Resources/Models/Qwen3-4bit/model.safetensors",
		"Resources/Detector.mlmodelc/coremldata.bin",
		"Resources/Classifier.mlpackage/Data/com.apple.CoreML/model.mlmodel",
	}
	for _, p := range skipped {
		if !isSkippedFile(p) {
			t.Errorf("isSkippedFile(%q) = false, want true", p)
		}
	}

	kept := []string{
		"main.go",
		"internal/build.go",
		"docs/build-notes.md",
		"src/targets.ts",
		"Sources/App/BuildInfo.swift",
		"cmd/vendorctl/main.go",
	}
	for _, p := range kept {
		if isSkippedFile(p) {
			t.Errorf("isSkippedFile(%q) = true, want false", p)
		}
	}

	lockfiles := []string{
		"package-lock.json",
		"web/yarn.lock",
		"Cargo.lock",
		"go.sum",
		"App/Package.resolved",
		"Podfile.lock",
	}
	for _, p := range lockfiles {
		if !isSkippedFile(p) {
			t.Errorf("isSkippedFile(%q) = false, want true (lockfile)", p)
		}
	}
}

func TestLooksLikeData(t *testing.T) {
	vocab := []byte(`{"tokens":{` + strings.Repeat(`"tok":1,`, 500) + `"end":2}}`)
	if !looksLikeData(vocab) {
		t.Error("looksLikeData(single-line vocab json) = false, want true")
	}

	source := []byte(strings.Repeat("func short() int { return 1 }\n", 400))
	if looksLikeData(source) {
		t.Error("looksLikeData(normal source) = true, want false")
	}

	// A long line past the 8KB sniff window must not trigger the check.
	tail := append([]byte(strings.Repeat("short line\n", 900)), bytes.Repeat([]byte{'x'}, 4000)...)
	if looksLikeData(tail) {
		t.Error("looksLikeData(long line beyond sniff window) = true, want false")
	}
}

// TestGitTrackedFilesSkipsCommittedJunk verifies the path filter applies to
// git-tracked files, not just the non-git fallback walk: committed build
// output and vendored trees must never reach the chunker.
func TestGitTrackedFilesSkipsCommittedJunk(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("main.swift", "print(\"hi\")\n")
	write(".build/arm64-apple-macosx/debug/foo.pcm.timestamp", "ts\n")
	write("vendor/dep/dep.go", "package dep\n")
	write("Assets.xcassets/AppIcon.appiconset/Contents.json", "{}\n")

	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "add", "."},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	files, err := gitTrackedFiles(root)
	if err != nil {
		t.Fatalf("gitTrackedFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "main.swift" {
		t.Fatalf("gitTrackedFiles = %v, want [main.swift]", files)
	}
}

// TestAuditRepoFilesMatchesWalk pins the audit to the indexer: every file
// AuditRepoFiles marks Index must be exactly the set walkSourceFiles visits.
func TestAuditRepoFilesMatchesWalk(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("main.go", "package main\nfunc A() {}\n")
	write("docs/notes.md", "# notes\n")
	write("vendor/dep/dep.go", "package dep\n")
	write("data/vocab.json", `{"a":`+strings.Repeat("1,", 2000)+`"z":0}`)
	write("bin/blob.txt", "x\x00y")
	write("go.sum", "mod v1.0.0 h1:abc=\n")
	write("fixtures/huge.txt", "fixture\n")
	write(".cslignore", "fixtures/\n")

	decisions, err := AuditRepoFiles(root)
	if err != nil {
		t.Fatalf("AuditRepoFiles: %v", err)
	}
	audited := make(map[string]bool)
	for _, d := range decisions {
		if d.Index {
			audited[d.Path] = true
		}
	}

	walked := make(map[string]bool)
	err = walkSourceFiles(root, func(rel string, _ []byte) error {
		walked[rel] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walkSourceFiles: %v", err)
	}

	if len(audited) != len(walked) {
		t.Fatalf("audit indexed %v, walk visited %v", audited, walked)
	}
	for p := range walked {
		if !audited[p] {
			t.Errorf("walk visited %s but audit skipped it", p)
		}
	}
	for _, want := range []string{"main.go", "docs/notes.md"} {
		if !audited[want] {
			t.Errorf("audit skipped %s, want indexed", want)
		}
	}

	reasons := make(map[string]string)
	for _, d := range decisions {
		reasons[d.Path] = d.Reason
	}
	if reasons["data/vocab.json"] == "" {
		t.Error("vocab.json indexed, want a skip reason")
	}
	if reasons["bin/blob.txt"] == "" {
		t.Error("null-byte file indexed, want a skip reason")
	}
	if reasons["go.sum"] == "" {
		t.Error("go.sum indexed, want a skip reason")
	}
	if reasons["fixtures/huge.txt"] != "matched .cslignore" {
		t.Errorf("fixtures/huge.txt reason = %q, want matched .cslignore", reasons["fixtures/huge.txt"])
	}
}

func TestStorePathForRepo(t *testing.T) {
	got := StorePathForRepo("/idx", finder.Repo{Name: "org/sub/repo"})
	want := filepath.Join("/idx", "org_sub_repo.gob")
	if got != want {
		t.Fatalf("StorePathForRepo = %q, want %q", got, want)
	}
}
