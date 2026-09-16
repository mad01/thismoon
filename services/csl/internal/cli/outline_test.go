package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/outline"
)

// setupOutlineRepo lays out one git repo where a.go defines Foo and NewFoo,
// b.go references both, and sub/c.go defines Baz, and points csl at it.
func setupOutlineRepo(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "org", "myrepo")
	files := map[string]string{
		"a.go":     "package fx\n\ntype Foo struct{}\n\nfunc NewFoo() *Foo { return nil }\n",
		"b.go":     "package fx\n\nvar y = NewFoo()\nvar z Foo\n",
		"sub/c.go": "package sub\n\nfunc Baz() {}\n",
	}
	for name, content := range files {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:testorg/myrepo.git")
	setupTestConfig(t, "dirs:\n  - "+tmp+"\n")
}

// runOutlineCmd runs `csl outline` with fresh flags and fails the test on
// error.
func runOutlineCmd(t *testing.T, args ...string) string {
	t.Helper()
	reset := func() {
		outlineKindsFlag, outlineLimitFlag = nil, outline.DefaultLimit
		outlineIncludeTestsFlag, outlineJSONFlag = false, false
		outlineMaxFilesFlag = outline.DefaultMaxFiles
	}
	reset()
	t.Cleanup(reset)
	out, err := runCLI(t, append([]string{"outline"}, args...)...)
	if err != nil {
		t.Fatalf("csl outline %v: %v", args, err)
	}
	return out
}

// TestOutlineCmd_Text: the command prints the shared grouped rendering with
// a trailing newline.
func TestOutlineCmd_Text(t *testing.T) {
	setupOutlineRepo(t)
	got := runOutlineCmd(t, "myrepo")
	want := strings.Join([]string{
		"testorg/myrepo/a.go",
		"3 struct Foo  refs=1",
		"5 function NewFoo  refs=1",
		"",
		"testorg/myrepo/sub/c.go",
		"3 function Baz  refs=0",
		"",
		"testorg/myrepo/b.go",
		"3 var y  refs=0",
		"4 var z  refs=0",
		"",
		"5 symbols in 3 files; 3 files scanned",
		"",
	}, "\n")
	if got != want {
		t.Errorf("output differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestOutlineCmd_JSON: --json emits the same object the MCP tool returns,
// and the path argument narrows the definitions.
func TestOutlineCmd_JSON(t *testing.T) {
	setupOutlineRepo(t)
	got := runOutlineCmd(t, "myrepo", "sub", "--json", "--limit", "1")

	var res outline.Result
	if err := json.Unmarshal([]byte(got), &res); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, got)
	}
	if res.Repo != "testorg/myrepo" || res.Path != "sub" {
		t.Errorf("repo/path = %q/%q, want testorg/myrepo/sub", res.Repo, res.Path)
	}
	if res.FilesScanned != 3 || res.SymbolsTotal != 1 || res.Truncated {
		t.Errorf("counts = %d files, %d symbols, truncated %v; want 3, 1, false",
			res.FilesScanned, res.SymbolsTotal, res.Truncated)
	}
	if len(res.Symbols) != 1 || res.Symbols[0].Name != "Baz" || res.Symbols[0].File != "sub/c.go" {
		t.Errorf("symbols = %+v, want Baz in sub/c.go", res.Symbols)
	}
}

func TestOutlineCmd_UnknownRepoErrors(t *testing.T) {
	setupOutlineRepo(t)
	if _, err := runCLI(t, "outline", "nosuchrepo"); err == nil {
		t.Fatal("csl outline nosuchrepo succeeded, want an error")
	}
}
