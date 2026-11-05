package drift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	testhome.Isolate(t) // git reads no global config of the machine's
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
		{URL: srv.URL + "/stable", ContentHash: pinned[:16]},
		{URL: srv.URL + "/edited", ContentHash: "sha256:" + pinned},
		{URL: srv.URL + "/gone", Archive: "https://archive.example/gone"},
		{URL: srv.URL + "/missing"},
		{URL: srv.URL + "/down"},
		{URL: srv.URL + "/stable", ContentHash: "md5:0123456789abcdef"},
	}
	offline := New(Options{})
	if got := offline.Sources(context.Background(), sources); len(got) != 0 ||
		offline.offline != len(sources) {
		t.Fatalf("offline = %+v (skipped %d), want no fetch", got, offline.offline)
	}
	got := New(Options{Online: true}).Sources(context.Background(), sources)
	want := []string{KindHashChanged, KindGone, KindGone, KindUnresolved, KindUnresolved}
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
