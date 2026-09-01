package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const scanFixture = `// Package fixture is scanned by the CLI test.
package fixture

import "errors"

// ErrEmpty is returned for an empty name.
var ErrEmpty = errors.New("fixture: the name is empty")
`

// runScanCmd executes the scan command and returns what it wrote. The flags
// bind to package vars cobra does not reset between Execute calls, so the
// helper restores the defaults first; a real invocation is a fresh process.
func runScanCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	scanGo, scanDetect, scanHolistic, scanKinds = false, false, false, nil
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"scan"}, args...))
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	err := rootCmd.Execute()
	return buf.String(), err
}

// writeFixture drops the fixture source in a temp dir and returns its path.
func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(scanFixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestScanCmdPrintsBlocks pins the plain output contract: a header per
// file, then every block behind its own file:line line, so the result pipes
// into `humanizer detect`.
func TestScanCmdPrintsBlocks(t *testing.T) {
	path := writeFixture(t)

	out, err := runScanCmd(t, "--go", path)
	if err != nil {
		t.Fatalf("scan --go: %v", err)
	}
	for _, want := range []string{
		"==> " + path + " <==",
		path + ":1  [doc] package fixture",
		"Package fixture is scanned by the CLI test.",
		"[error] errors.New",
		"fixture: the name is empty",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestScanCmdKindFilter checks --kind reaches the extractor.
func TestScanCmdKindFilter(t *testing.T) {
	path := writeFixture(t)

	out, err := runScanCmd(t, "--go", "--kind", "error", path)
	if err != nil {
		t.Fatalf("scan --kind error: %v", err)
	}
	if strings.Contains(out, "[doc]") {
		t.Errorf("--kind error let a doc block through:\n%s", out)
	}
	if !strings.Contains(out, "[error] errors.New") {
		t.Errorf("--kind error dropped the error block:\n%s", out)
	}
}

func TestScanCmdErrors(t *testing.T) {
	path := writeFixture(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no extractor", args: []string{path}, want: "--go is required"},
		{
			name: "unknown kind",
			args: []string{"--go", "--kind", "prose", path},
			want: "unknown kind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runScanCmd(t, tt.args...)
			if err == nil {
				t.Fatalf("scan %v returned no error", tt.args)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}
