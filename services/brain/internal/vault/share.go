package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// The private side of a promotion into a shared vault. The note travels
// in a pull request; until it merges, the private copy is promoting with
// shared_pr naming the pull request. brain sync settles it: a merge
// removes the private copy, a close returns it to validated.

// StartPromotion marks the note at path promoting through the pull request
// at url and commits it. A shared vault refuses with ErrReadOnly.
func (v *Vault) StartPromotion(path, url string, now time.Time) (string, error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	return v.rewriteWith(path, "", VerbShare, "pull request: "+url,
		func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
			lifecycle.StartPromotion(fm, url, now)
			fm.Touch(now)
			return body, nil
		})
}

// EndPromotion returns the promoting note at path to validated after its
// pull request at url closed without a merge, and commits it.
func (v *Vault) EndPromotion(path, url string, now time.Time) (string, error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	return v.rewriteWith(path, "", VerbUnshare, "pull request closed without a merge: "+url,
		func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
			lifecycle.EndPromotion(fm, now)
			fm.Touch(now)
			return body, nil
		})
}

// RemovePromoted deletes the private copy at path of a note whose pull
// request at url merged into a shared vault, and commits the removal. The
// shared copy keeps the note's id and path, so links resolve to it.
func (v *Vault) RemovePromoted(path, url string) (err error) {
	if err := v.writable(); err != nil {
		return err
	}
	release, err := v.lockWrites()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove the private copy %s: %w", v.Rel(path), err)
	}
	rel := v.Rel(path)
	return v.commit(subjectOf(VerbRemove, []string{rel}), []string{rel},
		"merged into a shared vault: "+url)
}

// CreateAt writes a new note at the vault-relative path rel from fm and
// body, exactly as given: no defaults, no new id. It refuses a taken path
// with ErrExists and a secret with ErrSecretFound, and commits nothing;
// brain uses it to place a note in a worktree it commits itself.
func (v *Vault) CreateAt(rel string, fm markdown.Frontmatter, body []byte) (string, error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	path, err := v.Resolve(rel)
	if err != nil {
		return "", err
	}
	content, err := markdown.Render(fm, body)
	if err != nil {
		return "", err
	}
	if err := v.refuseSecrets(path, content); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, v.create(path, content)
}
