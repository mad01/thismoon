package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/noteid"
)

// idRefPrefix marks a note ref that names a note by its id: id:<ulid>.
const idRefPrefix = "id"

// locateID returns the note whose id is id. An id held by more than one
// note (a copied file, say) is ErrAmbiguous; none is ErrNotFound.
func (s *Set) locateID(id string) (Ref, error) {
	if id == "" {
		return Ref{}, fmt.Errorf("%w: id: needs a note id", ErrInvalidPath)
	}
	var found []Ref
	for _, v := range s.vaults {
		err := v.walkTierNotes(func(path string, data []byte) error {
			if !bytes.Contains(data, []byte(id)) {
				return nil
			}
			fm, _, err := markdown.ParseFrontmatter(data)
			if err == nil && fm.ID == id {
				found = append(found, Ref{Vault: v, Path: v.Rel(path)})
			}
			return nil
		})
		if err != nil {
			return Ref{}, err
		}
	}
	switch len(found) {
	case 0:
		return Ref{}, fmt.Errorf("%w: no note with id %s", ErrNotFound, id)
	case 1:
		return found[0], nil
	}
	return Ref{}, ambiguous("id "+id, found)
}

// walkTierNotes calls visit with the path and content of every markdown
// note under the vault's tier directories. Files outside the tiers (a
// README at the root, say) are not notes and are not visited; unreadable
// files are skipped.
func (v *Vault) walkTierNotes(visit func(path string, data []byte) error) error {
	for _, t := range AllTiers {
		err := filepath.WalkDir(v.TierDir(t), func(path string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil //nolint:nilerr // skip an unreadable note, keep walking
			}
			return visit(path, data)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// IDChange is one note id migrate assigned, or would assign on a dry run.
type IDChange struct {
	Vault string `json:"vault"`
	Path  string `json:"path"`
	ID    string `json:"id"`
}

// IDSkip is a note migrate could not give an id, or a whole vault it left
// alone (Path empty), and why.
type IDSkip struct {
	Vault  string `json:"vault"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason"`
}

// MigrateReport is what one id migration changed.
type MigrateReport struct {
	DryRun  bool       `json:"dry_run"`
	Changed []IDChange `json:"changed"`
	Skipped []IDSkip   `json:"skipped,omitempty"`
}

// MigrateIDs gives every note in the vault's tiers that has no id a fresh
// one, leaving every other byte of the note as it was (updated is not
// touched). Running it again changes nothing. A dry run reports the ids it
// would assign and writes nothing. A shared vault refuses a real run with
// ErrReadOnly. A note whose frontmatter does not parse is skipped and
// reported.
func (v *Vault) MigrateIDs(dryRun bool, now time.Time) (report MigrateReport, err error) {
	report = MigrateReport{DryRun: dryRun, Changed: []IDChange{}}
	if !dryRun {
		if err := v.writable(); err != nil {
			return report, err
		}
		release, err := v.lockWrites()
		if err != nil {
			return report, err
		}
		defer func() { err = errors.Join(err, release()) }()
	}
	if now.IsZero() {
		now = time.Now()
	}
	err = v.walkTierNotes(func(path string, data []byte) error {
		rel := v.Rel(path)
		fm, body, err := markdown.ParseFrontmatter(data)
		if err != nil {
			report.Skipped = append(
				report.Skipped,
				IDSkip{Vault: v.Name, Path: rel, Reason: err.Error()},
			)
			return nil
		}
		if fm.ID != "" {
			return nil
		}
		fm.ID = noteid.New(now)
		report.Changed = append(report.Changed, IDChange{Vault: v.Name, Path: rel, ID: fm.ID})
		if dryRun {
			return nil
		}
		content, err := addID(fm, body)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		return atomicWrite(path, content, 0o644)
	})
	if err != nil || dryRun {
		return report, err
	}
	rels := make([]string, 0, len(report.Changed))
	for _, c := range report.Changed {
		rels = append(rels, c.Path)
	}
	return report, v.commit(subjectOf(VerbMigrate, rels), rels, "")
}

// addID returns the note with an id line for fm.ID added as the last
// frontmatter key, every other byte kept: Render patches the frontmatter it
// was parsed from. A note without frontmatter gets a block holding the id
// alone.
func addID(fm markdown.Frontmatter, body []byte) ([]byte, error) {
	return markdown.Render(fm, body)
}

// IDReport counts the note ids across a set and lists the ids more than one
// note holds.
type IDReport struct {
	Notes      int
	Missing    int
	Duplicates map[string][]Ref
}

// IDs reads the id of every note in the set's tiers.
func (s *Set) IDs() (IDReport, error) {
	report := IDReport{Duplicates: map[string][]Ref{}}
	holders := map[string][]Ref{}
	for _, v := range s.vaults {
		err := v.walkTierNotes(func(path string, data []byte) error {
			report.Notes++
			fm, _, err := markdown.ParseFrontmatter(data)
			if err != nil || fm.ID == "" {
				report.Missing++
				return nil
			}
			holders[fm.ID] = append(holders[fm.ID], Ref{Vault: v, Path: v.Rel(path)})
			return nil
		})
		if err != nil {
			return report, fmt.Errorf("%s: %w", v.Name, err)
		}
	}
	for id, refs := range holders {
		if len(refs) > 1 {
			report.Duplicates[id] = refs
		}
	}
	return report, nil
}

// DuplicateIDs returns the duplicated ids in sorted order.
func (r IDReport) DuplicateIDs() []string {
	ids := make([]string, 0, len(r.Duplicates))
	for id := range r.Duplicates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
