package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SettingsFileName is the harness settings file each run writes into its
// temp dir and passes to the arm session with --settings.
const SettingsFileName = "settings.json"

// Isolation builds the deny rules that keep an arm session off the user's
// checkouts. It is prevention for the paths it names only: a Bash command
// that reaches a checkout another way (cd first, $HOME, a symlink) is not
// matched, which is why every run also fingerprints the source checkout.
type Isolation struct {
	Home   string
	Source string
}

// DenyRules are the rules the settings file holds. Claude loads the file
// beside the user's own settings, and a deny rule beats every allow rule,
// so the user's allow list cannot reopen these paths. The rules cover
// ~/code, ~/.worktrees and the source checkout when it lives outside both.
// An Edit rule covers the Write tool too: Claude checks file writes
// against Edit rules and never consults a Write path rule. git -C is
// denied into the same dirs, in absolute and ~/ form, since the allow
// list lets an arm branch and commit with -C. Reads stay allowed.
func (iso Isolation) DenyRules() []string {
	dirs := iso.editDirs()
	rules := make([]string, 0, 1+3*len(dirs))
	for _, dir := range dirs {
		rules = append(rules, "Edit(/"+dir+"/**)")
	}
	rules = append(rules, "Bash(curl *)")
	for _, dir := range dirs {
		rules = append(rules, "Bash(git -C "+dir+"*)")
		if within(dir, iso.Home) {
			rel, _ := filepath.Rel(iso.Home, dir)
			rules = append(rules, "Bash(git -C ~/"+filepath.ToSlash(rel)+"*)")
		}
	}
	return rules
}

// Covers reports whether p sits in a directory the rules deny edits in. A
// run's temp dir must not: the arm could not edit its own clone there.
func (iso Isolation) Covers(p string) bool {
	for _, dir := range iso.editDirs() {
		if within(p, dir) {
			return true
		}
	}
	return false
}

func (iso Isolation) worktrees() string { return filepath.Join(iso.Home, ".worktrees") }

// editDirs are the user's code tree, the loom worktrees, and the source
// checkout when it lives outside both.
func (iso Isolation) editDirs() []string {
	code := filepath.Join(iso.Home, "code")
	dirs := []string{code, iso.worktrees()}
	if !within(iso.Source, code) && !within(iso.Source, iso.worktrees()) {
		dirs = append(dirs, iso.Source)
	}
	return dirs
}

// WriteSettings writes the settings file into dir, read-only, and returns
// its path.
func (iso Isolation) WriteSettings(dir string) (string, error) {
	raw, err := json.MarshalIndent(map[string]any{
		"permissions": map[string]any{"deny": iso.DenyRules()},
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("eval: encode settings: %w", err)
	}
	p := filepath.Join(dir, SettingsFileName)
	if err := os.WriteFile(p, append(raw, '\n'), 0o444); err != nil {
		return "", fmt.Errorf("eval: write settings: %w", err)
	}
	return p, nil
}

// within reports whether p is dir or sits under it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
