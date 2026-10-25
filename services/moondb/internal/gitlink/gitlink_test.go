package gitlink

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func TestBlobIDKnown(t *testing.T) {
	// The empty blob id is fixed by git itself.
	if got, want := BlobID(nil), "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"; got != want {
		t.Errorf("BlobID(empty) = %s, want %s", got, want)
	}
}

func TestBlobIDMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	contents := []string{
		"",
		"hello\n",
		"crlf\r\nlines\r\n",
		"no final newline",
		strings.Repeat("0123456789abcdef", 1<<12),
		"nul \x00 byte\n",
	}
	dir := t.TempDir()
	for i, c := range contents {
		path := filepath.Join(dir, "f")
		if err := os.WriteFile(path, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
		// --no-filters hashes the bytes as they are, without autocrlf or
		// attributes from the environment.
		out, err := exec.Command("git", "hash-object", "--no-filters", path).Output()
		if err != nil {
			t.Fatalf("git hash-object: %v", err)
		}
		if got, want := BlobID([]byte(c)), strings.TrimSpace(string(out)); got != want {
			t.Errorf("content %d: BlobID = %s, git says %s", i, got, want)
		}
	}
}

// Captured from `git diff-tree -r --no-commit-id -z` over a commit that
// adds an executable, deletes a file, modifies one and moves one; the
// second capture adds -M, so the move shows as a rename.
const (
	diffTreePlain = ":000000 100755 0000000000000000000000000000000000000000 " +
		"3e757656cf36eca53338e520d134963a44f793f8 A\x00add.txt\x00" +
		":100644 000000 286c5f5776916d7d7d5849988ca9d83e722cf9c2 " +
		"0000000000000000000000000000000000000000 D\x00del.txt\x00" +
		":100644 100644 78981922613b2afb6025042ff6bd878ac1994e85 " +
		"61780798228d17af2d34fce4cfbdf35556832472 M\x00keep.txt\x00" +
		":000000 100644 0000000000000000000000000000000000000000 " +
		"26e318d3c075a5bfa0f74e776b1670e4372debf3 A\x00new.txt\x00" +
		":100644 000000 26e318d3c075a5bfa0f74e776b1670e4372debf3 " +
		"0000000000000000000000000000000000000000 D\x00old.txt\x00"
	diffTreeRenames = ":100644 100644 78981922613b2afb6025042ff6bd878ac1994e85 " +
		"61780798228d17af2d34fce4cfbdf35556832472 M\x00keep.txt\x00" +
		":100644 100644 26e318d3c075a5bfa0f74e776b1670e4372debf3 " +
		"26e318d3c075a5bfa0f74e776b1670e4372debf3 R100\x00old.txt\x00new.txt\x00" +
		":160000 160000 1111111111111111111111111111111111111111 " +
		"2222222222222222222222222222222222222222 M\x00vendor/sub\x00"
)

func TestParseDiffTree(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []spool.ChangedFile
	}{
		{name: "empty", out: "", want: nil},
		{
			name: "add, delete, modify, move without rename detection",
			out:  diffTreePlain,
			want: []spool.ChangedFile{
				{Path: "add.txt", Blob: "3e757656cf36eca53338e520d134963a44f793f8", Status: "A"},
				{Path: "del.txt", Status: "D"},
				{Path: "keep.txt", Blob: "61780798228d17af2d34fce4cfbdf35556832472", Status: "M"},
				{Path: "new.txt", Blob: "26e318d3c075a5bfa0f74e776b1670e4372debf3", Status: "A"},
				{Path: "old.txt", Status: "D"},
			},
		},
		{
			name: "rename takes the new path, submodule has no blob",
			out:  diffTreeRenames,
			want: []spool.ChangedFile{
				{Path: "keep.txt", Blob: "61780798228d17af2d34fce4cfbdf35556832472", Status: "M"},
				{Path: "new.txt", Blob: "26e318d3c075a5bfa0f74e776b1670e4372debf3", Status: "R"},
				{Path: "vendor/sub", Status: "M"},
			},
		},
		{
			name: "path with spaces and a newline",
			out:  ":100644 100644 aaaa aaab M\x00dir with space/a\nb.go\x00",
			want: []spool.ChangedFile{
				{Path: "dir with space/a\nb.go", Blob: "aaab", Status: "M"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDiffTree(tt.out)
			if err != nil {
				t.Fatalf("ParseDiffTree: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseDiffTree =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestParseDiffTreeRejects(t *testing.T) {
	tests := []struct {
		name string
		out  string
	}{
		{"commit id line", "1a612e4313cddd60ef596c66f2fccaacea2e8410\x00"},
		{"short header", ":100644 100644 aaaa M\x00a.go\x00"},
		{"missing path", ":100644 100644 aaaa bbbb M"},
		{"rename missing new path", ":100644 100644 aaaa bbbb R090\x00old.go\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDiffTree(tt.out); err == nil {
				t.Errorf("ParseDiffTree(%q) succeeded, want an error", tt.out)
			}
		})
	}
}

func TestMatchCommit(t *testing.T) {
	t0 := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	edits := map[string][]store.Edit{
		"b1": {
			{ID: "old:0", Path: "a.go", AfterBlob: "b1", At: t0},
			{ID: "new:0", Path: "a.go", AfterBlob: "b1", At: t0.Add(time.Minute)},
			{ID: "elsewhere:0", Path: "z.go", AfterBlob: "b1", At: t0.Add(time.Hour)},
		},
		// A same-content edit of another path still matches when it is
		// the only one: the file moved before the commit.
		"b2": {{ID: "moved:0", Path: "before/b.go", AfterBlob: "b2", At: t0}},
		// The lookup returned an edit with a different blob; it is ignored.
		"b3": {{ID: "wrong:0", Path: "c.go", AfterBlob: "other", At: t0}},
	}
	files := []spool.ChangedFile{
		{Path: "a.go", Blob: "b1", Status: "M"},
		{Path: "b.go", Blob: "b2", Status: "R"},
		{Path: "c.go", Blob: "b3", Status: "M"},
		{Path: "gone.go", Status: "D"},
	}
	var asked []string
	got, err := MatchCommit(files, func(blob string) ([]store.Edit, error) {
		asked = append(asked, blob)
		return edits[blob], nil
	})
	if err != nil {
		t.Fatalf("MatchCommit: %v", err)
	}
	want := []Match{
		{File: files[0], Edit: edits["b1"][1]},
		{File: files[1], Edit: edits["b2"][0]},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MatchCommit =\n%+v\nwant\n%+v", got, want)
	}
	if want := []string{"b1", "b2", "b3"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("looked up %v, want %v (no lookup for a deletion)", asked, want)
	}
}

func TestMatchCommitLookupError(t *testing.T) {
	boom := errors.New("boom")
	_, err := MatchCommit(
		[]spool.ChangedFile{{Path: "a.go", Blob: "b1"}},
		func(string) ([]store.Edit, error) { return nil, boom },
	)
	if !errors.Is(err, boom) {
		t.Errorf("MatchCommit error = %v, want it to wrap %v", err, boom)
	}
}
