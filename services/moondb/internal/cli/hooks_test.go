package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
)

// wiredSettings is the hook wiring config.md documents, with the binary
// named by path as the fleet's template names it.
const wiredSettings = `{"hooks": {
  "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "~/code/bin/moondb record", "async": true}]}],
  "PostToolUse": [
    {"matcher": "Edit|Write|MultiEdit|NotebookEdit",
     "hooks": [{"type": "command", "command": "~/code/bin/moondb record", "timeout": 3}]},
    {"matcher": "Bash", "hooks": [{"type": "command", "command": "moondb record", "async": true}]}
  ],
  "Stop": [{"hooks": [{"type": "command", "command": "moondb record"}]}],
  "SubagentStop": [{"hooks": [{"type": "command", "command": "moondb record", "async": true}]}]
}}`

// TestCheckHooksWired passes a settings file with every moondb record
// hook, names the missing ones otherwise, fails on a missing file, and
// passes with a note when capture is off.
func TestCheckHooksWired(t *testing.T) {
	cases := []struct {
		name      string
		settings  string // "" writes no file
		captureOn bool
		want      string // a substring of the error; "" for a pass
	}{
		{"all wired", wiredSettings, true, ""},
		{
			"bash and subagent missing", strings.NewReplacer(`"matcher": "Bash"`, `"matcher": "Read"`,
				`"SubagentStop"`, `"Notification"`).Replace(wiredSettings), true,
			"for PostToolUse Bash, SubagentStop",
		},
		{
			"other command", strings.ReplaceAll(wiredSettings, "moondb record", "belt record"), true,
			"for UserPromptSubmit, PostToolUse Edit, PostToolUse Bash, Stop, SubagentStop",
		},
		{"no file", "", true, "no such file"},
		{"not json", "{", true, "parse"},
		{"capture off", "", false, "capture.enabled is false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if c.settings != "" {
				if err := os.WriteFile(path, []byte(c.settings), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := checkHooksWired(path, c.captureOn)
			if c.want == "" && err != nil ||
				c.want != "" && !strings.Contains(fmt.Sprint(err), c.want) {
				t.Errorf("checkHooksWired = %v, want %q", err, c.want)
			}
			r := doctor.Collect(context.Background(), []doctor.Check{{
				Name: "hooks-wired", Run: func(context.Context) error { return err },
			}})
			if pass := c.want == "" || !c.captureOn; r.OK != pass {
				t.Errorf("report OK = %v, want %v", r.OK, pass)
			}
		})
	}
}
