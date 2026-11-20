package secretscan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const stagedTestSecret = "AKIA1234567890ABCDEF"

// awsFindingFiles returns the repo-relative files with an AWS key finding,
// sorted.
func awsFindingFiles(t *testing.T, dir string, findings []Finding) []string {
	t.Helper()
	var files []string
	for _, f := range findings {
		if f.Rule.ID != "aws-access-key-id" {
			continue
		}
		rel, err := filepath.Rel(dir, f.File)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, rel)
	}
	slices.Sort(files)
	return files
}

func TestScanStaged_readsTheIndex(t *testing.T) {
	cases := []struct {
		name string
		// stage changes the repo after the initial commit of seed.txt.
		stage func(t *testing.T, dir string)
		want  []string
	}{
		{
			name: "working-tree edits after staging do not change what is scanned",
			stage: func(t *testing.T, dir string) {
				writeFiles(t, dir, map[string]string{
					"staged-secret.txt": stagedTestSecret + "\n",
					"staged-clean.txt":  "clean\n",
				})
				gitRun(t, dir, "add", "staged-secret.txt", "staged-clean.txt")
				writeFiles(t, dir, map[string]string{
					"staged-secret.txt": "scrubbed\n",
					"staged-clean.txt":  stagedTestSecret + "\n",
				})
			},
			want: []string{"staged-secret.txt"},
		},
		{
			name: "names with spaces, unicode and a newline",
			stage: func(t *testing.T, dir string) {
				writeFiles(t, dir, map[string]string{
					"dir with space/a file.txt": stagedTestSecret + "\n",
					"nyckel-åäö-鍵.txt":          stagedTestSecret + "\n",
					"new\nline.txt":             stagedTestSecret + "\n",
				})
				gitRun(t, dir, "add", "-A")
			},
			want: []string{
				"dir with space/a file.txt",
				"new\nline.txt",
				"nyckel-åäö-鍵.txt",
			},
		},
		{
			name: "a staged deletion is not scanned",
			stage: func(t *testing.T, dir string) {
				writeFiles(t, dir, map[string]string{"gone.txt": stagedTestSecret + "\n"})
				gitRun(t, dir, "add", "gone.txt")
				gitRun(t, dir, "commit", "-m", "add gone")
				gitRun(t, dir, "rm", "-q", "gone.txt")
				writeFiles(t, dir, map[string]string{"kept.txt": "clean\n"})
				gitRun(t, dir, "add", "kept.txt")
			},
			want: nil,
		},
		{
			name: "a rename is scanned under its new name",
			stage: func(t *testing.T, dir string) {
				writeFiles(t, dir, map[string]string{"old.txt": stagedTestSecret + "\n"})
				gitRun(t, dir, "add", "old.txt")
				gitRun(t, dir, "commit", "-m", "add old")
				gitRun(t, dir, "mv", "old.txt", "new.txt")
			},
			want: []string{"new.txt"},
		},
		{
			name: "a symlink's target is scanned as its content",
			stage: func(t *testing.T, dir string) {
				if err := os.Symlink(stagedTestSecret, filepath.Join(dir, "link")); err != nil {
					t.Fatal(err)
				}
				gitRun(t, dir, "add", "link")
			},
			want: []string{"link"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initGitRepo(t, dir, map[string]string{"seed.txt": "seed\n"})
			tc.stage(t, dir)

			findings, err := New(DefaultRules).ScanStaged(dir)
			if err != nil {
				t.Fatalf("ScanStaged error: %v", err)
			}
			if got := awsFindingFiles(t, dir, findings); !slices.Equal(got, tc.want) {
				t.Errorf("files with findings = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestScanStaged_largeBatch stages hundreds of files with an oversize blob
// between them, so a scan that lost its place in the blob stream would
// report a secret under the wrong file or miss one.
func TestScanStaged_largeBatch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"seed.txt": "seed\n"})

	files := make(map[string]string)
	for i := range 600 {
		files[fmt.Sprintf("pkg%d/f%03d.go", i%7, i)] = fmt.Sprintf(
			"package p\n\nvar v%d = %d\n",
			i,
			i,
		)
	}
	files["pkg3/secret.go"] = "package p\n\nconst key = \"" + stagedTestSecret + "\"\n"
	files["pkg0/same-as-secret.go"] = files["pkg3/secret.go"]
	files["pkg2/big.txt"] = stagedTestSecret + "\n" + strings.Repeat("a", maxFileBytes+1)
	files["zz-last.txt"] = "token " + stagedTestSecret + "\n"
	writeFiles(t, dir, files)
	gitRun(t, dir, "add", "-A")

	var skips []string
	sc := New(DefaultRules)
	sc.OnSkip = func(s SkippedFile) { skips = append(skips, filepath.Base(s.Path)) }

	findings, err := sc.ScanStaged(dir)
	if err != nil {
		t.Fatalf("ScanStaged error: %v", err)
	}
	want := []string{"pkg0/same-as-secret.go", "pkg3/secret.go", "zz-last.txt"}
	if got := awsFindingFiles(t, dir, findings); !slices.Equal(got, want) {
		t.Errorf("files with findings = %q, want %q", got, want)
	}
	for _, f := range findings {
		if strings.HasSuffix(f.File, "secret.go") && f.Line != 3 {
			t.Errorf("finding in %s on line %d, want line 3", f.File, f.Line)
		}
	}
	if !slices.Equal(skips, []string{"big.txt"}) {
		t.Errorf("skips = %q, want [big.txt]", skips)
	}
}

// TestScanStaged_gitlinkFailsClosed pins the treatment of a staged gitlink:
// its commit is not in the superproject's object store, so the blob cannot
// be read and the scan fails instead of passing the entry.
func TestScanStaged_gitlinkFailsClosed(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"seed.txt": "seed\n"})
	gitRun(t, dir, "update-index", "--add", "--cacheinfo",
		"160000,1111111111111111111111111111111111111111,sub")

	if _, err := New(DefaultRules).ScanStaged(dir); err == nil {
		t.Fatal("ScanStaged on a staged gitlink with no commit object: want error, got nil")
	}
}

// A gitlink whose commit is in the object store reads back as a commit, not
// a file, so the scan fails the same way instead of scanning the raw commit.
func TestScanStaged_gitlinkToKnownCommitFailsClosed(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir, map[string]string{"seed.txt": "seed\n"})
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	head := strings.TrimSpace(string(out))
	gitRun(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+",sub")

	_, err = New(DefaultRules).ScanStaged(dir)
	if err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("ScanStaged on a staged gitlink to a known commit: err %v, want a not-a-file error", err)
	}
}

func TestParseRawDiff(t *testing.T) {
	const (
		zero = "0000000000000000000000000000000000000000"
		oidA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		oidB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	cases := []struct {
		name    string
		raw     string
		want    []stagedEntry
		wantErr bool
	}{
		{name: "empty", raw: "", want: nil},
		{
			name: "add and modify",
			raw: ":000000 100644 " + zero + " " + oidA + " A\x00new.txt\x00" +
				":100644 100644 " + oidA + " " + oidB + " M\x00dir/mod.go\x00",
			want: []stagedEntry{{rel: "new.txt", oid: oidA}, {rel: "dir/mod.go", oid: oidB}},
		},
		{
			name: "rename and copy keep the destination",
			raw: ":100644 100644 " + oidA + " " + oidA + " R100\x00old.txt\x00new.txt\x00" +
				":100644 100644 " + oidA + " " + oidB + " C075\x00src.go\x00copy.go\x00",
			want: []stagedEntry{{rel: "new.txt", oid: oidA}, {rel: "copy.go", oid: oidB}},
		},
		{
			name: "a path with a newline and a space",
			raw:  ":000000 100644 " + zero + " " + oidA + " A\x00a\nb c.txt\x00",
			want: []stagedEntry{{rel: "a\nb c.txt", oid: oidA}},
		},
		{
			name:    "record without a path",
			raw:     ":000000 100644 " + zero + " " + oidA + " A\x00",
			wantErr: true,
		},
		{name: "not a raw record", raw: "new.txt\x00", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRawDiff([]byte(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseRawDiff: want error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRawDiff: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("parseRawDiff = %+v, want %+v", got, tc.want)
			}
		})
	}
}
