package hint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// OverlayFileName is the repo-root file hints read for per-repo overrides.
// It lives in this package on purpose: internal/guard cannot import
// internal/hint without a cycle, so guards structurally cannot read
// repo-controlled config (docs/adr/0012).
const OverlayFileName = ".belt.yaml"

// repoOverlay is the parsed repo-local overlay. Unknown hint ids and unknown
// keys are ignored rather than rejected — a repo may target a newer belt
// than this machine runs, and a repo file carries no enforcement to protect
// with loud validation (docs/adr/0012).
type repoOverlay struct {
	Hints map[string]repoHintOverlay `yaml:"hints"`
}

// repoHintOverlay is one hint's repo-local settings. Which keys a hint reads
// is that hint's contract; commit-policy reads all three.
type repoHintOverlay struct {
	// Exclude opts this repo out of the hint (true) or back in over the
	// machine's exclude_repos (false). Unset defers to the machine config.
	Exclude *bool `yaml:"exclude"`
	// ProtectedBranches replaces the hint's default branch set for this
	// repo. Entries match exactly, or by trailing "*" prefix wildcard
	// ("release/*" covers release/1.2).
	ProtectedBranches []string `yaml:"protected_branches"`
	// Message is one repo-authored line appended to the hint's advice.
	Message string `yaml:"message"`
}

// loadRepoOverlay reads root/.belt.yaml. A missing file (or empty root) is
// the zero overlay with no error; only a file that exists and fails to parse
// returns one, and callers surface that as advisory text — a repo file must
// never inherit the machine config's broken-config-denies semantic.
func loadRepoOverlay(root string) (repoOverlay, error) {
	if root == "" {
		return repoOverlay{}, nil
	}
	raw, err := os.ReadFile(filepath.Join(root, OverlayFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return repoOverlay{}, nil
		}
		return repoOverlay{}, fmt.Errorf("read %s: %w", OverlayFileName, err)
	}
	var o repoOverlay
	if err := yaml.Unmarshal(raw, &o); err != nil {
		return repoOverlay{}, fmt.Errorf("parse %s: %w", OverlayFileName, err)
	}
	return o, nil
}

// branchMatches reports whether branch is covered by the patterns: exact
// match, or prefix match for a trailing-"*" entry.
func branchMatches(patterns []string, branch string) bool {
	for _, p := range patterns {
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			if strings.HasPrefix(branch, prefix) {
				return true
			}
			continue
		}
		if p == branch {
			return true
		}
	}
	return false
}
