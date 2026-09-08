package cli

import (
	"io"
	"strings"
	"testing"
)

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
