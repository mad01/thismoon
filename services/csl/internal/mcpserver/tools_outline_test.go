package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/csl/internal/outline"
)

// setupOutlineTestRepo lays out one discoverable repo where a.go defines Foo
// and NewFoo, b.go references both, and a_test.go references Foo, then
// points csl at it.
func setupOutlineTestRepo(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	reposRoot := filepath.Join(tmp, "workspace")
	repoDir := filepath.Join(reposRoot, "org", "testrepo")
	files := map[string]string{
		".git/config": "[remote \"origin\"]\n\turl = git@github.com:org/testrepo.git\n",
		"a.go":        "package fx\n\ntype Foo struct{}\n\nfunc NewFoo() *Foo { return nil }\n",
		"b.go":        "package fx\n\nvar y = NewFoo()\nvar z Foo\n",
		"a_test.go":   "package fx\n\nfunc TestFoo() { _ = Foo{} }\n",
	}
	for name, content := range files {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfgDir := filepath.Join(tmp, ".config", "csl")
	_ = os.MkdirAll(cfgDir, 0o755)
	cslCfg := "dirs:\n  - " + reposRoot + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cslCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	isolateConfigEnv(t, tmp)
}

// TestHandleOutline_RanksAndRenders: the handler resolves the repo by regex,
// builds the ranked outline, and the text format is the shared renderer's
// grouped form.
func TestHandleOutline_RanksAndRenders(t *testing.T) {
	setupOutlineTestRepo(t)

	_, out, err := handleOutline(context.Background(), nil, outlineInput{Repo: "TESTREPO$"})
	if err != nil {
		t.Fatalf("handleOutline: %v", err)
	}
	if out.Repo != "org/testrepo" || out.Path != "." {
		t.Errorf("repo/path = %q/%q, want org/testrepo/.", out.Repo, out.Path)
	}
	if out.FilesScanned != 2 || out.SymbolsTotal != 4 || out.Truncated {
		t.Errorf("counts = %d files, %d symbols, truncated %v; want 2, 4, false",
			out.FilesScanned, out.SymbolsTotal, out.Truncated)
	}
	if len(out.Symbols) != 4 || out.Symbols[0].Name != "Foo" || out.Symbols[0].Refs != 1 {
		t.Fatalf("symbols = %+v, want Foo first with 1 ref", out.Symbols)
	}

	got := outline.Render(out)
	want := strings.Join([]string{
		"org/testrepo/a.go",
		"3 struct Foo  refs=1",
		"5 function NewFoo  refs=1",
		"",
		"org/testrepo/b.go",
		"3 var y  refs=0",
		"4 var z  refs=0",
		"",
		"4 symbols in 2 files; 2 files scanned",
	}, "\n")
	if got != want {
		t.Errorf("rendered text differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestHandleOutline_PassesOptionsThrough: path, kinds, limit and
// include_tests reach the builder.
func TestHandleOutline_PassesOptionsThrough(t *testing.T) {
	setupOutlineTestRepo(t)

	_, out, err := handleOutline(context.Background(), nil, outlineInput{
		Repo: "org/testrepo", Kinds: []string{"function"}, Limit: 1, IncludeTests: true,
	})
	if err != nil {
		t.Fatalf("handleOutline: %v", err)
	}
	if len(out.Symbols) != 1 || out.Symbols[0].Name != "NewFoo" {
		t.Errorf("symbols = %+v, want just NewFoo", out.Symbols)
	}
	if !out.Truncated || out.SymbolsTotal != 2 || out.FilesScanned != 3 {
		t.Errorf("truncated %v, total %d, scanned %d; want true, 2 (NewFoo, TestFoo), 3",
			out.Truncated, out.SymbolsTotal, out.FilesScanned)
	}
}

func TestHandleOutline_Errors(t *testing.T) {
	setupOutlineTestRepo(t)
	cases := map[string]outlineInput{
		"missing repo":   {},
		"negative limit": {Repo: "testrepo", Limit: -1},
		"unknown repo":   {Repo: "nosuchrepo"},
		"unknown path":   {Repo: "testrepo", Path: "nope"},
		"unknown kind":   {Repo: "testrepo", Kinds: []string{"widget"}},
	}
	for name, in := range cases {
		if _, _, err := handleOutline(context.Background(), nil, in); err == nil {
			t.Errorf("%s: handleOutline succeeded, want an error", name)
		}
	}
}

// TestOutlineTool_Schema pins the advertised contract: repo is the only
// required field, the options are optional, and response_format is there
// with its enum.
func TestOutlineTool_Schema(t *testing.T) {
	var tool *mcp.Tool
	for _, tl := range mcptest.ListTools(t, New("test", Options{})) {
		if tl.Name == "csl_outline" {
			tool = tl
		}
	}
	if tool == nil {
		t.Fatal("csl_outline is not registered")
	}
	props, required := schemaProperties(t, tool.InputSchema)
	for _, p := range []string{"repo", "path", "kinds", "limit", "include_tests", "response_format"} {
		if _, ok := props[p]; !ok {
			t.Errorf("input schema lacks %s", p)
		}
	}
	if len(required) != 1 || required[0] != "repo" {
		t.Errorf("required = %v, want [repo]", required)
	}
}
