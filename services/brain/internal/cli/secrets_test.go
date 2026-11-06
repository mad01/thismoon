package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureKey is AWS's documented example access key id: shaped like a real
// one so the scanner fires, and valid nowhere.
const fixtureKey = "AKIAIOSFODNN7EXAMPLE"

func TestWriteRefusesASecretWithExit8(t *testing.T) {
	root := setupCLI(t, "")
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyFile, []byte("key "+fixtureKey+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"stdin body", "key " + fixtureKey + "\n", []string{"write", "--title", "Leak"}},
		{"body file", "", []string{"write", "--title", "Leak", "--body-file", bodyFile}},
		{"edit append file", "", []string{"edit", "notes/alpha.md", "--append-file", bodyFile}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runBrain(t, tc.stdin, append(tc.args, "-o", "json")...)
			if got.code != exitSecretFound ||
				!strings.Contains(got.stdout, `"code": "secret_found"`) {
				t.Fatalf(
					"exit %d, stdout %q, want exit 8 and code secret_found",
					got.code,
					got.stdout,
				)
			}
			if strings.Contains(got.stdout+got.stderr, fixtureKey) {
				t.Error("the refusal printed the raw secret")
			}
			if _, err := os.Stat(filepath.Join(root, "inbox", "leak.md")); !os.IsNotExist(err) {
				t.Errorf("refused note exists: %v", err)
			}
		})
	}
}

func TestBodyFileFlags(t *testing.T) {
	root := setupCLI(t, "")
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyFile, []byte("from a file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	usage := [][]string{
		{"write", "--title", "X", "--body", "inline", "--body-file", bodyFile},
		{"edit", "notes/alpha.md", "--body", "inline", "--body-file", bodyFile},
		{"edit", "notes/alpha.md", "--append", "inline", "--append-file", bodyFile},
		{"edit", "notes/alpha.md", "--body-file", bodyFile, "--append-file", bodyFile},
		{"write", "--title", "X", "--body-file", filepath.Join(root, "missing.md")},
	}
	for _, args := range usage {
		if got := runBrain(t, "", args...); got.code != exitUsage {
			t.Errorf("%v: exit %d, want %d; stderr %q", args, got.code, exitUsage, got.stderr)
		}
	}
	if got := runBrain(t, "", "write", "--title", "From File", "--body-file", bodyFile); got.code != exitOK {
		t.Fatalf("write --body-file: exit %d, stderr %q", got.code, got.stderr)
	}
	if got := runBrain(t, "", "edit", "notes/beta.md", "--append-file", bodyFile); got.code != exitOK {
		t.Fatalf("edit --append-file: exit %d, stderr %q", got.code, got.stderr)
	}
	for rel, want := range map[string]string{
		"inbox/from-file.md": "from a file\n",
		"notes/beta.md":      "Beta extends [[alpha]].\nfrom a file\n",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || !strings.HasSuffix(string(data), "---\n"+want) {
			t.Errorf("%s = %q, want the body %q", rel, data, want)
		}
	}
}
