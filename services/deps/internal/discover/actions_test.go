package discover

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// thismoon-style workflow: every action pinned to a commit SHA with its
// version in a trailing comment, a local composite action, the same action in
// two jobs, and a docker:// step.
const shaPinnedWorkflow = `name: ci
on: [push]
jobs:
  test:
    runs-on: macos-15
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
      - uses: ./.github/actions/present-image
      - uses: docker://alpine:3.20
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7
      - uses: golangci/golangci-lint-action@BA0D7D2EC06A0EA1CB5FA41B2E4A3AB91D21278A
`

// thismoon's present-image composite action, trimmed.
const compositeAction = `name: present image
runs:
  using: composite
  steps:
    - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
    - if: inputs.push == 'true'
      uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0
`

// A tag-pinned workflow (dropbrain-app style), with a branch-pinned reusable
// workflow, a subpath action, a commented-out step, and an action input that
// happens to be called uses.
const tagPinnedWorkflow = `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      # - uses: actions/upload-artifact@v3
      - uses: github/codeql-action/init@v3
        with:
          uses: not/a-ref@v1
  shared:
    uses: acme/workflows/.github/workflows/release.yml@main
`

const (
	checkoutV4Tag    = "1111111111111111111111111111111111111111" // annotated tag object
	checkoutV4Commit = "11d5960a326750d5838078e36cf38b85af677262"
	codeqlV3Commit   = "2222222222222222222222222222222222222222"
	acmeMainCommit   = "3333333333333333333333333333333333333333"
)

// pkt frames s as one pkt-line.
func pkt(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }

// advertisement renders a ref list the way github.com's info/refs does.
func advertisement(lines ...string) string {
	var b strings.Builder
	b.WriteString(pkt("# service=git-upload-pack\n") + "0000")
	for i, l := range lines {
		if i == 0 {
			l += "\x00multi_ack side-band-64k symref=HEAD:refs/heads/main"
		}
		b.WriteString(pkt(l + "\n"))
	}
	b.WriteString("0000")
	return b.String()
}

// refServer serves canned ref advertisements per owner/repo and answers 401
// for any other repo, as github.com does for one that is missing or private.
// It counts the requests per repo.
func refServer(t *testing.T) (*httptest.Server, func(repo string) int) {
	t.Helper()
	ads := map[string]string{
		"actions/checkout": advertisement(
			checkoutV4Commit+" HEAD",
			checkoutV4Tag+" refs/tags/v4",
			checkoutV4Commit+" refs/tags/v4^{}",
			checkoutV4Commit+" refs/heads/main",
		),
		"github/codeql-action": advertisement(codeqlV3Commit + " refs/tags/v3"),
		"acme/workflows": advertisement(
			acmeMainCommit + " refs/heads/main",
		),
	}
	var (
		mu   sync.Mutex
		hits = map[string]int{}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".git/info/refs")
		mu.Lock()
		hits[repo]++
		mu.Unlock()
		if r.URL.Query().Get("service") != "git-upload-pack" {
			t.Errorf("request %s lacks service=git-upload-pack", r.URL)
		}
		ad, ok := ads[repo]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", advertisementType)
		_, _ = w.Write([]byte(ad))
	}))
	t.Cleanup(srv.Close)
	return srv, func(repo string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[repo]
	}
}

// writeRepo lays out files (repo-relative path to content) under a new repo
// root.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// byName indexes packages by name, failing on a duplicate.
func byName(t *testing.T, pkgs []Package) map[string]Package {
	t.Helper()
	out := map[string]Package{}
	for _, p := range pkgs {
		if _, dup := out[p.Name]; dup {
			t.Errorf("package %s listed twice", p.Name)
		}
		out[p.Name] = p
	}
	return out
}

func TestGitHubActionsSHAPins(t *testing.T) {
	srv, hits := refServer(t)
	root := writeRepo(t, map[string]string{
		".github/workflows/ci.yml":                    shaPinnedWorkflow,
		".github/workflows/notes.md":                  "uses: not/a-workflow@v1",
		".github/actions/present-image/action.yml":    compositeAction,
		"services/x/.github/workflows/ignored.yml":    tagPinnedWorkflow,
		".github/actions/present-image/README.md":     "nothing",
		".github/actions/nested/.git":                 "gitdir: elsewhere",
		".github/actions/nested/inner/action.yaml":    tagPinnedWorkflow,
		".github/workflows/sub/ignored-by-github.yml": tagPinnedWorkflow,
	})

	pkgs, err := GitHubActions{BaseURL: srv.URL}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	ci := filepath.Join(root, ".github/workflows/ci.yml")
	action := filepath.Join(root, ".github/actions/present-image/action.yml")
	want := map[string]Package{
		"actions/checkout": {
			Version:      "v7+9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0",
			ManifestPath: ci,
		},
		"actions/setup-go": {
			Version:      "v6+924ae3a1cded613372ab5595356fb5720e22ba16",
			ManifestPath: ci,
		},
		"golangci/golangci-lint-action": {
			Version:      "ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a",
			ManifestPath: ci,
		},
		"docker/setup-buildx-action": {
			Version:      "v4.4.1+f87e5991a6d7451dcb8d9637bfbc97413f497069",
			ManifestPath: action,
		},
		"docker/login-action": {
			Version:      "v4.6.0+dbcb813823bdd20940b903addbd779551569679f",
			ManifestPath: action,
		},
	}
	got := byName(t, pkgs)
	if len(got) != len(want) {
		t.Errorf("got %d packages, want %d: %+v", len(got), len(want), pkgs)
	}
	for name, w := range want {
		p, ok := got[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if p.Version != w.Version || p.ManifestPath != w.ManifestPath {
			t.Errorf("%s = %s in %s, want %s in %s",
				name, p.Version, p.ManifestPath, w.Version, w.ManifestPath)
		}
		if p.Ecosystem != ActionsEcosystem || !p.Direct || !p.Imported || len(p.Advisories) != 0 {
			t.Errorf(
				"%s = %+v, want a direct, imported GitHub Actions package with no signal",
				name,
				p,
			)
		}
	}
	if hits("actions/checkout") != 0 {
		t.Error("a SHA pin fetched its ref list")
	}
}

func TestGitHubActionsResolvesTagsAndBranches(t *testing.T) {
	srv, _ := refServer(t)
	root := writeRepo(t, map[string]string{".github/workflows/build.yaml": tagPinnedWorkflow})

	pkgs, err := GitHubActions{BaseURL: srv.URL}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	got := byName(t, pkgs)
	want := map[string]struct{ version, summary string }{
		// The annotated tag resolves to the commit it points to, not the tag object.
		"actions/checkout":     {"v4+" + checkoutV4Commit, "pinned to the tag v4"},
		"github/codeql-action": {"v3+" + codeqlV3Commit, "pinned to the tag v3"},
		"acme/workflows":       {"main+" + acmeMainCommit, "pinned to the branch main"},
	}
	if len(got) != len(want) {
		t.Errorf("got %d packages, want %d (commented-out and input refs skipped): %+v",
			len(got), len(want), pkgs)
	}
	for name, w := range want {
		p := got[name]
		if p.Version != w.version {
			t.Errorf("%s version = %q, want %q", name, p.Version, w.version)
		}
		if len(p.Advisories) != 1 {
			t.Errorf("%s advisories = %+v, want the movable-ref signal", name, p.Advisories)
			continue
		}
		a := p.Advisories[0]
		if a.ID != "deps-movable-ref" || a.Kind != store.KindSignal ||
			!strings.HasPrefix(a.Summary, w.summary) {
			t.Errorf("%s signal = %+v, want deps-movable-ref %q", name, a, w.summary)
		}
	}
}

// TestGitHubActionsUnresolvedRefIsAnError: a tag that can't be resolved is
// left out and named in the error, so it can't pass for clean, while the rest
// of the repo's refs still come back.
func TestGitHubActionsUnresolvedRefIsAnError(t *testing.T) {
	srv, hits := refServer(t)
	root := writeRepo(t, map[string]string{".github/workflows/ci.yml": `jobs:
  a:
    steps:
      - uses: private/action@v1
      - uses: actions/checkout@v9
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
  b:
    steps:
      - uses: private/action@v1
`})

	pkgs, err := GitHubActions{BaseURL: srv.URL}.Discover(root, Options{})
	if err == nil {
		t.Fatal("Discover: no error for refs it could not resolve")
	}
	for _, want := range []string{
		"resolve private/action@v1: ref list answered 401",
		"resolve actions/checkout@v9: no such tag or branch",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if n := strings.Count(err.Error(), "private/action@v1"); n != 1 {
		t.Errorf("private/action@v1 named %d times, want once", n)
	}
	if hits("private/action") != 1 {
		t.Errorf("private/action fetched %d times, want once", hits("private/action"))
	}
	if len(pkgs) != 1 || pkgs[0].Name != "actions/setup-go" {
		t.Errorf("packages = %+v, want only the SHA-pinned actions/setup-go", pkgs)
	}
}

// TestReposKeepsActionsBesideTheirError: Repos records the unresolved ref as a
// discovery error and still keeps the refs that were read, and it fetches an
// action repo's refs once per walk however many repos pin it.
func TestReposKeepsActionsBesideTheirError(t *testing.T) {
	srv, hits := refServer(t)
	wf := `jobs:
  a:
    steps:
      - uses: actions/checkout@v4
      - uses: private/action@v1
`
	r1 := writeRepo(t, map[string]string{".github/workflows/ci.yml": wf})
	r2 := writeRepo(t, map[string]string{".github/workflows/ci.yml": wf})

	deps, errs := Repos([]string{r1, r2}, []Ecosystem{GitHubActions{BaseURL: srv.URL}}, Options{})

	if len(deps) != 2 {
		t.Errorf("deps = %+v, want actions/checkout from both repos", deps)
	}
	if len(errs) != 2 {
		t.Fatalf("discovery errors = %+v, want one per repo", errs)
	}
	for i, repo := range []string{r1, r2} {
		e := errs[i]
		if e.Repo != repo || e.Ecosystem != "GitHub Actions" ||
			!strings.Contains(e.Error, "private/action@v1") {
			t.Errorf("errs[%d] = %+v, want private/action@v1 in %s", i, e, repo)
		}
	}
	if hits("actions/checkout") != 1 || hits("private/action") != 1 {
		t.Errorf("fetches: checkout %d, private %d; want 1 each per walk",
			hits("actions/checkout"), hits("private/action"))
	}

	// The next walk is a new scan and fetches again, so a moved tag shows.
	Repos([]string{r1}, []Ecosystem{GitHubActions{BaseURL: srv.URL}}, Options{})
	if hits("actions/checkout") != 2 {
		t.Errorf("checkout fetched %d times over two walks, want 2", hits("actions/checkout"))
	}
}

func TestGitHubActionsHonorsExcludePaths(t *testing.T) {
	srv, _ := refServer(t)
	root := writeRepo(t, map[string]string{
		".github/workflows/ci.yml":                 shaPinnedWorkflow,
		".github/actions/present-image/action.yml": compositeAction,
	})
	for _, excluded := range []string{".github", ".github/workflows"} {
		opts := Options{ExcludePath: func(rel string) bool { return rel == excluded }}
		pkgs, err := GitHubActions{BaseURL: srv.URL}.Discover(root, opts)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		for _, p := range pkgs {
			if strings.Contains(p.ManifestPath, excluded+"/") {
				t.Errorf("excluding %s still found %s in %s", excluded, p.Name, p.ManifestPath)
			}
		}
	}
}

func TestParseUses(t *testing.T) {
	tests := []struct {
		uses, comment string
		want          actionRef
		ok            bool
	}{
		{"actions/checkout@v4", "", actionRef{"actions/checkout", "v4", ""}, true},
		{
			"github/codeql-action/init@v3",
			"# v3.27.0",
			actionRef{"github/codeql-action", "v3", "v3.27.0"},
			true,
		},
		{
			"a/b@" + checkoutV4Commit,
			"#v4.2.2 latest",
			actionRef{"a/b", checkoutV4Commit, "v4.2.2"},
			true,
		},
		{"a/b@" + checkoutV4Commit, "# pin@v2", actionRef{"a/b", checkoutV4Commit, ""}, true},
		{"./.github/actions/x", "", actionRef{}, false},
		{"docker://alpine:3", "", actionRef{}, false},
		{"actions/checkout", "", actionRef{}, false},
		{"actions/checkout@", "", actionRef{}, false},
		{"checkout@v4", "", actionRef{}, false},
		{"a/b@v1@v2", "", actionRef{}, false},
		{"a/b@v1:x", "", actionRef{}, false},
	}
	for _, tt := range tests {
		got, ok := parseUses(tt.uses, tt.comment)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseUses(%q, %q) = %+v, %v; want %+v, %v",
				tt.uses, tt.comment, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSplitActionVersion(t *testing.T) {
	const sha = "a284dc1814e3fd07f2e34267fc8f81227ed29fb8"
	tests := []struct{ version, label, commit string }{
		{"v45.0.7+" + sha, "v45.0.7", sha},
		{sha, "", sha},
		{"v1.0.0+build+" + sha, "v1.0.0+build", sha},
		{"v45.0.7", "v45.0.7", ""},
		{"v1+abc", "v1+abc", ""},
	}
	for _, tt := range tests {
		label, commit := SplitActionVersion(tt.version)
		if label != tt.label || commit != tt.commit {
			t.Errorf("SplitActionVersion(%q) = %q, %q; want %q, %q",
				tt.version, label, commit, tt.label, tt.commit)
		}
		if tt.commit != "" && actionVersion(label, commit) != tt.version {
			t.Errorf("actionVersion(%q, %q) does not round-trip %q", label, commit, tt.version)
		}
	}
}

func TestParseAdvertisementRejectsTruncation(t *testing.T) {
	ad := advertisement(checkoutV4Commit + " refs/tags/v4")
	if _, err := parseAdvertisement([]byte(ad[:len(ad)-10])); err == nil {
		t.Error("a ref list cut inside a pkt-line parsed without error")
	}
}
