package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
)

func initGitRepo(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("git", "init", path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	cmd = exec.Command("git", "-C", path, "config", "user.email", "test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	cmd = exec.Command("git", "-C", path, "config", "user.name", "Test")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config name: %v", err)
	}
}

func TestCollectNames_blockedWords(t *testing.T) {
	workspace := t.TempDir()
	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{workspace},
		BlockedWords:  []string{"acmecorp", "internal-corp"},
		Allowlist:     nil,
		FilePatterns:  []string{"*.go"},
	})

	names, err := g.CollectNames()
	if err != nil {
		t.Fatalf("CollectNames: %v", err)
	}
	if len(names) < 2 {
		t.Fatalf("expected at least 2 names, got %d: %v", len(names), names)
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["acmecorp"] {
		t.Error("missing blocked word 'acmecorp'")
	}
	if !found["internal-corp"] {
		t.Error("missing blocked word 'internal-corp'")
	}
}

func TestCollectNames_allowlist(t *testing.T) {
	workspace := t.TempDir()
	// Create a repo that matches the allowlist.
	repoPath := filepath.Join(workspace, "grpc", "grpc-go")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoPath)

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{workspace},
		BlockedWords:  []string{"corp"},
		Allowlist:     []string{"grpc/grpc-go"},
	})

	names, err := g.CollectNames()
	if err != nil {
		t.Fatalf("CollectNames: %v", err)
	}
	for _, n := range names {
		if n == "grpc/grpc-go" {
			t.Error("allowlisted name should be excluded")
		}
	}
}

func TestCollectNames_discoversRepos(t *testing.T) {
	workspace := t.TempDir()
	repoPath := filepath.Join(workspace, "myorg", "secret-repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoPath)

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{workspace},
		BlockedWords:  nil,
	})

	names, err := g.CollectNames()
	if err != nil {
		t.Fatalf("CollectNames: %v", err)
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["myorg/secret-repo"] {
		t.Errorf("expected 'myorg/secret-repo' in names, got %v", names)
	}
}

func TestCheck_detectsMatch(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	// Create and commit an initial file so HEAD exists.
	initial := filepath.Join(repoPath, "init.txt")
	if err := os.WriteFile(initial, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repoPath, "add", "init.txt")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repoPath, "commit", "-m", "init")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Stage a file that mentions a blocked word.
	testFile := filepath.Join(repoPath, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n// connect to acmecorp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repoPath, "add", "main.go")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{t.TempDir()},
		BlockedWords:  []string{"acmecorp"},
		FilePatterns:  []string{"*.go"},
	})

	findings, err := g.Check(repoPath)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) == 0 {
		t.Error("expected findings for 'acmecorp' reference")
	}
}

func TestCheck_cleanCommit(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	initial := filepath.Join(repoPath, "init.txt")
	if err := os.WriteFile(initial, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repoPath, "add", "init.txt")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repoPath, "commit", "-m", "init")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(repoPath, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n// hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repoPath, "add", "main.go")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{t.TempDir()},
		BlockedWords:  []string{"acmecorp"},
		FilePatterns:  []string{"*.go"},
	})

	findings, err := g.Check(repoPath)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %v", findings)
	}
}

// addFile writes content to rel under repoPath and stages it so ls-files
// sees it.
func addFile(t *testing.T, repoPath, rel string, content []byte) {
	t.Helper()
	abs := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repoPath, "add", rel)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add %s: %v", rel, err)
	}
}

func TestCheckDir_detectsMatch(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	addFile(t, repoPath, "notes.md", []byte("intro\ntalk to AcmeCorp and acmecorp today\n"))
	addFile(t, repoPath, "clean.md", []byte("nothing to see\n"))

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{t.TempDir()},
		BlockedWords:  []string{"acmecorp"},
	})

	findings, err := g.CheckDir(repoPath)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	// Each casing is a distinct finding; the history rewrite replaces
	// case-sensitively, so both must surface.
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (one per casing), got %d: %v", len(findings), findings)
	}
	for i, want := range []string{"AcmeCorp", "acmecorp"} {
		f := findings[i]
		if f.File != "notes.md" || f.Line != 2 {
			t.Errorf("finding %d: expected notes.md:2, got %s:%d", i, f.File, f.Line)
		}
		if f.Match != want {
			t.Errorf("finding %d: expected match %q, got %q", i, want, f.Match)
		}
	}
}

func TestNewMatcher_domainsAndLinks(t *testing.T) {
	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: nil,
		BlockedWords:  []string{"internal.acmecorp.net", "docs.acmecorp.net/runbooks"},
	})

	matcher, err := g.NewMatcher()
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	matches := matcher.Find("see https://internal.acmecorp.net/service for details")
	if len(matches) != 1 || matches[0] != "internal.acmecorp.net" {
		t.Errorf("expected domain match, got %v", matches)
	}

	matches = matcher.Find("runbook: https://docs.acmecorp.net/runbooks/oncall")
	if len(matches) != 1 || matches[0] != "docs.acmecorp.net/runbooks" {
		t.Errorf("expected docs link match, got %v", matches)
	}

	// The dot is matched literally, not as a regex wildcard.
	matches = matcher.Find("internalXacmecorpYnet")
	if len(matches) != 0 {
		t.Errorf("expected no match for non-literal dot, got %v", matches)
	}
}

func TestNewMatcher_wildcard(t *testing.T) {
	g := New(config.GuardConfig{
		Enabled:      true,
		BlockedWords: []string{"*.acmecorp.net"},
	})

	matcher, err := g.NewMatcher()
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	// The wildcard covers any subdomain and swallows the URL scheme, so
	// cleanup replaces the whole reference up to the entry's literal tail.
	matches := matcher.Find("curl https://internal.acmecorp.net/api now")
	if len(matches) != 1 || matches[0] != "https://internal.acmecorp.net" {
		t.Errorf("expected scheme+host match, got %v", matches)
	}

	matches = matcher.Find("host foo.bar.acmecorp.net down")
	if len(matches) != 1 || matches[0] != "foo.bar.acmecorp.net" {
		t.Errorf("expected subdomain match, got %v", matches)
	}

	// The bare apex has no leading dot, so the wildcard entry does not
	// cover it; block it with its own entry.
	matches = matcher.Find("apex acmecorp.net here")
	if len(matches) != 0 {
		t.Errorf("expected no match for bare apex, got %v", matches)
	}
}

func TestNewMatcher_wordBoundaries(t *testing.T) {
	g := New(config.GuardConfig{
		Enabled:      true,
		BlockedWords: []string{"her", "corp/tools"},
	})

	matcher, err := g.NewMatcher()
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	// A short blocked word must not match inside ordinary words.
	for _, line := range []string{
		"lower waves complete before higher waves start",
		"the otherDir variable",
		"do it together",
	} {
		if matches := matcher.Find(line); len(matches) != 0 {
			t.Errorf("expected no match in %q, got %v", line, matches)
		}
	}

	// Standalone and punctuation-delimited references still match.
	for _, line := range []string{
		"ask her today",
		"host her.acmecorp.net down",
		"clone git@her:org/repo.git",
		"see corp/tools for the script",
		"https://example.com/corp/tools/blob/main/x",
	} {
		if matches := matcher.Find(line); len(matches) != 1 {
			t.Errorf("expected one match in %q, got %v", line, matches)
		}
	}

	// Word characters adjacent to the name's edges block the match.
	if matches := matcher.Find("acorp/toolset here"); len(matches) != 0 {
		t.Errorf("expected no match for embedded org/repo, got %v", matches)
	}
}

func TestNewMatcher_wildcardEdgesKeepReach(t *testing.T) {
	g := New(config.GuardConfig{
		Enabled:      true,
		BlockedWords: []string{"*.acmecorp.net"},
	})

	matcher, err := g.NewMatcher()
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	// The leading wildcard still swallows the URL scheme.
	matches := matcher.Find("curl https://internal.acmecorp.net/api now")
	if len(matches) != 1 || matches[0] != "https://internal.acmecorp.net" {
		t.Errorf("expected scheme+host match, got %v", matches)
	}

	// The literal tail now stops at a word boundary: a longer hostname
	// containing the entry as a prefix is not a reference to it.
	if matches := matcher.Find("host foo.acmecorp.network up"); len(matches) != 0 {
		t.Errorf("expected no match inside longer hostname, got %v", matches)
	}
}

func TestNewMatcher_prefersLongestMatch(t *testing.T) {
	g := New(config.GuardConfig{
		Enabled:      true,
		BlockedWords: []string{"docs.acmecorp.net", "docs.acmecorp.net/runbooks"},
	})

	matcher, err := g.NewMatcher()
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	matches := matcher.Find("link: docs.acmecorp.net/runbooks/oncall")
	if len(matches) != 1 || matches[0] != "docs.acmecorp.net/runbooks" {
		t.Errorf("expected the longer entry to win, got %v", matches)
	}
}

func TestCheckDir_filePatterns(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	addFile(t, repoPath, "main.go", []byte("// acmecorp client\n"))
	addFile(t, repoPath, "notes.txt", []byte("acmecorp\n"))

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{t.TempDir()},
		BlockedWords:  []string{"acmecorp"},
		FilePatterns:  []string{"*.go"},
	})

	findings, err := g.CheckDir(repoPath)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (*.go only), got %d: %v", len(findings), findings)
	}
	if findings[0].File != "main.go" {
		t.Errorf("expected finding in main.go, got %s", findings[0].File)
	}
}

func TestCheckDir_oversizeFileSurfacedAsSkip(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	// A tracked file larger than maxFileBytes that mentions a blocked word.
	// Its content must not be scanned, and the skip must be surfaced.
	big := []byte("acmecorp\n" + strings.Repeat("a", maxFileBytes+1))
	addFile(t, repoPath, "big.txt", big)

	var skips []SkippedFile
	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{t.TempDir()},
		BlockedWords:  []string{"acmecorp"},
	})
	g.OnSkip = func(s SkippedFile) { skips = append(skips, s) }

	findings, err := g.CheckDir(repoPath)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("oversize file content should not be scanned, got %v", findings)
	}
	if len(skips) != 1 || filepath.Base(skips[0].Path) != "big.txt" {
		t.Fatalf("expected one skip for big.txt, got %+v", skips)
	}
}

func TestCheckDir_skipsBinary(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	addFile(t, repoPath, "blob.bin", append([]byte{0x00, 0x01}, []byte("acmecorp")...))

	g := New(config.GuardConfig{
		Enabled:       true,
		WorkspaceDirs: []string{t.TempDir()},
		BlockedWords:  []string{"acmecorp"},
	})

	findings, err := g.CheckDir(repoPath)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings in binary file, got %v", findings)
	}
}

func TestStagedFiles(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepo(t, repoPath)

	f := filepath.Join(repoPath, "test.go")
	if err := os.WriteFile(f, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repoPath, "add", "test.go")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	files, err := StagedFiles(repoPath)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "test.go" {
		t.Errorf("expected [test.go], got %v", files)
	}
}
