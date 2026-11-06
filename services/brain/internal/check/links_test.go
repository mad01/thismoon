package check

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// TestRunGivesLinksAndNULBytesRulesOfTheirOwn pins that a note that is a
// symbolic link is a symlink finding and a NUL byte a nul-byte finding,
// each an error by default, not a schema or secret finding.
func TestRunGivesLinksAndNULBytesRulesOfTheirOwn(t *testing.T) {
	v, set := newVault(t, map[string]string{
		"notes/nul.md": note(validated(idA), "x\x00y\n"),
	})
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	if err := os.WriteFile(outside, []byte(note(validated(idB), "away\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(v.Root, "notes", "link.md")); err != nil {
		t.Fatal(err)
	}
	for name, opts := range map[string]Options{
		"every note":    {Set: set, Now: checkDay},
		"named targets": {Set: set, Now: checkDay, Targets: []vault.Ref{{Vault: v, Path: "notes/link.md"}, {Vault: v, Path: "notes/nul.md"}}},
	} {
		t.Run(name, func(t *testing.T) {
			r, err := Run(opts)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range r.Findings {
				got = append(got, f.Path+" "+f.Rule+" "+f.Level)
				if f.Rule == RuleSymlink && !strings.Contains(f.Message, "symbolic link") {
					t.Errorf("symlink message = %q, want it to name the link", f.Message)
				}
			}
			want := []string{"notes/link.md symlink error", "notes/nul.md nul-byte error"}
			if !slices.Equal(got, want) {
				t.Errorf("findings = %q, want %q", got, want)
			}
		})
	}
}
