package cli

import (
	"bytes"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// closedPort is a local port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func TestImportDryRunWithoutServe(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{
		"import", "--dry-run",
		"--port", strconv.Itoa(closedPort(t)),
		"--workdir", filepath.Join(dir, "work"),
		"--config", filepath.Join(dir, "config.toml"),
		"--projects", filepath.Join(dir, "projects"),
		"--file-history", filepath.Join(dir, "file-history"),
	})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		importDryRun = false
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import --dry-run with serve down: %v", err)
	}
	if got := errOut.String(); strings.Count(got, "\n") != 1 ||
		!strings.Contains(got, "skip information and known repos unavailable") {
		t.Errorf("stderr = %q, want one line saying the lookups were unavailable", got)
	}
	if !strings.Contains(out.String(), "dry run: nothing written") {
		t.Errorf("stdout = %q, want the dry-run counts", out.String())
	}
}
