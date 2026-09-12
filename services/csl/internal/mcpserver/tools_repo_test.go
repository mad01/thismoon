package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// makeFakeRepo creates a directory that looks like a git repo to finder.Walk:
// a .git/config file with an [remote "origin"] url. No git subprocess needed.
func makeFakeRepo(t *testing.T, root, org, repoName string) string {
	t.Helper()
	dir := filepath.Join(root, org, repoName)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	cfg := "[remote \"origin\"]\n\turl = git@github.com:" + org + "/" + repoName + ".git\n"
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write git config: %v", err)
	}
	return dir
}

// setupRepoEnv builds a fake HOME with a csl config pointing at a temp root
// containing the given repos (one per "org/name" pair). Returns the temp root.
func setupRepoEnv(t *testing.T, repos []struct{ Org, Name string }) string {
	t.Helper()
	tmp := t.TempDir()
	reposRoot := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(reposRoot, 0o755); err != nil {
		t.Fatalf("mkdir repos root: %v", err)
	}
	for _, r := range repos {
		makeFakeRepo(t, reposRoot, r.Org, r.Name)
	}

	cfgDir := filepath.Join(tmp, ".config", "csl")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := "dirs:\n  - " + reposRoot + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	isolateConfigEnv(t, tmp)
	return reposRoot
}

// TestResolveRepoWhitespace covers the whitespace normalization bug:
// users often type or paste a repo name with trailing/leading whitespace,
// including exotic Unicode spaces (NBSP, narrow no-break space, zero-width
// no-break space). Matching must tolerate all of these.
func TestResolveRepoWhitespace(t *testing.T) {
	setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
		{"mad01", "other-repo"},
		{"someone", "unrelated"},
	})

	tests := []struct {
		name    string
		query   string
		wantErr bool
		wantHit string // expected repo.Name on success
	}{
		{name: "clean match", query: "octo", wantHit: "mad01/octo"},
		{name: "trailing ascii space", query: "octo ", wantHit: "mad01/octo"},
		{name: "leading ascii space", query: " octo", wantHit: "mad01/octo"},
		{name: "leading and trailing space", query: "  octo  ", wantHit: "mad01/octo"},
		{name: "trailing tab", query: "octo\t", wantHit: "mad01/octo"},
		{name: "trailing newline", query: "octo\n", wantHit: "mad01/octo"},
		{name: "non-breaking space (U+00A0)", query: "octo ", wantHit: "mad01/octo"},
		{name: "narrow no-break space (U+202F)", query: "octo ", wantHit: "mad01/octo"},
		{name: "ideographic space (U+3000)", query: "octo　", wantHit: "mad01/octo"},
		{name: "mixed case still works", query: "OCTO ", wantHit: "mad01/octo"},
		{name: "whitespace-only is rejected", query: "   ", wantErr: true},
		{name: "empty is rejected", query: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := resolveRepo(tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveRepo(%q) = %+v, want error", tc.query, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRepo(%q) error: %v", tc.query, err)
			}
			if repo.Name != tc.wantHit {
				t.Errorf("resolveRepo(%q).Name = %q, want %q", tc.query, repo.Name, tc.wantHit)
			}
		})
	}
}

// TestResolveRepoExactDisambiguation checks that trailing whitespace does not
// defeat the exact-match tiebreaker when multiple repos fuzzy-match.
func TestResolveRepoExactDisambiguation(t *testing.T) {
	setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
		{"mad01", "octopus"},
	})

	// Query "octo" fuzzy-matches both; the exact-match tiebreaker should
	// still pick "mad01/octo" even when the query has trailing whitespace.
	repo, err := resolveRepo("mad01/octo ")
	if err != nil {
		t.Fatalf("resolveRepo: %v", err)
	}
	if repo.Name != "mad01/octo" {
		t.Errorf("expected exact match to win, got %q", repo.Name)
	}
}

// TestHandleRepoLookupWhitespace verifies the MCP tool layer handles
// whitespace-padded names gracefully.
func TestHandleRepoLookupWhitespace(t *testing.T) {
	setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
		{"someone", "unrelated"},
	})

	queries := []string{"octo", "octo ", " octo", "octo ", "OCTO\t"}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			_, out, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Name: q})
			if err != nil {
				t.Fatalf("handleRepoLookup(%q) error: %v", q, err)
			}
			if len(out.Matches) == 0 {
				t.Fatalf("handleRepoLookup(%q) returned no matches", q)
			}
			found := false
			for _, m := range out.Matches {
				if m.Name == "mad01/octo" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("handleRepoLookup(%q) did not return mad01/octo; got %+v", q, out.Matches)
			}
		})
	}
}

// TestHandleRepoLookupRejectsEmpty ensures whitespace-only names are rejected
// the same way empty names are.
func TestHandleRepoLookupRejectsEmpty(t *testing.T) {
	setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
	})

	for _, q := range []string{"", " ", "\t\n", " "} {
		_, _, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Name: q})
		if err == nil {
			t.Errorf("handleRepoLookup(%q) expected error, got nil", q)
		}
	}
}

// TestHandleRepoLookupReportsDropped: a repo hidden by index.hosts comes back
// under dropped with its reason instead of as bare empty matches, so an agent
// does not report a checkout that exists as missing.
func TestHandleRepoLookupReportsDropped(t *testing.T) {
	reposRoot := setupRepoEnv(t, []struct{ Org, Name string }{{"mad01", "octo"}})
	cfgPath := filepath.Join(filepath.Dir(reposRoot), ".config", "csl", "config.yaml")
	cfg := "dirs:\n  - " + reposRoot + "\nindex:\n  hosts:\n    - nowhere.example\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, out, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Name: "octo"})
	if err != nil {
		t.Fatalf("handleRepoLookup: %v", err)
	}
	if len(out.Matches) != 0 {
		t.Fatalf("matches = %+v, want none: the host filter should have dropped it", out.Matches)
	}
	if len(out.Dropped) != 1 || out.Dropped[0].Name != "mad01/octo" {
		t.Fatalf("dropped = %+v, want just mad01/octo", out.Dropped)
	}
	if !strings.Contains(out.Dropped[0].Reason, "index.hosts") {
		t.Errorf("reason = %q, want it to name index.hosts", out.Dropped[0].Reason)
	}
}

// writeDescriptor gives a fake repo a root catalog descriptor.
func writeDescriptor(t *testing.T, dir, name, owner, system string) {
	t.Helper()
	content := "apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: " + name +
		"\nspec:\n  owner: " + owner + "\n  system: " + system + "\n"
	if err := os.WriteFile(filepath.Join(dir, "catalog-info.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}
}

// TestHandleRepoLookupCatalogFilters pins the catalog side of the tool:
// owner, system, and component select by the root descriptor, all set
// fields must agree, a repo with no descriptor never matches a catalog
// field, and every match carries the descriptor's fields.
func TestHandleRepoLookupCatalogFilters(t *testing.T) {
	root := setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "code-search-local"},
		{"mad01", "ralph"},
		{"mad01", "dotfiles"},
	})
	writeDescriptor(
		t,
		filepath.Join(root, "mad01", "code-search-local"),
		"csl",
		"group:default/platform",
		"thismoon",
	)
	writeDescriptor(t, filepath.Join(root, "mad01", "ralph"), "ralph", "mad01", "thismoon")

	// Discovery walks concurrently, so match order is not stable; sort.
	names := func(out repoLookupOutput) []string {
		var got []string
		for _, m := range out.Matches {
			got = append(got, m.Name)
		}
		sort.Strings(got)
		return got
	}
	tests := []struct {
		name string
		in   repoLookupInput
		want []string
	}{
		{
			"by system",
			repoLookupInput{System: "thismoon"},
			[]string{"mad01/code-search-local", "mad01/ralph"},
		},
		{
			"by owner regex",
			repoLookupInput{Owner: "^group:.*/platform$"},
			[]string{"mad01/code-search-local"},
		},
		{
			"by component name that differs from the repo name",
			repoLookupInput{Component: "^csl$"},
			[]string{"mad01/code-search-local"},
		},
		{
			"name and owner together",
			repoLookupInput{Name: "mad01", Owner: "mad01"},
			[]string{"mad01/ralph"},
		},
		{
			"permissive owner skips repos without a descriptor",
			repoLookupInput{Owner: ".*"},
			[]string{"mad01/code-search-local", "mad01/ralph"},
		},
		{"no such system", repoLookupInput{System: "shop"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, out, err := handleRepoLookup(context.Background(), nil, tc.in)
			if err != nil {
				t.Fatalf("handleRepoLookup(%+v): %v", tc.in, err)
			}
			got := names(out)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("matches = %v, want %v", got, tc.want)
			}
		})
	}

	_, out, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Name: "code-search"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Matches) != 1 {
		t.Fatalf("matches = %+v, want one", out.Matches)
	}
	m := out.Matches[0]
	if m.Component != "csl" || m.Owner != "group:default/platform" || m.System != "thismoon" {
		t.Errorf("match = %+v, want the descriptor's component, owner, and system", m)
	}
}

// TestHandleRepoLookupRequiresSomeField: with no name the tool needs a
// catalog field, and an invalid pattern is reported against its own field.
func TestHandleRepoLookupRequiresSomeField(t *testing.T) {
	setupRepoEnv(t, []struct{ Org, Name string }{{"mad01", "octo"}})

	_, _, err := handleRepoLookup(context.Background(), nil, repoLookupInput{})
	if err == nil {
		t.Fatal("handleRepoLookup with no fields = nil error, want one")
	}
	_, _, err = handleRepoLookup(context.Background(), nil, repoLookupInput{System: "("})
	if err == nil || !strings.Contains(err.Error(), "system") {
		t.Errorf("bad system pattern error = %v, want one naming system", err)
	}
}

// TestHandleRepoLookupDroppedCarriesCatalog: the dropped fallback filters by
// the same query and carries the descriptor fields, so "csl saw it, a filter
// hid it" still answers an owner question.
func TestHandleRepoLookupDroppedCarriesCatalog(t *testing.T) {
	root := setupRepoEnv(t, []struct{ Org, Name string }{{"mad01", "octo"}})
	writeDescriptor(t, filepath.Join(root, "mad01", "octo"), "octo", "team-x", "sea")
	cfgPath := filepath.Join(filepath.Dir(root), ".config", "csl", "config.yaml")
	cfg := "dirs:\n  - " + root + "\nindex:\n  hosts:\n    - nowhere.example\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, out, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Owner: "team-x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Matches) != 0 || len(out.Dropped) != 1 {
		t.Fatalf("out = %+v, want no matches and one dropped", out)
	}
	if d := out.Dropped[0]; d.Owner != "team-x" || d.System != "sea" || d.Component != "octo" {
		t.Errorf("dropped = %+v, want the descriptor fields", d)
	}
}
