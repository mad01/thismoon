package vault

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// rewriteWikilinkTarget walks the vault and rewrites every [[oldStem...]]
// wikilink to [[newStem...]], preserving any heading (#), block (^) or alias
// (|) suffix. Returns the list of files rewritten.
func rewriteWikilinkTarget(vaultRoot, oldStem, newStem string) ([]string, error) {
	if oldStem == newStem {
		return nil, nil
	}
	// Match [[oldStem<suffix>]] where suffix is anything up to the closing ]].
	// Stem is matched case-sensitively; Obsidian is case-insensitive but we
	// keep the stricter rule here to avoid unwanted rewrites.
	escaped := regexp.QuoteMeta(oldStem)
	// Capture group 1 = suffix (heading/block/alias or empty).
	re := regexp.MustCompile(`\[\[` + escaped + `((?:#|\^|\|)[^\]]*)?\]\]`)
	replacement := []byte("[[" + newStem + "$1]]")

	var rewrites []string
	err := filepath.WalkDir(vaultRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip .git, .obsidian, .trash.
			name := d.Name()
			if name == ".git" || name == ".obsidian" || name == ".trash" || name == "attachments" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte("[["+oldStem)) {
			return nil
		}
		newData := re.ReplaceAll(data, replacement)
		if bytes.Equal(newData, data) {
			return nil
		}
		if err := atomicWrite(path, newData, 0o644); err != nil {
			return err
		}
		rewrites = append(rewrites, path)
		return nil
	})
	if err != nil {
		return rewrites, err
	}
	return rewrites, nil
}

// batchIndexNamePattern matches MOC filenames that act as batch-provenance
// indexes (the kickoff registry of a research wave) rather than topical maps
// of content. Examples: "research-index-2026-134-reference-topics.md",
// "kickoff-q2-2026.md". Such files become graph hubs purely by batch
// membership and distort centrality, so Promote refuses to append to them.
var batchIndexNamePattern = regexp.MustCompile(`^(research-index|kickoff)-`)

// isBatchIndexMOC returns true when the target MOC is a batch-provenance
// index that should not be used as a moc-append target. Detection rules,
// either of which is sufficient:
//   - filename matches batchIndexNamePattern (case-insensitive)
//   - existing frontmatter carries the "meta/index" tag
//
// A non-existent file with a non-matching name returns (false, nil) — that's
// a legitimate "create a new topical MOC" case. A read error other than
// not-exist surfaces; malformed frontmatter is treated as not-a-batch-index
// so the existing best-effort behavior is preserved.
func isBatchIndexMOC(mocPath string) (bool, error) {
	base := strings.ToLower(filepath.Base(mocPath))
	if batchIndexNamePattern.MatchString(base) {
		return true, nil
	}
	data, err := os.ReadFile(mocPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	fm, _, parseErr := markdown.ParseFrontmatter(data)
	if parseErr != nil {
		return false, nil
	}
	return fm.HasTag("meta/index"), nil
}

// appendMOCBullet appends a "- [[stem|title]]  <!-- added YYYY-MM-DD -->" line
// to the end of the MOC file. Creates the MOC if missing.
func appendMOCBullet(mocPath, title, stem string, now time.Time) error {
	line := fmt.Sprintf(
		"- [[%s|%s]]  <!-- added %s -->\n",
		stem,
		title,
		now.UTC().Format("2006-01-02"),
	)
	if _, err := os.Stat(mocPath); os.IsNotExist(err) {
		return os.WriteFile(mocPath, []byte(line), 0o644)
	}
	f, err := os.OpenFile(mocPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
