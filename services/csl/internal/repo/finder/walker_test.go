package finder

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalk(t *testing.T) {
	tmp := t.TempDir()

	// Create repo1 with git init and a remote
	repo1 := filepath.Join(tmp, "org1", "repo1")
	if err := os.MkdirAll(repo1, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repo1)
	gitSetRemote(t, repo1, "git@github.com:testorg/repo1.git")

	// Create repo2
	repo2 := filepath.Join(tmp, "org2", "repo2")
	if err := os.MkdirAll(repo2, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repo2)
	gitSetRemote(t, repo2, "https://github.com/testorg/repo2.git")

	// Create a non-repo directory (should be skipped)
	nonRepo := filepath.Join(tmp, "not-a-repo")
	if err := os.MkdirAll(nonRepo, 0o755); err != nil {
		t.Fatal(err)
	}

	repos, err := Walk([]string{tmp})
	if err != nil {
		t.Fatal(err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	byName := make(map[string]Repo)
	for _, r := range repos {
		byName[r.Name] = r
	}

	r1, ok := byName["testorg/repo1"]
	if !ok {
		t.Fatal("expected testorg/repo1 in results")
	}
	if r1.Remote != "git@github.com:testorg/repo1.git" {
		t.Errorf("repo1 Remote = %q, want %q", r1.Remote, "git@github.com:testorg/repo1.git")
	}
	if r1.Host != "github.com" {
		t.Errorf("repo1 Host = %q, want %q", r1.Host, "github.com")
	}

	r2, ok := byName["testorg/repo2"]
	if !ok {
		t.Fatal("expected testorg/repo2 in results")
	}
	if r2.Remote != "https://github.com/testorg/repo2.git" {
		t.Errorf("repo2 Remote = %q, want %q", r2.Remote, "https://github.com/testorg/repo2.git")
	}
	if r2.Host != "github.com" {
		t.Errorf("repo2 Host = %q, want %q", r2.Host, "github.com")
	}
}

func TestWalkSkipsMissingDirs(t *testing.T) {
	repos, err := Walk([]string{"/nonexistent/path/that/does/not/exist"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

func TestWalkNoRemoteFallback(t *testing.T) {
	tmp := t.TempDir()

	// Create a repo with no remote — should fallback to dir name
	repoDir := filepath.Join(tmp, "myorg", "myrepo")
	_ = os.MkdirAll(repoDir, 0o755)
	gitInit(t, repoDir)

	repos, err := Walk([]string{tmp})
	if err != nil {
		t.Fatal(err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "myorg/myrepo" {
		t.Errorf("expected fallback name myorg/myrepo, got %s", repos[0].Name)
	}
	if repos[0].Remote != "" {
		t.Errorf("expected empty Remote for no-remote repo, got %q", repos[0].Remote)
	}
	if repos[0].Host != "" {
		t.Errorf("expected empty Host for no-remote repo, got %q", repos[0].Host)
	}
}

func TestReadOriginURL(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "ssh remote",
			content: `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@github.com:mad01/dotfiles.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
`,
			want: "git@github.com:mad01/dotfiles.git",
		},
		{
			name: "https remote",
			content: `[remote "origin"]
	url = https://github.com/mad01/gadget.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`,
			want: "https://github.com/mad01/gadget.git",
		},
		{
			name: "no origin",
			content: `[core]
	repositoryformatversion = 0
[remote "upstream"]
	url = git@github.com:other/repo.git
`,
			want: "",
		},
		{
			name:    "empty file",
			content: "",
			want:    "",
		},
		{
			name: "origin without url",
			content: `[remote "origin"]
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			cfgPath := filepath.Join(tmp, "config")
			_ = os.WriteFile(cfgPath, []byte(tt.content), 0o644)

			got := readOriginURL(cfgPath)
			if got != tt.want {
				t.Errorf("readOriginURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInspect(t *testing.T) {
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "myorg", "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:testorg/myrepo.git")

	got, err := Inspect(repoDir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if got.Name != "testorg/myrepo" {
		t.Errorf("Name = %q, want testorg/myrepo", got.Name)
	}
	if got.Path != repoDir {
		t.Errorf("Path = %q, want %q", got.Path, repoDir)
	}
	if got.Host != "github.com" {
		t.Errorf("Host = %q, want github.com", got.Host)
	}
}

func TestInspectNotARepo(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "not-a-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(dir); err == nil {
		t.Error("expected error for non-repo directory")
	}
}

func TestInspectMissingPath(t *testing.T) {
	if _, err := Inspect("/nonexistent/path"); err == nil {
		t.Error("expected error for missing path")
	}
}

func TestReadOriginURLMissingFile(t *testing.T) {
	got := readOriginURL("/nonexistent/config")
	if got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}
}

func TestFilteredWalkNoFilter(t *testing.T) {
	tmp := t.TempDir()

	repo1 := filepath.Join(tmp, "org", "repo1")
	_ = os.MkdirAll(repo1, 0o755)
	gitInit(t, repo1)
	gitSetRemote(t, repo1, "git@github.com:testorg/repo1.git")

	repo2 := filepath.Join(tmp, "org", "repo2")
	_ = os.MkdirAll(repo2, 0o755)
	gitInit(t, repo2)
	gitSetRemote(t, repo2, "git@githost.example.com:testorg/repo2.git")

	// Empty allowedHosts — no filtering, both repos returned.
	repos, err := FilteredWalk([]string{tmp}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos with empty allowedHosts, got %d", len(repos))
	}
}

func TestFilteredWalkWithHostFilter(t *testing.T) {
	tmp := t.TempDir()

	repo1 := filepath.Join(tmp, "org", "repo1")
	_ = os.MkdirAll(repo1, 0o755)
	gitInit(t, repo1)
	gitSetRemote(t, repo1, "git@github.com:testorg/repo1.git")

	repo2 := filepath.Join(tmp, "org", "repo2")
	_ = os.MkdirAll(repo2, 0o755)
	gitInit(t, repo2)
	gitSetRemote(t, repo2, "git@githost.example.com:testorg/repo2.git")

	// Only allow github.com — repo2 (githost.example.com) should be excluded.
	repos, err := FilteredWalk([]string{tmp}, []string{"github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo after host filter, got %d", len(repos))
	}
	if repos[0].Host != "github.com" {
		t.Errorf("expected github.com repo, got host %q", repos[0].Host)
	}
}

func TestFilteredWalkExcludesNoRemote(t *testing.T) {
	tmp := t.TempDir()

	// Repo with no remote — Host is empty.
	repoDir := filepath.Join(tmp, "myorg", "myrepo")
	_ = os.MkdirAll(repoDir, 0o755)
	gitInit(t, repoDir)

	// With an allowlist set, no-remote repos (empty Host) must be excluded.
	repos, err := FilteredWalk([]string{tmp}, []string{"github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos (no-remote excluded by host filter), got %d", len(repos))
	}
}

// TestFilteredWalkReportReasons pins the reporting half of the host filter: a
// repo the allowlist removes comes back with the rule and a reason naming the
// setting, and the two ways it can be removed do not read the same.
func TestFilteredWalkReportReasons(t *testing.T) {
	tmp := t.TempDir()

	kept := filepath.Join(tmp, "org", "kept")
	_ = os.MkdirAll(kept, 0o755)
	gitInit(t, kept)
	gitSetRemote(t, kept, "git@github.com:testorg/kept.git")

	otherHost := filepath.Join(tmp, "org", "other-host")
	_ = os.MkdirAll(otherHost, 0o755)
	gitInit(t, otherHost)
	gitSetRemote(t, otherHost, "git@githost.example.com:testorg/other-host.git")

	noRemote := filepath.Join(tmp, "org", "no-remote")
	_ = os.MkdirAll(noRemote, 0o755)
	gitInit(t, noRemote)

	repos, dropped, err := FilteredWalkReport([]string{tmp}, []string{"github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "testorg/kept" {
		t.Fatalf("kept = %v, want just testorg/kept", repos)
	}
	if len(dropped) != 2 {
		t.Fatalf("dropped = %v, want 2 entries", dropped)
	}

	byKind := make(map[DropKind]Dropped, len(dropped))
	for _, d := range dropped {
		byKind[d.Kind] = d
	}
	host, ok := byKind[DropHost]
	if !ok {
		t.Fatalf("no %q drop in %v", DropHost, dropped)
	}
	if want := "host githost.example.com not in index.hosts"; host.Reason() != want {
		t.Errorf("host drop reason = %q, want %q", host.Reason(), want)
	}
	if host.Repo.Path != otherHost {
		t.Errorf("host drop path = %q, want %q", host.Repo.Path, otherHost)
	}
	missing, ok := byKind[DropNoRemote]
	if !ok {
		t.Fatalf("no %q drop in %v", DropNoRemote, dropped)
	}
	if missing.Reason() != ReasonNoRemote {
		t.Errorf("no-remote drop reason = %q, want %q", missing.Reason(), ReasonNoRemote)
	}
}

// TestFilteredWalkReportNoAllowlist covers the pass-through case: with no
// allowlist there is nothing to explain, so nothing is reported as dropped.
func TestFilteredWalkReportNoAllowlist(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "org", "repo")
	_ = os.MkdirAll(repo, 0o755)
	gitInit(t, repo)

	repos, dropped, err := FilteredWalkReport([]string{tmp}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("kept = %v, want 1 repo", repos)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none without an allowlist", dropped)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s: %v\n%s", dir, err, out)
	}
}

func gitSetRemote(t *testing.T, dir, url string) {
	t.Helper()
	cmd := exec.Command("git", "remote", "add", "origin", url)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add in %s: %v\n%s", dir, err, out)
	}
}

func gitCommit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git",
		"-c", "user.email=test@example.com",
		"-c", "user.name=test",
		"commit", "--allow-empty", "-m", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit in %s: %v\n%s", dir, err, out)
	}
}

func gitWorktreeAdd(t *testing.T, repoDir, worktreePath, branch string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreePath)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add %s (%s) in %s: %v\n%s", worktreePath, branch, repoDir, err, out)
	}
}

// TestWorktreeResolvesRemoteAndBranch covers the layout of tools that keep many
// git worktrees per repo (one worktree per feature branch): a primary clone plus
// a linked worktree on a feature branch. The worktree's .git is a pointer file,
// so before the worktree-aware repoInfo it resolved no remote (empty host) and
// was silently dropped by the host allowlist. It must now resolve the same
// remote/host as its clone, carry a branch-suffixed name, and survive
// FilteredWalk.
func TestWorktreeResolvesRemoteAndBranch(t *testing.T) {
	tmp := t.TempDir()

	// Primary clone with an origin and one commit (worktree add needs a HEAD).
	repo := filepath.Join(tmp, "Repositories", "service-a")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitInit(t, repo)
	gitSetRemote(t, repo, "git@github.com:testorg/service-a.git")
	gitCommit(t, repo)

	// Linked worktree on a nested feature branch, the shape such tools create.
	wt := filepath.Join(tmp, "Projects", "feat", "service-a")
	gitWorktreeAdd(t, repo, wt, "project/feat")

	repos, err := Walk([]string{tmp})
	if err != nil {
		t.Fatal(err)
	}
	byPath := make(map[string]Repo, len(repos))
	for _, r := range repos {
		byPath[r.Path] = r
	}

	primary, ok := byPath[repo]
	if !ok {
		t.Fatalf("primary clone not discovered; got %+v", repos)
	}
	if primary.Name != "testorg/service-a" || primary.Host != "github.com" {
		t.Errorf(
			"primary: got name=%q host=%q, want testorg/service-a / github.com",
			primary.Name,
			primary.Host,
		)
	}

	worktree, ok := byPath[wt]
	if !ok {
		t.Fatalf("worktree not discovered; got %+v", repos)
	}
	if worktree.Name != "testorg/service-a@project/feat" {
		t.Errorf("worktree name: got %q, want testorg/service-a@project/feat", worktree.Name)
	}
	if worktree.Host != "github.com" {
		t.Errorf(
			"worktree host: got %q, want github.com (must survive the host allowlist)",
			worktree.Host,
		)
	}
	if worktree.Remote != primary.Remote {
		t.Errorf(
			"worktree remote: got %q, want same as primary %q",
			worktree.Remote,
			primary.Remote,
		)
	}

	// The host allowlist must keep the worktree, not silently drop it.
	filtered, err := FilteredWalk([]string{tmp}, []string{"github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf(
			"FilteredWalk[github.com]: got %d repos, want 2 (clone + worktree): %+v",
			len(filtered),
			filtered,
		)
	}
}

// TestFilteredWalkReportRemoteWithoutHost separates "no remote" from "a remote
// with no host to match": a local-path origin is a real remote, so the drop is
// a host mismatch and the reason names the remote rather than calling it
// missing.
func TestFilteredWalkReportRemoteWithoutHost(t *testing.T) {
	tmp := t.TempDir()
	local := filepath.Join(tmp, "org", "mirror")
	_ = os.MkdirAll(local, 0o755)
	gitInit(t, local)
	gitSetRemote(t, local, "/srv/mirrors/mirror.git")

	_, dropped, err := FilteredWalkReport([]string{tmp}, []string{"github.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 1 {
		t.Fatalf("dropped = %v, want 1 entry", dropped)
	}
	d := dropped[0]
	if d.Kind != DropHost {
		t.Errorf("kind = %q, want %q (a remote without a host is not a missing remote)", d.Kind, DropHost)
	}
	if !strings.Contains(d.Reason(), "/srv/mirrors/mirror.git") || !strings.Contains(d.Reason(), "no host") {
		t.Errorf("reason = %q, want it to name the remote and say it has no host", d.Reason())
	}
}
