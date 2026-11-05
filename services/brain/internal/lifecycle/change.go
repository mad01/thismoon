package lifecycle

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// ApplyDefaults fills the fields a new note gets from its status, leaving
// any the caller set alone: a raw capture expires RawLifetimeDays after now,
// and a validated note is due for review after its volatility's window.
func ApplyDefaults(fm *markdown.Frontmatter, now time.Time) {
	f := Read(*fm)
	switch Normalize(fm.Status) {
	case Raw:
		if f.ExpiresAt == "" {
			setDate(fm, KeyExpiresAt, now.AddDate(0, 0, RawLifetimeDays))
		}
	case Validated:
		if f.ReviewBy == "" {
			setDate(fm, KeyReviewBy, now.AddDate(0, 0, ReviewDays(f.Volatility)))
		}
	}
}

// Transition records that fm's status changed from the status from at now:
// status_at becomes now's date and the new status gets its defaults. A note
// leaving raw drops expires_at, which only an inbox capture carries.
func Transition(fm *markdown.Frontmatter, from string, now time.Time) {
	if Normalize(from) == Normalize(fm.Status) {
		return
	}
	setDate(fm, KeyStatusAt, now)
	if Normalize(from) == Raw {
		fm.Unset(KeyExpiresAt)
	}
	ApplyDefaults(fm, now)
}

// Supersede marks fm superseded by the note ref by.
func Supersede(fm *markdown.Frontmatter, by string, now time.Time) {
	fm.Status = Superseded
	setString(fm, KeySupersededBy, by)
	setDate(fm, KeyStatusAt, now)
}

// Retract marks fm retracted for reason, one of RetractReasons. A non-empty
// note becomes status_reason.
func Retract(fm *markdown.Frontmatter, reason, note string, now time.Time) error {
	if !slices.Contains(RetractReasons, reason) {
		return fmt.Errorf(
			"retract reason %q is not one of %s", reason, strings.Join(RetractReasons, ", "),
		)
	}
	fm.Status = Retracted
	setString(fm, KeyRetractedReason, reason)
	setDate(fm, KeyRetractedAt, now)
	setDate(fm, KeyStatusAt, now)
	if strings.TrimSpace(note) != "" {
		setString(fm, KeyStatusReason, strings.TrimSpace(note))
	}
	return nil
}

// Tombstone returns what stays of a withdrawn note: its id, the withdrawn
// status and the day it was withdrawn.
func Tombstone(id string, now time.Time) markdown.Frontmatter {
	fm := markdown.Frontmatter{ID: id, Status: Withdrawn}
	setDate(&fm, KeyStatusAt, now)
	return fm
}

// Banner returns the line read shows above a note that should not be cited
// as it stands, or "" for any other note.
func Banner(f Fields) string {
	since := ""
	if f.StatusAt != "" {
		since = " on " + f.StatusAt
	}
	reason := ""
	if f.StatusReason != "" {
		reason = ": " + f.StatusReason
	}
	switch Normalize(f.Status) {
	case Retracted:
		why := f.RetractedReason
		if why == "" {
			why = "no reason given"
		}
		return fmt.Sprintf("RETRACTED%s (%s)%s. Do not cite this note.", since, why, reason)
	case Superseded:
		by := f.SupersededBy
		if by == "" {
			by = "a note it does not name"
		}
		return fmt.Sprintf("SUPERSEDED%s by %s%s. Cite that note instead.", since, by, reason)
	case Withdrawn:
		return fmt.Sprintf("WITHDRAWN%s. Its content was removed.", since)
	case Disputed:
		return fmt.Sprintf("DISPUTED%s%s. Check the claims before citing them.", since, reason)
	}
	return ""
}

// setString stores s under key as a string, quoted when YAML would read it
// as something else.
func setString(fm *markdown.Frontmatter, key, s string) {
	_ = fm.Set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
}

// setDate stores t's date under key, unquoted, the way a hand-written date
// reads.
func setDate(fm *markdown.Frontmatter, key string, t time.Time) {
	_ = fm.Set(key, &yaml.Node{Kind: yaml.ScalarNode, Value: Date(t)})
}
