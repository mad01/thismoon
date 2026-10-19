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

// DenyRules are the isolation rules every run's settings file holds after
// the arm's own deny list. Claude loads the file beside the user's own
// settings, and a deny rule beats every allow rule, so neither the user's
// allow list nor the arm's can reopen these paths. The rules cover
// ~/code, ~/.worktrees and the source checkout when it lives outside both.
// An Edit rule covers the Write tool too: Claude checks file writes
// against Edit rules and never consults a Write path rule. Each command
// in dirCommands is denied into the same dirs, in absolute and ~/ form.
// Reads stay allowed.
func (iso Isolation) DenyRules() []string {
	dirs := iso.editDirs()
	rules := make([]string, 0, 1+len(dirs)*(1+2*len(dirCommands)))
	for _, dir := range dirs {
		rules = append(rules, "Edit(/"+dir+"/**)")
	}
	rules = append(rules, "Bash(curl *)")
	for _, cmd := range dirCommands {
		for _, dir := range dirs {
			for _, spelling := range iso.spellings(dir) {
				rules = append(rules, "Bash("+cmd+spelling+"*)")
			}
		}
	}
	return rules
}

// dirCommands are the Bash command prefixes the isolation rules deny into
// each protected dir. The allow list permits all three with any dir: git
// -C branches and commits there, make -C builds there, and go build -o
// writes its binary there, which under ~/code/bin overwrites an installed
// tool.
var dirCommands = []string{"git -C ", "make -C ", "go build *-o "}

// spellings are the ways a Bash command names dir: its absolute path, and
// its ~/ form when it sits under the home dir.
func (iso Isolation) spellings(dir string) []string {
	if !within(dir, iso.Home) {
		return []string{dir}
	}
	rel, _ := filepath.Rel(iso.Home, dir)
	return []string{dir, "~/" + filepath.ToSlash(rel)}
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

// Permissions is the permissions block of a run's settings file.
type Permissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// Settings is a run's settings file: the arm's permissions, and the
// sandbox every Bash command of the session runs in.
type Settings struct {
	Permissions Permissions `json:"permissions"`
	Sandbox     Sandbox     `json:"sandbox"`
}

// WriteSettings writes the settings file into dir, read-only, and returns
// its path.
func WriteSettings(dir string, s Settings) (string, error) {
	raw, err := json.MarshalIndent(s, "", "  ")
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
