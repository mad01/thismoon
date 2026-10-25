package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"
)

// TestConfigLoadMessageNamesSandboxFix covers the failure a sandboxed
// speak-web otherwise hides behind 503s: srt checks the resolved path, so a
// config symlinked into a checkout cannot be opened, and the log line has to
// say which expansion-layer file takes the link target.
func TestConfigLoadMessageNamesSandboxFix(t *testing.T) {
	const path = "/home/u/.config/speak/config.yaml"
	denied := fmt.Errorf("config: open %s: %w", path,
		&fs.PathError{Op: "open", Path: path, Err: syscall.EPERM})
	missing := fmt.Errorf("config: open %s: %w", path,
		&fs.PathError{Op: "open", Path: path, Err: syscall.ENOENT})
	parse := errors.New("config: parse " + path + ": yaml: line 3: bad indent")

	tests := []struct {
		name     string
		err      error
		label    string
		wantHint bool
	}{
		{"denied inside the sandbox", denied, "speak-web", true},
		{"denied outside any sandbox", denied, "", false},
		{"missing inside the sandbox", missing, "speak-web", false},
		{"parse error inside the sandbox", parse, "speak-web", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configLoadMessage(tt.err, tt.label)
			if !strings.HasPrefix(got, "speak: "+tt.err.Error()) {
				t.Errorf("message %q does not start with the load error", got)
			}
			if strings.Contains(got, "\n") {
				t.Errorf("message %q spans more than one line", got)
			}
			hasHint := strings.Contains(got, "~/.config/srt/"+tt.label+".local.json")
			if hasHint != tt.wantHint {
				t.Errorf("message %q: names the expansion layer = %v, want %v", got, hasHint, tt.wantHint)
			}
		})
	}
}
