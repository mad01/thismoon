package vault

import (
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/noteid"
)

// The revocation commands take a note out of circulation without deleting
// it: Supersede points at its replacement, Retract keeps the body under a
// banner, and Withdraw leaves a tombstone. Each sets status_at, refuses a
// shared vault with ErrReadOnly and a withdrawn note with
// ErrInvalidStatusChange, and returns the sha of what it wrote.

// Supersede marks the note at path superseded by the note ref by, which
// should be an id:<ulid> ref so it survives a rename or promote.
func (v *Vault) Supersede(path, by string, now time.Time) (string, error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	return v.rewrite(path, "", func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
		if err := v.revocable(path, *fm); err != nil {
			return nil, err
		}
		if fm.ID != "" && by == idRefPrefix+":"+fm.ID {
			return nil, fmt.Errorf(
				"%w: %s cannot supersede itself", ErrInvalidStatusChange, v.Rel(path),
			)
		}
		lifecycle.Supersede(fm, by, now)
		fm.Touch(now)
		return body, nil
	})
}

// Retract marks the note at path retracted for reason (one of
// lifecycle.RetractReasons), with note as its status_reason when set. The
// body stays; read shows a banner above it.
func (v *Vault) Retract(path, reason, note string, now time.Time) (string, error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	return v.rewrite(path, "", func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
		if err := v.revocable(path, *fm); err != nil {
			return nil, err
		}
		if err := lifecycle.Retract(fm, reason, note, now); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidStatusChange, err)
		}
		fm.Touch(now)
		return body, nil
	})
}

// Withdraw replaces the note at path with a tombstone: its id (a fresh one
// when it had none), status withdrawn and status_at, with no body. A note
// already withdrawn keeps the day it was withdrawn. The caller purges the
// note's chunks from the indexes.
func (v *Vault) Withdraw(path string, now time.Time) (string, error) {
	if err := v.writable(); err != nil {
		return "", err
	}
	return v.rewrite(path, "", func(fm *markdown.Frontmatter, _ []byte) ([]byte, error) {
		id := fm.ID
		if id == "" {
			id = noteid.New(now)
		}
		when := now
		if lifecycle.Normalize(fm.Status) == lifecycle.Withdrawn {
			if t, ok := lifecycle.ParseDate(lifecycle.Read(*fm).StatusAt); ok {
				when = t
			}
		}
		*fm = lifecycle.Tombstone(id, when)
		return nil, nil
	})
}

// Flag marks the note at path disputed for reason: status disputed,
// status_reason and status_at. A shared vault refuses with ErrReadOnly, since
// a flag on a shared note is an issue on the vault's repo. A revoked note is
// out of circulation already and refuses with ErrInvalidStatusChange.
func (v *Vault) Flag(path, reason string, now time.Time) (string, error) {
	if v.Shared {
		return "", fmt.Errorf(
			"%w: %s is a shared vault; flag %s by opening an issue on the vault's repo "+
				"with the reason (a brain command that opens one is planned)",
			ErrReadOnly, v.Name, v.Name+":"+v.Rel(path),
		)
	}
	if strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("%w: a flag needs a reason", ErrInvalidStatusChange)
	}
	return v.rewrite(path, "", func(fm *markdown.Frontmatter, body []byte) ([]byte, error) {
		if lifecycle.Revoked(fm.Status) {
			return nil, fmt.Errorf(
				"%w: %s is %s and out of circulation already; flag a note that is in use",
				ErrInvalidStatusChange, v.Rel(path), lifecycle.Normalize(fm.Status),
			)
		}
		lifecycle.Dispute(fm, reason, now)
		fm.Touch(now)
		return body, nil
	})
}

// revocable refuses a withdrawn note: a tombstone keeps no fields a
// supersede or retract could add to.
func (v *Vault) revocable(path string, fm markdown.Frontmatter) error {
	if lifecycle.Normalize(fm.Status) == lifecycle.Withdrawn {
		return fmt.Errorf(
			"%w: %s is withdrawn; a tombstone takes no further status change",
			ErrInvalidStatusChange, v.Rel(path),
		)
	}
	return nil
}
