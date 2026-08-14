package history

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{
		"-C", dir,
		"-c", "user.name=Test",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
		// No background maintenance: a detached auto-gc can still be
		// writing .git/objects/pack when t.TempDir cleanup runs, which
		// fails the test with "directory not empty".
		"-c", "gc.auto=0",
		"-c", "gc.autoDetach=false",
		"-c", "maintenance.auto=false",
	}
	cmd := exec.Command("git", append(base, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitTry runs git ignoring a non-zero exit, for commands expected to fail
// (a conflicting merge) whose failure the test then resolves.
func gitTry(dir string, args ...string) {
	base := []string{
		"-C", dir,
		"-c", "user.name=Test",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
		"-c", "gc.auto=0",
		"-c", "gc.autoDetach=false",
		"-c", "maintenance.auto=false",
	}
	_ = exec.Command("git", append(base, args...)...).Run()
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupWalkRepo builds: c1 adds clean app.go, c2 adds a secret line to
// config.txt, c3 removes it again, c4 renames app.go.
func setupWalkRepo(t *testing.T) (dir string, hashes map[string]string) {
	t.Helper()
	dir = t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")

	writeFile(t, dir, "app.go", "package main\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c1: initial")

	writeFile(t, dir, "config.txt", "url=example.com\ntoken=SECRETTOKEN123\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c2: add config")

	writeFile(t, dir, "config.txt", "url=example.com\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c3: remove token")

	git(t, dir, "mv", "app.go", "renamed.go")
	git(t, dir, "commit", "-q", "-m", "c4: rename")

	hashes = make(map[string]string)
	for line := range strings.SplitSeq(git(t, dir, "log", "--format=%H %s"), "\n") {
		hash, subject, _ := strings.Cut(line, " ")
		hashes[strings.SplitN(subject, ":", 2)[0]] = hash
	}
	return dir, hashes
}

func TestWalkAttributesAddedLinesToIntroducingCommit(t *testing.T) {
	dir, hashes := setupWalkRepo(t)

	type added struct {
		commit, path, line string
		lineNum            int
	}
	var secretHits []added
	var messages []string
	newFiles := make(map[string][]string) // commit hash -> new paths

	v := Visitor{
		OnMessage: func(c Commit, message string) error {
			messages = append(messages, c.Subject)
			return nil
		},
		OnAddedLine: func(c Commit, path string, lineNum int, line string) error {
			if strings.Contains(line, "SECRETTOKEN123") {
				secretHits = append(secretHits, added{c.Hash, path, line, lineNum})
			}
			return nil
		},
		OnNewFile: func(c Commit, path string) error {
			newFiles[c.Hash] = append(newFiles[c.Hash], path)
			return nil
		},
	}
	if err := Walk(dir, "main", v); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(secretHits) != 1 {
		t.Fatalf("secret attributed %d time(s), want exactly 1: %+v", len(secretHits), secretHits)
	}
	hit := secretHits[0]
	if hit.commit != hashes["c2"] {
		t.Errorf("secret attributed to %s, want c2 (%s)", hit.commit, hashes["c2"])
	}
	if hit.path != "config.txt" || hit.lineNum != 2 {
		t.Errorf("secret located at %s:%d, want config.txt:2", hit.path, hit.lineNum)
	}

	if len(messages) != 4 {
		t.Errorf("visited %d messages, want 4: %v", len(messages), messages)
	}
	if messages[0] != "c1: initial" {
		t.Errorf("walk must be oldest-first, got first message %q", messages[0])
	}

	if got := newFiles[hashes["c1"]]; len(got) != 1 || got[0] != "app.go" {
		t.Errorf("c1 new files = %v, want [app.go]", got)
	}
	if got := newFiles[hashes["c4"]]; len(got) != 1 || got[0] != "renamed.go" {
		t.Errorf("c4 new files = %v, want [renamed.go] (rename target)", got)
	}
}

// TestWalkSeesMergeConflictResolution pins finding 1: a secret introduced
// only while resolving a merge conflict exists in neither parent, so it is
// visible only in the merge commit's first-parent diff.
func TestWalkSeesMergeConflictResolution(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	writeFile(t, dir, "app.go", "package main\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c1")

	git(t, dir, "checkout", "-q", "-b", "feature")
	writeFile(t, dir, "shared.txt", "feature-side\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "feat")

	git(t, dir, "checkout", "-q", "main")
	writeFile(t, dir, "shared.txt", "main-side\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "main-change")

	gitTry(dir, "merge", "feature") // add/add conflict on shared.txt
	writeFile(t, dir, "shared.txt", "resolved\nMERGESECRETaZ9kQ2mX7pL4vB8nT1cW\n")
	git(t, dir, "add", "shared.txt")
	git(t, dir, "commit", "-q", "-m", "merge feature")

	var found bool
	v := Visitor{
		OnAddedLine: func(_ Commit, _ string, _ int, line string) error {
			if strings.Contains(line, "MERGESECRETaZ9kQ2mX7pL4vB8nT1cW") {
				found = true
			}
			return nil
		},
	}
	if err := Walk(dir, "main", v); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !found {
		t.Fatal("secret introduced during conflict resolution was invisible to the walker")
	}
}

// TestWalkSeesTypechangeContent pins finding 2: a symlink converted to a
// regular file whose content holds a secret is a typechange (T); without T in
// the diff filter it produces no diff output.
func TestWalkSeesTypechangeContent(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	writeFile(t, dir, "app.go", "package main\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c1")

	if err := os.Symlink("app.go", filepath.Join(dir, "linkfile")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c2 symlink")

	if err := os.Remove(filepath.Join(dir, "linkfile")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "linkfile", "TYPECHANGESECRETpQ7wE2rT9yU4iO6aX3z\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c3 typechange")

	var found bool
	v := Visitor{
		OnAddedLine: func(_ Commit, _ string, _ int, line string) error {
			if strings.Contains(line, "TYPECHANGESECRETpQ7wE2rT9yU4iO6aX3z") {
				found = true
			}
			return nil
		},
	}
	if err := Walk(dir, "main", v); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if !found {
		t.Fatal("typechange (symlink->file) content was invisible to the walker")
	}
}

// TestWalkEmitsNewFileForBinaryAndEmptyFiles pins finding 3: binary and empty
// new files have no "+++" line, so OnNewFile must still fire for them (this is
// exactly what file-name rules like *.p12 need).
func TestWalkEmitsNewFileForBinaryAndEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	writeFile(t, dir, "app.go", "package main\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c1")

	if err := os.WriteFile(filepath.Join(dir, "key.p12"), []byte{0, 1, 2, 0, 3, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "empty.pem", "")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c2 binary and empty")

	newFiles := make(map[string]bool)
	v := Visitor{
		OnNewFile: func(_ Commit, path string) error {
			newFiles[path] = true
			return nil
		},
	}
	if err := Walk(dir, "main", v); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	for _, want := range []string{"app.go", "key.p12", "empty.pem"} {
		if !newFiles[want] {
			t.Errorf("OnNewFile did not fire for %q; fired for %v", want, newFiles)
		}
	}
}

// TestWalkScansAnnotatedTagMessages pins finding 9: git log never visits tag
// objects, so annotated tag messages must be walked separately. Lightweight
// tags carry no message and are skipped.
func TestWalkScansAnnotatedTagMessages(t *testing.T) {
	dir, _ := setupWalkRepo(t)
	git(t, dir, "tag", "-a", "v1.0", "-m", "release TAGSECRETaZ9kQ2mX7pL4vB8nT1")
	git(t, dir, "tag", "lightweight")

	var tagHits int
	var sawIsTag bool
	v := Visitor{
		OnMessage: func(c Commit, message string) error {
			if strings.Contains(message, "TAGSECRETaZ9kQ2mX7pL4vB8nT1") {
				tagHits++
				sawIsTag = c.IsTag
			}
			return nil
		},
	}
	if err := Walk(dir, "main", v); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if tagHits != 1 {
		t.Fatalf("annotated tag secret seen %d time(s), want 1", tagHits)
	}
	if !sawIsTag {
		t.Error("tag message not flagged with Commit.IsTag")
	}
}

func TestCleanRefusesNonEmptyStash(t *testing.T) {
	dir, _ := setupWalkRepo(t)
	writeFile(t, dir, "config.txt", "url=example.com\nstashed=STASHSECRET123\n")
	git(t, dir, "stash", "push", "-q")

	_, err := Clean(dir, &Rewriter{Replacements: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "stash") {
		t.Fatalf("Clean with a non-empty stash: err = %v, want stash refusal", err)
	}
}

func TestCleanRefusesOtherWorktrees(t *testing.T) {
	dir, _ := setupWalkRepo(t)
	other := filepath.Join(t.TempDir(), "linked")
	git(t, dir, "worktree", "add", "-q", other, "-b", "wtbranch")

	_, err := Clean(dir, &Rewriter{Replacements: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("Clean with a linked worktree: err = %v, want worktree refusal", err)
	}
}

func TestResolveRef(t *testing.T) {
	dir, _ := setupWalkRepo(t)

	ref, err := ResolveRef(dir, "")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}
	if ref != "main" {
		t.Errorf("ResolveRef = %q, want main", ref)
	}

	if _, err := ResolveRef(dir, "does-not-exist"); err == nil {
		t.Error("ResolveRef must reject a missing branch")
	}
}

func TestCleanRewritesHistoryEndToEnd(t *testing.T) {
	dir, _ := setupWalkRepo(t)

	rw := &Rewriter{Replacements: []string{"SECRETTOKEN123"}}
	result, err := Clean(dir, rw)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}

	fullLog := git(t, dir, "log", "--all", "-p", "--format=%H %ae %B")
	if strings.Contains(fullLog, "SECRETTOKEN123") {
		t.Fatal("secret still reachable from refs after Clean")
	}
	if !strings.Contains(fullLog, RedactedPlaceholder) {
		t.Fatal("placeholder missing from rewritten history")
	}

	// The latest tree must be untouched apart from the replaced line, which
	// c3 already removed — so HEAD's tree content is identical.
	if got := git(t, dir, "show", "HEAD:config.txt"); got != "url=example.com" {
		t.Errorf("HEAD config.txt = %q, want unchanged content", got)
	}
	if got := git(t, dir, "show", "HEAD:renamed.go"); got != "package main" {
		t.Errorf("HEAD renamed.go = %q, want unchanged content", got)
	}

	if result.BackupPath == "" {
		t.Fatal("no backup bundle written")
	}
	git(t, dir, "bundle", "verify", result.BackupPath)

	if result.Stats.Replacements["SECRETTOKEN123"] == 0 {
		t.Errorf("stats missing replacement count: %+v", result.Stats)
	}
}

func TestCleanRefusesDirtyWorktree(t *testing.T) {
	dir, _ := setupWalkRepo(t)
	writeFile(t, dir, "dirty.txt", "uncommitted\n")
	git(t, dir, "add", "dirty.txt")

	_, err := Clean(dir, &Rewriter{Replacements: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("Clean on dirty tree: err = %v, want dirty-tree refusal", err)
	}
}

func TestDryRunRewritesNothing(t *testing.T) {
	dir, _ := setupWalkRepo(t)
	before := git(t, dir, "rev-parse", "HEAD")

	result, err := DryRun(dir, &Rewriter{Replacements: []string{"SECRETTOKEN123"}})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if result.Stats.Replacements["SECRETTOKEN123"] == 0 {
		t.Errorf("dry run found no replacements: %+v", result.Stats)
	}
	if got := git(t, dir, "rev-parse", "HEAD"); got != before {
		t.Error("dry run must not move HEAD")
	}
	if result.BackupPath != "" {
		t.Error("dry run must not write a backup bundle")
	}
}
