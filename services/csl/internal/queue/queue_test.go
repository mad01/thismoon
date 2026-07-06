package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

func TestEnqueueAndClaim(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	for _, repo := range []string{"/a/repo1", "/b/repo2", "/a/repo1"} {
		if err := Enqueue(qpath, repo); err != nil {
			t.Fatalf("Enqueue(%q): %v", repo, err)
		}
	}

	claimed, repos, err := Claim(qpath)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == "" {
		t.Fatal("expected non-empty claimed path")
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 deduplicated repos, got %d: %v", len(repos), repos)
	}
	if repos[0] != "/a/repo1" || repos[1] != "/b/repo2" {
		t.Errorf("unexpected repos: %v", repos)
	}

	if _, err := os.Stat(qpath); !os.IsNotExist(err) {
		t.Error("original queue file should be gone after claim")
	}

	if err := Release(claimed); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(claimed); !os.IsNotExist(err) {
		t.Error("claimed file should be gone after release")
	}
}

func TestClaimEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	claimed, repos, err := Claim(qpath)
	if err != nil {
		t.Fatalf("Claim non-existent: %v", err)
	}
	if claimed != "" || repos != nil {
		t.Errorf(
			"expected empty result for non-existent queue, got claimed=%q repos=%v",
			claimed,
			repos,
		)
	}
}

func TestClaimEmptyFile(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	if err := os.WriteFile(qpath, []byte("\n\n  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	claimed, repos, err := Claim(qpath)
	if err != nil {
		t.Fatalf("Claim empty file: %v", err)
	}
	if claimed != "" || repos != nil {
		t.Errorf("expected empty result for blank queue, got claimed=%q repos=%v", claimed, repos)
	}
}

func TestEnqueueConcurrent(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	const writers = 20
	const perWriter = 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for w := range writers {
		go func(id int) {
			defer wg.Done()
			for i := range perWriter {
				repo := fmt.Sprintf("/repos/writer-%d/iter-%d", id, i)
				if err := Enqueue(qpath, repo); err != nil {
					t.Errorf("Enqueue: %v", err)
				}
			}
		}(w)
	}
	wg.Wait()

	claimed, repos, err := Claim(qpath)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	defer func() { _ = Release(claimed) }()

	if len(repos) != writers*perWriter {
		t.Errorf("expected %d unique repos, got %d", writers*perWriter, len(repos))
	}
}

func TestEnqueueAfterClaim(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	if err := Enqueue(qpath, "/repo/before"); err != nil {
		t.Fatal(err)
	}

	claimed, repos, err := Claim(qpath)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(repos) != 1 || repos[0] != "/repo/before" {
		t.Fatalf("unexpected claimed repos: %v", repos)
	}

	if err := Enqueue(qpath, "/repo/after"); err != nil {
		t.Fatal(err)
	}

	claimed2, repos2, err := Claim(qpath)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if len(repos2) != 1 || repos2[0] != "/repo/after" {
		t.Fatalf("second claim should only see new entry, got: %v", repos2)
	}

	_ = Release(claimed)
	_ = Release(claimed2)
}

func TestPendingCount(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	if n := PendingCount(qpath); n != 0 {
		t.Errorf("expected 0 for non-existent queue, got %d", n)
	}

	for _, r := range []string{"/a", "/b", "/a", "/c"} {
		_ = Enqueue(qpath, r)
	}

	if n := PendingCount(qpath); n != 3 {
		t.Errorf("expected 3 pending (deduped), got %d", n)
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %q", p)
	}
	if filepath.Base(p) != fileName {
		t.Errorf("expected filename %q, got %q", fileName, filepath.Base(p))
	}
}

func TestClaimDeduplicatesPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	qpath := filepath.Join(dir, fileName)

	paths := []string{"/z/repo", "/a/repo", "/m/repo", "/a/repo", "/z/repo"}
	for _, p := range paths {
		_ = Enqueue(qpath, p)
	}

	claimed, repos, err := Claim(qpath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Release(claimed) }()

	want := []string{"/z/repo", "/a/repo", "/m/repo"}
	if len(repos) != len(want) {
		t.Fatalf("got %v, want %v", repos, want)
	}
	for i, r := range repos {
		if r != want[i] {
			t.Errorf("repos[%d] = %q, want %q", i, r, want[i])
		}
	}

	sorted := make([]string, len(repos))
	copy(sorted, repos)
	sort.Strings(sorted)
	if sorted[0] == repos[0] && sorted[1] == repos[1] && sorted[2] == repos[2] {
		t.Log("order happened to be sorted — test is valid but not asserting insertion order")
	}
}
