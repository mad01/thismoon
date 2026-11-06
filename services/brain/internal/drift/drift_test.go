package drift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/testgit"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

// codeFixture is a checkout at <root>/github.com/acme/app with two commits:
// the first pins every file, the second changes a.go, drops Bar from it and
// renames b.go to c.go.
type codeFixture struct {
	root, first string
}

func newCodeFixture(t *testing.T) codeFixture {
	t.Helper()
	testhome.Isolate(t)
	testgit.Isolate(t) // drift's own git calls read no global or system config
	root := t.TempDir()
	dir := filepath.Join(root, "github.com", "acme", "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	testgit.Init(t, dir)
	moved := "package pkg\n\n// Moved stays the same across the rename, line one.\n" +
		"// Line two keeps git's rename detection sure of it.\nfunc Moved() {}\n"
	first := testgit.Commit(t, dir, "first", map[string]string{
		"pkg/a.go":    "package pkg\n\nfunc Foo() {}\n\nfunc Bar() {}\n",
		"pkg/b.go":    moved,
		"pkg/same.go": "package pkg\n\nfunc Same() {}\n",
	})
	testgit.Run(t, dir, "mv", "pkg/b.go", "pkg/c.go")
	testgit.Commit(t, dir, "second", map[string]string{
		"pkg/a.go": "package pkg\n\nfunc Foo() { println() }\n\nfunc FooBar() {}\n",
	})
	return codeFixture{root: root, first: first}
}

func (f codeFixture) ref(repo, path, symbol string) lifecycle.Source {
	r := repo + ":" + path + "@" + f.first
	if symbol != "" {
		r += "#" + symbol
	}
	return lifecycle.Source{Ref: r}
}

func TestCodeRefs(t *testing.T) {
	f := newCodeFixture(t)
	tests := []struct {
		name   string
		source lifecycle.Source
		kinds  []string
	}{
		{"unchanged", f.ref("github.com/acme/app", "pkg/same.go", "Same"), nil},
		{"changed, symbol kept", f.ref("acme/app", "pkg/a.go", "Foo"), []string{KindChanged}},
		{
			"changed, symbol gone (FooBar is not Bar)",
			f.ref("acme/app", "pkg/a.go", "Bar"),
			[]string{KindChanged, KindSymbolMissing},
		},
		{"moved", f.ref("acme/app", "pkg/b.go", ""), []string{KindMoved}},
		{"repo not checked out", f.ref("acme/other", "x.go", ""), []string{KindUnresolved}},
		{"path not at the commit", f.ref("acme/app", "pkg/none.go", ""), []string{KindUnresolved}},
		{
			"commit not in the checkout",
			lifecycle.Source{Ref: "acme/app:pkg/a.go@deadbeefdeadbeef"},
			[]string{KindUnresolved},
		},
	}
	c := New(Options{CodeRoots: []string{t.TempDir(), f.root}})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Sources(context.Background(), []lifecycle.Source{tt.source})
			if kinds := kindsOf(got); strings.Join(kinds, ",") != strings.Join(tt.kinds, ",") {
				t.Errorf("kinds = %v, want %v (%+v)", kinds, tt.kinds, got)
			}
		})
	}
	moved := c.Sources(context.Background(), []lifecycle.Source{f.ref("acme/app", "pkg/b.go", "")})
	if moved[0].MovedTo != "pkg/c.go" {
		t.Errorf("moved_to = %q, want pkg/c.go", moved[0].MovedTo)
	}
}

func TestCodeRefWithoutRoots(t *testing.T) {
	got := New(Options{}).Sources(context.Background(),
		[]lifecycle.Source{{Ref: "acme/app:a.go@abcdef1"}})
	if len(got) != 1 || got[0].Kind != KindUnresolved ||
		!strings.Contains(got[0].Message, "code_roots") {
		t.Errorf("findings = %+v, want unresolved naming code_roots", got)
	}
}

func TestExternalSources(t *testing.T) {
	sum := sha256.Sum256([]byte("stable page"))
	pinned := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable":
			_, _ = w.Write([]byte("stable page"))
		case "/edited":
			_, _ = w.Write([]byte("edited page"))
		case "/gone":
			w.WriteHeader(http.StatusGone)
		case "/missing":
			http.NotFound(w, r)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	sources := []lifecycle.Source{
		{URL: srv.URL + "/stable", ContentHash: "sha256:" + pinned},
		{URL: srv.URL + "/edited", ContentHash: "sha256:" + pinned},
		{URL: srv.URL + "/gone", Archive: "https://archive.example/gone"},
		{URL: srv.URL + "/missing"},
		{URL: srv.URL + "/down"},
	}
	offline := New(Options{})
	if got := offline.Sources(context.Background(), sources); len(got) != 0 ||
		offline.offline != len(sources) {
		t.Fatalf("offline = %+v (skipped %d), want no fetch", got, offline.offline)
	}
	got := New(Options{Online: true, AllowLocal: true}).Sources(context.Background(), sources)
	want := []string{KindHashChanged, KindGone, KindGone, KindUnresolved}
	if kinds := kindsOf(got); strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds = %v, want %v (%+v)", kinds, want, got)
	}
	if got[1].Archive != "https://archive.example/gone" ||
		!strings.Contains(got[2].Message, "no archive") {
		t.Errorf("gone findings = %+v", got[1:3])
	}
}

func kindsOf(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		out = append(out, f.Kind)
	}
	return out
}

func TestCodeRefOverTheBlobCapIsNotRead(t *testing.T) {
	f := newCodeFixture(t)
	c := New(Options{CodeRoots: []string{f.root}})
	c.git.maxBlob = 8
	got := c.Sources(context.Background(), []lifecycle.Source{f.ref("acme/app", "pkg/same.go", "Same")})
	if kinds := kindsOf(got); strings.Join(kinds, ",") != KindTooLarge {
		t.Fatalf("kinds = %v, want %s (%+v)", kinds, KindTooLarge, got)
	}
	if got[0].Drifted() {
		t.Errorf("a blob over the cap counts as drift: %+v", got[0])
	}
	// Without a symbol nothing reads the blob, so the cap does not apply.
	if got := c.Sources(context.Background(), []lifecycle.Source{f.ref("acme/app", "pkg/same.go", "")}); len(got) != 0 {
		t.Errorf("findings without a symbol = %+v, want none", got)
	}
}

// TestCodeRefInAPartialCloneFetchesNothing pins GIT_NO_LAZY_FETCH: in a
// blobless clone with no checkout the file's blob is not local, and drift
// reports that instead of fetching it from the promisor remote.
func TestCodeRefInAPartialCloneFetchesNothing(t *testing.T) {
	testhome.Isolate(t)
	testgit.Isolate(t)
	src := t.TempDir()
	testgit.Init(t, src)
	pinned := testgit.Commit(t, src, "first", map[string]string{"main.go": "func Serve() {}\n"})
	testgit.Run(t, src, "config", "uploadpack.allowFilter", "true")
	root := t.TempDir()
	dir := filepath.Join(root, "github.com", "acme", "app")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	testgit.Run(t, filepath.Dir(dir), "clone", "-q", "--filter=blob:none", "--no-checkout",
		"file://"+src, dir)
	blob := testgit.Run(t, dir, "rev-parse", "HEAD:main.go")

	got := New(Options{CodeRoots: []string{root}}).Sources(context.Background(),
		[]lifecycle.Source{{Ref: "acme/app:main.go@" + pinned + "#Serve"}})
	if len(got) != 1 || got[0].Kind != KindUnresolved ||
		!strings.Contains(got[0].Message, "not in the checkout") {
		t.Fatalf("findings = %+v, want unresolved: blob not local", got)
	}
	probe := exec.Command("git", "-C", dir, "cat-file", "-e", blob)
	probe.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=1")
	if probe.Run() == nil {
		t.Error("drift fetched the blob from the promisor remote")
	}
}

// TestGitCallsAreBoundedAndPromptless runs drift against a git that
// records its environment and hangs.
func TestGitCallsAreBoundedAndPromptless(t *testing.T) {
	bin := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "env")
	script := "#!/bin/sh\nenv > \"$FAKE_GIT_ENV\"\nexec sleep 10\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GIT_ENV", envFile)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "github.com", "acme", "app", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := New(Options{CodeRoots: []string{root}})
	c.git.timeout = 2 * time.Second // room for a first exec of a new script
	start := time.Now()
	got := c.Sources(context.Background(), []lifecycle.Source{{Ref: "acme/app:a.go@abcdef1"}})
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Errorf("drift waited %s on a hung git", elapsed)
	}
	if len(got) != 1 || got[0].Kind != KindUnresolved || !strings.Contains(got[0].Message, "timed out") {
		t.Errorf("findings = %+v, want unresolved: timed out", got)
	}
	env, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1"} {
		if !strings.Contains(string(env), want+"\n") {
			t.Errorf("git ran without %s", want)
		}
	}
}

func TestContentHashIsComparedInFull(t *testing.T) {
	sum := sha256.Sum256([]byte("stable page"))
	pinned := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("stable page"))
	}))
	defer srv.Close()
	tests := []struct {
		name, contentHash, want string
	}{
		{"full digest", "sha256:" + pinned, ""},
		{"no prefix means sha256", pinned, ""},
		{"case and hyphen ignored", "SHA-256:" + strings.ToUpper(pinned), ""},
		{"empty digest", "sha256:", KindBadHash},
		{"one character", "sha256:" + pinned[:1], KindBadHash},
		{"a prefix of the digest", pinned[:16], KindBadHash},
		{"too long", pinned + "00", KindBadHash},
		{"not hex", "sha256:" + strings.Repeat("z", 64), KindBadHash},
		{"sha512 length for sha256", "sha256:" + strings.Repeat("0", 128), KindBadHash},
		{"unknown algorithm", "md5:0123456789abcdef0123456789abcdef", KindBadHash},
		{"other full digest", "sha256:" + strings.Repeat("0", 64), KindHashChanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := lifecycle.Source{URL: srv.URL + "/page", ContentHash: tt.contentHash}
			got := New(Options{Online: true, AllowLocal: true}).Sources(context.Background(), []lifecycle.Source{source})
			if kinds := strings.Join(kindsOf(got), ","); kinds != tt.want {
				t.Errorf("kinds = %q, want %q (%+v)", kinds, tt.want, got)
			}
		})
	}
}

// TestBadContentHashNeedsNoFetch pins that a malformed content_hash is a
// finding offline too, since it says nothing about the source itself.
func TestBadContentHashNeedsNoFetch(t *testing.T) {
	c := New(Options{})
	got := c.Sources(context.Background(), []lifecycle.Source{
		{URL: "https://example.invalid/a", ContentHash: "sha256:"},
		{URL: "https://example.invalid/b", ContentHash: "sha256:" + strings.Repeat("0", 64)},
	})
	if len(got) != 1 || got[0].Kind != KindBadHash || !got[0].Drifted() || c.offline != 1 {
		t.Errorf("findings = %+v (skipped %d), want one bad-hash and one skipped", got, c.offline)
	}
}

func TestOnlineRefusesLocalAddressesUnlessAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("internal admin page"))
	}))
	defer srv.Close()
	source := []lifecycle.Source{{URL: srv.URL + "/admin"}}
	got := New(Options{Online: true}).Sources(context.Background(), source)
	if len(got) != 1 || got[0].Kind != KindUnresolved ||
		!strings.Contains(got[0].Message, "private address") {
		t.Errorf("findings = %+v, want unresolved: a local address is refused", got)
	}
	if got := New(Options{Online: true, AllowLocal: true}).Sources(context.Background(), source); len(got) != 0 {
		t.Errorf("findings with AllowLocal = %+v, want none", got)
	}
}

func TestOnlineFollowsOnlyWebRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/hosts", http.StatusFound)
	}))
	defer srv.Close()
	got := New(Options{Online: true, AllowLocal: true}).Sources(context.Background(),
		[]lifecycle.Source{{URL: srv.URL + "/hop"}})
	if len(got) != 1 || got[0].Kind != KindUnresolved ||
		!strings.Contains(got[0].Message, "only http and https") {
		t.Errorf("findings = %+v, want unresolved: a file redirect is refused", got)
	}
}

func TestRefuseLocal(t *testing.T) {
	tests := []struct {
		address string
		refused bool
	}{
		{"127.0.0.1:80", true},
		{"127.10.0.1:80", true},
		{"[::1]:80", true},
		{"0.0.0.0:80", true},
		{"[::]:80", true},
		{"10.1.2.3:443", true},
		{"172.16.0.1:443", true},
		{"192.168.1.1:443", true},
		{"169.254.169.254:80", true},
		{"[fe80::1%en0]:80", true},
		{"[fd00::1]:80", true},
		{"[::ffff:127.0.0.1]:80", true},
		{"[::ffff:10.0.0.1]:80", true},
		{"100.64.0.1:80", true},
		{"224.0.0.1:80", true},
		{"93.184.216.34:443", false},
		{"[2606:2800:220:1:248:1893:25c8:1946]:443", false},
		{"100.128.0.1:80", false},
	}
	for _, tt := range tests {
		err := refuseLocal("tcp", tt.address, nil)
		if (err != nil) != tt.refused {
			t.Errorf("refuseLocal(%s) = %v, want refused %v", tt.address, err, tt.refused)
		}
	}
}

// TestOnlineBudgetBoundsEveryFetch runs fetches against a server that
// never answers: the run stops at the budget, not at one fetch timeout per
// source.
func TestOnlineBudgetBoundsEveryFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()
	var sources []lifecycle.Source
	for _, p := range []string{"/a", "/b", "/c", "/d"} {
		sources = append(sources, lifecycle.Source{URL: srv.URL + p})
	}
	c := New(Options{Online: true, AllowLocal: true, Budget: 300 * time.Millisecond})
	start := time.Now()
	got := c.Sources(context.Background(), sources)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the run took %s, want it bounded by the 300ms budget", elapsed)
	}
	if len(got) != len(sources) {
		t.Fatalf("findings = %+v, want one per source", got)
	}
	for _, f := range got {
		if f.Kind != KindUnresolved || !strings.Contains(f.Message, "budget") {
			t.Errorf("finding = %+v, want unresolved: the budget ran out", f)
		}
	}
}
