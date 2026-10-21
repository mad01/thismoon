package pypath

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

// The wants below are what Python 3.13 printed for the same input.

func TestCleanAndJoinFollowPathlib(t *testing.T) {
	for in, want := range map[string]string{
		"":            ".",
		".":           ".",
		"./x":         "x",
		"/a//b/./c/":  "/a/b/c",
		"a/../b":      "a/../b",
		"//a/b":       "//a/b",
		"///a":        "/a",
		"~/sources/x": "~/sources/x",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range []struct{ base, rel, want string }{
		{"/mkt", "./mods/x", "/mkt/mods/x"},
		{"/mkt", "/abs/y", "/abs/y"},
		{"/", "x", "/x"},
		{"//", "x", "//x"},
		{"//net", "x", "//net/x"},
		{"rel", "x", "rel/x"},
		{"", "x", "x"},
		{"/a", "", "/a"},
	} {
		if got := Join(c.base, c.rel); got != c.want {
			t.Errorf("Join(%q, %q) = %q, want %q", c.base, c.rel, got, c.want)
		}
	}
}

func TestExpandUser(t *testing.T) {
	cases := []struct{ home, in, want string }{
		{"/home/tester/", "~", "/home/tester"},
		{"/home/tester/", "~/x/", "/home/tester/x"},
		{"/home/tester/", "./~/x", "/home/tester/x"},
		{"/home/tester/", "/abs/~/x", "/abs/~/x"},
		{"/home/tester/", "rel//path/", "rel/path"},
		{"/h/me", "~/a/./b//c/", "/h/me/a/b/c"},
		{"/h/me", "~/../x", "/h/me/../x"},
		{"/", "~/x", "/x"},
		{"", "~/x", "/x"},
	}
	for _, tc := range cases {
		t.Setenv("HOME", tc.home)
		got, err := ExpandUser(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("HOME=%q ExpandUser(%q) = %q, %v; want %q", tc.home, tc.in, got, err, tc.want)
		}
	}
	// root's home differs by OS (/var/root on macOS, /root on Linux), so the
	// want is Python's formula over the user database: pw_dir.rstrip("/") + rest.
	if root, err := user.Lookup("root"); err == nil {
		want := strings.TrimRight(root.HomeDir, "/") + "/x"
		if got, err := ExpandUser("~root/x"); err != nil || got != want {
			t.Errorf("ExpandUser(~root/x) = %q, %v; want %q", got, err, want)
		}
	}
	if got, err := ExpandUser("~no-such-user-for-agent-sync/x"); err == nil {
		t.Errorf("ExpandUser(~no-such-user-for-agent-sync/x) = %q, want an error", got)
	}
}

func TestGlobListsMatchesInNameOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.json", ".hidden.json", "a.json", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		filepath.Join(dir, ".hidden.json"),
		filepath.Join(dir, "a.json"),
		filepath.Join(dir, "b.json"),
	}
	if got := Glob(dir, "*.json"); !slices.Equal(got, want) {
		t.Errorf("Glob = %q, want %q", got, want)
	}
	if got := Glob(filepath.Join(dir, "absent"), "*.json"); got != nil {
		t.Errorf("Glob of a missing dir = %q, want none", got)
	}
}

func TestErrorTextSpellsAnOSError(t *testing.T) {
	err := &fs.PathError{Op: "open", Path: "/x/it's", Err: syscall.ENOENT}
	if got, want := ErrorText(err), `[Errno 2] No such file or directory: "/x/it's"`; got != want {
		t.Errorf("ErrorText = %s, want %s", got, want)
	}
	if got := ErrorText(errors.New("plain")); got != "plain" {
		t.Errorf("ErrorText(plain) = %s", got)
	}
}

// TestRealPathMatchesPython builds the layout the Python 3.13 probe used and
// expects what os.path.realpath returned there: links resolve, a dangling
// link resolves to its target, and a loop stops at the link.
func TestRealPathMatchesPython(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		"a":    "b",
		"b":    "a",
		"up":   "d/../d",
		"dang": "nowhere/x",
		"absd": filepath.Join(root, "d"),
	} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	for in, want := range map[string]string{
		"a":      "a",
		"b":      "b",
		"a/x":    "a/x",
		"up/y":   "d/y",
		"dang":   "nowhere/x",
		"dang/z": "nowhere/x/z",
		"absd/q": "d/q",
		"d/../a": "a",
		"d/./":   "d",
	} {
		if got := RealPath(root + "/" + in); got != root+"/"+want {
			t.Errorf("RealPath(%s) = %s, want {root}/%s", in, got, want)
		}
	}
	if got := RealPath("/"); got != "/" {
		t.Errorf("RealPath(/) = %s", got)
	}
}

func TestMkdirParentsMatchesPathMkdir(t *testing.T) {
	dir := t.TempDir()
	if err := MkdirParents(filepath.Join(dir, "a/b/c")); err != nil {
		t.Fatal(err)
	}
	if err := MkdirParents(filepath.Join(dir, "a/b")); err != nil {
		t.Errorf("MkdirParents of an existing directory = %v, want nil", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := MkdirParents(filepath.Join(file, "sub"))
	want := "[Errno 20] Not a directory: '" + filepath.Join(file, "sub") + "'"
	if err == nil || ErrorText(err) != want {
		t.Errorf("MkdirParents under a file = %v (%s), want %s", err, ErrorText(err), want)
	}
	err = MkdirParents(file)
	want = "[Errno 17] File exists: '" + file + "'"
	if err == nil || ErrorText(err) != want {
		t.Errorf("MkdirParents of a file = %v, want %s", err, want)
	}
}
