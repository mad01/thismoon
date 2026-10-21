package apply

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestMainExitCodes(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantCode   int
		wantStderr string
	}{
		{"success", nil, CodeOK, ""},
		{"drift prints nothing", Exit(CodeDrift), CodeDrift, ""},
		{
			"plain error", errors.New("template not found: /t.json"), CodeError,
			"error: template not found: /t.json\n",
		},
		{
			"coded error", Fail(CodeExternal, errors.New("claude mcp list failed")), CodeExternal,
			"error: claude mcp list failed\n",
		},
		{"wrapped exit keeps its code", fmt.Errorf("mods: %w", Exit(CodeDrift)), CodeDrift, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if got := Main(&stderr, tc.err); got != tc.wantCode {
				t.Errorf("Main code = %d, want %d", got, tc.wantCode)
			}
			if stderr.String() != tc.wantStderr {
				t.Errorf("Main stderr = %q, want %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

func TestModeWrites(t *testing.T) {
	for mode, want := range map[Mode]bool{Sync: true, Plan: false, Check: false} {
		if got := mode.Writes(); got != want {
			t.Errorf("%s.Writes() = %v, want %v", mode, got, want)
		}
	}
}
