package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// fakeSyncEmbedder is a hermetic Embedder for sync tests: it returns a fixed
// non-zero vector per text so no real model is loaded.
type fakeSyncEmbedder struct{ dim int }

func (f fakeSyncEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		v := make([]float32, f.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}

func (f fakeSyncEmbedder) Dim() int { return f.dim }

func resetSyncFlags() {
	syncConcurrencyFlag = 0
	syncDryRunFlag = false
}

type syncRepoOpt func(t *testing.T, repoDir string, run func(args ...string))

func withTrackedDirty(t *testing.T, repoDir string, run func(args ...string)) {
	t.Helper()
	_ = os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("modified\n"), 0o644)
}

func withUntrackedFiles(t *testing.T, repoDir string, _ func(args ...string)) {
	t.Helper()
	_ = os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("new\n"), 0o644)
}

func withNoRemote(t *testing.T, repoDir string, run func(args ...string)) {
	t.Helper()
	run("remote", "remove", "origin")
}

func setupSyncRepo(t *testing.T, name, branch string, dirty bool, opts ...syncRepoOpt) string {
	t.Helper()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, name)
	_ = os.MkdirAll(repoDir, 0o755)
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:org/"+name+".git")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("checkout", "-b", branch)
	_ = os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("content\n"), 0o644)
	run("add", "file.txt")
	run("commit", "-m", "init")

	if dirty {
		_ = os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("modified\n"), 0o644)
	}

	for _, opt := range opts {
		opt(t, repoDir, run)
	}

	return repoDir
}

func TestPullRepo_DetachedHead(t *testing.T) {
	dir := setupSyncRepo(t, "detached", "main", false)

	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detach: %v\n%s", err, out)
	}

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/detached", Path: dir, Remote: "git@github.com:org/detached.git"},
		false,
	)
	if r.Status != "detach" {
		t.Errorf("expected detach, got %s", r.Status)
	}
}

func TestPullRepo_NonDefaultBranch(t *testing.T) {
	dir := setupSyncRepo(t, "feature", "feature-x", false)

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/feature", Path: dir, Remote: "git@github.com:org/feature.git"},
		false,
	)
	if r.Status != "skip" {
		t.Errorf("expected skip for non-default branch, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "feature-x") {
		t.Errorf("expected message to mention branch, got %q", r.Message)
	}
}

func TestPullRepo_DirtyDefaultBranch(t *testing.T) {
	dir := setupSyncRepo(t, "dirtymain", "main", true)

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/dirtymain", Path: dir, Remote: "git@github.com:org/dirtymain.git"},
		false,
	)
	if r.Status != "dirty" {
		t.Errorf("expected dirty, got %s", r.Status)
	}
}

func TestPullRepo_DryRun(t *testing.T) {
	dir := setupSyncRepo(t, "dryrun", "main", false)

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/dryrun", Path: dir, Remote: "git@github.com:org/dryrun.git"},
		true,
	)
	if r.Status != "ok" {
		t.Errorf("expected ok for dry-run, got %s", r.Status)
	}
	if !strings.Contains(r.Message, "dry-run") {
		t.Errorf("expected dry-run message, got %q", r.Message)
	}
}

func TestPullRepo_NotGitRepo(t *testing.T) {
	dir := t.TempDir()

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/notgit", Path: dir, Remote: "git@github.com:org/notgit.git"},
		false,
	)
	if r.Status != "notgit" {
		t.Errorf("expected notgit, got %s", r.Status)
	}
}

func TestPullRepo_NoRemote(t *testing.T) {
	dir := setupSyncRepo(t, "noremote", "main", false, withNoRemote)

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "noremote/noremote", Path: dir, Remote: ""},
		false,
	)
	if r.Status != "noremote" {
		t.Errorf("expected noremote, got %s", r.Status)
	}
}

func TestPullRepo_UntrackedFilesNotDirty(t *testing.T) {
	dir := setupSyncRepo(t, "untracked", "main", false, withUntrackedFiles)

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/untracked", Path: dir, Remote: "git@github.com:org/untracked.git"},
		true,
	)
	if r.Status == "dirty" {
		t.Errorf("untracked files should not be treated as dirty, got %s", r.Status)
	}
	if r.Status != "ok" {
		t.Errorf("expected ok (dry-run), got %s", r.Status)
	}
}

func TestPullRepo_TrackedModifiedIsDirty(t *testing.T) {
	dir := setupSyncRepo(t, "tracked", "main", false, withTrackedDirty)

	r := pullRepo(
		context.Background(),
		finder.Repo{Name: "org/tracked", Path: dir, Remote: "git@github.com:org/tracked.git"},
		false,
	)
	if r.Status != "dirty" {
		t.Errorf("tracked uncommitted changes should be dirty, got %s", r.Status)
	}
}

func TestPullAll_BoundedConcurrency(t *testing.T) {
	var repos []finder.Repo
	for i := range 5 {
		name := "repo" + string(rune('a'+i))
		dir := setupSyncRepo(t, name, "main", false)
		repos = append(
			repos,
			finder.Repo{
				Name:   "org/" + name,
				Path:   dir,
				Remote: "git@github.com:org/" + name + ".git",
			},
		)
	}

	results := pullAll(context.Background(), repos, 2, true, nil)
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != "ok" {
			t.Errorf("repo %s: expected ok, got %s", r.Repo.Name, r.Status)
		}
	}
}

func TestResolveDefaultBranch_Main(t *testing.T) {
	dir := setupSyncRepo(t, "mainbranch", "main", false)
	got := resolveDefaultBranch(dir)
	if got != "main" {
		t.Errorf("expected main, got %q", got)
	}
}

func TestResolveDefaultBranch_Master(t *testing.T) {
	dir := setupSyncRepo(t, "masterbranch", "master", false)
	got := resolveDefaultBranch(dir)
	if got != "master" {
		t.Errorf("expected master, got %q", got)
	}
}

func TestSyncDryRunE2E(t *testing.T) {
	dir := setupSyncRepo(t, "e2e", "main", false)
	parent := filepath.Dir(dir)

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"sync", "--dry-run"})
	resetSyncFlags()
	defer resetSyncFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\noutput: %s", err, buf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "dry-run") {
		t.Errorf("expected dry-run in output, got: %s", output)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

func TestAcquireSyncLock(t *testing.T) {
	dir := t.TempDir()

	unlock, err := acquireSyncLock(dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	lockPath := filepath.Join(dir, syncLockFile)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should exist: %v", err)
	}

	_, err = acquireSyncLock(dir)
	if err == nil {
		t.Fatal("second acquire should fail")
	}

	unlock()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after unlock")
	}

	unlock2, err := acquireSyncLock(dir)
	if err != nil {
		t.Fatalf("re-acquire after unlock should succeed: %v", err)
	}
	unlock2()
}

func TestAcquireSyncLock_WritesPID(t *testing.T) {
	dir := t.TempDir()

	unlock, err := acquireSyncLock(dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer unlock()

	lockPath := filepath.Join(dir, syncLockFile)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		t.Fatal("lock file should contain a PID")
	}
	pid, err := strconv.Atoi(content)
	if err != nil {
		t.Fatalf("lock file content %q is not a valid PID: %v", content, err)
	}
	if pid != os.Getpid() {
		t.Errorf("lock PID = %d, want current PID %d", pid, os.Getpid())
	}
}

func TestAcquireSyncLock_RemovesStaleLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, syncLockFile)

	// Write a lock file with a dead PID (PID 2 is kthreadd on Linux, not
	// a real user process; on macOS high PIDs are recycled so use 99999999).
	deadPID := "99999999"
	_ = os.WriteFile(lockPath, []byte(deadPID+"\n"), 0o600)

	// Acquiring should succeed by detecting the stale lock.
	unlock, err := acquireSyncLock(dir)
	if err != nil {
		t.Fatalf("acquire with stale lock should succeed: %v", err)
	}
	unlock()
}

func TestAcquireSyncLock_RemovesStaleLockNoPID(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, syncLockFile)

	// Write a lock file with no PID (old format).
	_ = os.WriteFile(lockPath, []byte(""), 0o600)

	unlock, err := acquireSyncLock(dir)
	if err != nil {
		t.Fatalf("acquire with old-format lock should succeed: %v", err)
	}
	unlock()
}

func TestAcquireSyncLock_RefusesLiveLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, syncLockFile)

	// Write a lock file with our own PID (definitely alive).
	_ = os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)

	_, err := acquireSyncLock(dir)
	if err == nil {
		t.Fatal("acquire should fail when lock holder is alive")
	}
	if !strings.Contains(err.Error(), "held by another process") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemoveStaleLock_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")
	_ = os.WriteFile(lockPath, []byte("99999999\n"), 0o600)

	if !removeStaleLock(lockPath) {
		t.Error("removeStaleLock should return true for dead PID")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed")
	}
}

func TestRemoveStaleLock_LiveProcess(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")
	_ = os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)

	if removeStaleLock(lockPath) {
		t.Error("removeStaleLock should return false for live PID")
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Error("lock file should still exist")
	}
}

func TestRemoveStaleLock_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")
	_ = os.WriteFile(lockPath, []byte(""), 0o600)

	if !removeStaleLock(lockPath) {
		t.Error("removeStaleLock should return true for empty lock (old format)")
	}
}

func TestRemoveStaleLock_GarbageContent(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")
	_ = os.WriteFile(lockPath, []byte("not-a-pid\n"), 0o600)

	if !removeStaleLock(lockPath) {
		t.Error("removeStaleLock should return true for garbage content")
	}
}

func TestReposToIndex_NewRepoIncluded(t *testing.T) {
	state := search.EmptyState()
	state.SetRepo("/repos/known", search.RepoState{Fingerprint: "abc"})

	results := []pullResult{
		{Repo: finder.Repo{Path: "/repos/known", Name: "org/known"}, Status: "ok"},
		{Repo: finder.Repo{Path: "/repos/new", Name: "org/new"}, Status: "ok"},
	}

	got := reposToIndex(results, state, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 repo to index, got %d", len(got))
	}
	if got[0].Path != "/repos/new" {
		t.Errorf("expected /repos/new, got %s", got[0].Path)
	}
}

func TestReposToIndex_UpdatedAlwaysIncluded(t *testing.T) {
	state := search.EmptyState()
	state.SetRepo("/repos/a", search.RepoState{Fingerprint: "abc"})

	results := []pullResult{
		{Repo: finder.Repo{Path: "/repos/a", Name: "org/a"}, Status: "updated"},
	}

	got := reposToIndex(results, state, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(got))
	}
	if got[0].Path != "/repos/a" {
		t.Errorf("expected /repos/a, got %s", got[0].Path)
	}
}

func TestReposToIndex_KnownOkRepoSkipped(t *testing.T) {
	state := search.EmptyState()
	state.SetRepo("/repos/known", search.RepoState{Fingerprint: "abc"})

	results := []pullResult{
		{Repo: finder.Repo{Path: "/repos/known", Name: "org/known"}, Status: "ok"},
	}

	got := reposToIndex(results, state, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 repos to index, got %d", len(got))
	}
}

func TestReposToIndex_DirtyRepoNotIndexed(t *testing.T) {
	state := search.EmptyState()

	results := []pullResult{
		{Repo: finder.Repo{Path: "/repos/dirty", Name: "org/dirty"}, Status: "dirty"},
	}

	got := reposToIndex(results, state, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 repos (dirty should not trigger first-time index), got %d", len(got))
	}
}

func TestReposToIndex_QueuedReposMerged(t *testing.T) {
	state := search.EmptyState()

	results := []pullResult{
		{Repo: finder.Repo{Path: "/repos/a", Name: "org/a"}, Status: "updated"},
	}
	queued := []finder.Repo{
		{Path: "/repos/b", Name: "org/b"},
		{Path: "/repos/a", Name: "org/a"}, // dup of updated
	}

	got := reposToIndex(results, state, queued)
	if len(got) != 2 {
		t.Fatalf("expected 2 repos (updated + queued deduped), got %d", len(got))
	}
	paths := map[string]bool{}
	for _, r := range got {
		paths[r.Path] = true
	}
	if !paths["/repos/a"] || !paths["/repos/b"] {
		t.Errorf("expected /repos/a and /repos/b, got %v", paths)
	}
}

func TestReposToIndex_MixedStatuses(t *testing.T) {
	state := search.EmptyState()
	state.SetRepo("/repos/existing", search.RepoState{Fingerprint: "abc"})

	results := []pullResult{
		{Repo: finder.Repo{Path: "/repos/existing", Name: "org/existing"}, Status: "ok"},
		{Repo: finder.Repo{Path: "/repos/new1", Name: "org/new1"}, Status: "ok"},
		{Repo: finder.Repo{Path: "/repos/new2", Name: "org/new2"}, Status: "ok"},
		{Repo: finder.Repo{Path: "/repos/updated", Name: "org/updated"}, Status: "updated"},
		{Repo: finder.Repo{Path: "/repos/dirty", Name: "org/dirty"}, Status: "dirty"},
		{Repo: finder.Repo{Path: "/repos/detach", Name: "org/detach"}, Status: "detach"},
	}

	got := reposToIndex(results, state, nil)
	paths := map[string]bool{}
	for _, r := range got {
		paths[r.Path] = true
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 repos (1 updated + 2 new), got %d: %v", len(got), paths)
	}
	if !paths["/repos/updated"] {
		t.Error("updated repo should be included")
	}
	if !paths["/repos/new1"] || !paths["/repos/new2"] {
		t.Error("new repos should be included")
	}
	if paths["/repos/existing"] {
		t.Error("known ok repo should be skipped")
	}
	if paths["/repos/dirty"] || paths["/repos/detach"] {
		t.Error("dirty/detached repos should not be included")
	}
}

func TestSemanticSyncRepos_BestEffort(t *testing.T) {
	semDir := t.TempDir()

	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, "main.go"),
		[]byte("package main\n\nfunc Alpha() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goodRepo := finder.Repo{Name: "x/good", Path: good}
	badRepo := finder.Repo{Name: "x/bad", Path: t.TempDir()}

	// Corrupt the bad repo's store file so IndexRepoSemantic fails to load it,
	// exercising the best-effort skip without aborting the rest.
	if err := os.WriteFile(semantic.StorePathForRepo(semDir, badRepo),
		[]byte("not a valid gob"), 0o644); err != nil {
		t.Fatal(err)
	}

	var w bytes.Buffer
	files, chunks, failed := semanticSyncRepos(
		context.Background(), &w, semDir,
		fakeSyncEmbedder{dim: 8},
		[]finder.Repo{goodRepo, badRepo},
		false,
	)

	if failed != 1 {
		t.Fatalf("failed = %d, want 1 (the corrupt bad repo)", failed)
	}
	if files < 1 || chunks < 1 {
		t.Fatalf("good repo not embedded: files=%d chunks=%d", files, chunks)
	}
	if _, err := semantic.LoadStore(semantic.StorePathForRepo(semDir, goodRepo)); err != nil {
		t.Fatalf("good repo store missing: %v", err)
	}
}

func TestIsTransientPullError(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{"ssh kex timeout", "kex_exchange_identification: read: Operation timed out", true},
		{"connect timeout", "ssh: connect to host github.com port 22: Operation timed out", true},
		{"connection reset", "fatal: the remote end hung up unexpectedly\nConnection reset by peer", true},
		{"dns offline", "ssh: Could not resolve hostname github.com: nodename nor servname provided", true},
		{"rpc failed", "error: RPC failed; curl 92 HTTP/2 stream 5 was reset", true},
		{"auth not transient", "git@github.com: Permission denied (publickey).", false},
		{"diverged not transient", "fatal: Not possible to fast-forward, aborting.", false},
		{"repo not found not transient", "ERROR: Repository not found.", false},
		{"empty not transient", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientPullError(tt.out); got != tt.want {
				t.Errorf("isTransientPullError(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}

func TestClassifyPullError(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "ssh kex timeout",
			out:  "kex_exchange_identification: read: Operation timed out\nfatal: Could not read from remote repository.",
			want: "remote unreachable (ssh/network timeout — VPN down?)",
		},
		{
			name: "ssh connect timeout",
			out:  "ssh: connect to host github.com port 22: Operation timed out\nfatal: Could not read from remote repository.",
			want: "remote unreachable (ssh/network timeout — VPN down?)",
		},
		{
			name: "dns failure",
			out:  "ssh: Could not resolve hostname github.com: nodename nor servname provided",
			want: "cannot resolve remote host (DNS/offline)",
		},
		{
			name: "auth failure",
			out:  "git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.",
			want: "ssh auth failed (no valid key for remote)",
		},
		{
			name: "repo not found",
			out:  "ERROR: Repository not found.\nfatal: Could not read from remote repository.",
			want: "remote repo not found (deleted/renamed/no access)",
		},
		{
			name: "diverged ff-only",
			out:  "fatal: Not possible to fast-forward, aborting.",
			want: "diverged from remote (ff-only failed — needs rebase/merge)",
		},
		{
			name: "unrecognized falls back to fatal line",
			out:  "Some warning text\nfatal: unable to access 'https://x/': server error",
			want: "fatal: unable to access 'https://x/': server error",
		},
		{
			name: "empty output",
			out:  "",
			want: "pull failed (no output)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPullError(tt.out); got != tt.want {
				t.Errorf("classifyPullError() = %q, want %q", got, tt.want)
			}
		})
	}
}
