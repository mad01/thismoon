package discover

import (
	"path/filepath"
	"slices"
	"testing"
)

// The manifest discoverers skip vendor; skipDirKeepingVendor walks into it but
// still skips the other trees.
func TestFindFilesSkipsVendorOnlyForManifests(t *testing.T) {
	root := t.TempDir()
	web := writeRepoFile(t, root, "web/package-lock.json", nil)
	vendored := writeRepoFile(t, root, "static/vendor/package-lock.json", nil)
	writeRepoFile(t, root, "node_modules/x/package-lock.json", nil)
	writeRepoFile(t, root, "testdata/package-lock.json", nil)
	isLock := func(rel string) bool { return filepath.Base(rel) == "package-lock.json" }

	manifests, err := findManifests(root, "package-lock.json", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(manifests, []string{web}) {
		t.Errorf("findManifests = %v, want only %s", manifests, web)
	}
	files, err := findFiles(root, Options{}, skipDirKeepingVendor, isLock)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{vendored, web}) {
		t.Errorf("findFiles keeping vendor = %v, want %s and %s", files, vendored, web)
	}
}
