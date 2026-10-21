package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/d-man/internal/profile"
)

// TestProfileLongNamesTheFiles keeps the help text in step with the file
// names and default profile the profile package actually uses.
func TestProfileLongNamesTheFiles(t *testing.T) {
	for _, want := range []string{
		profile.BaseFile,
		profile.BlocklistFile("<name>"),
		`"` + profile.RecordFile + `"`,
		profile.Default,
		statusArg,
		"games_dir",
	} {
		if !strings.Contains(profileCmd.Long, want) {
			t.Errorf("profile --help does not mention %q", want)
		}
	}
}
