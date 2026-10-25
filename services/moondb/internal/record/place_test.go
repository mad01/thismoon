package record

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	// real/deep holds the file; link points at real/deep. A path that
	// exists resolves as the kernel opens it, so ".." after the link walks
	// out of real/deep, not out of link's directory. A gone file under the
	// link resolves through the link with its missing tail kept.
	root := physicalTempDir(t)
	deep := filepath.Join(root, "real", "deep")
	writeFile(t, filepath.Join(deep, "in.txt"), "in\n")
	writeFile(t, filepath.Join(root, "real", "x.txt"), "real\n")
	writeFile(t, filepath.Join(root, "x.txt"), "root\n")
	link := filepath.Join(root, "link")
	if err := os.Symlink(deep, link); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, in, want string
	}{
		{"existing through the link", filepath.Join(link, "in.txt"), filepath.Join(deep, "in.txt")},
		// Spelled by hand: filepath.Join would fold the ".." away before
		// Resolve sees it.
		{"dot-dot after the link", link + "/../x.txt", filepath.Join(root, "real", "x.txt")},
		{
			"gone under the link",
			filepath.Join(link, "gone", "g.txt"),
			filepath.Join(deep, "gone", "g.txt"),
		},
		{
			"gone with no symlink",
			filepath.Join(root, "nope", "n.txt"),
			filepath.Join(root, "nope", "n.txt"),
		},
		{"relative to cwd", "real/x.txt", filepath.Join(root, "real", "x.txt")},
	}
	for _, tc := range cases {
		if got := Resolve(tc.in, root); got != tc.want {
			t.Errorf("%s: Resolve(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
