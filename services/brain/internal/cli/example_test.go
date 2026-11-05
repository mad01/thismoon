package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

// exampleVault is the example shared vault, relative to this package.
const exampleVault = "../../examples/shared-vault"

// TestExampleSharedVaultPassesCheck checks a copy of the example shared
// vault, so a change to brain's rules that the example no longer meets
// fails here before anyone copies it.
func TestExampleSharedVaultPassesCheck(t *testing.T) {
	home := testhome.Isolate(t)
	root := filepath.Join(t.TempDir(), "shared-vault")
	if err := os.CopyFS(root, os.DirFS(exampleVault)); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(home, ".config", "brain")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "vaults:\n  - {name: team, path: " + root + ", shared: true}\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	policy, err := check.LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"public", "internal"}; !slices.Equal(policy.Classifications, want) {
		t.Errorf("policy classifications = %v, want %v", policy.Classifications, want)
	}
	got := runBrain(t, "", "check", "-o", "json")
	r := decodeStrict[check.Report](t, got.stdout)
	if got.code != exitOK || !r.OK || r.Checked != 4 {
		t.Errorf("check the example vault: exit %d, report %+v, stderr %q", got.code, r, got.stderr)
	}
}

// TestExampleWorkflowPinsActions checks that every action the example
// workflow uses is pinned by a full commit sha.
func TestExampleWorkflowPinsActions(t *testing.T) {
	data, err := os.ReadFile(
		filepath.Join(exampleVault, ".github", "workflows", "brain-check.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	pinned := regexp.MustCompile(`^[\w.-]+/[\w./-]+@[0-9a-f]{40}$`)
	uses := 0
	for job, j := range wf.Jobs {
		for _, s := range j.Steps {
			if s.Uses == "" {
				continue
			}
			uses++
			if !pinned.MatchString(s.Uses) {
				t.Errorf("job %s uses %s, which is not pinned by a commit sha", job, s.Uses)
			}
		}
	}
	if uses == 0 {
		t.Error("the example workflow uses no action; the test reads the wrong file")
	}
}
