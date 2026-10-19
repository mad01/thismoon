package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Profile selects the STE-lite rule set for one kind of text. The STE style
// holds rules shaped on Simplified Technical English (ASD-STE100); a rule's
// humanizer-profile header names the profiles that run it.
type Profile string

const (
	// ProfileProcedure is for instructions: task briefs, steps, checklists.
	ProfileProcedure Profile = "procedure"
	// ProfileDescription is for descriptive text: status reports, summaries.
	ProfileDescription Profile = "description"
)

// ParseProfile validates a profile name from a CLI flag or an MCP field.
func ParseProfile(s string) (Profile, error) {
	switch p := Profile(strings.ToLower(strings.TrimSpace(s))); p {
	case ProfileProcedure, ProfileDescription:
		return p, nil
	}
	return "", fmt.Errorf("rules: unknown profile %q (want procedure or description)", s)
}

const (
	// steStyle is the vale style directory and rule-ID prefix of the STE rules.
	steStyle = "STE"
	// steRunRoot is the cache subdirectory holding one run directory per glossary.
	steRunRoot = "ste"
	// defaultRunKey names the run directory of a profile run with no glossary.
	defaultRunKey = "default"
	// runKeyHexLen is how many hex digits of the glossary hash name its run directory.
	runKeyHexLen = 16
	// glossaryFileName makes the generated rule's ID STE.Glossary.
	glossaryFileName = "Glossary.yml"
	// glossaryRuleID is the ID of the rule generated from CONTEXT.md files.
	glossaryRuleID = steStyle + ".Glossary"
	// runDirMaxAge is how long an STE run directory may go unused before a
	// later run prunes it. A week keeps the directories of every glossary
	// scope in recent use, however many there are, while a revision nobody
	// runs any more goes away.
	runDirMaxAge = 7 * 24 * time.Hour
)

// steValeIni is the vale config of an STE run directory. StylesPath resolves
// against the config's own directory, so one template serves every run dir.
const steValeIni = `# humanizer STE-lite vale config, written by humanizer beside a copy of the
# STE style and, for a repo glossary, its generated Glossary.yml.
StylesPath = styles

MinAlertLevel = suggestion

[formats]
txt = md

[*.md]
BasedOnStyles = STE
`

func isSTERule(id string) bool {
	return strings.HasPrefix(id, steStyle+".")
}

// profileRuleIDs returns the IDs of the rules that p runs.
func profileRuleIDs(p Profile) []string {
	var ids []string
	for _, r := range All() {
		if slices.Contains(r.Profiles, string(p)) {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// ruleFilter resolves the rule IDs a run reports. Without a profile it is the
// caller's list, which must not name an STE rule: the default config never
// loads the STE style, so the filter would pass silently with no findings.
// With a profile it is the profile's rules, narrowed to the caller's list.
func ruleFilter(profile Profile, requested []string) ([]string, error) {
	if profile == "" {
		for _, id := range requested {
			if isSTERule(id) {
				return nil, fmt.Errorf(
					"rules: %s runs only with a profile (procedure or description)",
					id,
				)
			}
		}
		return requested, nil
	}
	inProfile := profileRuleIDs(profile)
	if len(inProfile) == 0 {
		return nil, fmt.Errorf("rules: unknown profile %q", profile)
	}
	if len(requested) == 0 {
		return inProfile, nil
	}
	for _, id := range requested {
		if !slices.Contains(inProfile, id) {
			return nil, fmt.Errorf("rules: %s is not in the %s profile", id, profile)
		}
	}
	return requested, nil
}

// ensureSTERunDir writes an STE run directory under cacheDir and returns the
// path of its vale config. The directory holds the config, a copy of the
// embedded STE style, and the glossary rule when one is given. It is keyed by
// the glossary's content hash, so the glossary in a directory never changes
// and repos with the same glossary share one. Every write is atomic and
// skipped when the file is unchanged, so concurrent runs need no lock.
//
// Each run marks its directory as used by setting its mtime before it writes
// anything, then prunes the run directories nobody used for runDirMaxAge, so
// one directory per glossary revision does not pile up.
func ensureSTERunDir(cacheDir string, gloss []byte) (string, error) {
	runRoot := filepath.Join(cacheDir, steRunRoot)
	dir := filepath.Join(runRoot, runKey(gloss))
	now := time.Now()
	if err := touchDir(dir, now); err != nil {
		return "", err
	}
	styleDir := filepath.Join(dir, "styles", steStyle)
	if err := extractTree(path.Join(stylesRoot, steStyle), styleDir); err != nil {
		return "", err
	}
	if len(gloss) > 0 {
		if err := writeFileIfChanged(filepath.Join(styleDir, glossaryFileName), gloss); err != nil {
			return "", err
		}
	}
	cfg := filepath.Join(dir, ".vale.ini")
	if err := writeFileIfChanged(cfg, []byte(steValeIni)); err != nil {
		return "", err
	}
	if err := pruneRunDirs(runRoot, now.Add(-runDirMaxAge)); err != nil {
		return "", err
	}
	return cfg, nil
}

// touchDir creates dir if needed and sets its mtime to now, the mark
// pruneRunDirs reads as the directory's last use.
func touchDir(dir string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chtimes(dir, now, now); err != nil {
		return fmt.Errorf("touch %s: %w", dir, err)
	}
	return nil
}

// pruneRunDirs removes the run directories under runRoot whose last use is
// before cutoff. A directory that another run removes first is skipped. A
// run that picks up a directory in the moment it is pruned can fail once; a
// directory in use is never pruned, because its own run just touched it.
func pruneRunDirs(runRoot string, cutoff time.Time) error {
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		return fmt.Errorf("read %s: %w", runRoot, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat %s: %w", e.Name(), err)
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(runRoot, e.Name())); err != nil {
			return fmt.Errorf("prune STE run dir %s: %w", e.Name(), err)
		}
	}
	return nil
}

// runKey names the run directory for a glossary: defaultRunKey without one,
// else a prefix of its SHA-256.
func runKey(gloss []byte) string {
	if len(gloss) == 0 {
		return defaultRunKey
	}
	sum := sha256.Sum256(gloss)
	return hex.EncodeToString(sum[:])[:runKeyHexLen]
}
