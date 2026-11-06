package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// ArchiveDir is where brain inbox archive moves expired raw notes, relative
// to the vault root. Every walk skips .brain, so an archived note leaves
// search, check and the link graph, while the file and its git history
// stay in the vault. Moving it back under inbox/ restores it.
const ArchiveDir = ".brain/archive"

// maxArchiveCopies bounds the -2, -3, ... suffixes an archive move tries
// when a note of the same name was archived before.
const maxArchiveCopies = 100

// InboxNote is one raw note of an inbox listing.
type InboxNote struct {
	Vault      string `json:"vault"`
	Path       string `json:"path"`
	Title      string `json:"title,omitempty"`
	CapturedBy string `json:"captured_by,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Expired    bool   `json:"expired"`
}

// InboxListing is the raw notes of the inbox tiers, soonest to expire first.
type InboxListing struct {
	Total int         `json:"total"`
	Notes []InboxNote `json:"notes"`
}

// Inbox lists the raw notes in the inbox tier of every vault of the set,
// soonest expires_at first (a note without one last), then by vault in
// config order and path. expiredOnly keeps the notes past expires_at.
func (s *Set) Inbox(now time.Time, expiredOnly bool) (InboxListing, error) {
	order := map[string]int{}
	notes := []InboxNote{}
	for i, v := range s.vaults {
		order[v.Name] = i
		got, err := v.rawInbox(now)
		if err != nil {
			return InboxListing{}, fmt.Errorf("%s: %w", v.Name, err)
		}
		for _, n := range got {
			if !expiredOnly || n.Expired {
				notes = append(notes, n)
			}
		}
	}
	sort.SliceStable(notes, func(i, j int) bool {
		a, b := notes[i], notes[j]
		if (a.ExpiresAt == "") != (b.ExpiresAt == "") {
			return b.ExpiresAt == ""
		}
		if a.ExpiresAt != b.ExpiresAt {
			return a.ExpiresAt < b.ExpiresAt
		}
		if a.Vault != b.Vault {
			return order[a.Vault] < order[b.Vault]
		}
		return a.Path < b.Path
	})
	return InboxListing{Total: len(notes), Notes: notes}, nil
}

// rawInbox returns the notes under the vault's inbox/ whose effective
// status is raw. A note whose frontmatter does not parse is skipped.
func (v *Vault) rawInbox(now time.Time) ([]InboxNote, error) {
	var out []InboxNote
	err := filepath.WalkDir(v.TierDir(TierInbox), func(p string, d fs.DirEntry, err error) error {
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
		if isLink(d) || !strings.HasSuffix(strings.ToLower(p), ".md") {
			return nil
		}
		fm, _, err := v.ReadFrontmatter(p)
		if err != nil {
			return nil //nolint:nilerr // skip a note that does not parse, keep listing
		}
		rel := v.Rel(p)
		if StatusOf(rel, fm) != lifecycle.Raw {
			return nil
		}
		f := lifecycle.Read(fm)
		out = append(out, InboxNote{
			Vault:      v.Name,
			Path:       rel,
			Title:      fm.Title,
			CapturedBy: f.CapturedBy,
			ExpiresAt:  f.ExpiresAt,
			Expired:    lifecycle.Standing(lifecycle.Raw, f, now) == lifecycle.Expired,
		})
		return nil
	})
	return out, err
}

// KeepResult is the JSON result of brain inbox keep.
type KeepResult struct {
	Vault     string `json:"vault"`
	Path      string `json:"path"`
	SHA       string `json:"sha"`
	ExpiresAt string `json:"expires_at"`
}

// Keep extends the expiry of the raw note at path to days after now. It
// never moves expires_at earlier: a note that already expires later keeps
// its date. It is the one way brain moves a raw note's expiry; an edit
// leaves it alone. A note that is not raw refuses with
// ErrInvalidStatusChange, a shared vault with ErrReadOnly.
func (v *Vault) Keep(p string, days int, now time.Time) (KeepResult, error) {
	if err := v.writable(); err != nil {
		return KeepResult{}, err
	}
	if days <= 0 {
		return KeepResult{}, fmt.Errorf("%w: keep takes a positive number of days, got %d",
			ErrInvalidStatusChange, days)
	}
	res := KeepResult{Vault: v.Name, Path: v.Rel(p)}
	sha, err := v.rewrite(
		p,
		"",
		VerbKeep,
		func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
			if status := StatusOf(res.Path, *fm); status != lifecycle.Raw {
				return nil, fmt.Errorf("%w: only a raw note expires, and %s is %s",
					ErrInvalidStatusChange, res.Path, status)
			}
			res.ExpiresAt = lifecycle.Keep(fm, days, now)
			fm.Touch(now)
			return body, nil
		},
	)
	res.SHA = sha
	return res, err
}

// Archived is one raw note brain inbox archive moved, or would move on a
// dry run.
type Archived struct {
	Vault      string `json:"vault"`
	Path       string `json:"path"`
	ArchivedTo string `json:"archived_to"`
}

// ArchiveExpired moves every raw note of the vault's inbox that is past its
// expires_at to the same path under ArchiveDir, adding -2, -3, ... to the
// name when an archived note holds it already. Nothing is deleted. A dry
// run reports the moves and makes none. A shared vault refuses with
// ErrReadOnly.
func (v *Vault) ArchiveExpired(now time.Time, dryRun bool) (moved []Archived, err error) {
	if err := v.writable(); err != nil {
		return nil, err
	}
	release, err := v.lockWrites()
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	notes, err := v.rawInbox(now)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		if !n.Expired {
			continue
		}
		to := path.Join(ArchiveDir, n.Path)
		if !dryRun {
			if to, err = v.archive(n.Path, to); err != nil {
				return moved, errors.Join(err, v.commitArchive(moved))
			}
		}
		moved = append(moved, Archived{Vault: v.Name, Path: n.Path, ArchivedTo: to})
	}
	if dryRun {
		return moved, nil
	}
	return moved, v.commitArchive(moved)
}

// commitArchive commits the moves of one archive run as a single change.
func (v *Vault) commitArchive(moved []Archived) error {
	rels := make([]string, 0, len(moved))
	paths := make([]string, 0, 2*len(moved))
	for _, m := range moved {
		rels = append(rels, m.Path)
		paths = append(paths, m.Path, m.ArchivedTo)
	}
	return v.commit(subjectOf(VerbArchive, rels), paths, "")
}

// archive moves the note at rel to the vault-relative path to, or to the
// first free -N variant of it, without replacing any file. It returns where
// the note went.
func (v *Vault) archive(rel, to string) (string, error) {
	src := filepath.Join(v.Root, filepath.FromSlash(rel))
	dst := filepath.Join(v.Root, filepath.FromSlash(to))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(dst)
	stem := strings.TrimSuffix(dst, ext)
	for n := 1; n <= maxArchiveCopies; n++ {
		cand := dst
		if n > 1 {
			cand = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		// A hard link never replaces an existing file, unlike a rename.
		err := os.Link(src, cand)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("archive %s: %w", rel, err)
		}
		if err := os.Remove(src); err != nil {
			return "", fmt.Errorf("archive %s: remove the inbox copy: %w", rel, err)
		}
		return v.Rel(cand), nil
	}
	return "", fmt.Errorf(
		"archive %s: %s and %d numbered copies exist",
		rel,
		to,
		maxArchiveCopies-1,
	)
}
