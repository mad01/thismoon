package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/ledger"
)

func TestRecordCmdEventValidation(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		wantErr bool
	}{
		{"subagent-stop", "subagent-stop", false},
		{"session-end", "session-end", false},
		{"mixed case", "Subagent-Stop", false},
		{"hook event name", "SubagentStop", true},
		{"hint event", "session-start", true},
		{"guard event", "bash", true},
		{"unknown", "frobnicate", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := rootCmd()
			root.SetArgs([]string{"record", tt.event})
			root.SetIn(strings.NewReader("{}"))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			err := root.Execute()
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Errorf("record %q error = %v, want error %v", tt.event, err, tt.wantErr)
			}
		})
	}
}

// TestRecordCmdIgnoresABrokenConfig pins the record entrypoint's contract
// against the one input that makes `belt hook` deny everything: with a
// config file that does not parse, every record event still exits 0 and
// prints nothing on the command's streams or the process's.
func TestRecordCmdIgnoresABrokenConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	broken := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(broken, []byte("guards: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := `{"session_id":"s1","cwd":"` + dir + `","agent_type":"Explore"}`
	for _, event := range ledger.Events() {
		t.Run(event, func(t *testing.T) {
			root := rootCmd()
			root.SetArgs([]string{"--config", broken, "record", event})
			root.SetIn(strings.NewReader(payload))
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			var err error
			leaked := captureStdio(t, func() { err = root.Execute() })
			if err != nil {
				t.Errorf("record %s with a broken config: error %v, want exit 0", event, err)
			}
			if got := out.String() + errOut.String() + leaked; got != "" {
				t.Errorf("record %s wrote %q, want nothing", event, got)
			}
		})
	}
}

// captureStdio runs fn with the process stdout and stderr pointed at one
// pipe and returns everything written to either.
func captureStdio(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	read := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		read <- string(b)
	}()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return <-read
}

// TestHookCmdEventValidation pins the wiring failure mode: an event that
// matches no guards must error instead of silently allowing every tool call.
func TestHookCmdEventValidation(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		wantErr bool
	}{
		{"bash", "bash", false},
		{"write", "write", false},
		{"external-text", "external-text", false},
		{"mixed case tool name", "Bash", false},
		{"hook event name", "PreToolUse", true},
		{"unknown", "frobnicate", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := rootCmd()
			root.SetArgs([]string{"hook", tt.event})
			root.SetIn(strings.NewReader("{}"))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			err := root.Execute()
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Errorf("hook %q error = %v, want error %v", tt.event, err, tt.wantErr)
			}
		})
	}
}
